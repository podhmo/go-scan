package minigo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	stdstrings "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

func TestStdlib_strings_comprehensive(t *testing.T) {
	script := `
package main
import "strings"

var s = "hello world, hello"

var r_contains = strings.Contains(s, "world")
var r_not_contains = strings.Contains(s, "goodbye")

var r_has_prefix = strings.HasPrefix(s, "hello")
var r_not_has_prefix = strings.HasPrefix(s, "world")

var r_has_suffix = strings.HasSuffix(s, "hello")
var r_not_has_suffix = strings.HasSuffix(s, "world")

var r_index = strings.Index(s, "world")
var r_last_index = strings.LastIndex(s, "hello")

var r_replace = strings.Replace(s, "hello", "hi", 1)
var r_replace_all = strings.ReplaceAll(s, "l", "L")

var r_to_lower = strings.ToLower(s)
var r_to_upper = strings.ToUpper(s)

var spaced = "  \t hello \n  "
var r_trim_space = strings.TrimSpace(spaced)

var list = []string{"a", "b", "c"}
var r_join = strings.Join(list, "-")

var csv = "a,b,c"
var r_split = strings.Split(csv, ",")
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdstrings.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	_, err = interp.Eval(context.Background())
	if err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()

	tests := []struct {
		name     string
		expected any
	}{
		{"r_contains", true},
		{"r_not_contains", false},
		{"r_has_prefix", true},
		{"r_not_has_prefix", false},
		{"r_has_suffix", true},
		{"r_not_has_suffix", false},
		{"r_index", int64(6)},
		{"r_last_index", int64(13)},
		{"r_replace", "hi world, hello"},
		{"r_replace_all", "heLLo worLd, heLLo"},
		{"r_to_lower", "hello world, hello"},
		{"r_to_upper", "HELLO WORLD, HELLO"},
		{"r_trim_space", "hello"},
		{"r_join", "a-b-c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := env.Get(tt.name)
			if !ok {
				t.Fatalf("variable '%s' not found", tt.name)
			}
			switch expected := tt.expected.(type) {
			case bool:
				if b, ok := val.(*object.Boolean); !ok || b.Value != expected {
					t.Errorf("got %v, want %v", val.Inspect(), expected)
				}
			case int64:
				if i, ok := val.(*object.Integer); !ok || i.Value != expected {
					t.Errorf("got %v, want %v", val.Inspect(), expected)
				}
			case string:
				if s, ok := val.(*object.String); !ok || s.Value != expected {
					t.Errorf("got %v, want %v", val.Inspect(), expected)
				}
			default:
				t.Fatalf("unsupported expected type %T", tt.expected)
			}
		})
	}

	// Special check for r_split as it returns a slice
	t.Run("r_split", func(t *testing.T) {
		val, ok := env.Get("r_split")
		if !ok {
			t.Fatalf("variable 'r_split' not found")
		}
		arr, ok := val.(*object.Array)
		if !ok {
			t.Fatalf("r_split is not an Array, got %T", val)
		}
		expected := []string{"a", "b", "c"}
		if len(arr.Elements) != len(expected) {
			t.Fatalf("split result has wrong length, got %d, want %d", len(arr.Elements), len(expected))
		}
		for i, el := range arr.Elements {
			s, ok := el.(*object.String)
			if !ok {
				t.Fatalf("element %d is not a String, got %T", i, el)
			}
			if s.Value != expected[i] {
				t.Errorf("element %d is wrong, got %q, want %q", i, s.Value, expected[i])
			}
		}
	})
}
