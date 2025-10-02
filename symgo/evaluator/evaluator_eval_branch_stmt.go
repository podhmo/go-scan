package evaluator

import (
	"context"
	"go/ast"
	"go/token"

	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalBranchStmt(ctx context.Context, n *ast.BranchStmt) object.Object {
	var label string
	if n.Label != nil {
		label = n.Label.Name
	}

	switch n.Tok {
	case token.BREAK:
		return &object.Break{Label: label}
	case token.CONTINUE:
		return &object.Continue{Label: label}
	case token.FALLTHROUGH:
		return object.FALLTHROUGH
	default:
		return e.newError(ctx, n.Pos(), "unsupported branch statement: %s", n.Tok)
	}
}
