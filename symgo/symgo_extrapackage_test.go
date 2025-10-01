package symgo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSymgo_WithExtraPackages(t *testing.T) {
	// This test simulates a workspace with two modules:
	// - app: the main application
	// - helper: a library that app depends on
	// We want to test that by default, calls from app to helper are not deeply evaluated,
	// but when a scan policy is used to include the helper, they are.
	source := map[string]string{
		"go.mod": "module example.com/app\ngo 1.22\n\nrequire example.com/helper v0.0.0\n\nreplace example.com/helper => ../helper\n",
		"main.go": `
package main

import (
	"fmt"
	"example.com/helper"
)

func main() {
	fmt.Println(helper.Greet("world"))
}
`,
		"../helper/go.mod": "module example.com/helper\ngo 1.22\n",
		"../helper/greet.go": `
package helper

func Greet(name string) string {
	return "Hello, " + name
}
`,
	}

	var capturedArg object.Object
	intrinsic := symgotest.WithIntrinsic("fmt.Println", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		if len(args) > 0 {
			if retVal, ok := args[0].(*object.ReturnValue); ok {
				capturedArg = retVal.Value
			} else {
				capturedArg = args[0]
			}
		}
		return nil
	})

	t.Run("default behavior: external calls are symbolic", func(t *testing.T) {
		capturedArg = nil // reset
		tc := symgotest.TestCase{
			WorkDir:    ".",
			Source:     source,
			EntryPoint: "example.com/app.main",
			Options:    []symgotest.Option{intrinsic},
		}

		action := func(t *testing.T, r *symgotest.Result) {
			if r.Error != nil {
				t.Fatalf("symgotest.Run failed: %+v", r.Error)
			}
			if capturedArg == nil {
				t.Fatal("fmt.Println was not called")
			}
			_, ok := capturedArg.(*object.SymbolicPlaceholder)
			if !ok {
				t.Errorf("expected return value to be a *symgo.SymbolicPlaceholder, but got %T: %v", capturedArg, capturedArg.Inspect())
			}
		}

		symgotest.Run(t, tc, action)
	})

	t.Run("with scan policy: external calls are evaluated", func(t *testing.T) {
		capturedArg = nil // reset
		policy := symgotest.WithScanPolicy(func(path string) bool {
			return strings.HasPrefix(path, "example.com/app") || strings.HasPrefix(path, "example.com/helper")
		})

		tc := symgotest.TestCase{
			WorkDir:    ".",
			Source:     source,
			EntryPoint: "example.com/app.main",
			Options:    []symgotest.Option{intrinsic, policy},
		}

		action := func(t *testing.T, r *symgotest.Result) {
			if r.Error != nil {
				t.Fatalf("symgotest.Run failed: %+v", r.Error)
			}
			if capturedArg == nil {
				t.Fatal("fmt.Println was not called")
			}

			retStr, ok := capturedArg.(*object.String)
			if !ok {
				t.Fatalf("expected return value to be a *symgo.String, but got %T: %v", capturedArg, capturedArg.Inspect())
			}
			if retStr.Value != "Hello, world" {
				t.Errorf("expected return value to be 'Hello, world', but got %q", retStr.Value)
			}
		}

		symgotest.Run(t, tc, action)
	})
}
