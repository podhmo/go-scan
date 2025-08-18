package scope

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
)

func TestScope_SetAndGet(t *testing.T) {
	scope := NewScope()
	expected := &object.String{Value: "hello"}
	scope.Set("myVar", expected)

	val, ok := scope.Get("myVar")
	if !ok {
		t.Fatal("Get() failed, myVar not found")
	}

	if val != expected {
		t.Errorf("Get() returned wrong value. want=%+v, got=%+v", expected, val)
	}
}

func TestScope_GetFromOuter(t *testing.T) {
	outer := NewScope()
	expected := &object.String{Value: "world"}
	outer.Set("myVar", expected)

	inner := NewEnclosedScope(outer)
	val, ok := inner.Get("myVar")
	if !ok {
		t.Fatal("Get() failed, myVar not found in outer scope")
	}

	if val != expected {
		t.Errorf("Get() returned wrong value from outer scope. want=%+v, got=%+v", expected, val)
	}
}

func TestScope_SetInInnerShadowsOuter(t *testing.T) {
	outer := NewScope()
	outer.Set("myVar", &object.String{Value: "outer"})

	inner := NewEnclosedScope(outer)
	innerExpected := &object.String{Value: "inner"}
	inner.Set("myVar", innerExpected)

	// Get from inner scope should return inner value
	val, ok := inner.Get("myVar")
	if !ok {
		t.Fatal("Get() from inner failed")
	}
	if val != innerExpected {
		t.Errorf("Get() from inner returned wrong value. want=%+v, got=%+v", innerExpected, val)
	}

	// Get from outer scope should still return outer value
	outerVal, ok := outer.Get("myVar")
	if !ok {
		t.Fatal("Get() from outer failed")
	}
	if outerVal.(*object.String).Value != "outer" {
		t.Errorf("outer scope value was incorrectly modified. want='outer', got=%q", outerVal.(*object.String).Value)
	}
}

func TestScope_GetNotFound(t *testing.T) {
	scope := NewScope()
	_, ok := scope.Get("nonExistent")
	if ok {
		t.Error("Get() found a non-existent variable")
	}
}
