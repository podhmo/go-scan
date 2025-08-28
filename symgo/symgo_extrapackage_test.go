package symgo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestSymgo_WithExtraPackages(t *testing.T) {
	// This test simulates a workspace with two modules:
	// - app: the main application
	// - helper: a library that app depends on
	// We want to test that by default, calls from app to helper are not deeply evaluated,
	// but when `WithExtraPackages` is used, they are.
	files := map[string]string{
		"app/go.mod": "module example.com/app\ngo 1.22\n\nrequire example.com/helper v0.0.0\n\nreplace example.com/helper => ../helper\n",
		"app/main.go": `
package main

import (
	"fmt"
	"example.com/helper"
)

func main() {
	fmt.Println(helper.Greet("world"))
}
`,
		"helper/go.mod": "module example.com/helper\ngo 1.22\n",
		"helper/greet.go": `
package helper

func Greet(name string) string {
	return "Hello, " + name
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Setup a go.work file to create a multi-module workspace
	scantest.RunCommand(t, dir, "go", "work", "init", "./app", "./helper")
	scantest.RunCommand(t, dir, "go", "work", "use", "./app")
	scantest.RunCommand(t, dir, "go", "work", "use", "./helper")
	scantest.RunCommand(t, dir, "go", "mod", "tidy", "-C", filepath.Join(dir, "app"))

	mainModuleDir := filepath.Join(dir, "app")
	mainPkgPath := "example.com/app"
	ctx := context.Background()

	t.Run("default behavior: external calls are symbolic", func(t *testing.T) {
		scanner, err := goscan.New(
			goscan.WithWorkDir(mainModuleDir),
			goscan.WithGoModuleResolver(),
		)
		if err != nil {
			t.Fatalf("New scanner failed: %v", err)
		}

		interp, err := symgo.NewInterpreter(scanner)
		if err != nil {
			t.Fatalf("NewInterpreter failed: %v", err)
		}

		result := runAnalysis(t, ctx, interp, mainPkgPath)

		_, ok := result.(*object.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("expected return value to be a *symgo.SymbolicPlaceholder, but got %T: %v", result, result.Inspect())
		}
	})

	t.Run("with extra package: external calls are evaluated", func(t *testing.T) {
		scanner, err := goscan.New(
			goscan.WithWorkDir(mainModuleDir),
			goscan.WithGoModuleResolver(),
		)
		if err != nil {
			t.Fatalf("New scanner failed: %v", err)
		}

		policy := func(importPath string) bool {
			// The default policy would only scan example.com/app.
			// We extend it to also scan example.com/helper for this test.
			return strings.HasPrefix(importPath, "example.com/app") || strings.HasPrefix(importPath, "example.com/helper")
		}
		interp, err := symgo.NewInterpreter(scanner, symgo.WithScanPolicy(policy))
		if err != nil {
			t.Fatalf("NewInterpreter with extra packages failed: %v", err)
		}

		result := runAnalysis(t, ctx, interp, mainPkgPath)

		retStr, ok := result.(*object.String)
		if !ok {
			t.Fatalf("expected return value to be a *symgo.String, but got %T: %v", result, result.Inspect())
		}
		if retStr.Value != "Hello, world" {
			t.Errorf("expected return value to be 'Hello, world', but got %q", retStr.Value)
		}
	})
}

// runAnalysis is a helper to perform the symbolic execution of the main function.
func runAnalysis(t *testing.T, ctx context.Context, interp *symgo.Interpreter, mainPkgPath string) object.Object {
	t.Helper()
	pkg, err := interp.Scanner().ScanPackageByImport(ctx, mainPkgPath)
	if err != nil {
		t.Fatalf("ScanPackageByImport failed: %v", err)
	}

	mainFile := FindFile(t, pkg, "main.go")

	_, err = interp.Eval(ctx, mainFile, pkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	mainObj, ok := interp.FindObject("main")
	if !ok {
		t.Fatal("main function not found in interpreter environment")
	}
	mainFunc, ok := mainObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'main' is not a function, but %T", mainObj)
	}

	var capturedArg object.Object
	interp.RegisterIntrinsic("fmt.Println", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		if len(args) > 0 {
			if retVal, ok := args[0].(*object.ReturnValue); ok {
				capturedArg = retVal.Value
			} else {
				capturedArg = args[0]
			}
		}
		return nil
	})

	_, err = interp.Apply(ctx, mainFunc, nil, pkg)
	if err != nil {
		t.Fatalf("Apply main function failed: %v", err)
	}

	if capturedArg == nil {
		t.Fatal("fmt.Println was not called or was called with no arguments")
	}

	return capturedArg
}
