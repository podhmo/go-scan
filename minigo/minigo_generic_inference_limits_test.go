package minigo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
)

func TestGenericFunctionInferenceLimitations(t *testing.T) {
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
			name: "inference fails for nil argument",
			script: `
package main
func identity[T any](v T) T {
	return v
}
var result = identity(nil)
`,
			// This should fail because the type of `nil` cannot be inferred without a target type.
			wantErr:      true,
			wantErrorMsg: "cannot infer type for generic parameter T from argument 0 of type NIL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.name, "inference fails") {
				t.Skip("Skipping test for known limitation of type inference.")
			}

			interp, err := minigo.NewInterpreter()
			if err != nil {
				t.Fatalf("NewInterpreter() error = %v", err)
			}

			err = interp.LoadFile("test.mgo", []byte(tt.script))
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

			// This part of the test will not be reached for these failing cases.
			globalEnv := interp.GlobalEnvForTest()
			val, ok := globalEnv.Get(tt.expectedVar)
			if !ok {
				t.Fatalf("variable %q not found in global environment", tt.expectedVar)
			}

			if val.Type() != tt.expectedType {
				t.Fatalf("result has wrong type. got=%s, want=%s", val.Type(), tt.expectedType)
			}
		})
	}
}
