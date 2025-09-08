package symgo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

const (
	pkgA = "myapp/a"
	pkgB = "myapp/b"
	pkgC = "myapp/c"
)

var fileA = `
package a
type I interface {
	M()
}`

var fileB = `
package b
import (
	"myapp/a"
)
func Use(i a.I) {
	i.M()
}`

var fileC = `
package c
type S struct{}
func (s *S) M() {}`

func TestInterfaceDiscovery(t *testing.T) {
	baseFiles := map[string]string{
		"go.mod":   "module myapp",
		"a/a.go":   fileA,
		"b/b.go":   fileB,
		"c/c.go":   fileC,
		"a/go.mod": "module myapp/a",
		"b/go.mod": "module myapp/b",
		"c/go.mod": "module myapp/c",
	}

	// Define the 6 permutations of discovery order
	discoveryOrders := [][]string{
		{pkgA, pkgB, pkgC}, // I -> U -> S
		{pkgA, pkgC, pkgB}, // I -> S -> U
		{pkgB, pkgA, pkgC}, // U -> I -> S
		{pkgB, pkgC, pkgA}, // U -> S -> I
		{pkgC, pkgA, pkgB}, // S -> I -> U
		{pkgC, pkgB, pkgA}, // S -> U -> I
	}

	for _, order := range discoveryOrders {
		orderName := strings.Join(order, "_then_")
		t.Run(orderName, func(t *testing.T) {
			var intrinsicCalled bool

			dir, cleanup := scantest.WriteFiles(t, baseFiles)
			defer cleanup()

			// Since this test involves multiple modules (a, b, c), we need to configure the scanner
			// with the workspace root and tell it to find all modules.
			s, err := goscan.New(
				goscan.WithWorkspaceRoot(dir),
				goscan.WithGoModuleWorkspaces(), // Discover go.mod files in subdirectories
			)
			if err != nil {
				t.Fatalf("failed to create scanner: %v", err)
			}

			// Set the primary analysis scope to include all our packages
			interp, err := symgo.New(s, symgo.WithPrimaryAnalysisScope("myapp/..."))
			if err != nil {
				t.Fatalf("failed to create interpreter: %v", err)
			}

			// Register the intrinsic on the concrete method implementation.
			interp.RegisterIntrinsic("(*c.S).M", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				intrinsicCalled = true
				return nil
			})

			// Get all packages that the scanner found.
			allPkgs, err := s.Scan(context.Background(), "./...")
			if err != nil {
				t.Fatalf("scanner.Scan failed: %v", err)
			}

			// Evaluate files in the specified discovery order
			for _, pkgPath := range order {
				var foundPkg *goscan.Package
				for _, p := range allPkgs {
					if p.ImportPath == pkgPath {
						foundPkg = p
						break
					}
				}
				if foundPkg == nil {
					t.Fatalf("package not found: %s", pkgPath)
				}

				// Eval each file in the package
				for _, file := range foundPkg.Files {
					astFile := foundPkg.AstFiles[filepath.Join(dir, file)]
					if _, err := interp.Eval(context.Background(), astFile, foundPkg); err != nil {
						t.Fatalf("interp.Eval failed for %s: %+v", file, err)
					}
				}
			}

			// Now, find the 'Use' function in package B and call it.
			var useFuncPkg *goscan.Package
			for _, p := range allPkgs {
				if p.ImportPath == pkgB {
					useFuncPkg = p
					break
				}
			}
			if useFuncPkg == nil {
				t.Fatalf("package not found: %s", pkgB)
			}

			useFunc, ok := interp.FindObject("Use")
			if !ok {
				// The object might be in the package's scope, not global
				pkgObj, ok := interp.FindObject(pkgB)
				if !ok {
					t.Fatalf("could not find package object for %s", pkgB)
				}
				useFunc, ok = pkgObj.Env.Get("Use")
				if !ok {
					t.Fatalf("could not find Use function in package %s", pkgB)
				}
			}

			// Create a symbolic argument for the interface parameter `a.I`.
			arg, err := interp.NewSymbolic("arg", "myapp/a.I")
			if err != nil {
				t.Fatalf("failed to create symbolic arg: %v", err)
			}

			// Apply the function.
			if _, err := interp.Apply(context.Background(), useFunc, []symgo.Object{arg}, useFuncPkg); err != nil {
				t.Fatalf("symgo.Apply failed: %+v", err)
			}

			if !intrinsicCalled {
				t.Errorf("expected intrinsic for (*c.S).M to be called, but it was not")
			}
		})
	}
}
