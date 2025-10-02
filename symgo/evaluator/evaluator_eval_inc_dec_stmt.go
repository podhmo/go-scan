package evaluator

import (
	"context"
	"go/ast"
	"go/token"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalIncDecStmt(ctx context.Context, n *ast.IncDecStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Evaluate the expression to trace any calls, but we need the identifier.
	ident, ok := n.X.(*ast.Ident)
	if !ok {
		e.Eval(ctx, n.X, env, pkg)
		return nil // Cannot perform state change on complex expression.
	}

	obj, ok := env.Get(ident.Name)
	if !ok {
		return e.newError(ctx, n.Pos(), "identifier not found for IncDec: %s", ident.Name)
	}

	variable, ok := obj.(*object.Variable)
	if !ok {
		return e.newError(ctx, n.Pos(), "cannot increment/decrement non-variable: %s", ident.Name)
	}

	val := e.evalVariable(ctx, variable, pkg)
	if isError(val) {
		return val
	}

	var newInt int64
	switch v := val.(type) {
	case *object.Integer:
		newInt = v.Value
	case *object.SymbolicPlaceholder:
		// If it's a placeholder, the result of inc/dec is still a placeholder.
		// We don't change the variable's value, just acknowledge the operation.
		return nil
	default:
		// For other types, we can't meaningfully inc/dec.
		return nil
	}

	switch n.Tok {
	case token.INC:
		newInt++
	case token.DEC:
		newInt--
	}

	// Update the variable's value in place.
	variable.Value = &object.Integer{Value: newInt}
	// Also mark it as evaluated, since it now has a concrete value.
	variable.IsEvaluated = true
	// No need to call env.Set here because we have modified the object in place.
	return nil
}
