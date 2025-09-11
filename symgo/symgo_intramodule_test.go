package symgo_test

import (
	"context"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestIntraModuleCall(t *testing.T) {
	// This test simulates a call to a function in another package within the same module.
	// The symgo engine should recursively evaluate this call, not treat it as a symbolic placeholder.
	ctx := context.Background()

	// The testdata directory contains a simple multi-package module.
	moduleDir := filepath.Join("testdata", "intramodule")
	mainPkgPath := "example.com/intramodule/main"

	// Use a module-aware scanner. This is crucial for the test.
	scanner, err := goscan.New(
		goscan.WithWorkDir(moduleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Create a symgo interpreter.
	interp, err := symgo.NewInterpreter(scanner)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	// Scan the main package.
	pkg, err := scanner.ScanPackageByImport(ctx, mainPkgPath)
	if err != nil {
		t.Fatalf("ScanPackageByImport failed: %v", err)
	}

	// Find the main file and eval it to populate the env
	mainFile := findFile(t, pkg, "main.go")

	_, err = interp.Eval(ctx, mainFile, pkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	// Get the function object from the environment.
	mainObj, ok := interp.FindObjectInPackage(mainPkgPath, "main")
	if !ok {
		t.Fatal("main function not found in interpreter environment")
	}
	mainFunc, ok := mainObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'main' is not a function, but %T", mainObj)
	}

	// Evaluate the call to main().
	result, err := interp.Apply(ctx, mainFunc, nil, pkg)
	if err != nil {
		t.Fatalf("Apply main function failed: %v", err)
	}

	// The result from Apply is a ReturnValue, we need to unwrap it.
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		if result == nil {
			t.Fatal("expected result to be a *symgo.ReturnValue, but got nil")
		}
		t.Fatalf("expected result to be a *symgo.ReturnValue, but got %T: %v", result, result.Inspect())
	}

	// Check the result.
	// With the bug, `retVal.Value` will be a *symgo.SymbolicPlaceholder.
	// After the fix, it should be a *symgo.String.
	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be a *symgo.String, but got %T: %v", retVal.Value, retVal.Value.Inspect())
	}

	expected := "hello from helper"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}
