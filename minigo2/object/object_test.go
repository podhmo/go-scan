package object

import (
	"testing"
)

func TestObjectTypes(t *testing.T) {
	tests := []struct {
		obj             Object
		expectedType    ObjectType
		expectedInspect string
	}{
		{
			obj:             &Integer{Value: 123},
			expectedType:    INTEGER_OBJ,
			expectedInspect: "123",
		},
		{
			obj:             &String{Value: "hello"},
			expectedType:    STRING_OBJ,
			expectedInspect: "hello",
		},
		{
			obj:             TRUE,
			expectedType:    BOOLEAN_OBJ,
			expectedInspect: "true",
		},
		{
			obj:             FALSE,
			expectedType:    BOOLEAN_OBJ,
			expectedInspect: "false",
		},
		{
			obj:             NIL,
			expectedType:    NIL_OBJ,
			expectedInspect: "nil",
		},
	}

	for _, tt := range tests {
		if tt.obj.Type() != tt.expectedType {
			t.Errorf("wrong type: expected=%q, got=%q", tt.expectedType, tt.obj.Type())
		}
		if tt.obj.Inspect() != tt.expectedInspect {
			t.Errorf("wrong inspect: expected=%q, got=%q", tt.expectedInspect, tt.obj.Inspect())
		}
	}
}
