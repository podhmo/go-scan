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
	}
	return newError("evaluation not implemented for %T", node)
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
