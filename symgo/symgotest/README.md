# symgotest

The `symgotest` package provides helpers to streamline testing of `symgo`-based analyses. It offers a convenient way to run the `symgo.Interpreter` on a set of in-memory source files and make assertions on the resulting state.

## Quick Start

The primary function is `symgotest.Run`, which takes a `*testing.T`, a `TestCase`, and an action function. The `TestCase` struct defines all the inputs for the test run, including source code, entry point, and options. The action function receives the `Result` of the execution for your assertions.

### Example

```go
package symgotest_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSymgotest(t *testing.T) {
	// 1. Define the test case.
	tc := symgotest.TestCase{
		// Source files for the test. A go.mod is usually needed.
		Source: map[string]string{
			"go.mod": "module example.com/mypkg",
			"main.go": `
package mypkg
func Add(x, y int) int {
    return x + y
}
func main() {
    Add(1, 2)
}`,
		},
		// The function that the interpreter should execute.
		EntryPoint: "example.com/mypkg.main",
	}

	// 2. Define the action to perform with the result.
	action := func(t *testing.T, r *symgotest.Result) {
		// 3. Make assertions on the result.
		if r.Error != nil {
			t.Fatalf("test failed unexpectedly: %+v", r.Error)
		}

		// The result's FinalEnv holds the state of global variables.
		addFn, ok := r.FinalEnv.Get("Add")
		if !ok {
			t.Fatal("Add function not found in final environment")
		}

		if _, isFunc := addFn.(*object.Function); !isFunc {
			t.Errorf("expected 'Add' to be a function object, but got %T", addFn)
		}
	}

	// 4. Run the test.
	symgotest.Run(t, tc, action)
}
```

By encapsulating the test setup in a `TestCase`, `symgotest` allows you to write clear, declarative, and maintainable tests for your symbolic analysis logic.