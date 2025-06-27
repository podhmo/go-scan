package parser_test

import (
	"testing"

	"github.com/podhmo/go-scan/examples/derivingbind/parser"
)

func TestString(t *testing.T) {
	val, err := parser.String("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %q", val)
	}
}

func TestInt(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		val, err := parser.Int("123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != 123 {
			t.Errorf("expected 123, got %d", val)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := parser.Int("abc")
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
	})
}

func TestBool(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
		hasError bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"1", true, false},
		{"0", false, false},
		{"T", true, false},
		{"F", false, false},
		{"invalid", false, true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			val, err := parser.Bool(tc.input)
			if tc.hasError {
				if err == nil {
					t.Fatal("expected an error, but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if val != tc.expected {
					t.Errorf("expected %v, got %v", tc.expected, val)
				}
			}
		})
	}
}

func TestFloat64(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		val, err := parser.Float64("3.14")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != 3.14 {
			t.Errorf("expected 3.14, got %f", val)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := parser.Float64("not-a-float")
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
	})
}
