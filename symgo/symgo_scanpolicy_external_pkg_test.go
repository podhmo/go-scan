package symgo_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	goscan "github.com/podhmo/go-scan"
)

func TestScanPolicy_ExternalPackage(t *testing.T) {
	t.Run("calling a function in an unscanned package should be handled symbolically", func(t *testing.T) {
		// This test simulates the user's scenario.
		// We have a main app in "mymodule".
		// It imports and uses a library from "mymodule/lib".
		// The library in turn imports and uses an external dependency, "github.com/user/external",
		// where the package name `externalpkg` does not match the import path.
		//
		// We use an explicit alias in this version of the test to isolate the bug.
		// If this test passes, the problem is in the automatic package name resolution.
		// If it fails, the problem is deeper.

		libSource := `
package lib
import externalpkg "github.com/user/external"
func UseExternal() string {
	return externalpkg.DoSomething()
}
`
		mainSource := `
package main
import "mymodule/lib"
func main() {
	lib.UseExternal()
}
`
		externalSource := `
package externalpkg
func DoSomething() string {
	return "did something external"
}
`
		dir, cleanup := scantest.WriteFiles(t, map[string]string{
			"go.mod":            "module mymodule",
			"lib/lib.go":        libSource,
			"main/main.go":      mainSource,
			"external/go.mod":   "module github.com/user/external",
			"external/pkg.go":   externalSource,
		})
		defer cleanup()

		// Use go.work to create a multi-module workspace that includes the main module
		// and the simulated external module.
		goWorkPath := filepath.Join(dir, "go.work")
		goWorkContent := "go 1.21\n\nuse (\n\t.\n\t./external\n)\n"
		if err := os.WriteFile(goWorkPath, []byte(goWorkContent), 0644); err != nil {
			t.Fatalf("failed to write go.work: %v", err)
		}

		s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
		if err != nil {
			t.Fatalf("goscan.New() failed: %+v", err)
		}

		// Scan the entire workspace to populate the scanner.
		pkgs, err := s.Scan(context.Background(), "./...")
		if err != nil {
			t.Fatalf("s.Scan() failed: %+v", err)
		}

		var mainPkg *goscan.Package
		for _, p := range pkgs {
			if p.ImportPath == "mymodule/main" {
				mainPkg = p
				break
			}
		}
		if mainPkg == nil {
			t.Fatal("could not find main package")
		}

		// This policy mimics find-orphans: only scan packages in our own module.
		// "github.com/user/external" will be excluded from symbolic execution,
		// but its type information should be available from the initial scan.
		scanPolicy := func(path string) bool {
			return strings.HasPrefix(path, "mymodule")
		}

		interp, err := symgo.NewInterpreter(s, symgo.WithScanPolicy(scanPolicy))
		if err != nil {
			t.Fatalf("NewInterpreter() failed: %+v", err)
		}

		// Find the main file's AST node.
		mainFilePath := filepath.Join(dir, "main/main.go")
		mainAstFile, ok := mainPkg.AstFiles[mainFilePath]
		if !ok {
			t.Fatalf("could not find AST for main file: %s", mainFilePath)
		}

		// Evaluate the main file to load all symbols.
		_, err = interp.Eval(context.Background(), mainAstFile, mainPkg)
		if err != nil {
			t.Fatalf("interp.Eval(file) failed unexpectedly: %+v", err)
		}

		// Find and apply the main function.
		mainFnObj, ok := interp.FindObject("main")
		if !ok {
			t.Fatal("could not find main function")
		}
		mainFn, ok := mainFnObj.(*object.Function)
		if !ok {
			t.Fatalf("found main but it's not a function, it's a %T", mainFnObj)
		}

		// This should succeed. The evaluator should correctly identify the package name
		// for the external dependency as `externalpkg`. Because it's excluded by the
		// scan policy, the call to `DoSomething` should be treated as a symbolic
		// placeholder, and the whole execution should complete without error.
		_, err = interp.Apply(context.Background(), mainFn, nil, mainPkg)
		if err != nil {
			t.Fatalf("expected Apply to succeed by treating external call symbolically, but got error: %v", err)
		}
	})
}
