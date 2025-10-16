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

		// --- NEW: Propagate named return type info to function literals ---
		// If we are returning a function literal from a function that has a
		// named function type as its return value, we need to attach that
		// type info to the function object.
		if fn, isFunc := val.(*object.Function); isFunc {
			if len(e.callStack) > 0 {
				// Get the definition of the function we are returning from.
				caller := e.callStack[len(e.callStack)-1].Fn
				if caller != nil && caller.Def != nil && len(caller.Def.Results) == 1 {
					// Get the expected return type from the function signature.
					expectedReturnType := caller.Def.Results[0]
					if expectedReturnType.Type != nil && expectedReturnType.Type.Name != "" {
						// It's a named type. Resolve it and set it on the function object.
						if resolvedType, err := expectedReturnType.Type.Resolve(ctx); err == nil && resolvedType != nil {
							fn.SetTypeInfo(resolvedType)
						}
					}
				}
			}
		}
		// --- End NEW ---

		return &object.ReturnValue{Value: val}
	}

	// Handle multiple return values
	vals := e.evalExpressions(ctx, n.Results, env, pkg)
	if len(vals) == 1 && isError(vals[0]) {
		return vals[0] // Error occurred during expression evaluation
	}

	return &object.ReturnValue{Value: &object.MultiReturn{Values: vals}}
}