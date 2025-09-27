# `symgotest`

`symgotest` is a testing library designed specifically for the `symgo` symbolic execution engine. Its primary goal is to improve the testing experience by reducing boilerplate and providing powerful, built-in debugging capabilities.

The core philosophy of `symgotest` is **debugging-first**. It aims to reduce boilerplate and provide clear, actionable insights when a test fails.

## Features

- **Reduced Boilerplate**: Abstracts away the repetitive setup of scanners, interpreters, and in-memory file systems.
- **Deterministic Failure Reporting**: Turns hangs and infinite loops into deterministic test failures by enforcing a configurable execution step limit.
- **Execution Tracing**: Automatically captures a trace of evaluation steps, printing a detailed report on failure to pinpoint the exact cause.
- **Type-Safe Assertion Helper**: Provides a generic helper, `AssertAs`, to simplify assertions on return values.

## Usage

### Basic Test

The main entry point is `symgotest.Run`. It handles all the setup and teardown for a test case. The `symgotest.AssertAs` helper simplifies checking the type of the result.

```go
import (
	"testing"

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

		// AssertAs unwraps the result and asserts the type.
		// For single return values, use index 0.
		ptr := symgotest.AssertAs[*object.Pointer](r, t, 0)

		// To assert the type of the pointed-to value, we can create a temporary result.
		instance := symgotest.AssertAs[*object.Instance](&symgotest.Result{ReturnValue: ptr.Value}, t, 0)

		if instance.Fields["Name"].(*object.String).Value != "Alice" {
			t.Errorf("Expected user name to be Alice")
		}
	}

	symgotest.Run(t, tc, action)
}
```

### Handling Multiple Return Values

The `AssertAs` helper can access any return value by its index.

```go
func TestMultiReturn(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com",
			"main.go": `
package main
func GetPair() (string, int) {
	return "hello", 42
}
`,
		},
		EntryPoint: "example.com/main.GetPair",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}

		// Access the first return value (index 0)
		str := symgotest.AssertAs[*object.String](r, t, 0)
		if str.Value != "hello" {
			t.Errorf("expected first return value to be %q", "hello")
		}

		// Access the second return value (index 1)
		num := symgotest.AssertAs[*object.Integer](r, t, 1)
		if num.Value != 42 {
			t.Errorf("expected second return value to be %d", 42)
		}
	}

	symgotest.Run(t, tc, action)
}
```
