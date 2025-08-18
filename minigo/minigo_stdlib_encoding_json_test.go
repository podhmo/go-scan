package minigo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	stdjson "github.com/podhmo/go-scan/minigo/stdlib/encoding/json"
)

func TestStdlib_json_UnmarshalTypeError(t *testing.T) {
	jsonData := `{"Name":"John","Age":"forty-two"}` // Age is a string, but should be int
	script := `
package main
import "encoding/json"

type Person struct {
	Name string
	Age  int
}

var result Person
var err = json.Unmarshal(data, &result)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdjson.Install(interp)

	// Inject the data variable into the interpreter's global environment
	dataBytes := []byte(jsonData)
	elements := make([]object.Object, len(dataBytes))
	for i, b := range dataBytes {
		elements[i] = &object.Integer{Value: int64(b)}
	}
	interp.GlobalEnvForTest().Set("data", &object.Array{Elements: elements})

	// Load and evaluate the script
	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}

	// The evaluation itself should return the type error from our logic.
	// We check the error from Eval, as a runtime error should halt execution.
	_, err = interp.Eval(context.Background())
	if err == nil {
		t.Fatalf("expected evaluation to fail with a type error, but it did not")
	}

	// Check that the error message is what we expect.
	// This confirms the error is from our new validation logic.
	expectedError := "json: cannot unmarshal string into Go value of type int"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("expected error message to contain %q, but got %q", expectedError, err.Error())
	}
}

func TestJSONUnmarshalErrorPropagation(t *testing.T) {
	t.Run("it should propagate type mismatch errors from json.Unmarshal", func(t *testing.T) {
		jsonData := `{"Name":"Test","Value":"not-a-number"}`
		script := `
package main
import "encoding/json"

type Data struct {
	Name  string
	Value int
}

var d Data
var err = json.Unmarshal(data, &d)
`
		interp, err := minigo.NewInterpreter()
		if err != nil {
			t.Fatalf("failed to create interpreter: %+v", err)
		}
		stdjson.Install(interp)

		dataBytes := []byte(jsonData)
		elements := make([]object.Object, len(dataBytes))
		for i, b := range dataBytes {
			elements[i] = &object.Integer{Value: int64(b)}
		}
		interp.GlobalEnvForTest().Set("data", &object.Array{Elements: elements})

		if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
			t.Fatalf("failed to load script: %+v", err)
		}

		_, err = interp.Eval(context.Background())
		if err == nil {
			t.Fatalf("expected evaluation to fail with a type error, but it did not")
		}

		expectedError := "json: cannot unmarshal string into Go value of type int"
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("error message mismatch:\n- want to contain: %q\n- got: %q", expectedError, err.Error())
		}
	})
}
