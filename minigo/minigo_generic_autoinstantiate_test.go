package minigo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo/object"
)

func TestGenericFunctionAutoInstantiation(t *testing.T) {
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
			name: "call generic identity function with int",
			script: `
package main
func identity[T any](v T) T {
	return v
}
var result = identity(10)
`,
			expectedVar:   "result",
			expectedValue: int64(10),
			expectedType:  object.INTEGER_OBJ,
			wantErr:       false,
		},
		{
			name: "call generic identity function with string",
			script: `
package main
func identity[T any](v T) T {
	return v
}
var result = identity("hello")
`,
			expectedVar:   "result",
			expectedValue: "hello",
			expectedType:  object.STRING_OBJ,
			wantErr:       false,
		},
		{
			name: "call generic function with non-leading generic parameter",
			script: `
package main
func takeStringAndT[T any](s string, v T) T {
	return v
}
var result = takeStringAndT("hello", 10)
`,
			expectedVar:   "result",
			expectedValue: int64(10),
			expectedType:  object.INTEGER_OBJ,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := newTestInterpreter(t)

			err := interp.LoadFile("test.mgo", []byte(tt.script))
			if err != nil {
				if tt.wantErr && tt.wantErrorMsg != "" {
					if strings.Contains(err.Error(), tt.wantErrorMsg) {
						return
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
			case *object.String:
				if v.Value != tt.expectedValue.(string) {
					t.Errorf("result has wrong value. got=%q, want=%q", v.Value, tt.expectedValue)
				}
			default:
				t.Errorf("unhandled result type for checking: %s", val.Type())
			}
		})
	}
}
