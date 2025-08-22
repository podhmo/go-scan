package minigo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
)

func TestStringIndex(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		script := `
package main

var s = "hello"
var c = s[1] // 'e'
`
		var out, errOut strings.Builder
		interp := newTestInterpreter(t, minigo.WithStdout(&out), minigo.WithStderr(&errOut))

		_, err := interp.EvalString(script)
		if err != nil {
			t.Fatalf("eval failed: %v\nstderr:\n%s", err, errOut.String())
		}

		env := interp.GlobalEnvForTest()
		val, ok := env.Get("c")
		if !ok {
			t.Fatal("variable 'c' not found")
		}

		intVal, ok := val.(*object.Integer)
		if !ok {
			t.Fatalf("c is not an integer, got %T", val)
		}

		expected := int64('e')
		if intVal.Value != expected {
			t.Errorf("got %d, want %d", intVal.Value, expected)
		}
	})

	t.Run("out of bounds", func(t *testing.T) {
		script := `
package main

var s = "hello"
var c = s[10]
`
		var out, errOut strings.Builder
		interp := newTestInterpreter(t, minigo.WithStdout(&out), minigo.WithStderr(&errOut))

		_, err := interp.EvalString(script)
		if err == nil {
			t.Fatal("expected an error for out-of-bounds access, but got none")
		}

		expectedErr := "runtime error: index out of range [10] with length 5"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("expected error to contain %q, but got %q", expectedErr, err.Error())
		}
	})
}
