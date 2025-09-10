package object

import (
	"go/ast"
	"testing"
	"time"
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
		{
			obj:             &AstNode{Node: &ast.Ident{Name: "x"}},
			expectedType:    AST_NODE_OBJ,
			expectedInspect: "ast.Node[*ast.Ident]",
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

func TestEnvironment_CycleDetection(t *testing.T) {
	// Setup: Create two environments and make them point to each other,
	// creating a cycle in the `outer` chain.
	env1 := NewEnvironment()
	env2 := NewEnclosedEnvironment(env1) // env2.outer = env1

	// Introduce the cycle by setting env1's outer to env2.
	// This is possible because we are in the same package.
	env1.outer = env2

	// We define a variable in env2 to test the 'Assign' method's traversal.
	env2.Set("myVar", &Integer{Value: 1})

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "Get",
			fn: func() {
				_, found := env1.Get("nonexistent")
				if found {
					t.Error("Get: expected not to find the object, but it was found")
				}
			},
		},
		{
			name: "GetAddress",
			fn: func() {
				_, found := env1.GetAddress("nonexistent")
				if found {
					t.Error("GetAddress: expected not to find the address, but it was found")
				}
			},
		},
		{
			name: "GetConstant",
			fn: func() {
				_, found := env1.GetConstant("nonexistent")
				if found {
					t.Error("GetConstant: expected not to find the constant, but it was found")
				}
			},
		},
		{
			name: "Assign",
			fn: func() {
				// Test successful assignment traversal
				if !env1.Assign("myVar", &Integer{Value: 2}) {
					t.Error("Assign: expected to find and assign 'myVar'")
				}
				// Test failed assignment traversal
				if env1.Assign("nonexistent", &Integer{Value: 99}) {
					t.Error("Assign: expected not to find and assign 'nonexistent'")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan bool)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("The code panicked: %v", r)
					}
					close(done)
				}()
				tt.fn()
			}()

			select {
			case <-done:
				// Test completed successfully. This is the expected behavior AFTER the fix.
				// Before the fix, this test will time out.
			case <-time.After(2 * time.Second):
				t.Fatalf("Test timed out, infinite recursion suspected in Environment.%s()", tt.name)
			}
		})
	}
}
