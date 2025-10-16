package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalSliceExpr(ctx context.Context, node *ast.SliceExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Evaluate the expression being sliced to trace any calls within it.
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}

	// Evaluate index expressions to trace calls.
	if node.Low != nil {
		if low := e.Eval(ctx, node.Low, env, pkg); isError(low) {
			return low
		}
	}
	if node.High != nil {
		if high := e.Eval(ctx, node.High, env, pkg); isError(high) {
			return high
		}
	}
	if node.Max != nil {
		if max := e.Eval(ctx, node.Max, env, pkg); isError(max) {
			return max
		}
	}

	// The result of a slice expression is another slice (or array), which we represent
	// with a placeholder that carries the original type information.
	placeholder := &object.SymbolicPlaceholder{
		Reason: "result of slice expression",
	}
	if left.TypeInfo() != nil {
		placeholder.SetTypeInfo(left.TypeInfo())
	}
	if left.FieldType() != nil {
		placeholder.SetFieldType(left.FieldType())
	}
	return placeholder
}
