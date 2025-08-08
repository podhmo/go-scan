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

			goVal, ok := res.Value.(*object.GoValue)
			if !ok {
				t.Fatalf("result is not a GoValue, but a %T", res.Value)
			}

			if goVal.Value.Interface() != tt.expectedVal {
				t.Errorf("wrong value. got=%v, want=%v", goVal.Value.Interface(), tt.expectedVal)
			}
		})
	}
}
