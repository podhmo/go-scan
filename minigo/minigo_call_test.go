package minigo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
)

func TestInterpreter_Call(t *testing.T) {
	script := `
package main

func add(a int, b int) {
	return a + b
}

func get_message() {
	return "hello world"
}
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() error = %v", err)
	}

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	// Eval populates the environment with the function definitions
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("Eval() error = %v", err)
	}

	t.Run("call add", func(t *testing.T) {
		args := []object.Object{
			&object.Integer{Value: 10},
			&object.Integer{Value: 20},
		}
		result, err := interp.Call(context.Background(), "add", args...)
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if result.Value.Type() != object.INTEGER_OBJ {
			t.Errorf("result.Value.Type() got = %s, want = %s", result.Value.Type(), object.INTEGER_OBJ)
		}
		if got := result.Value.(*object.Integer).Value; got != 30 {
			t.Errorf("result.Value.Inspect() got = %d, want = 30", got)
		}
	})

	t.Run("call get_message", func(t *testing.T) {
		result, err := interp.Call(context.Background(), "get_message")
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if result.Value.Type() != object.STRING_OBJ {
			t.Errorf("result.Value.Type() got = %s, want = %s", result.Value.Type(), object.STRING_OBJ)
		}
		if got := result.Value.(*object.String).Value; got != "hello world" {
			t.Errorf("result.Value.Inspect() got = %q, want = %q", got, "hello world")
		}
	})

	t.Run("function not found", func(t *testing.T) {
		_, err := interp.Call(context.Background(), "non_existent_func")
		if err == nil {
			t.Fatal("Call() error = nil, want non-nil")
		}
		expectedMsg := `function "non_existent_func" not found in global scope`
		if err.Error() != expectedMsg {
			t.Errorf("Call() error got = %q, want = %q", err.Error(), expectedMsg)
		}
	})
}
