# `symgotest`

`symgotest` is a testing library designed specifically for the `symgo` symbolic execution engine. Its primary goal is to improve the testing experience by reducing boilerplate and providing powerful, built-in debugging capabilities.

The core philosophy of `symgotest` is **debugging-first**. The library is designed not just to test `symgo`'s behavior, but to provide clear, actionable insights when a test fails, hangs, or encounters an error.

## Features

- **Reduced Boilerplate**: Abstract away the repetitive setup of scanners, interpreters, and file systems.
- **Deterministic Failure Reporting**: Turns hangs and infinite loops into deterministic test failures by enforcing a configurable execution step limit.
- **Execution Tracing**: Automatically captures a trace of evaluation steps, printing a detailed report on failure to pinpoint the exact cause.
- **Expressive, High-Level API**: Offers intuitive functions for common testing scenarios.
- **Type-Safe Assertion Helpers**: Provides generic helpers like `AssertAs` and `AssertEqual` to simplify test assertions.

## Usage

### Basic Test with `symgotest.Run`

The main entry point is `symgotest.Run`. It handles all the setup and teardown for a test case.

```go
import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestNewUser(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com",
			"me/me.go": `
package me
type User struct { Name string }
func NewUser(name string) *User {
	return &User{Name: name}
}
`,
		},
		EntryPoint: "example.com/me.NewUser",
		Args:       []object.Object{object.NewString("Alice")},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}

		ptr := symgotest.AssertAs[*object.Pointer](t, r.ReturnValue)
		instance := symgotest.AssertAs[*object.Instance](t, ptr.Value)

		expectedTypeName := "example.com/me.User"
		if diff := cmp.Diff(expectedTypeName, instance.TypeName); diff != "" {
			t.Errorf("instance type name mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}
```

### Testing a Single Expression

Use `symgotest.RunExpression` to quickly test the evaluation of a single Go expression. The generic helper `symgotest.AssertEqual` simplifies type and value assertions.

```go
func TestAddition(t *testing.T) {
	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		symgotest.AssertEqual(t, r.ReturnValue, 3)
	}
	symgotest.RunExpression(t, "1 + 2", action)
}
```

### Testing Statements

Use `symgotest.RunStatements` to test the side-effects of a block of statements. The `FinalEnv` in the result contains the global environment after execution.

```go
func TestAssignment(t *testing.T) {
	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		val, ok := r.FinalEnv.Get("x")
		if !ok {
			t.Fatalf("variable 'x' not found in final environment")
		}

		variable := symgotest.AssertAs[*object.Variable](t, val)
		symgotest.AssertEqual(t, variable.Value, 10)
	}
	symgotest.RunStatements(t, "x := 10", action)
}
```
