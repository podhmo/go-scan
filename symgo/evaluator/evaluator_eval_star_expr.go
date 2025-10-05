package evaluator

import (
	"context"
	"fmt"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalStarExpr(ctx context.Context, node *ast.StarExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	val := e.Eval(ctx, node.X, env, pkg)
	if isError(val) {
		return val
	}

	// If the expression is a function call, it might be wrapped in a ReturnValue.
	if ret, ok := val.(*object.ReturnValue); ok {
		val = ret.Value
	}

	// First, unwrap any variable to get to the underlying value.
	if v, ok := val.(*object.Variable); ok {
		val = v.Value
	}

	if ptr, ok := val.(*object.Pointer); ok {
		// If we are dereferencing a pointer to an unresolved type, the result is
		// a symbolic placeholder representing an instance of that type.
		if ut, ok := ptr.Value.(*object.UnresolvedType); ok {
			placeholder := &object.SymbolicPlaceholder{
				Reason: fmt.Sprintf("instance of unresolved type %s.%s", ut.PkgPath, ut.TypeName),
			}
			// Attempt to resolve the type to attach its info to the placeholder
			if resolvedType, err := e.resolver.ResolvePackage(ctx, ut.PkgPath); err == nil {
				for _, t := range resolvedType.Types {
					if t.Name == ut.TypeName {
						placeholder.SetTypeInfo(t)
						break
					}
				}
			}
			return placeholder
		}

		// The value of a pointer is the object it points to.
		// By returning the pointee directly, a selector expression like `(*p).MyMethod`
		// will operate on the instance, which is the correct behavior.
		return ptr.Value
	}

	// If we have a symbolic placeholder that represents a pointer type,
	// dereferencing it should result in a new placeholder representing the element type.
	if sp, ok := val.(*object.SymbolicPlaceholder); ok {
		var elemFieldType *scan.FieldType
		var resolvedElem *scan.TypeInfo
		if ft := sp.FieldType(); ft != nil && ft.IsPointer && ft.Elem != nil {
			elemFieldType = ft.Elem
			resolvedElem = e.resolver.ResolveType(ctx, elemFieldType)
		}
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("dereferenced from %s", sp.Reason),
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  resolvedElem,
				ResolvedFieldType: elemFieldType,
			},
		}
	}

	// NEW: Handle dereferencing a type object itself.
	// This can happen in method calls on symbolic receivers.
	if t, ok := val.(*object.Type); ok {
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("instance of type %s from dereference", t.TypeName),
			BaseObject: object.BaseObject{
				ResolvedTypeInfo: t.ResolvedType,
			},
		}
	}

	// Handle dereferencing an unresolved type object itself. This is the source
	// of the "invalid indirect" errors seen in the find-orphans run.
	if ut, ok := val.(*object.UnresolvedType); ok {
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("instance of unresolved type %s.%s from dereference", ut.PkgPath, ut.TypeName),
		}
	}

	// Handle dereferencing an unresolved function object.
	if uf, ok := val.(*object.UnresolvedFunction); ok {
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("instance of unresolved function %s.%s from dereference", uf.PkgPath, uf.FuncName),
		}
	}

	// If we are trying to dereference a symbolic placeholder that isn't a pointer,
	// we shouldn't error out, but return another placeholder. This allows analysis
	// of incorrect but plausible code paths to continue.
	if _, ok := val.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("dereference of non-pointer symbolic value %s", val.Inspect())}
	}

	return e.newError(ctx, node.Pos(), "invalid indirect of %s (type %T)", val.Inspect(), val)
}
