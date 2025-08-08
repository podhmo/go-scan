package minigo2

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/minigo2/object"
)

func TestGoInterop_InjectGlobals(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		globals      map[string]any
		expectedType object.ObjectType
		expectedVal  any
	}{
		{
			name:         "inject integer",
			script:       "package main\n\nvar result = myVar",
			globals:      map[string]any{"myVar": 42},
			expectedType: object.GO_VALUE_OBJ,
			expectedVal:  42,
		},
		{
			name:         "inject string",
			script:       "package main\n\nvar result = myStr",
			globals:      map[string]any{"myStr": "hello world"},
			expectedType: object.GO_VALUE_OBJ,
			expectedVal:  "hello world",
		},
		{
			name: "inject struct",
			script: `package main
var result = myStruct
`,
			globals: map[string]any{
				"myStruct": struct{ Name string }{Name: "test"},
			},
			expectedType: object.GO_VALUE_OBJ,
			expectedVal:  struct{ Name string }{Name: "test"},
		},
		{
			name:   "use injected int in expression",
			script: "package main\n\nvar result = myVar + 5",
			globals: map[string]any{
				"myVar": int(10), // Use int to test conversion
			},
			expectedType: object.INTEGER_OBJ,
			expectedVal:  int64(15),
		},
		{
			name:   "use injected int64 in expression",
			script: "package main\n\nvar result = 2 * myVar",
			globals: map[string]any{
				"myVar": int64(21),
			},
			expectedType: object.INTEGER_OBJ,
			expectedVal:  int64(42),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interpreter, err := NewInterpreter()
			if err != nil {
				t.Fatalf("NewInterpreter() failed: %v", err)
			}

			res, err := interpreter.Eval(context.Background(), Options{
				Source:   []byte(tt.script),
				Filename: "test.mgo",
				Globals:  tt.globals,
			})

			if err != nil {
				t.Fatalf("Eval() returned an error: %v", err)
			}

			if res.Value == nil {
				t.Fatalf("Eval() result value is nil")
			}

			if res.Value.Type() != tt.expectedType {
				t.Errorf("wrong object type. got=%q, want=%q", res.Value.Type(), tt.expectedType)
			}

			switch tt.expectedType {
			case object.GO_VALUE_OBJ:
				goVal, ok := res.Value.(*object.GoValue)
				if !ok {
					t.Fatalf("result is not a GoValue, but a %T", res.Value)
				}
				if goVal.Value.Interface() != tt.expectedVal {
					t.Errorf("wrong GoValue value. got=%#v, want=%#v", goVal.Value.Interface(), tt.expectedVal)
				}
			case object.INTEGER_OBJ:
				intVal, ok := res.Value.(*object.Integer)
				if !ok {
					t.Fatalf("result is not an Integer, but a %T", res.Value)
				}
				expected, ok := tt.expectedVal.(int64)
				if !ok {
					t.Fatalf("expectedVal for INTEGER_OBJ is not an int64, but %T", tt.expectedVal)
				}
				if intVal.Value != expected {
					t.Errorf("wrong Integer value. got=%d, want=%d", intVal.Value, expected)
				}
			default:
				t.Fatalf("unhandled expected type in test: %s", tt.expectedType)
			}
		})
	}
}
