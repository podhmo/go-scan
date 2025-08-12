package minigo

import (
	"context"
	"strings"
	"testing"

	"go/token"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo/object"
)

// toGoValue is a test helper to convert minigo objects to native Go values for comparison.
func toGoValue(obj object.Object) (any, error) {
	switch o := obj.(type) {
	case *object.Integer:
		return o.Value, nil
	case *object.String:
		return o.Value, nil
	case *object.Boolean:
		return o.Value, nil
	case *object.Nil:
		return nil, nil
	case *object.Array:
		s := make([]any, len(o.Elements))
		for i, elem := range o.Elements {
			var err error
			s[i], err = toGoValue(elem)
			if err != nil {
				return nil, err
			}
		}
		return s, nil
	default:
		return obj.Inspect(), nil // fallback to inspect for others
	}
}

func TestRangeFunction(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expected     any // Use 'any' for easier comparison with native Go slices
		wantErrorMsg string
	}{
		{
			name: "simple iteration with one value",
			input: `
package main
var result = func() {
	seq := func(yield int) {
		if !yield(10) { return }
		if !yield(20) { return }
		if !yield(30) { return }
	}
	var r = []int{}
	for v := range seq {
		r = append(r, v)
	}
	return r
}()
`,
			expected:     []any{int64(10), int64(20), int64(30)},
			wantErrorMsg: "",
		},
		{
			name: "simple iteration with two values",
			input: `
package main
var result = func() {
	seq := func(yield int) { // dummy type
		if !yield(0, "a") { return }
		if !yield(1, "b") { return }
	}
	var r = []int{}
	for k, v := range seq {
		r = append(r, k)
		r = append(r, v)
	}
	return r
}()
`,
			expected:     []any{int64(0), "a", int64(1), "b"},
			wantErrorMsg: "",
		},
		{
			name: "break statement",
			input: `
package main
var result = func() {
	seq := func(yield int) {
		if !yield(10) { return }
		if !yield(20) { return }
		if !yield(30) { return }
	}
	var r = []int{}
	for v := range seq {
		if v == 20 {
			break
		}
		r = append(r, v)
	}
	return r
}()
`,
			expected: []any{int64(10)},
		},
		{
			name: "continue statement",
			input: `
package main
var result = func() {
	seq := func(yield int) {
		if !yield(10) { return }
		if !yield(20) { return }
		if !yield(30) { return }
	}
	var r = []int{}
	for v := range seq {
		if v == 20 {
			continue
		}
		r = append(r, v)
	}
	return r
}()
`,
			expected: []any{int64(10), int64(30)},
		},
		{
			name: "error propagation from loop body",
			input: `
package main
var result = func() {
	seq := func(yield int) {
		if !yield(1) { return }
		if !yield(2) { return }
	}
	var r = []int{}
	for v := range seq {
		if v == 2 {
			panic("stop!")
		}
		r = append(r, v)
	}
	return r
}()
`,
			wantErrorMsg: "panic: stop!",
		},
		{
			name: "return propagation",
			input: `
package main
var result = func() any {
	seq := func(yield int) {
		if !yield(1) { return }
		if !yield(2) { return }
	}

	var r = []int{}
	for v := range seq {
		if v == 2 {
			return "early exit"
		}
		r = append(r, v)
	}
	return r
}()
`,
			expected: "early exit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp, err := NewInterpreter()
			if err != nil {
				t.Fatalf("NewInterpreter() failed: %v", err)
			}
			interp.GlobalEnvForTest().Set("panic", &object.Builtin{
				Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
					if len(args) != 1 {
						return ctx.NewError(pos, "panic requires 1 argument")
					}
					str, ok := args[0].(*object.String)
					if !ok {
						return ctx.NewError(pos, "panic argument must be a string")
					}
					// The interpreter's Eval loop has a recover that turns this into an Error object.
					// For testing, we can just return the error directly.
					return ctx.NewError(pos, "panic: %s", str.Value)
				},
			})

			if err := interp.LoadFile("test.mgo", []byte(tt.input)); err != nil {
				t.Fatalf("LoadFile() failed: %v", err)
			}
			_, err = interp.Eval(context.Background())

			if tt.wantErrorMsg != "" {
				if err == nil {
					// To see the actual result in case of unexpected success
					val, _ := interp.globalEnv.Get("result")
					nativeVal, _ := toGoValue(val)
					t.Fatalf("expected error, but got none. result was: %#v", nativeVal)
				}
				if !strings.Contains(err.Error(), tt.wantErrorMsg) {
					t.Errorf("wrong error message.\n- want: %q\n- got:  %q", tt.wantErrorMsg, err.Error())
				}
				return // Test passes if the correct error is found
			}

			if err != nil {
				t.Fatalf("minigo.Eval() returned an unexpected error: %v", err)
			}

			val, ok := interp.globalEnv.Get("result")
			if !ok {
				t.Fatalf("variable 'result' not found in environment")
			}

			nativeResult, err := toGoValue(val)
			if err != nil {
				t.Fatalf("failed to convert result to Go value: %v", err)
			}

			if diff := cmp.Diff(tt.expected, nativeResult); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
