package minigo_test

import (
	"context"
	"math"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	stdstrconv "github.com/podhmo/go-scan/minigo/stdlib/strconv"
)

func TestStdlib_strconv_comprehensive(t *testing.T) {
	script := `
package main
import "strconv"

var i, err_atoi = strconv.Atoi("123")
var s = strconv.Itoa(-42)

var f, err_parse_float = strconv.ParseFloat("3.1415", 64)
var s_float = strconv.FormatFloat(1.23e4, 'f', -1, 64)

var b, err_parse_bool = strconv.ParseBool("true")
var s_bool = strconv.FormatBool(false)

// Error cases
var _, err_bad_int = strconv.Atoi("abc")
var _, err_bad_float = strconv.ParseFloat("xyz", 64)
var _, err_bad_bool = strconv.ParseBool("nope")
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdstrconv.Install(interp)

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
		{"i", int64(123)},
		{"err_atoi", object.NIL},
		{"s", "-42"},
		{"f", 3.1415},
		{"err_parse_float", object.NIL},
		{"s_float", "12300"},
		{"b", true},
		{"err_parse_bool", object.NIL},
		{"s_bool", "false"},
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
			case float64:
				// Allow for small floating point inaccuracies
				if f, ok := val.(*object.Float); !ok || math.Abs(f.Value-expected) > 1e-9 {
					t.Errorf("got %v, want %v", val.Inspect(), expected)
				}
			case string:
				if s, ok := val.(*object.String); !ok || s.Value != expected {
					t.Errorf("got %v, want %v", val.Inspect(), expected)
				}
			case object.Object: // For NIL
				if val != expected {
					t.Errorf("got %v, want %v", val.Inspect(), expected.Inspect())
				}
			default:
				t.Fatalf("unsupported expected type %T", tt.expected)
			}
		})
	}

	// Check error cases
	t.Run("error_cases", func(t *testing.T) {
		for _, name := range []string{"err_bad_int", "err_bad_float", "err_bad_bool"} {
			val, ok := env.Get(name)
			if !ok {
				t.Fatalf("variable '%s' not found", name)
			}
			if val == object.NIL {
				t.Errorf("expected '%s' to be a non-nil error, but it was nil", name)
			}
			if _, ok := val.(*object.GoValue); !ok {
				t.Errorf("expected '%s' to be a GoValue, but got %T", name, val)
			}
		}
	})
}
