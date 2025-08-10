package minigo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
)

func TestGenericFunctions(t *testing.T) {
	tests := []struct {
		name          string
		script        string
		expectedVar   string
		expectedValue any
		expectedType  object.ObjectType
		wantErr       bool
		wantErrorMsg  string
	}{
		{
			name: "simple identity function with int",
			script: `
package main
func identity[T any](v T) T {
	return v
}
var result = identity[int](10)
`,
			expectedVar:   "result",
			expectedValue: int64(10),
			expectedType:  object.INTEGER_OBJ,
			wantErr:       false,
		},
		{
			name: "identity function with a struct",
			script: `
package main
type Box struct { Value int }
func identity[T any](v T) T {
	return v
}
var result = identity[Box](Box{Value: 42})
`,
			expectedVar:   "result",
			expectedValue: map[string]any{"Value": int64(42)},
			expectedType:  object.STRUCT_INSTANCE_OBJ,
			wantErr:       false,
		},
		{
			name: "generic function using type parameter internally",
			script: `
package main
type Box[T any] struct { Value T }
func newBox[T any](v T) Box[T] {
	var b Box[T]
	b = Box[T]{Value: v}
	return b
}
var result = newBox[string]("hello")
`,
			expectedVar:   "result",
			expectedValue: map[string]any{"Value": "hello"},
			expectedType:  object.STRUCT_INSTANCE_OBJ,
			wantErr:       false,
		},
		{
			name: "error: call generic function without instantiation",
			script: `
package main
func identity[T any](v T) T {
	return v
}
var result = identity(10)
`,
			wantErr:      true,
			wantErrorMsg: "cannot call generic function identity without instantiation",
		},
		{
			name: "error: wrong number of type arguments",
			script: `
package main
func identity[T any](v T) T {
	return v
}
var result = identity[int, string](10)
`,
			// This error happens during parsing, not evaluation.
			// So we check for it in the LoadFile step.
			wantErr:      true,
			wantErrorMsg: "wrong number of type arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp, err := minigo.NewInterpreter()
			if err != nil {
				t.Fatalf("NewInterpreter() error = %v", err)
			}

			err = interp.LoadFile("test.mgo", []byte(tt.script))
			if err != nil {
				if tt.wantErr && tt.wantErrorMsg != "" {
					if strings.Contains(err.Error(), tt.wantErrorMsg) {
						return // Correctly failed at parse time
					}
					t.Fatalf("LoadFile() error = %v, want error msg containing %q", err, tt.wantErrorMsg)
				}
				t.Fatalf("LoadFile() unexpected error = %v", err)
			}

			_, err = interp.Eval(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Eval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				} else if !strings.Contains(err.Error(), tt.wantErrorMsg) {
					t.Errorf("Expected error message to contain %q, but got %q", tt.wantErrorMsg, err.Error())
				}
				return
			}

			globalEnv := interp.GlobalEnvForTest()
			val, ok := globalEnv.Get(tt.expectedVar)
			if !ok {
				t.Fatalf("variable %q not found in global environment", tt.expectedVar)
			}

			if val.Type() != tt.expectedType {
				t.Fatalf("result has wrong type. got=%s, want=%s", val.Type(), tt.expectedType)
			}

			switch v := val.(type) {
			case *object.Integer:
				if v.Value != tt.expectedValue.(int64) {
					t.Errorf("result has wrong value. got=%d, want=%d", v.Value, tt.expectedValue)
				}
			case *object.StructInstance:
				expectedFields := tt.expectedValue.(map[string]any)
				if len(v.Fields) != len(expectedFields) {
					t.Errorf("wrong number of fields. got=%d, want=%d", len(v.Fields), len(expectedFields))
				}
				for name, expectedFieldVal := range expectedFields {
					actualFieldVal, ok := v.Fields[name]
					if !ok {
						t.Errorf("field %s not found", name)
						continue
					}
					switch actual := actualFieldVal.(type) {
					case *object.Integer:
						if actual.Value != expectedFieldVal.(int64) {
							t.Errorf("field %s has wrong value. got=%d, want=%d", name, actual.Value, expectedFieldVal)
						}
					case *object.String:
						if actual.Value != expectedFieldVal.(string) {
							t.Errorf("field %s has wrong value. got=%q, want=%q", name, actual.Value, expectedFieldVal)
						}
					default:
						t.Errorf("unhandled field type for checking: %T", actual)
					}
				}

			default:
				t.Errorf("unhandled result type for checking: %s", val.Type())
			}
		})
	}
}
