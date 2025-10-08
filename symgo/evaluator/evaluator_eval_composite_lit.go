package evaluator

import (
	"bytes"
	"context"
	"go/ast"
	"go/printer"
	"log/slog"
	"strings"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalCompositeLit(ctx context.Context, node *ast.CompositeLit, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if e.evaluationInProgress[node] {
		e.logc(ctx, slog.LevelWarn, "cyclic dependency detected in composite literal", "pos", node.Pos())
		return &object.SymbolicPlaceholder{Reason: "cyclic reference in composite literal"}
	}
	e.evaluationInProgress[node] = true
	defer delete(e.evaluationInProgress, node)

	var fieldType *scan.FieldType
	var resolvedType *scan.TypeInfo
	var aliasTypeInfo *scan.TypeInfo // To hold the original alias type

	// First, try to resolve the type from the local environment. This handles locally defined type aliases.
	if node.Type != nil {
		typeObj := e.Eval(ctx, node.Type, env, pkg)

		if t, ok := typeObj.(*object.Type); ok && t.ResolvedType != nil {
			originalTypeInfo := t.ResolvedType
			if originalTypeInfo.Kind == scan.AliasKind && originalTypeInfo.Underlying != nil {
				aliasTypeInfo = originalTypeInfo // Capture the alias type info
				// It's an alias. fieldType represents the alias itself.
				fieldType = &scan.FieldType{
					Name:           originalTypeInfo.Name,
					FullImportPath: originalTypeInfo.PkgPath,
					Definition:     originalTypeInfo,
				}
				// Copy structural info from underlying type to the alias's FieldType
				underlying := originalTypeInfo.Underlying
				fieldType.IsSlice = underlying.IsSlice
				fieldType.IsMap = underlying.IsMap
				fieldType.IsPointer = underlying.IsPointer
				fieldType.Elem = underlying.Elem
				fieldType.MapKey = underlying.MapKey
				// For further processing, resolvedType must be the underlying type.
				resolvedType = e.resolver.ResolveType(ctx, underlying)
			} else {
				// It's a regular type resolved from env.
				resolvedType = originalTypeInfo
			}
		}
	}

	// If type information is still missing (e.g., for implicit types in slice literals
	// or if resolution from env failed), fall back to the scanner.
	if resolvedType == nil {
		if pkg == nil || pkg.Fset == nil {
			// Cannot proceed without package info, might be an implicit type we can't infer.
			return &object.SymbolicPlaceholder{Reason: "unresolved composite literal with no type"}
		}
		file := pkg.Fset.File(node.Pos())
		if file == nil {
			return e.newError(ctx, node.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(ctx, node.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		// This handles cases where node.Type is specified but wasn't in the env.
		if node.Type != nil {
			fieldType = e.scanner.TypeInfoFromExpr(ctx, node.Type, nil, pkg, importLookup)
			if fieldType == nil {
				var typeNameBuf bytes.Buffer
				printer.Fprint(&typeNameBuf, pkg.Fset, node.Type)
				return e.newError(ctx, node.Pos(), "could not resolve type for composite literal: %s", typeNameBuf.String())
			}
			resolvedType = e.resolver.ResolveType(ctx, fieldType)
		}
		// Note: We don't handle implicit types (where node.Type is nil) here yet.
		// That would require type inference from the context (e.g., assignment statement).
	}

	// If fieldType wasn't set for an alias, create it now from the resolvedType.
	if fieldType == nil && resolvedType != nil {
		isSlice := false
		isMap := false
		var elem, mapKey *scan.FieldType

		if resolvedType.Kind == scan.AliasKind && resolvedType.Underlying != nil {
			isSlice = resolvedType.Underlying.IsSlice
			isMap = resolvedType.Underlying.IsMap
			elem = resolvedType.Underlying.Elem
			mapKey = resolvedType.Underlying.MapKey
		}

		fieldType = &scan.FieldType{
			Name:           resolvedType.Name,
			FullImportPath: resolvedType.PkgPath,
			Definition:     resolvedType,
			IsSlice:        isSlice,
			IsMap:          isMap,
			Elem:           elem,
			MapKey:         mapKey,
			IsPointer:      strings.HasPrefix(resolvedType.Name, "*"),
		}
	}

	// Now that we have the type, evaluate the elements of the literal.
	elements := make([]object.Object, 0, len(node.Elts))
	for _, elt := range node.Elts {
		switch v := elt.(type) {
		case *ast.KeyValueExpr:
			value := e.Eval(ctx, v.Value, env, pkg)
			elements = append(elements, value)
			if fieldType != nil && fieldType.IsMap {
				e.Eval(ctx, v.Key, env, pkg)
			}
		default:
			element := e.Eval(ctx, v, env, pkg)
			elements = append(elements, element)
		}
	}

	// Finally, construct the appropriate object based on the type.
	if fieldType != nil && fieldType.IsMap {
		mapObj := &object.Map{MapFieldType: fieldType}
		mapObj.SetFieldType(fieldType)
		if aliasTypeInfo != nil {
			// If it was a named map type, attach the alias's TypeInfo.
			// This is crucial for method lookups.
			mapObj.SetTypeInfo(aliasTypeInfo)
		} else {
			mapObj.SetTypeInfo(resolvedType)
		}
		return mapObj
	}

	if fieldType != nil && fieldType.IsSlice {
		sliceLen := int64(len(elements))
		sliceObj := &object.Slice{
			SliceFieldType: fieldType,
			Elements:       elements,
			Len:            sliceLen,
			Cap:            sliceLen, // For a slice literal, len and cap are the same.
		}
		sliceObj.SetFieldType(fieldType)
		sliceObj.SetTypeInfo(resolvedType)
		return sliceObj
	}

	if resolvedType != nil && resolvedType.Unresolved && resolvedType.Kind == scan.UnknownKind {
		// This is an unresolved type used in a composite literal. We can infer its kind.
		isStructLike := false
		if len(node.Elts) > 0 {
			if _, ok := node.Elts[0].(*ast.KeyValueExpr); ok {
				isStructLike = true
			}
		} else {
			// An empty literal {} is ambiguous, but often a struct.
			isStructLike = true
		}

		if isStructLike {
			e.logc(ctx, slog.LevelDebug, "inferred struct kind for unresolved type", "type", resolvedType.Name)
			resolvedType.Kind = scan.StructKind
		}
	}

	if resolvedType != nil && resolvedType.Kind == scan.StructKind && !resolvedType.Unresolved {
		structObj := &object.Struct{
			StructType: resolvedType,
			Fields:     make(map[string]object.Object),
		}
		structObj.SetTypeInfo(resolvedType)
		structObj.SetFieldType(fieldType)

		initializedFields := make(map[string]bool)
		for _, elt := range node.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					val := e.Eval(ctx, kv.Value, env, pkg)
					if isError(val) {
						return val
					}
					structObj.Set(key.Name, val)
					initializedFields[key.Name] = true
				}
			}
			// TODO: Handle positional struct fields.
		}

		// Set zero values for uninitialized fields. This is crucial for correctly
		// handling nil pointer fields.
		if resolvedType.Struct != nil {
			for _, fieldDef := range resolvedType.Struct.Fields {
				if !initializedFields[fieldDef.Name] {
					// For now, we only care about pointer types being nil.
					// Other types will implicitly be symbolic placeholders when accessed.
					if fieldDef.Type != nil && fieldDef.Type.IsPointer {
						structObj.Set(fieldDef.Name, object.NIL)
					}
				}
			}
		}
		// Return an Instance that wraps the Struct, for compatibility with method lookups.
		instance := &object.Instance{
			TypeName:   resolvedType.PkgPath + "." + resolvedType.Name,
			Underlying: structObj,
			BaseObject: object.BaseObject{
				ResolvedTypeInfo: resolvedType,
			},
		}
		instance.SetFieldType(fieldType)
		return instance
	}

	if resolvedType == nil || resolvedType.Unresolved {
		reason := "unresolved composite literal type"
		if fieldType != nil {
			reason = "unresolved composite literal of type " + fieldType.String()
		}
		placeholder := &object.SymbolicPlaceholder{
			Reason: reason,
		}
		placeholder.SetFieldType(fieldType)
		placeholder.SetTypeInfo(resolvedType)
		return placeholder
	}

	// The failing test expects the underlying type name, not the alias name.
	instance := &object.Instance{
		TypeName: resolvedType.PkgPath + "." + resolvedType.Name,
		BaseObject: object.BaseObject{
			ResolvedTypeInfo: resolvedType,
		},
	}
	instance.SetFieldType(fieldType)
	return instance
}
