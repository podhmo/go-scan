package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestInterpreter_MethodCallOnExternalPointerType(t *testing.T) {
	// This test verifies that the interpreter can handle a method call on a
	// symbolic object whose type comes from an external, non-analyzed package.
	// symgotest automatically creates a symbolic placeholder for the function argument.
	tc := symgotest.TestCase{
		Source: map[string]string{
			// Note: The original test had a `replace` directive in go.mod.
			// This is not needed with symgotest as it runs in the context
			// of the current project, which already has the correct go.mod.
			"go.mod": `
module mytest
go 1.24
`,
			"usecase/usecase.go": `
package usecase
import "github.com/podhmo/go-scan/locator"

// UseIt takes a pointer to a type from an external (non-analyzed) package.
func UseIt(l *locator.Locator) {
	// The bug is triggered when calling a method on the symbolic pointer 'l'.
	// The interpreter should handle this gracefully by returning a symbolic placeholder
	// for the result of the method call, rather than panicking.
	_, _ = l.PathToImport(".")
}
`,
		},
		EntryPoint: "mytest/usecase.UseIt",
		// No arguments are provided, so symgotest will create a symbolic
		// placeholder for the '*locator.Locator' parameter automatically.
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
		// The main check is that the execution completes without panicking or returning an error.
	}

	symgotest.Run(t, tc, action)
}
