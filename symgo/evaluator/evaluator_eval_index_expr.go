package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalIndexExpr(ctx context.Context, node *ast.IndexExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}

	// Handle generic instantiation `F[T]`
	if fn, ok := left.(*object.Function); ok {
		if fn.Def != nil && len(fn.Def.TypeParams) > 0 {
			return e.evalGenericInstantiation(ctx, fn, []ast.Expr{node.Index}, node.Pos(), pkg)
		}
	}
	if t, ok := left.(*object.Type); ok {
		if t.ResolvedType != nil && len(t.ResolvedType.TypeParams) > 0 {
			return &object.SymbolicPlaceholder{Reason: "instantiated generic type"}
		}
	}

	// Fallback to original logic for slice/map indexing at runtime.
	if index := e.Eval(ctx, node.Index, env, pkg); isError(index) {
		return index
	}

	var elemFieldType *scan.FieldType
	var resolvedElem *scan.TypeInfo

	// Determine the element type from the collection being indexed.
	var collectionFieldType *scan.FieldType
	switch l := left.(type) {
	case *object.Slice:
		collectionFieldType = l.SliceFieldType
	case *object.Map:
		collectionFieldType = l.MapFieldType
	case *object.Variable:
		// Check the variable's value first, then its static type.
		if s, ok := l.Value.(*object.Slice); ok {
			collectionFieldType = s.SliceFieldType
		} else if m, ok := l.Value.(*object.Map); ok {
			collectionFieldType = m.MapFieldType
		} else if ft := l.FieldType(); ft != nil && (ft.IsSlice || ft.IsMap) {
			collectionFieldType = ft
		} else if ti := l.TypeInfo(); ti != nil && ti.Underlying != nil && (ti.Underlying.IsSlice || ti.Underlying.IsMap) {
			collectionFieldType = ti.Underlying
		}
	case *object.SymbolicPlaceholder:
		if ft := l.FieldType(); ft != nil && (ft.IsSlice || ft.IsMap) {
			collectionFieldType = ft
		} else if ti := l.TypeInfo(); ti != nil && ti.Underlying != nil && (ti.Underlying.IsSlice || ti.Underlying.IsMap) {
			collectionFieldType = ti.Underlying
		}
	}

	// If we found a collection type, get its element type.
	if collectionFieldType != nil && collectionFieldType.Elem != nil {
		elemFieldType = collectionFieldType.Elem
		resolvedElem = e.resolver.ResolveType(ctx, elemFieldType)
	}

	return &object.SymbolicPlaceholder{
		Reason: "result of index expression",
		BaseObject: object.BaseObject{
			ResolvedTypeInfo:  resolvedElem,
			ResolvedFieldType: elemFieldType,
		},
	}
}
