package symgo_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestGenericFunctionCall(t *testing.T) {
	source := `
package main

var V int

func identity[T any](v T) T {
	return v
}

func main() {
	V = identity[int](42)
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		vObj, ok := r.Interpreter.FindObjectInPackage(t.Context(), "mymodule", "V")
		if !ok {
			t.Fatalf("global variable V not found")
		}
		vVar, ok := vObj.(*object.Variable)
		if !ok {
			t.Fatalf("V is not a variable, got %T", vObj)
		}

		want := &object.Integer{Value: 42}
		if diff := cmp.Diff(want, vVar.Value, cmp.AllowUnexported(object.Integer{})); diff != "" {
			t.Errorf("V mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestGenericCallWithInterfaceConstraint(t *testing.T) {
	source := `
package main

import "fmt"

func PrintStringer[T fmt.Stringer](v T) {
	// We don't need to evaluate the body for this test,
	// just ensure the call doesn't crash.
}

type MyString string

func (s MyString) String() string {
	return string(s)
}

func main() {
	var s MyString = "hello"
	PrintStringer[MyString](s)
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestGenericTypeDefinition(t *testing.T) {
	source := `
package main

type MySlice[T any] struct {
	Data []T
}

var V MySlice[int]

func main() {
	V = MySlice[int]{Data: []int{10, 20}}
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		vObj, ok := r.Interpreter.FindObjectInPackage(t.Context(), "mymodule", "V")
		if !ok {
			t.Fatalf("global variable V not found")
		}
		vVar, ok := vObj.(*object.Variable)
		if !ok {
			t.Fatalf("V is not a variable, got %T", vObj)
		}

		instance, ok := vVar.Value.(*object.Instance)
		if !ok {
			t.Fatalf("V is not an instance, got %T: %s", vVar.Value, vVar.Value.Inspect())
		}

		expectedTypeNamePrefix := "mymodule.MySlice"
		if !strings.HasPrefix(instance.TypeName, expectedTypeNamePrefix) {
			t.Errorf("expected V to be of type %q, but got %q", expectedTypeNamePrefix, instance.TypeName)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestGenericCallWithOmittedArgs(t *testing.T) {
	source := `
package main

var V int

func identity[T any](v T) T {
	return v
}

func main() {
	// This call omits the type argument, relying on type inference.
	// The evaluator should not crash, but can return a symbolic value.
	V = identity(42)
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		vObj, ok := r.Interpreter.FindObjectInPackage(t.Context(), "mymodule", "V")
		if !ok {
			t.Fatalf("global variable V not found")
		}
		vVar, ok := vObj.(*object.Variable)
		if !ok {
			t.Fatalf("V is not a variable, got %T", vObj)
		}

		want := &object.Integer{Value: 42}
		if diff := cmp.Diff(want, vVar.Value, cmp.AllowUnexported(object.Integer{})); diff != "" {
			t.Errorf("V mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}
