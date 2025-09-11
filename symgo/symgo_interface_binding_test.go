package symgo_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestInterfaceBinding(t *testing.T) {
	// State to be modified by the intrinsic.
	var intrinsicCalled bool

	// Define the test files, including a go.mod to define the module context.
	files := map[string]string{
		"go.mod": "module myapp\n\ngo 1.22",
		"main.go": `
package main
import "io"
// TargetFunc is the function we will analyze.
func TargetFunc(writer io.Writer) {
	writer.WriteString("hello")
}`,
	}

	// Create a temporary directory with the files.
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Create a scanner configured to use the temporary directory as its root
	// and to resolve stdlib packages.
	s, err := goscan.New(
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Run the test using the scantest harness.
	_, err = scantest.Run(t, context.Background(), dir, []string{"."}, func(ctx context.Context, scanner *goscan.Scanner, pkgs []*goscan.Package) error {
		// Setup: Create a symgo interpreter using the scanner from the outer scope.
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		// Action: Bind the interface `io.Writer` to the concrete type `*bytes.Buffer`.
		if err := interp.BindInterface("io.Writer", "*bytes.Buffer"); err != nil {
			return fmt.Errorf("failed to bind interface: %w", err)
		}

		// Action: Register an intrinsic for the method on the concrete type.
		interp.RegisterIntrinsic("(*bytes.Buffer).WriteString", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			intrinsicCalled = true
			return nil
		})

		// Find the main package and AST file.
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, but got %d", len(pkgs))
		}
		pkg := pkgs[0]
		mainFile, ok := pkg.AstFiles[dir+"/main.go"]
		if !ok {
			return fmt.Errorf("could not find AST for main.go")
		}

		// Eval the file to load top-level declarations.
		if _, err := interp.Eval(ctx, mainFile, pkg); err != nil {
			return fmt.Errorf("symgo eval failed: %+v", err)
		}

		// Find the target function to analyze.
		targetFn, ok := interp.FindObjectInPackage("myapp", "TargetFunc")
		if !ok {
			return fmt.Errorf("TargetFunc function not found")
		}

		// Create a symbolic argument for the `writer` parameter.
		writerArg, err := interp.NewSymbolic("writer", "io.Writer")
		if err != nil {
			return fmt.Errorf("failed to create symbolic arg: %w", err)
		}

		// Apply the function with the symbolic argument.
		if _, err := interp.Apply(context.Background(), targetFn, []symgo.Object{writerArg}, pkg); err != nil {
			return fmt.Errorf("symgo apply failed: %+v", err)
		}
		return nil
	}, scantest.WithScanner(s)) // Pass the pre-configured scanner to scantest.
	if err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}

	// Verification: Check if the intrinsic was called.
	if !intrinsicCalled {
		t.Errorf("expected intrinsic for (*bytes.Buffer).WriteString to be called, but it was not")
	}
}
