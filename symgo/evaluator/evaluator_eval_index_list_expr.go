package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalIndexListExpr(ctx context.Context, node *ast.IndexListExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}

	// Handle generic instantiation `F[T, U]`
	if fn, ok := left.(*object.Function); ok {
		if fn.Def != nil && len(fn.Def.TypeParams) > 0 {
			return e.evalGenericInstantiation(ctx, fn, node.Indices, node.Pos(), pkg)
		}
	}
	if t, ok := left.(*object.Type); ok {
		if t.ResolvedType != nil && len(t.ResolvedType.TypeParams) > 0 {
			return &object.SymbolicPlaceholder{Reason: "instantiated generic type"}
		}
	}

	// This AST node is only for generics, so if we fall through, it's an unhandled case.
	return e.newError(ctx, node.Pos(), "unhandled generic instantiation for %T", left)
}
