package evaluator

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/podhmo/go-scan/minigo2/object"
)

// Eval is the central function of the evaluator. It traverses the AST
// and returns the result of the evaluation as an object.Object.
func Eval(node ast.Node) object.Object {
	switch n := node.(type) {
	// Statements
	case *ast.ExprStmt:
		// For an expression statement, we evaluate the underlying expression.
		return Eval(n.X)

	// Literals
	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			i, err := strconv.ParseInt(n.Value, 0, 64)
			if err != nil {
				// In a real interpreter, we'd return an error object.
				// For now, we return nil.
				return nil
			}
			return &object.Integer{Value: i}
		case token.STRING:
			s, err := strconv.Unquote(n.Value)
			if err != nil {
				// Return nil on error for now.
				return nil
			}
			return &object.String{Value: s}
		}
	}

	return nil
}
