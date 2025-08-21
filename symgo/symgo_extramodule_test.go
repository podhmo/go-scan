package symgo_test

import (
	"context"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestExtraModuleCall(t *testing.T) {
	// This test simulates a call to a function in an external, third-party module.
	// The symgo engine should NOT evaluate this call, but treat it as a symbolic placeholder.
	ctx := context.Background()

	moduleDir := filepath.Join("testdata", "extramodule")
	mainPkgPath := "example.com/extramodule/main"

	scanner, err := goscan.New(
		goscan.WithWorkDir(moduleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}


	interp, err := symgo.NewInterpreter(scanner)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	pkg, err := scanner.ScanPackageByImport(ctx, mainPkgPath)
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

	result, err := interp.Apply(ctx, mainFunc, nil, pkg)
	if err != nil {
		t.Fatalf("Apply main function failed: %v", err)
	}

	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *symgo.ReturnValue, but got %T: %v", result, result.Inspect())
	}

	// The result of calling an external function should be a symbolic placeholder.
	_, ok = retVal.Value.(*object.SymbolicPlaceholder)
	if !ok {
		t.Fatalf("expected return value to be a *symgo.SymbolicPlaceholder, but got %T: %v", retVal.Value, retVal.Value.Inspect())
	}
}
