package symgo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
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

	_, err = interp.Apply(ctx, mainFunc, nil, pkg)
	if err == nil {
		t.Fatal("Apply main function should have returned an error, but it did not")
	}

	// Because the scanner no longer scans external modules, the symbolic execution
	// cannot resolve the `err.Error()` call. The interpreter correctly returns
	// an error in this case. We check for a specific substring to ensure it's
	// the error we expect. This confirms the scanner is blocking the scan and
	// symgo is reacting to the missing type information.
	expectedErr := "cannot access field or method on variable with no type info: err"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error to contain %q, but got: %v", expectedErr, err)
	}
}
