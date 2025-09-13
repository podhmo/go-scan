package integration_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// TestCrossMainPackageSymbolCollision reproduces a bug where the symgo evaluator
// would confuse functions with the same name from different `main` packages when
// analyzing a whole workspace.
func TestCrossMainPackageSymbolCollision(t *testing.T) {
	// Source code for two separate 'main' packages. Both define a `run` function.
	// If the evaluator is working correctly, calling `pkg_b.main` should only
	// execute symbols from `pkg_b`.
	pkgA_main := `
package main
import "fmt"
func main() {
	run()
}
func run() {
	helper_a()
}
func helper_a() {
	fmt.Println("helper_a called")
}
`
	pkgB_main := `
package main
import "fmt"
func main() {
	run()
}
func run() {
	fmt.Println("pkg_b.run called")
}
`

	// Use scantest to create a temporary workspace.
	// We add a dummy go.mod at the root to satisfy the scanner's legacy
	// initialization checks, while the go.work file correctly defines the workspace.
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":        "module example.com/workspace\n\ngo 1.21",
		"go.work":       "go 1.21\nuse (\n./pkg_a\n./pkg_b\n)\n",
		"pkg_a/go.mod":  "module example.com/pkg_a\n\ngo 1.21",
		"pkg_a/main.go": pkgA_main,
		"pkg_b/go.mod":  "module example.com/pkg_b\n\ngo 1.21",
		"pkg_b/main.go": pkgB_main,
	})
	defer cleanup()

	ctx := context.Background()

	// Manually create the scanner, pointing its working directory to the temporary
	// workspace root. This will make it discover the go.work file.
	scannerOpts := []goscan.ScannerOption{
		goscan.WithWorkDir(tmpdir),
	}
	s, err := goscan.New(scannerOpts...)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Scan all packages in the temporary workspace.
	pkgs, err := s.Scan(ctx, "./...")
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// This is the core test logic.
	var pkgA, pkgB *goscan.Package
	var foundPaths []string
	for _, p := range pkgs {
		foundPaths = append(foundPaths, p.ImportPath)
		if p.ImportPath == "example.com/workspace/pkg_a" {
			pkgA = p
		} else if p.ImportPath == "example.com/workspace/pkg_b" {
			pkgB = p
		}
	}
	if pkgA == nil || pkgB == nil {
		t.Fatalf("could not find both pkg_a and pkg_b main packages, found paths: %v", foundPaths)
	}

	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))

	interp, err := symgo.NewInterpreter(s,
		symgo.WithLogger(logger),
		symgo.WithPrimaryAnalysisScope("example.com/workspace/pkg_a", "example.com/workspace/pkg_b"),
	)
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	for _, p := range []*goscan.Package{pkgA, pkgB} {
		for _, fileAst := range p.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, p); err != nil {
				t.Fatalf("toplevel eval failed for %s: %v", p.ImportPath, err)
			}
		}
	}

	mainFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/workspace/pkg_b", "main")
	if !ok {
		t.Fatalf("could not find main function in pkg_b")
	}
	mainFunc, ok := mainFuncObj.(*object.Function)
	if !ok {
		t.Fatalf("main object in pkg_b is not a function, but %T", mainFuncObj)
	}

	_, err = interp.Apply(ctx, mainFunc, []object.Object{}, pkgB)
	if err != nil {
		// With the fix, there should be no error.
		t.Fatalf("symbolic execution failed unexpectedly: %v", err)
	}
}
