package evaluator_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestSelectorRobustness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		want object.ObjectType
	}{
		{
			name: "selector on function",
			code: `
package main
func main() {
	f := func() {}
	_ = f.NonExistent
}`,
			want: object.SYMBOLIC_OBJ,
		},
		{
			name: "selector on map",
			code: `
package main
func main() {
	m := make(map[string]int)
	_ = m.NonExistent
}`,
			want: object.SYMBOLIC_OBJ,
		},
		{
			name: "selector on slice",
			code: `
package main
func main() {
	s := []int{1, 2, 3}
	_ = s.NonExistent
}`,
			want: object.SYMBOLIC_OBJ,
		},
		{
			name: "selector on string",
			code: `
package main
func main() {
	s := "hello"
	_ = s.NonExistent
}`,
			want: object.SYMBOLIC_OBJ,
		},
		{
			name: "selector on builtin",
			code: `
package main
func main() {
	_ = new.NonExistent
}`,
			// `new` is an intrinsic, so this tests the case for *object.Intrinsic
			want: object.SYMBOLIC_OBJ,
		},
		{
			name: "selector on panic object",
			code: `
package main
func mightPanic() {
	panic("oops")
}
func main() {
	mightPanic()
}`,
			// The evaluation of `mightPanic()` results in a PanicError.
			// This error propagates up and should be the result of the `evalCallExpr`.
			// The important part is that the evaluator doesn't crash with a Go panic.
			// The final result of the whole program analysis will be the PanicError object.
			want: object.PANIC_OBJ,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tc := symgotest.TestCase{
				Source: map[string]string{
					"go.mod":  "module example.com/main",
					"main.go": tt.code,
				},
				EntryPoint: "example.com/main.main",
			}

			if tt.want == object.PANIC_OBJ {
				tc.ExpectError = true
			}

			symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
				if tt.want == object.PANIC_OBJ {
					if r.Error == nil {
						t.Fatal("expected a panic error, but got none")
					}
					if _, ok := r.Error.(*object.PanicError); !ok {
						t.Fatalf("expected a PanicError, but got %T: %v", r.Error, r.Error)
					}
				} else {
					// For other cases, we expect the analysis to complete without error.
					// The robustness fix means the selector expression itself doesn't cause a symgo error.
					if r.Error != nil {
						t.Fatalf("evaluation failed unexpectedly: %v", r.Error)
					}
				}
			})
		})
	}
}