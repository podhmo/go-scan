package minigo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	stdbytes "github.com/podhmo/go-scan/minigo/stdlib/bytes"
)

func TestStdlib_bytes_comprehensive(t *testing.T) {
	script := `
package main
import "bytes"

var s = "hello world, hello"
var b = []byte(s)

var r_contains = bytes.Contains(b, []byte("world"))
var r_not_contains = bytes.Contains(b, []byte("goodbye"))

var r_has_prefix = bytes.HasPrefix(b, []byte("hello"))
var r_not_has_prefix = bytes.HasPrefix(b, []byte("world"))

var r_has_suffix = bytes.HasSuffix(b, []byte("hello"))
var r_not_has_suffix = bytes.HasSuffix(b, []byte("world"))

var r_index = bytes.Index(b, []byte("world"))
var r_last_index = bytes.LastIndex(b, []byte("hello"))

var r_replace = string(bytes.Replace(b, []byte("hello"), []byte("hi"), 1))
var r_replace_all = string(bytes.ReplaceAll(b, []byte("l"), []byte("L")))

var r_to_lower = string(bytes.ToLower(b))
var r_to_upper = string(bytes.ToUpper(b))

var spaced = "  \t hello \n  "
var r_trim_space = string(bytes.TrimSpace([]byte(spaced)))

`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdbytes.Install(interp)

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
}
