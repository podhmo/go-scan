package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalRangeStmt(ctx context.Context, n *ast.RangeStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// For symbolic execution, the most important part is to evaluate the expression
	// being ranged over, as it might contain function calls we need to trace.
	e.Eval(ctx, n.X, env, pkg)

	// We symbolically execute the body once.
	rangeEnv := object.NewEnclosedEnvironment(env)

	// Create placeholder variables for the key and value in the loop's scope.
	if n.Key != nil {
		if ident, ok := n.Key.(*ast.Ident); ok && ident.Name != "_" {
			keyVar := &object.Variable{
				Name:  ident.Name,
				Value: &object.SymbolicPlaceholder{Reason: "range loop key"},
			}
			rangeEnv.Set(ident.Name, keyVar)
		}
	}
	if n.Value != nil {
		if ident, ok := n.Value.(*ast.Ident); ok && ident.Name != "_" {
			valueVar := &object.Variable{
				Name:  ident.Name,
				Value: &object.SymbolicPlaceholder{Reason: "range loop value"},
			}
			rangeEnv.Set(ident.Name, valueVar)
		}
	}

	result := e.Eval(ctx, n.Body, rangeEnv, pkg)
	if result != nil {
		switch obj := result.(type) {
		case *object.Break:
			if obj.Label != "" {
				return obj
			}
			return &object.SymbolicPlaceholder{Reason: "for-range loop"}
		case *object.Continue:
			if obj.Label != "" {
				return obj
			}
			return &object.SymbolicPlaceholder{Reason: "for-range loop"}
		case *object.Error:
			return result // Propagate errors.
		}
	}

	return &object.SymbolicPlaceholder{Reason: "for-range loop"}
}
