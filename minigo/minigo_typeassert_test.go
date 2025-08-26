package minigo

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo/object"
)

func TestTypeAssertion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		check  func(t *testing.T, i *Interpreter)
		panics string
		err    string
	}{
		{
			name: "successful single-value assertion",
			input: `package main
var i interface{} = "hello"
var s = i.(string)
`,
			check: func(t *testing.T, i *Interpreter) {
				val, _ := i.globalEnv.Get("s")
				str, ok := val.(*object.String)
				if !ok {
					t.Fatalf("s is not a string, got %T", val)
				}
				if str.Value != "hello" {
					t.Errorf(`s should be "hello", got %q`, str.Value)
				}
			},
		},
		{
			name: "successful two-value assertion",
			input: `package main
var i interface{} = "hello"
var s, ok = i.(string)
`,
			check: func(t *testing.T, i *Interpreter) {
				s, _ := i.globalEnv.Get("s")
				str, ok := s.(*object.String)
				if !ok {
					t.Fatalf("s is not a string, got %T", s)
				}
				if str.Value != "hello" {
					t.Errorf(`s should be "hello", got %q`, str.Value)
				}
				okVal, _ := i.globalEnv.Get("ok")
				if okVal != object.TRUE {
					t.Errorf("ok should be true, got %v", okVal)
				}
			},
		},
		{
			name: "failing two-value assertion",
			input: `package main
var i interface{} = 123
var s, ok = i.(string)
`,
			check: func(t *testing.T, i *Interpreter) {
				s, _ := i.globalEnv.Get("s")
				str, ok := s.(*object.String)
				if !ok {
					t.Fatalf("s is not a string, got %T", s)
				}
				if str.Value != "" { // zero value for string
					t.Errorf(`s should be "", got %q`, str.Value)
				}
				okVal, _ := i.globalEnv.Get("ok")
				if okVal != object.FALSE {
					t.Errorf("ok should be false, got %v", okVal)
				}
			},
		},
		{
			name: "failing single-value assertion, should panic",
			input: `package main
var i interface{} = 123
var s = i.(string)
`,
			panics: "interface conversion: type assertion failed",
		},
		{
			name: "failing single-value assertion on nil interface, should panic",
			input: `package main
var i interface{}
var s = i.(string)
`,
			panics: "interface conversion: type assertion failed",
		},
		{
			name: "successful assertion to interface type",
			input: `package main
type Stringer interface {
	String() string
}
type MyStruct struct {
	val string
}
func (s MyStruct) String() string {
	return s.val
}
var i interface{} = MyStruct{val: "world"}
var s, ok = i.(Stringer)
`,
			check: func(t *testing.T, i *Interpreter) {
				okVal, _ := i.globalEnv.Get("ok")
				if okVal != object.TRUE {
					t.Errorf("ok should be true, got %v", okVal)
				}
				s, _ := i.globalEnv.Get("s")
				if _, isIface := s.(*object.InterfaceInstance); !isIface {
					t.Errorf("s should be an interface instance, got %T", s)
				}
			},
		},
		{
			name: "failing assertion to interface type",
			input: `package main
type Stringer interface {
	String() string
}
type OtherStruct struct {}
var i interface{} = OtherStruct{}
var s, ok = i.(Stringer)
`,
			check: func(t *testing.T, i *Interpreter) {
				okVal, _ := i.globalEnv.Get("ok")
				if okVal != object.FALSE {
					t.Errorf("ok should be false, got %v", okVal)
				}
			},
		},
		{
			name: "assertion on non-interface type, should error",
			input: `package main
var s string = "hello"
var v = s.(string)
`,
			err: "invalid type assertion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := newTestInterpreter(t)

			err := i.LoadFile("test.mgo", []byte(tt.input))
			if err != nil {
				if tt.err != "" && strings.Contains(err.Error(), tt.err) {
					return // Expected load error
				}
				t.Fatalf("LoadFile() failed: %+v", err)
			}

			_, err = i.Eval(context.Background())

			if tt.panics != "" {
				if err == nil {
					t.Fatalf("expected a panic, but got no error")
				}
				// The panic message is wrapped in the error.
				if !strings.Contains(err.Error(), tt.panics) {
					t.Fatalf("unexpected panic message.\ngot error: %v\nwant contains: %s", err, tt.panics)
				}
				return // Test passed
			}

			if tt.err != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, but got nil", tt.err)
				}
				if !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("unexpected error: %+v, want contains %q", err, tt.err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Eval() failed unexpectedly: %+v", err)
			}

			if tt.check != nil {
				tt.check(t, i)
			}
		})
	}
}
