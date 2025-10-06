package evaluator

import (
	"context"
	"go/ast"
	"go/token"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalGenDecl(ctx context.Context, node *ast.GenDecl, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	switch node.Tok {
	case token.CONST:
		var lastValues []ast.Expr
		for iota, spec := range node.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Handle implicit value repetition.
			if len(valSpec.Values) > 0 {
				lastValues = valSpec.Values
			}

			for i, name := range valSpec.Names {
				if name.Name == "_" {
					continue
				}

				if i >= len(lastValues) {
					e.logc(ctx, slog.LevelWarn, "not enough values for constant declaration", "const", name.Name)
					continue
				}

				exprToEval := lastValues[i]

				// Create a temporary environment for this expression evaluation
				// to correctly handle `iota`.
				exprEnv := object.NewEnclosedEnvironment(env)
				exprEnv.SetLocal("iota", &object.Integer{Value: int64(iota)})

				val := e.Eval(ctx, exprToEval, exprEnv, pkg)
				if isError(val) {
					e.logc(ctx, slog.LevelWarn, "could not evaluate constant expression", "const", name.Name, "error", val)
					continue
				}

				// A const declaration is a local definition.
				env.SetLocal(name.Name, val)
			}
		}
	case token.VAR:
		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, node.Pos(), "package info or fset is missing, cannot resolve types")
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

		for _, spec := range node.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			var staticFieldType *scan.FieldType
			if valSpec.Type != nil {
				staticFieldType = e.scanner.TypeInfoFromExpr(ctx, valSpec.Type, nil, pkg, importLookup)
			}

			for i, name := range valSpec.Names {
				var val object.Object
				var resolvedTypeInfo *scan.TypeInfo
				if staticFieldType != nil {
					resolvedTypeInfo = e.resolver.ResolveType(ctx, staticFieldType)
				}

				if i < len(valSpec.Values) {
					val = e.Eval(ctx, valSpec.Values[i], env, pkg)
					if isError(val) {
						return val
					}
					if ret, ok := val.(*object.ReturnValue); ok {
						val = ret.Value
					}
				} else {
					placeholder := &object.SymbolicPlaceholder{Reason: "uninitialized variable"}
					if staticFieldType != nil {
						placeholder.SetFieldType(staticFieldType)
						placeholder.SetTypeInfo(resolvedTypeInfo)
					}
					val = placeholder
				}

				v := &object.Variable{
					Name:        name.Name,
					Value:       val,
					IsEvaluated: true,
					DeclPkg:     pkg,
				}
				v.SetFieldType(val.FieldType())
				v.SetTypeInfo(val.TypeInfo())

				if staticFieldType != nil {
					if v.FieldType() == nil {
						v.SetFieldType(staticFieldType)
					}
					if v.TypeInfo() == nil {
						v.SetTypeInfo(resolvedTypeInfo)
					}
				}
				env.SetLocal(name.Name, v)
			}
		}
	case token.TYPE:
		e.evalTypeDecl(ctx, node, env, pkg)
	}
	return nil
}

func (e *Evaluator) evalTypeDecl(ctx context.Context, d *ast.GenDecl, env *object.Environment, pkg *scan.PackageInfo) {
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		// Find the TypeInfo that the scanner created for this TypeSpec.
		var typeInfo *scan.TypeInfo
		if pkg != nil { // pkg can be nil in some tests
			for _, ti := range pkg.Types {
				if ti.Node == ts {
					typeInfo = ti
					break
				}
			}
		}

		if typeInfo == nil {
			// This could be a local type definition inside a function.
			// The scanner does not create TypeInfo for these, so we create one on the fly.
			if pkg == nil || pkg.Fset == nil {
				e.logc(ctx, slog.LevelWarn, "cannot create local type info without package context", "type", ts.Name.Name)
				continue
			}
			file := pkg.Fset.File(ts.Pos())
			if file == nil {
				e.logc(ctx, slog.LevelWarn, "could not find file for local type node position", "type", ts.Name.Name)
				continue
			}
			astFile, fileOK := pkg.AstFiles[file.Name()]
			if !fileOK {
				e.logc(ctx, slog.LevelWarn, "could not find ast.File for local type", "type", ts.Name.Name, "path", file.Name())
				continue
			}
			importLookup := e.scanner.BuildImportLookup(astFile)

			// Determine the underlying type information.
			underlyingFieldType := e.scanner.TypeInfoFromExpr(ctx, ts.Type, nil, pkg, importLookup)
			// Note: We don't resolve the underlying type here. The important part is to
			// capture the AST (`ts`) and the textual representation of the underlying type (`underlyingFieldType`).
			// The resolution will happen later when this type is actually used.

			// Create a new TypeInfo for the local alias.
			typeInfo = &scan.TypeInfo{
				Name:       ts.Name.Name,
				PkgPath:    pkg.ImportPath, // Local types belong to the current package.
				Node:       ts,             // IMPORTANT: Store the AST node.
				Underlying: underlyingFieldType,
				Kind:       scan.AliasKind, // Mark it as an alias.
			}
		}

		typeObj := &object.Type{
			TypeName:     typeInfo.Name,
			ResolvedType: typeInfo,
		}
		typeObj.SetTypeInfo(typeInfo)
		env.Set(ts.Name.Name, typeObj)
	}
}
