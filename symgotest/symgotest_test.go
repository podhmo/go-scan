package symgotest_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestRunner_Apply_Simple(t *testing.T) {
	source := `
func add(a, b int) int {
	return a + b
}
`
	runner := symgotest.NewRunner(t, source)
	result := runner.Apply("add", &object.Integer{Value: 2}, &object.Integer{Value: 3})

	// The result of a function call is wrapped in a ReturnValue
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected a ReturnValue, got %T", result)
	}

	symgotest.AssertInteger(t, retVal.Value, 5)
}

func TestRunner_Apply_WithSetup(t *testing.T) {
	source := `
func main() {
	myintrinsic("hello")
}
`
	var intrinsicCalledWith string
	setup := func(interp *symgo.Interpreter) {
		interp.RegisterIntrinsic("example.com/symgotest/module.myintrinsic", func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
			if len(args) > 0 {
				if str, ok := args[0].(*object.String); ok {
					intrinsicCalledWith = str.Value
				}
			}
			return object.NIL
		})
	}

	runner := symgotest.NewRunner(t, source).WithSetup(setup)
	result := runner.Apply("main")

	symgotest.AssertSuccess(t, result)
	symgotest.AssertEqual(t, "hello", intrinsicCalledWith)
}

func TestRunner_Apply_GlobalVar(t *testing.T) {
	source := `
var greeting = "hello world"

func getGreeting() string {
	return greeting
}
`
	runner := symgotest.NewRunner(t, source)
	result := runner.Apply("getGreeting")

	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected a ReturnValue, got %T", result)
	}
	symgotest.AssertString(t, retVal.Value, "hello world")
}

func TestRunner_Apply_WithError(t *testing.T) {
	source := `
func main() {
	panic("oh no")
}
`
	runner := symgotest.NewRunner(t, source)
	result := runner.Apply("main")

	symgotest.AssertError(t, result, "panic: oh no")
}

func TestAssertHelpers_SuccessCases(t *testing.T) {
	// This test only checks the success paths of the assertion helpers,
	// as testing the failure paths (which call t.Fatalf) is problematic.

	t.Run("AssertSuccess", func(t *testing.T) {
		mockT := new(testing.T)
		symgotest.AssertSuccess(mockT, object.NIL)
		if mockT.Failed() {
			t.Error("AssertSuccess should not fail for NIL")
		}
	})

	t.Run("AssertError", func(t *testing.T) {
		mockT := new(testing.T)
		symgotest.AssertError(mockT, &object.Error{Message: "something went wrong"}, "wrong")
		if mockT.Failed() {
			t.Error("AssertError should not fail for matching error")
		}
	})

	t.Run("AssertSymbolicNil", func(t *testing.T) {
		mockT := new(testing.T)
		symgotest.AssertSymbolicNil(mockT, object.NIL)
		if mockT.Failed() {
			t.Error("AssertSymbolicNil should not fail for NIL object")
		}
	})
}
