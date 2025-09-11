package symgo_test

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestIntraPackage_UnexportedConstant(t *testing.T) {
	ctx := context.Background()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module myapp",
		"main.go": `
package main

const unexportedConstant = "hello world"

func GetValue() string {
	return unexportedConstant
}
`,
	})
	defer cleanup()

	// 1. Create a scanner and interpreter.
	s, err := goscan.New(goscan.WithWorkDir(tmpdir))
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}
	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	// 2. Scan the package.
	mainPkg, err := s.ScanPackage(ctx, tmpdir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	// 3. Eval the file to populate the environment.
	mainFile := findFile(t, mainPkg, "main.go")
	if _, err := interp.Eval(ctx, mainFile, mainPkg); err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	// 4. Find the target function.
	fnObj, ok := interp.FindObjectInPackage("myapp", "GetValue")
	if !ok {
		t.Fatal("GetValue function not found in interpreter environment")
	}
	fn, ok := fnObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'GetValue' is not a function, but %T", fnObj)
	}

	// 5. Apply the function.
	result, err := interp.Apply(ctx, fn, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply GetValue function failed: %v", err)
	}

	// 6. Assert the result.
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}

	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be *object.String, but got %T", retVal.Value)
	}

	expected := "hello world"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}
