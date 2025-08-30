package testlog

import (
	"testing"
)

func TestNewContext(t *testing.T) {
	// Call the function to be tested
	ctx := NewContext(t)

	// Retrieve the value from the context using the exported key
	val := ctx.Value(ContextKeyTesting)
	if val == nil {
		t.Fatal("context value for key", ContextKeyTesting, "is nil")
	}

	// Assert that the value is the testing object we passed in
	if got, ok := val.(*testing.T); !ok {
		t.Fatalf("expected value to be of type *testing.T, but got %T", val)
	} else if got != t {
		t.Errorf("got %v, want %v", got, t)
	}
}
