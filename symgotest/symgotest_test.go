package symgotest_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestEvalExpr(t *testing.T) {
	t.Run("integer literal", func(t *testing.T) {
		result := symgotest.EvalExpr(t, "42")
		symgotest.AssertInteger(t, result, 42)
	})

	t.Run("string literal", func(t *testing.T) {
		result := symgotest.EvalExpr(t, `"hello"`)
		symgotest.AssertString(t, result, "hello")
	})

	t.Run("binary expression", func(t *testing.T) {
		result := symgotest.EvalExpr(t, "5 + 5")
		symgotest.AssertInteger(t, result, 10)
	})
}

func TestRunner_Apply_Simple(t *testing.T) {
	source := `
func add(a, b int) int {
	return a + b
}
`
	runner := symgotest.NewRunner(t, source)
	result := runner.Apply("add", &object.Integer{Value: 2}, &object.Integer{Value: 3})

	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected a ReturnValue, got %T", result)
	}

	symgotest.AssertInteger(t, retVal.Value, 5)
}

func TestRunner_Apply_WithSetup(t *testing.T) {
	source := `
func main() {
	myIntrinsic("data")
}
`
	var intrinsicCalledWith string
	setup := func(interp *symgo.Interpreter) {
		// Note: The module name is fixed in the runner for simplicity.
		interp.RegisterIntrinsic("example.com/symgotest/module.myIntrinsic", func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
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
	symgotest.AssertEqual(t, "data", intrinsicCalledWith)
}

func TestRunner_Apply_WithError(t *testing.T) {
	source := `
func main() {
	panic("something went wrong")
}
`
	runner := symgotest.NewRunner(t, source)
	result := runner.Apply("main")

	symgotest.AssertError(t, result, "panic: something went wrong")
}

func TestRunnerWithMultiFiles(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/multifile",
		"main.go": `package main
import "example.com/multifile/helpers"
func main() {
	helpers.SayHello()
}`,
		"helpers/helpers.go": `package helpers
func SayHello() {
	// In a real scenario, this might be a function we want to track.
}
`,
	}

	var helperCalled bool
	setup := func(interp *symgo.Interpreter) {
		interp.RegisterIntrinsic("example.com/multifile/helpers.SayHello", func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
			helperCalled = true
			return object.NIL
		})
	}

	runner := symgotest.NewRunnerWithMultiFiles(t, files).WithSetup(setup)
	result := runner.Apply("main")

	symgotest.AssertSuccess(t, result)
	symgotest.AssertEqual(t, true, helperCalled)
}

func TestAssertions(t *testing.T) {
	t.Run("AssertSuccess", func(t *testing.T) {
		mockT := new(testing.T)
		symgotest.AssertSuccess(mockT, &object.Integer{Value: 1})
		if mockT.Failed() {
			t.Error("AssertSuccess should not fail for a valid object")
		}
	})

	t.Run("AssertError", func(t *testing.T) {
		mockT := new(testing.T)
		symgotest.AssertError(mockT, &object.Error{Message: "this is an error"}, "is an error")
		if mockT.Failed() {
			t.Error("AssertError should not fail for a matching error")
		}
	})
}
