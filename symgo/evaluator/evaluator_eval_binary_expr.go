package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalBinaryExpr(ctx context.Context, node *ast.BinaryExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	leftObj := e.Eval(ctx, node.X, env, pkg)
	if isError(leftObj) {
		return leftObj
	}
	rightObj := e.Eval(ctx, node.Y, env, pkg)
	if isError(rightObj) {
		return rightObj
	}

	left := e.forceEval(ctx, leftObj, pkg)
	if isError(left) {
		return left
	}
	right := e.forceEval(ctx, rightObj, pkg)
	if isError(right) {
		return right
	}

	lType := left.Type()
	rType := right.Type()

	switch {
	case lType == object.INTEGER_OBJ && rType == object.INTEGER_OBJ:
		return e.evalIntegerInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.STRING_OBJ && rType == object.STRING_OBJ:
		return e.evalStringInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.COMPLEX_OBJ || rType == object.COMPLEX_OBJ:
		return e.evalComplexInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.FLOAT_OBJ || rType == object.FLOAT_OBJ:
		// For now, treat float operations as complex to simplify.
		// A more complete implementation would have a separate float path.
		return e.evalComplexInfixExpression(ctx, node.Pos(), node.Op, left, right)
	default:
		return &object.SymbolicPlaceholder{Reason: "binary expression"}
	}
}
