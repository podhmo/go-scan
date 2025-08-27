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

func TestSymgo_WithExtraPackages(t *testing.T) {
	t.Run("with extra package: external calls are evaluated", func(t *testing.T) {
		ctx := context.Background()

		tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
			"main/go.mod": `
module example.com/main
go 1.21
replace example.com/helper => ../helper
`,
			"main/main.go": `
package main
import "example.com/helper"
func GetGreeting() string {
    return helper.Greet("world")
}
`,
			"helper/go.mod": `
module example.com/helper
go 1.21
`,
			"helper/helper.go": `
package helper
import "fmt"
func Greet(name string) string {
    return fmt.Sprintf("hello, %s", name)
}
`,
		})
		defer cleanup()

		mainModuleDir := filepath.Join(tmpdir, "main")

		scanner, err := goscan.New(
			goscan.WithWorkDir(mainModuleDir),
			goscan.WithGoModuleResolver(),
		)
		if err != nil {
			t.Fatalf("New scanner failed: %v", err)
		}

		// Create the interpreter, explicitly allowing "example.com/helper" to be scanned.
		interp, err := symgo.NewInterpreter(scanner, symgo.WithExtraPackages([]string{"example.com/helper"}))
		if err != nil {
			t.Fatalf("NewInterpreter failed: %v", err)
		}

		mainPkg, err := scanner.ScanPackage(ctx, mainModuleDir)
		if err != nil {
			t.Fatalf("ScanPackage failed: %v", err)
		}

		mainFile := FindFile(t, mainPkg, "main.go")
		_, err = interp.Eval(ctx, mainFile, mainPkg)
		if err != nil {
			t.Fatalf("Eval main file failed: %v", err)
		}

		getGreetingObj, ok := interp.FindObject("GetGreeting")
		if !ok {
			t.Fatal("GetGreeting function not found")
		}
		getGreetingFunc, ok := getGreetingObj.(*symgo.Function)
		if !ok {
			t.Fatalf("'GetGreeting' is not a function, but %T", getGreetingObj)
		}

		// Since "example.com/helper" is an extra package, the call should be evaluated.
		result, err := interp.Apply(ctx, getGreetingFunc, nil, mainPkg)
		if err != nil {
			t.Fatalf("Apply GetGreeting function failed: %v", err)
		}

		// The important part is that the call did not fail. The exact return value
		// depends on the intrinsics, but it should be a ReturnValue.
		if _, ok := result.(*object.ReturnValue); !ok {
			t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
		}
	})
}
