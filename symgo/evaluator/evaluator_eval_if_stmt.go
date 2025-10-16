package evaluator

import (
	"context"
	"go/ast"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalIfStmt(ctx context.Context, n *ast.IfStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	ifStmtEnv := env
	if n.Init != nil {
		ifStmtEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, ifStmtEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	// Also evaluate the condition to trace any function calls within it.
	if n.Cond != nil {
		if condResult := e.Eval(ctx, n.Cond, ifStmtEnv, pkg); isError(condResult) {
			// If the condition errors, we can't proceed.
			return condResult
		}
	}

	// Evaluate both branches. Each gets its own enclosed environment.
	thenEnv := object.NewEnclosedEnvironment(ifStmtEnv)
	thenResult := e.Eval(ctx, n.Body, thenEnv, pkg)

	var elseResult object.Object
	if n.Else != nil {
		elseEnv := object.NewEnclosedEnvironment(ifStmtEnv)
		elseResult = e.Eval(ctx, n.Else, elseEnv, pkg)
	}

	// If the 'then' branch returned a control flow object, propagate it.
	// This is a heuristic; a more complex analysis might merge states.
	// We prioritize the 'then' branch's signal.
	// We do NOT propagate ReturnValue, as that would prematurely terminate
	// the analysis of the current function just because one symbolic path returned.
	switch thenResult.(type) {
	case *object.Error, *object.Break, *object.Continue:
		return thenResult
	}
	// Otherwise, check the 'else' branch.
	switch elseResult.(type) {
	case *object.Error, *object.Break, *object.Continue:
		return elseResult
	}

	// A more sophisticated, path-sensitive analysis would require a different
	// approach. For now, if no control flow signal was returned, we continue.
	return nil
}
