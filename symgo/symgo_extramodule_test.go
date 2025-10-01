package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestExtraModuleCall(t *testing.T) {
	// This test simulates a call to a function in an external, third-party module
	// (in this case, the standard library `errors` package).
	// The symgo engine should NOT evaluate this call deeply, but treat it as a
	// symbolic placeholder, and subsequent method calls on the result should also
	// yield a symbolic placeholder.
	source := map[string]string{
		"go.mod": `
module example.com/extramodule
go 1.22
`,
		"main.go": `
package main

import (
	"errors"
)

func main() string {
	err := errors.New("a test error")
	return err.Error()
}
`,
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/extramodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Apply main function should have succeeded, but it failed: %v", r.Error)
		}

		// The final result of `err.Error()` should be a symbolic placeholder because `err` itself
		// is a placeholder returned by the out-of-scope `errors.New` call.
		if _, ok := r.ReturnValue.(*object.SymbolicPlaceholder); !ok {
			t.Errorf("expected return value to be a SymbolicPlaceholder, but got %T", r.ReturnValue)
		}
	}

	symgotest.Run(t, tc, action)
}
