package minigo

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/minigo/object"
)

func TestInterpreterEval_IncDec(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		setup     func(i *Interpreter)
		check     func(t *testing.T, i *Interpreter)
		expectErr bool
	}{
		{
			name: "increment global variable",
			input: `package main
var x = 10
func main() {
	x++
}`,
			check: func(t *testing.T, i *Interpreter) {
				val, ok := i.globalEnv.Get("x")
				if !ok {
					t.Fatalf("variable 'x' not found")
				}
				integer, ok := val.(*object.Integer)
				if !ok {
					t.Fatalf("x is not Integer, got %T", val)
				}
				if integer.Value != 11 {
					t.Errorf("x should be 11, got %d", integer.Value)
				}
			},
		},
		{
			name: "decrement global variable",
			input: `package main
var x = 10
func main() {
	x--
}`,
			check: func(t *testing.T, i *Interpreter) {
				val, ok := i.globalEnv.Get("x")
				if !ok {
					t.Fatalf("variable 'x' not found")
				}
				integer, ok := val.(*object.Integer)
				if !ok {
					t.Fatalf("x is not Integer, got %T", val)
				}
				if integer.Value != 9 {
					t.Errorf("x should be 9, got %d", integer.Value)
				}
			},
		},
		{
			name: "increment local variable",
			input: `package main
func main() {
	x := 5
	x++
}`,
			// Note: We can't check the final value of a local variable directly from the test.
			// This test just ensures it runs without error. A more complex test could
			// return the value or assign it to a global.
			check: func(t *testing.T, i *Interpreter) {
				// No-op, just checking for successful execution
			},
		},
		{
			name: "increment non-integer",
			input: `package main
var s = "hello"
func main() {
	s++
}`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := newTestInterpreter(t)

			if tt.setup != nil {
				tt.setup(i)
			}

			if err := i.LoadFile("test.go", []byte(tt.input)); err != nil {
				t.Fatalf("LoadFile() failed: %v", err)
			}

			if err := i.EvalDeclarations(context.Background()); err != nil {
				t.Fatalf("EvalDeclarations() failed: %v", err)
			}

			mainFunc, fscope, err := i.FindFunction("main")
			if err != nil {
				// Not all tests have a main, this is fine
				if tt.check != nil {
					tt.check(t, i)
				}
				return
			}

			_, err = i.Execute(context.Background(), mainFunc, nil, fscope)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("Execute() failed: %v", err)
				}
				if tt.check != nil {
					tt.check(t, i)
				}
			}
		})
	}
}
