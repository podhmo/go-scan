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
	}
	return newError("evaluation not implemented for %T", node)
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
