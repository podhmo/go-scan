package evaluator

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/scope"
)

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	// We can add more context here later, like a scanner instance.
}

// New creates a new Evaluator.
func New() *Evaluator {
	return &Evaluator{}
}

// Eval is the main dispatch loop for the evaluator.
func (e *Evaluator) Eval(node ast.Node, s *scope.Scope) object.Object {
	switch n := node.(type) {
	case *ast.BasicLit:
		return e.evalBasicLit(n)
	case *ast.Ident:
		return e.evalIdent(n, s)
	case *ast.AssignStmt:
		return e.evalAssignStmt(n, s)
	case *ast.BlockStmt:
		return e.evalBlockStatement(n, s)
	case *ast.ReturnStmt:
		return e.evalReturnStmt(n, s)
	case *ast.IfStmt:
		return e.evalIfStmt(n, s)
	case *ast.ForStmt:
		return e.evalForStmt(n, s)
	case *ast.SwitchStmt:
		return e.evalSwitchStmt(n, s)
	}
	return newError("evaluation not implemented for %T", node)
}

// evalSwitchStmt evaluates a switch statement. It traverses all case clauses
// to discover patterns that could occur in any branch.
func (e *Evaluator) evalSwitchStmt(n *ast.SwitchStmt, s *scope.Scope) object.Object {
	// The result of a switch statement is the result of the last evaluated statement
	// in the taken branch. Since we evaluate all branches, we'll just return the
	// result of the last statement in the last case block for now.
	var result object.Object
	if n.Body != nil {
		for _, c := range n.Body.List {
			if caseClause, ok := c.(*ast.CaseClause); ok {
				for _, stmt := range caseClause.Body {
					result = e.Eval(stmt, s)
					// We don't break early on return/error here because we want
					// to analyze all branches. A more sophisticated implementation
					// might collect results from all branches.
				}
			}
		}
	}
	return result
}

// evalForStmt evaluates a for statement. Following the "bounded analysis" principle,
// it evaluates the loop body exactly once to find patterns within it.
// It ignores the loop's condition, initializer, and post-iteration statement.
func (e *Evaluator) evalForStmt(n *ast.ForStmt, s *scope.Scope) object.Object {
	return e.Eval(n.Body, s)
}

// evalIfStmt evaluates an if statement. Following our heuristic-based approach,
// it evaluates the body to see what *could* happen, without complex path forking.
// For simplicity, it currently ignores the condition and the else block.
func (e *Evaluator) evalIfStmt(n *ast.IfStmt, s *scope.Scope) object.Object {
	return e.Eval(n.Body, s)
}

func (e *Evaluator) evalBlockStatement(block *ast.BlockStmt, s *scope.Scope) object.Object {
	var result object.Object

	for _, stmt := range block.List {
		result = e.Eval(stmt, s)

		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
		}
	}

	return result
}

func (e *Evaluator) evalReturnStmt(n *ast.ReturnStmt, s *scope.Scope) object.Object {
	if len(n.Results) != 1 {
		// For now, we only support single return values.
		return newError("unsupported return statement: expected 1 result")
	}
	val := e.Eval(n.Results[0], s)
	if isError(val) {
		return val
	}
	return &object.ReturnValue{Value: val}
}

func (e *Evaluator) evalAssignStmt(n *ast.AssignStmt, s *scope.Scope) object.Object {
	// For now, we only support simple assignment like `x = ...`
	if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
		return newError("unsupported assignment: expected 1 expression on each side")
	}

	// We only support assigning to an identifier.
	ident, ok := n.Lhs[0].(*ast.Ident)
	if !ok {
		return newError("unsupported assignment target: expected an identifier")
	}

	// Evaluate the right-hand side.
	val := e.Eval(n.Rhs[0], s)
	if isError(val) {
		return val
	}

	// We only support `=` for now, not `:=`.
	// This means the variable must already exist in the scope.
	if _, ok := s.Get(ident.Name); !ok {
		// TODO: This check might be too restrictive. For now, we allow setting new variables.
		// return newError("cannot assign to undeclared identifier: %s", ident.Name)
	}

	// Set the value in the scope.
	return s.Set(ident.Name, val)
}

func (e *Evaluator) evalBasicLit(n *ast.BasicLit) object.Object {
	switch n.Kind {
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return newError("could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	// TODO: Add support for INT, FLOAT, etc. later.
	default:
		return newError("unsupported literal type: %s", n.Kind)
	}
}

func (e *Evaluator) evalIdent(n *ast.Ident, s *scope.Scope) object.Object {
	if val, ok := s.Get(n.Name); ok {
		return val
	}
	return newError("identifier not found: %s", n.Name)
}

// newError is a helper to create a new Error object.
func newError(format string, args ...interface{}) *object.Error {
	return &object.Error{Message: fmt.Sprintf(format, args...)}
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}
