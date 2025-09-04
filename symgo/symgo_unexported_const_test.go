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

func TestSymgo_UnexportedConstantResolution(t *testing.T) {
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
func CallHelper() string {
    return helper.GetUnexportedConstant()
}
`,
		"helper/go.mod": `
module example.com/helper
go 1.21
`,
		"helper/helper.go": `
package helper
const myUnexportedConstant = "hello from unexported"
func GetUnexportedConstant() string {
	return myUnexportedConstant
}
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

	// 2. Create the symgo interpreter with a policy to scan both modules.
	interp, err := symgo.NewInterpreter(scanner, symgo.WithPrimaryAnalysisScope("example.com/main/...", "example.com/helper/..."))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	// 3. Scan the main package.
	mainPkg, err := scanner.ScanPackage(ctx, mainModuleDir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	// 4. Eval the main file to populate the interpreter's environment.
	mainFile := findFile(t, mainPkg, "main.go")
	_, err = interp.Eval(ctx, mainFile, mainPkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	// 5. Find the target function in the environment.
	callHelperObj, ok := interp.FindObject("CallHelper")
	if !ok {
		t.Fatal("CallHelper function not found in interpreter environment")
	}
	callHelperFunc, ok := callHelperObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'CallHelper' is not a function, but %T", callHelperObj)
	}

	// 6. Apply the function.
	result, err := interp.Apply(ctx, callHelperFunc, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply CallHelper function failed: %v", err)
	}

	// 7. Assert the result.
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}

	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be *object.String, but got %T", retVal.Value)
	}

	expected := "hello from unexported"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}

func TestSymgo_IntraPackageConstantResolution(t *testing.T) {
	ctx := context.Background()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/main\ngo 1.21\n",
		"main.go": `
package main
import "fmt"
const myConstant = "hello intra-package"

func formatConstant() string {
	return fmt.Sprintf("value is %s", myConstant)
}

func main() {
	_ = formatConstant()
}
`,
	})
	defer cleanup()

	scanner, err := goscan.New(goscan.WithWorkDir(tmpdir))
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	interp, err := symgo.NewInterpreter(scanner, symgo.WithPrimaryAnalysisScope("example.com/main/..."))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	mainPkg, err := scanner.ScanPackage(ctx, tmpdir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	mainFile := findFile(t, mainPkg, "main.go")
	if _, err := interp.Eval(ctx, mainFile, mainPkg); err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	mainFuncObj, ok := interp.FindObject("main")
	if !ok {
		t.Fatal("main function not found in interpreter environment")
	}
	mainFunc, ok := mainFuncObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'main' is not a function, but %T", mainFuncObj)
	}

	// We don't care about the result, we just want to ensure it doesn't crash.
	// The original error was a crash due to "identifier not found".
	_, err = interp.Apply(ctx, mainFunc, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply main function failed: %v", err)
	}
}

// Test case for nested function call
func TestSymgo_UnexportedConstantResolution_NestedCall(t *testing.T) {
	ctx := context.Background()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"loglib/go.mod": `
module example.com/loglib
go 1.21
replace example.com/driver => ../driver
`,
		"loglib/context.go": `
package loglib
import "example.com/driver"
func FuncA() string {
	return driver.FuncB()
}
`,
		"driver/go.mod": `
module example.com/driver
go 1.21
`,
		"driver/db.go": `
package driver
const privateConst = "hello from private"
func FuncB() string {
	return privateConst
}
`,
	})
	defer cleanup()
	loglibModuleDir := filepath.Join(tmpdir, "loglib")
	goscanner, err := goscan.New(
		goscan.WithWorkDir(loglibModuleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}
	interp, err := symgo.NewInterpreter(goscanner, symgo.WithPrimaryAnalysisScope("example.com/loglib/...", "example.com/driver/..."))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}
	loglibPkg, err := goscanner.ScanPackage(ctx, loglibModuleDir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}
	loglibFile := findFile(t, loglibPkg, "context.go")
	if _, err := interp.Eval(ctx, loglibFile, loglibPkg); err != nil {
		t.Fatalf("Eval loglib file failed: %v", err)
	}
	entrypointObj, ok := interp.FindObject("FuncA")
	if !ok {
		t.Fatal("FuncA function not found in interpreter environment")
	}
	entrypointFunc, ok := entrypointObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'FuncA' is not a function, but %T", entrypointObj)
	}
	result, err := interp.Apply(ctx, entrypointFunc, nil, loglibPkg)
	if err != nil {
		t.Fatalf("Apply FuncA function failed: %v", err)
	}
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}
	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be *object.String, but got %T", retVal.Value)
	}
	expected := "hello from private"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}

// Test case for nested method call
func TestSymgo_UnexportedConstantResolution_NestedMethodCall(t *testing.T) {
	ctx := context.Background()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"main/go.mod": `
module example.com/main
go 1.21
replace example.com/remotedb => ../remotedb
`,
		"main/main.go": `
package main
import "example.com/remotedb"
func DoWork() string {
	var client remotedb.Client
	return client.GetValue()
}
`,
		"remotedb/go.mod": `
module example.com/remotedb
go 1.21
`,
		"remotedb/db.go": `
package remotedb
const secretKey = "unexported-secret-key"
type Client struct{}
func (c *Client) GetValue() string {
	return secretKey
}
`,
	})
	defer cleanup()
	mainModuleDir := filepath.Join(tmpdir, "main")
	goscanner, err := goscan.New(
		goscan.WithWorkDir(mainModuleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}
	interp, err := symgo.NewInterpreter(goscanner, symgo.WithPrimaryAnalysisScope("example.com/main/...", "example.com/remotedb/..."))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}
	mainPkg, err := goscanner.ScanPackage(ctx, mainModuleDir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}
	mainFile := findFile(t, mainPkg, "main.go")
	if _, err := interp.Eval(ctx, mainFile, mainPkg); err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}
	entrypointObj, ok := interp.FindObject("DoWork")
	if !ok {
		t.Fatal("DoWork function not found in interpreter environment")
	}
	entrypointFunc, ok := entrypointObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'DoWork' is not a function, but %T", entrypointObj)
	}
	result, err := interp.Apply(ctx, entrypointFunc, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply DoWork function failed: %v", err)
	}
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}
	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be *object.String, but got %T", retVal.Value)
	}
	expected := "unexported-secret-key"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}
