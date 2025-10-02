package evaluator

import (
	"context"
	"go/ast"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalSelectStmt(ctx context.Context, n *ast.SelectStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if n.Body == nil {
		return &object.SymbolicPlaceholder{Reason: "empty select statement"}
	}
	// Symbolically execute all cases.
	for _, c := range n.Body.List {
		if caseClause, ok := c.(*ast.CommClause); ok {
			caseEnv := object.NewEnclosedEnvironment(env)

			// Evaluate the communication expression (e.g., the channel operation).
			if caseClause.Comm != nil {
				if res := e.Eval(ctx, caseClause.Comm, caseEnv, pkg); isError(res) {
					e.logc(ctx, slog.LevelWarn, "error evaluating select case communication", "error", res)
				}
			}

			// Evaluate the body of the case.
			for _, stmt := range caseClause.Body {
				if res := e.Eval(ctx, stmt, caseEnv, pkg); isError(res) {
					e.logc(ctx, slog.LevelWarn, "error evaluating statement in select case", "error", res)
					if isInfiniteRecursionError(res) {
						return res // Stop processing on infinite recursion
					}
				}
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "select statement"}
}
