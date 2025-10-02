package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalLabeledStmt(ctx context.Context, n *ast.LabeledStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	result := e.Eval(ctx, n.Stmt, env, pkg)

	switch obj := result.(type) {
	case *object.Break:
		if obj.Label == n.Label.Name {
			// This break was for this label. Absorb it.
			return &object.SymbolicPlaceholder{Reason: "labeled statement"}
		}
	case *object.Continue:
		if obj.Label == n.Label.Name {
			// This continue was for this label. Absorb it and continue symbolic execution.
			return &object.SymbolicPlaceholder{Reason: "labeled statement"}
		}
	}

	// If it's a break/continue for another label, or any other kind of object,
	// just propagate it up.
	return result
}
