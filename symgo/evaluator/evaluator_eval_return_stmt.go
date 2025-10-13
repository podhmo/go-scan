package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalReturnStmt(ctx context.Context, n *ast.ReturnStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if len(n.Results) == 0 {
		return &object.ReturnValue{Value: object.NIL} // naked return
	}

	if len(n.Results) == 1 {
		val := e.Eval(ctx, n.Results[0], env, pkg)
		if isError(val) {
			return val
		}
		// The result of an expression must be fully evaluated before being returned.
		val = e.forceEval(ctx, val, pkg)
		if isError(val) {
			return val
		}

		if _, ok := val.(*object.ReturnValue); ok {
			return val
		}
		return &object.ReturnValue{Value: val}
	}

	// Handle multiple return values
	vals := e.evalExpressions(ctx, n.Results, env, pkg)
	if len(vals) == 1 && isError(vals[0]) {
		return vals[0] // Error occurred during expression evaluation
	}

	return &object.ReturnValue{Value: &object.MultiReturn{Values: vals}}
}
