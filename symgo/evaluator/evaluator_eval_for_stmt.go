package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalForStmt(ctx context.Context, n *ast.ForStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// For symbolic execution, we unroll the loop once.
	// A more sophisticated engine might unroll N times or use summaries.
	forEnv := object.NewEnclosedEnvironment(env)

	if n.Init != nil {
		if initResult := e.Eval(ctx, n.Init, forEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	// Also evaluate the condition to trace any function calls within it.
	if n.Cond != nil {
		if condResult := e.Eval(ctx, n.Cond, forEnv, pkg); isError(condResult) {
			// If the condition errors, we can't proceed with analysis of this loop.
			return condResult
		}
	}

	// We don't check the condition's result, just execute the body once symbolically.
	if n.Body != nil {
		result := e.Eval(ctx, n.Body, object.NewEnclosedEnvironment(forEnv), pkg)
		if result != nil {
			switch obj := result.(type) {
			case *object.Break:
				// If the break has a label, it's for an outer loop. Propagate it.
				if obj.Label != "" {
					return obj
				}
				// Otherwise, it's for this loop, so we absorb it.
				return &object.SymbolicPlaceholder{Reason: "for loop"}
			case *object.Continue:
				// If the continue has a label, it's for an outer loop. Propagate it.
				if obj.Label != "" {
					return obj
				}
				// Otherwise, it's for this loop, so we absorb it.
				return &object.SymbolicPlaceholder{Reason: "for loop"}
			case *object.Error:
				return result // Propagate errors.
			}
		}
	}

	// The result of a for statement is not a value.
	return &object.SymbolicPlaceholder{Reason: "for loop"}
}
