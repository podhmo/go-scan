package symgo_test

import (
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
)

func TestExtraModuleCall(t *testing.T) {
	// This test simulates a call to a function in an external, third-party module.
	// The symgo engine should NOT evaluate this call, but treat it as a symbolic placeholder.
	ctx := t.Context()

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

	mainFile := findFile(t, pkg, "main.go")

	_, err = interp.Eval(ctx, mainFile, pkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	mainObj, ok := interp.FindObjectInPackage(ctx, mainPkgPath, "main")
	if !ok {
		t.Fatal("main function not found in interpreter environment")
	}
	mainFunc, ok := mainObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'main' is not a function, but %T", mainObj)
	}

	_, err = interp.Apply(ctx, mainFunc, nil, pkg)
	if err != nil {
		t.Fatalf("Apply main function should have succeeded, but it failed: %v", err)
	}
}
