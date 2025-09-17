package symgotest

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo/object"
)

// AssertSuccess fails the test if the object is an error.
func AssertSuccess(t *testing.T, obj object.Object) {
	t.Helper()
	if obj == nil {
		t.Fatalf("expected success, but got a nil object")
	}
	if err, ok := obj.(*object.Error); ok {
		t.Fatalf("expected success, but got error: %s", err.Message)
	}
	if ret, ok := obj.(*object.ReturnValue); ok {
		if err, ok := ret.Value.(*object.Error); ok {
			t.Fatalf("expected success, but got error in return value: %s", err.Message)
		}
	}
}

// AssertError fails the test if the object is not an error.
// If `contains` has one or more elements, it also checks if the error message contains each of them.
func AssertError(t *testing.T, obj object.Object, contains ...string) {
	t.Helper()
	if obj == nil {
		t.Fatalf("expected an error, but got a nil object")
	}
	err, ok := obj.(*object.Error)
	if !ok {
		// Sometimes the error is wrapped in a ReturnValue
		if ret, ok := obj.(*object.ReturnValue); ok {
			err, ok = ret.Value.(*object.Error)
		}
	}

	if !ok || err == nil {
		t.Fatalf("expected an error, but got %T (%s)", obj, obj.Inspect())
	}

	for _, c := range contains {
		if !strings.Contains(err.Message, c) {
			t.Errorf("error message %q does not contain %q", err.Message, c)
		}
	}
}

// AssertInteger fails the test if the object is not an Integer with the expected value.
func AssertInteger(t *testing.T, obj object.Object, expected int64) {
	t.Helper()
	integer, ok := obj.(*object.Integer)
	if !ok {
		t.Fatalf("object is not Integer. got=%T (%+v)", obj, obj)
	}
	if integer.Value != expected {
		t.Errorf("integer has wrong value. want=%d, got=%d", expected, integer.Value)
	}
}

// AssertString fails the test if the object is not a String with the expected value.
func AssertString(t *testing.T, obj object.Object, expected string) {
	t.Helper()
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("object is not String. got=%T (%+v)", obj, obj)
	}
	if str.Value != expected {
		t.Errorf("String has wrong value. want=%q, got=%q", expected, str.Value)
	}
}

// AssertSymbolicNil fails the test if the object is not the symbolic NIL object.
func AssertSymbolicNil(t *testing.T, obj object.Object) {
	t.Helper()
	if obj != object.NIL {
		t.Fatalf("expected symbolic NIL, but got %T (%s)", obj, obj.Inspect())
	}
}

// AssertPlaceholder fails the test if the object is not a SymbolicPlaceholder.
func AssertPlaceholder(t *testing.T, obj object.Object) {
	t.Helper()
	if _, ok := obj.(*object.SymbolicPlaceholder); !ok {
		t.Fatalf("expected a SymbolicPlaceholder, but got %T (%s)", obj, obj.Inspect())
	}
}

// AssertEqual uses go-cmp to compare two values and fails the test if they are not equal.
// This is a generic helper for comparing complex structs or slices.
func AssertEqual(t *testing.T, want, got any, opts ...cmp.Option) {
	t.Helper()
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Errorf("values are not equal (-want +got):\n%s", diff)
	}
}
