package symgo_test

import (
	"context"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestSymgo_ExtraModule_ConstantResolution(t *testing.T) {
	ctx := context.Background()

	// Setup: Create a temporary directory with two modules.
	// main module depends on helper module.
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"main/go.mod": `
module example.com/main
go 1.21
replace example.com/helper => ../helper
`,
		"main/main.go": `
package main
import "example.com/helper"
func GetConstant() string {
    return helper.MyConstant
}
`,
		"helper/go.mod": `
module example.com/helper
go 1.21
`,
		"helper/helper.go": `
package helper
const MyConstant = "hello from another module"
`,
	})
	defer cleanup()

	mainModuleDir := filepath.Join(tmpdir, "main")

	// 1. Create a scanner configured for the main module.
	scanner, err := goscan.New(
		goscan.WithWorkDir(mainModuleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	// 2. Create the symgo interpreter.
	interp, err := symgo.NewInterpreter(scanner)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	// 3. Scan the main package.
	mainPkg, err := scanner.ScanPackage(ctx, mainModuleDir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	// 4. Eval the main file to populate the interpreter's environment.
	mainFile := FindFile(t, mainPkg, "main.go") // Using helper from another test
	_, err = interp.Eval(ctx, mainFile, mainPkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	// 5. Find the target function in the environment.
	getConstantObj, ok := interp.FindObject("GetConstant")
	if !ok {
		t.Fatal("GetConstant function not found in interpreter environment")
	}
	getConstantFunc, ok := getConstantObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'GetConstant' is not a function, but %T", getConstantObj)
	}

	// 6. Apply the function.
	result, err := interp.Apply(ctx, getConstantFunc, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply GetConstant function failed: %v", err)
	}

	// 7. Assert the result.
	// With the new optimization, the interpreter does not scan the helper module
	// because it's not explicitly included. Therefore, the constant value is not
	// resolved, and we get a placeholder, which is the desired behavior.
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}

	placeholder, ok := retVal.Value.(*object.SymbolicPlaceholder)
	if !ok {
		t.Fatalf("expected return value to be *object.SymbolicPlaceholder, but got %T", retVal.Value)
	}

	expectedReason := "unscanned external symbol example.com/helper.MyConstant"
	if placeholder.Reason != expectedReason {
		t.Errorf("expected placeholder reason to be %q, but got %q", expectedReason, placeholder.Reason)
	}
}

func TestSymgo_ExtraModule_ConstantResolution_WithExtraPackages(t *testing.T) {
	ctx := context.Background()

	// Setup: Create a temporary directory with two modules.
	// main module depends on helper module.
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"main/go.mod": `
module example.com/main
go 1.21
replace example.com/helper => ../helper
`,
		"main/main.go": `
package main
import "example.com/helper"
func GetConstant() string {
    return helper.MyConstant
}
`,
		"helper/go.mod": `
module example.com/helper
go 1.21
`,
		"helper/helper.go": `
package helper
const MyConstant = "hello from another module"
`,
	})
	defer cleanup()

	mainModuleDir := filepath.Join(tmpdir, "main")

	// 1. Create a scanner configured for the main module.
	scanner, err := goscan.New(
		goscan.WithWorkDir(mainModuleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	// 2. Create the symgo interpreter, making the helper module scannable.
	interp, err := symgo.NewInterpreter(scanner, symgo.WithExtraPackages([]string{"example.com/helper"}))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	// 3. Scan the main package.
	mainPkg, err := scanner.ScanPackage(ctx, mainModuleDir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	// 4. Eval the main file to populate the interpreter's environment.
	mainFile := FindFile(t, mainPkg, "main.go") // Using helper from another test
	_, err = interp.Eval(ctx, mainFile, mainPkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	// 5. Find the target function in the environment.
	getConstantObj, ok := interp.FindObject("GetConstant")
	if !ok {
		t.Fatal("GetConstant function not found in interpreter environment")
	}
	getConstantFunc, ok := getConstantObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'GetConstant' is not a function, but %T", getConstantObj)
	}

	// 6. Apply the function.
	result, err := interp.Apply(ctx, getConstantFunc, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply GetConstant function failed: %v", err)
	}

	// 7. Assert the result.
	// Because the helper module was included in WithExtraPackages, the constant
	// should be fully resolved to its concrete string value.
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}

	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be *object.String, but got %T", retVal.Value)
	}

	expected := "hello from another module"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}
