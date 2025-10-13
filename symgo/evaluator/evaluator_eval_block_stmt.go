package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"os"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalBlockStmt(ctx context.Context, block *ast.BlockStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if block == nil {
		return nil // Function has no body, which is valid for declarations-only scanning.
	}
	var result object.Object
	// The caller is responsible for creating a new scope if one is needed.
	// We evaluate the statements in the provided environment.
	for _, stmt := range block.List {
		// If a statement is itself a block, it introduces a new lexical scope.
		if innerBlock, ok := stmt.(*ast.BlockStmt); ok {
			blockEnv := object.NewEnclosedEnvironment(env)
			result = e.evalBlockStmt(ctx, innerBlock, blockEnv, pkg)
		} else {
			result = e.Eval(ctx, stmt, env, pkg)
		}

		// It's possible for a statement (like a declaration) to evaluate to a nil object.
		// We must check for this before calling .Type() to avoid a panic.
		if result == nil {
			continue
		}

		switch result.(type) {
		case *object.Error:
			fmt.Fprintf(os.Stderr, "!!!!!!!!!error: %s\n", result.Inspect())
			return result
		case *object.ReturnValue, *object.PanicError, *object.Break, *object.Continue:
			return result
		}
	}

	return result
}
