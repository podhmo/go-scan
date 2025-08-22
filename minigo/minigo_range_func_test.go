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
		{
			name: "generator function pattern",
			input: `
package main
var result = func() {
	rangeUpTo := func(n int) {
		return func(yield int) {
			for i := 0; i < n; i = i + 1 {
				if !yield(i) {
					return
				}
			}
		}
	}

	var r = []int{}
	for v := range rangeUpTo(5) {
		r = append(r, v)
	}
	return r
}()
`,
			expected: []any{int64(0), int64(1), int64(2), int64(3), int64(4)},
		},
		{
			name: "range over integer",
			input: `
package main
var result = func() {
	var r = []int{}
	for i := range 10 {
		r = append(r, i)
	}
	return r
}()
`,
			expected: []any{int64(0), int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7), int64(8), int64(9)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := newTestInterpreter(t)
			interp.GlobalEnvForTest().Set("panic", &object.Builtin{
				Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
					if len(args) != 1 {
						return ctx.NewError(pos, "panic requires 1 argument")
					}
					str, ok := args[0].(*object.String)
					if !ok {
						return ctx.NewError(pos, "panic argument must be a string")
					}
					return ctx.NewError(pos, "panic: %s", str.Value)
				},
			})

			if err := interp.LoadFile("test.mgo", []byte(tt.input)); err != nil {
				t.Fatalf("LoadFile() failed: %v", err)
			}
			var err error
			_, err = interp.Eval(context.Background())

			if tt.wantErrorMsg != "" {
				if err == nil {
					val, _ := interp.globalEnv.Get("result")
					nativeVal, _ := toGoValue(val)
					t.Fatalf("expected error, but got none. result was: %#v", nativeVal)
				}
				if !strings.Contains(err.Error(), tt.wantErrorMsg) {
					t.Errorf("wrong error message.\n- want: %q\n- got:  %q", tt.wantErrorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("minigo.Eval() returned an unexpected error: %v", err)
			}

			// Handle special cases for stateful/stateless tests
			if tt.name == "stateless iterator resumes from start" || tt.name == "stateful iterator resumes from last position" {
				val1, ok1 := interp.globalEnv.Get("result1")
				if !ok1 {
					t.Fatalf("variable 'result1' not found")
				}
				val2, ok2 := interp.globalEnv.Get("result2")
				if !ok2 {
					t.Fatalf("variable 'result2' not found")
				}
				native1, _ := toGoValue(val1)
				native2, _ := toGoValue(val2)
				combinedResult := []any{native1, native2}
				if diff := cmp.Diff(tt.expected, combinedResult); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				return
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
