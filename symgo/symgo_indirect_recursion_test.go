package symgo

import (
	"context"
	"fmt"
	"go/ast"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestSymgo_IndirectRecursion(t *testing.T) {
	source := `
package main

// cont is a helper function to create an indirect call.
func cont(f func(int), n int) {
	f(n)
}

// Ping is an indirectly mutually recursive function with Pong.
func Ping(n int) {
	if n > 1 {
		return
	}
	// Calls Pong via the cont helper
	cont(Pong, n+1)
}

// Pong is an indirectly mutually recursive function with Ping.
func Pong(n int) {
	if n > 1 {
		return
	}
	// Calls Ping via the cont helper
	cont(Ping, n+1)
}

func main() {
	Ping(0)
}
`
	files := map[string]string{
		"go.mod":  "module main",
		"main.go": source,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// Use the scanner provided by scantest.Run to create the interpreter.
		// We enable memoization as it's often used with recursion.
		interp, err := NewInterpreter(s, WithMemoization(true))
		if err != nil {
			return fmt.Errorf("NewInterpreter: %w", err)
		}

		// The packages are already scanned by scantest.Run.
		// We need to get the AST for the main file to start evaluation.
		if len(pkgs) == 0 || len(pkgs[0].AstFiles) == 0 {
			return fmt.Errorf("no packages or files scanned")
		}

		var fileNode *ast.File
		for _, f := range pkgs[0].AstFiles {
			fileNode = f // Get the first (and only) file.
			break
		}
		if fileNode == nil {
			return fmt.Errorf("AST file node not found in scanned package")
		}

		// We need to evaluate the file to populate the interpreter's environment with top-level decls.
		if _, err := interp.Eval(ctx, fileNode, pkgs[0]); err != nil {
			return fmt.Errorf("Eval: %w", err)
		}

		// Find the main function to start analysis.
		mainFn, ok := interp.FindObjectInPackage(ctx, "main", "main")
		if !ok {
			return fmt.Errorf("main function not found")
		}

		// Apply the main function. The test passes if this does not cause a stack overflow.
		// The timeout on the test command itself will catch any hangs.
		_, err = interp.Apply(ctx, mainFn, nil, pkgs[0])
		if err != nil {
			return fmt.Errorf("Apply: %w", err)
		}
		return nil
	}

	_, err := scantest.Run(t, context.Background(), dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}