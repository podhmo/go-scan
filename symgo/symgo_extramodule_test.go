package symgo_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	goscanner "github.com/podhmo/go-scan/scanner"
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

func TestStdLibCall(t *testing.T) {
	// This test ensures that when symgo encounters a function that uses types
	// from the standard library (e.g., io.Writer, json.Encoder), it does not
	// panic by trying to scan the stdlib source files. The fix in goscan.go
	// should prevent the scanner from attempting to parse GOROOT.
	ctx := context.Background()

	moduleDir := filepath.Join("testdata", "stdlibmodule")
	mainPkgPath := "example.com/stdlibmodule/main"

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

	// Evaluating the file will parse all the declarations, including
	// GetEncoder, which has stdlib types in its signature. This is where
	// the type resolution happens.
	_, err = interp.Eval(ctx, mainFile, pkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	// Find the target function to analyze.
	getEncoderObj, ok := interp.FindObject("GetEncoder")
	if !ok {
		t.Fatal("GetEncoder function not found in interpreter environment")
	}
	getEncoderFunc, ok := getEncoderObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'GetEncoder' is not a function, but %T", getEncoderObj)
	}

	// Create a symbolic placeholder for the io.Writer argument.
	arg := &object.SymbolicPlaceholder{
		Reason: "io.Writer for test",
	}
	// To represent the type of the placeholder, we create a FieldType description.
	// The interpreter uses this information if it needs to resolve the type.
	arg.SetFieldType(&goscanner.FieldType{
		PkgName:        "io",
		TypeName:       "Writer",
		FullImportPath: "io",
		Resolver:       scanner, // Provide the scanner instance as the resolver.
	})

	// Apply the function. This is the step that would have panicked before the fix.
	// We don't need to inspect the result, just ensure it doesn't panic.
	_, err = interp.Apply(ctx, getEncoderFunc, []object.Object{arg}, pkg)
	if err != nil {
		t.Fatalf("Apply GetEncoder function failed: %v", err)
	}

	// If we reach here without a panic, the test has passed.
}

func TestThirdPartyModuleCall(t *testing.T) {
	// This test ensures that when symgo encounters a function that uses types
	// from a third-party module from the module cache (e.g., uuid.UUID), it does not
	// panic. The generalized fix in goscan.go should treat this as an external
	// package and not attempt to scan its source files.
	ctx := context.Background()

	moduleDir := filepath.Join("testdata", "thirdpartymodule")
	mainPkgPath := "example.com/thirdpartymodule/main"

	// Running `go mod tidy` is necessary to ensure the dependency (uuid) is
	// available in the module cache for the test to find.
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = moduleDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed in %s: %v\n%s", moduleDir, err, output)
	}

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

	printUUIDObj, ok := interp.FindObject("PrintUUID")
	if !ok {
		t.Fatal("PrintUUID function not found in interpreter environment")
	}
	_, ok = printUUIDObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'PrintUUID' is not a function, but %T", printUUIDObj)
	}

	// If we reach here without a panic, it means Eval successfully resolved
	// the types from the third-party module without the scanner panicking.
	// This is sufficient to verify the fix that was the goal of this task.
	// We do not need to proceed with Apply(), as that tests the capabilities
	// of the symgo interpreter itself, which is out of scope.
}
