package object

import (
	"fmt"
	"go/ast"
	"reflect"
	"testing"
	"time"
)

func TestString_Type(t *testing.T) {
	s := &String{Value: "hello"}
	if s.Type() != STRING_OBJ {
		t.Errorf("s.Type() wrong. want=%q, got=%q", STRING_OBJ, s.Type())
	}
}

func TestString_Inspect(t *testing.T) {
	s := &String{Value: "hello world"}
	if s.Inspect() != `"hello world"` {
		t.Errorf(`s.Inspect() wrong. want=%q, got=%q`, `"hello world"`, s.Inspect())
	}
}

func TestFunction_Type(t *testing.T) {
	f := &Function{Name: &ast.Ident{Name: "myFunc"}}
	if f.Type() != FUNCTION_OBJ {
		t.Errorf("f.Type() wrong. want=%q, got=%q", FUNCTION_OBJ, f.Type())
	}
}

func TestFunction_Inspect(t *testing.T) {
	f := &Function{Name: &ast.Ident{Name: "myFunc"}}
	expected := "func myFunc() { ... }"
	if f.Inspect() != expected {
		t.Errorf("f.Inspect() wrong. want=%q, got=%q", expected, f.Inspect())
	}
}

func TestError_Type(t *testing.T) {
	e := &Error{Message: "an error"}
	if e.Type() != ERROR_OBJ {
		t.Errorf("e.Type() wrong. want=%q, got=%q", ERROR_OBJ, e.Type())
	}
}

func TestError_Inspect(t *testing.T) {
	e := &Error{Message: "file not found"}
	expected := "symgo runtime error: file not found\n"
	if e.Inspect() != expected {
		t.Errorf("e.Inspect() wrong. want=%q, got=%q", expected, e.Inspect())
	}
}

func TestSymbolicPlaceholder_Type(t *testing.T) {
	sp := &SymbolicPlaceholder{Reason: "test"}
	if sp.Type() != SYMBOLIC_OBJ {
		t.Errorf("sp.Type() wrong. want=%q, got=%q", SYMBOLIC_OBJ, sp.Type())
	}
}

func TestSymbolicPlaceholder_Inspect(t *testing.T) {
	reason := "result of external call"
	sp := &SymbolicPlaceholder{Reason: reason}
	expected := fmt.Sprintf("<Symbolic: %s>", reason)
	if sp.Inspect() != expected {
		t.Errorf("sp.Inspect() wrong. want=%q, got=%q", expected, sp.Inspect())
	}
}

func TestEnvironment_Get_InfiniteRecursion(t *testing.T) {
	// This test is designed to fail before cycle detection is added to Environment.Get.
	// It uses reflection to manually create a cycle in the environment's outer chain.
	env1 := NewEnvironment()
	env2 := NewEnclosedEnvironment(env1) // env2.outer = env1

	// Use reflection to create a cycle: env1.outer = env2
	outerField := reflect.ValueOf(env1).Elem().FieldByName("outer")
	if !outerField.IsValid() {
		t.Fatal("could not find 'outer' field in Environment struct")
	}
	outerFieldPtr := reflect.NewAt(outerField.Type(), outerField.Addr().UnsafePointer()).Elem()
	outerFieldPtr.Set(reflect.ValueOf(env2))

	// Now we have the cycle: env1.outer -> env2 and env2.outer -> env1

	done := make(chan bool)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Get panicked: %v", r)
			}
			close(done)
		}()
		// This call will hang if there's no cycle detection.
		_, ok := env1.Get("nonexistent")
		if ok {
			t.Error("expected not found, but got a value")
		}
	}()

	select {
	case <-done:
		// Test finished, which means it didn't hang. This is the desired outcome after the fix.
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out. Infinite recursion in Get() detected.")
	}
}
