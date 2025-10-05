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
	case token.GOTO:
		// Treat goto as a no-op for symbolic analysis.
		// This avoids errors and complex control flow simulation,
		// allowing the tracer to continue to the next statement sequentially.
		return nil
	case token.FALLTHROUGH:
		return object.FALLTHROUGH
	default:
		return e.newError(ctx, n.Pos(), "unsupported branch statement: %s", n.Tok)
	}
}