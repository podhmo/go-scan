package symgo_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestMultiValueAssignmentWithBlank(t *testing.T) {
	var intrinsicCalled bool

	source := `
package main
func myFunc() (string, error) {
	return "", nil
}
var x string
func main() {
	x, _ = myFunc()
}`

	intrinsic := func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		intrinsicCalled = true
		return &object.MultiReturn{
			Values: []symgo.Object{
				&object.String{Value: "hello"},
				object.NIL,
			},
		}
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module myapp",
			"main.go": source,
		},
		EntryPoint: "myapp.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("myapp.myFunc", intrinsic),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
		if !intrinsicCalled {
			t.Errorf("intrinsic for myFunc was not called")
		}

		// After running main, check the value of the assigned variable 'x'.
		// Global variables are stored on the interpreter's package scope, not the function's final env.
		xVar, ok := r.Interpreter.FindObjectInPackage(t.Context(), "myapp", "x")
		if !ok {
			t.Fatalf("variable 'x' was not found in the interpreter's package scope")
		}

		// The object in the env is the *Variable* itself. We need its value.
		v, ok := xVar.(*object.Variable)
		if !ok {
			t.Fatalf("expected 'x' to be a *object.Variable, but got %T", xVar)
		}

		want := &object.String{Value: "hello"}
		if diff := cmp.Diff(want, v.Value); diff != "" {
			t.Errorf("assigned value 'x' mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestAssignmentToFieldOfTypeAssertion(t *testing.T) {
	// This test verifies that a complex assignment like `v.(*S).X = 10` is
	// correctly evaluated.
	var checkCalled bool

	source := `
package main

type S struct { X int }

func check(v any) {}

func main() {
	var v any = &S{X: 0}
	v.(*S).X = 10
	check(v)
}`

	checkIntrinsic := func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		checkCalled = true
		// We could inspect the value of 'v' here if needed, but for this test,
		// we just need to confirm the call happens.
		return nil
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module myapp",
			"main.go": source,
		},
		EntryPoint: "myapp.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("myapp.check", checkIntrinsic),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
		if !checkCalled {
			t.Fatalf("the check() function was not called, indicating evaluation did not complete")
		}
	}

	symgotest.Run(t, tc, action)
}
