package symgo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestServeError(t *testing.T) {
	t.Run("it should not recurse infinitely", func(t *testing.T) {
		code := `
package main

import (
	"errors"
	"net/http"
	"strings"
)

type CompositeError struct {
	Errors []error
}

func (e *CompositeError) Error() string {
	return "composite error"
}

func flattenComposite(e *CompositeError) *CompositeError {
	return e
}

func ServeError(rw http.ResponseWriter, r *http.Request, err error) {
	switch e := err.(type) {
	case *CompositeError:
		er := flattenComposite(e)
		ServeError(rw, r, er.Errors[0])
	default:
		// do nothing
	}
}

func main() {
	ServeError(nil, nil, &CompositeError{Errors: []error{errors.New("test error")}})
}
`
		tc := symgotest.TestCase{
			Source: map[string]string{
				"go.mod":  "module myapp",
				"main.go": code,
			},
			EntryPoint: "myapp.main",
		}

		symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
			// The original test checked for a timeout.
			// With symgotest, the check is that the run completes without error,
			// relying on the interpreter's bounded recursion to prevent a hang.
			if r.Error != nil {
				t.Fatalf("expected no error, but got: %+v", r.Error)
			}
		})
	})
}

func TestRecursion_method(t *testing.T) {
	cases := []struct {
		Name string
		Code string
	}{
		{
			Name: "linked list traversal (should not be infinite recursion)",
			Code: `
package main
type Node struct {
	Name string
	Next *Node
}
func (n *Node) Traverse() {
	if n.Next != nil {
		n.Next.Traverse()
	}
}
func main() {
	last := &Node{Name: "last"}
	first := &Node{Name: "first", Next: last}
	first.Traverse()
}
`,
		},
		{
			Name: "actual infinite recursion in method (should be bounded)",
			Code: `
package main
type Looper struct {}
func (l *Looper) Loop() {
	l.Loop()
}
func main() {
	l := &Looper{}
	l.Loop()
}
`,
		},
		{
			Name: "no-arg function recursion (should be bounded)",
			Code: `
package main
func Recur() {
	Recur()
}
func main() {
	Recur()
}
`,
		},
		{
			Name: "deep but finite recursion (should be bounded)",
			Code: `
package main
func Recur(n int) {
	if n > 0 {
		Recur(n - 1)
	}
}
func main() {
	Recur(20) // A depth that would be slow but not infinite
}
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			tc := symgotest.TestCase{
				Source: map[string]string{
					"go.mod":  "module myapp",
					"main.go": tt.Code,
				},
				EntryPoint: "myapp.main",
			}
			symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
				// In all cases, we expect the interpreter's bounded recursion to prevent
				// a timeout or a max steps error.
				if r.Error != nil {
					t.Fatalf("expected no error, but got: %+v", r.Error)
				}
			})
		})
	}
}

func TestEval_CompositeLiteral_RecursiveVar(t *testing.T) {
	// This test case uses invalid Go code (`var V = T{F: &V}`).
	// The Go compiler would reject this. The goal of this test is to ensure
	// that the symbolic evaluator is robust enough to handle such a case
	// without panicking, by correctly detecting the evaluation cycle.
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/m",
			"main.go": `
package main
type T struct { F *T }
var V = T{F: &V}
func main() { _ = V }
`,
		},
		EntryPoint: "example.com/m.main",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		// The key is that the evaluator should not panic.
		// It's acceptable for it to return an error since the code is invalid.
		// The original test was happy with "identifier not found".
		if r.Error != nil {
			t.Logf("Interpreter returned an expected error: %v", r.Error)
			err, ok := r.Error.(*object.Error)
			if !ok {
				t.Fatalf("expected error to be of type *object.Error, but got %T", r.Error)
			}
			if !strings.Contains(err.Message, "identifier not found: V") {
				t.Errorf("expected 'identifier not found' error, but got: %v", err)
			}
		}
	})
}

func TestRecursion_HigherOrder(t *testing.T) {
	// This test case reproduces a stack overflow that occurs when two functions
	// recursively call each other indirectly through a higher-order function.
	// The evaluator's recursion detection must be smart enough to distinguish
	// between `cont(Ping)` and `cont(Pong)` and not just see two calls to `cont`.
	source := `
package myapp

// cont is a helper function to create an indirect call.
func cont(f func(int), n int) {
	f(n)
}

// Ping is an indirectly mutually recursive function with Pong.
func Ping(n int) {
	if n > 1 {
		return
	}
	// Calls Pong via the cont helper
	cont(Pong, n+1)
}

// Pong is an indirectly mutually recursive function with Ping.
func Pong(n int) {
	if n > 1 {
		return
	}
	// Calls Ping via the cont helper
	cont(Ping, n+1)
}

func main() {
	Ping(0)
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module myapp",
			"main.go": source,
		},
		EntryPoint: "myapp.main",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		// The key is that the run should complete without error, relying on the
		// interpreter's recursion detection to prevent a stack overflow.
		if r.Error != nil {
			t.Fatalf("expected no error, but got: %+v", r.Error)
		}
	})
}

func TestRecursionWithMultiReturn(t *testing.T) {
	// This test case reproduces a hang that occurred when a recursive function
	// had multiple return values. The test passes if it completes without error.
	source := `
package main

type Env struct {
	Outer *Env
}

func (e *Env) Get(name string) (any, bool) {
	if e.Outer != nil {
		return e.Outer.Get(name) // Recursive call
	}
	return nil, false
}

func main() {
	env := &Env{Outer: &Env{}}
	env.Get("foo")
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/recursion",
			"main.go": source,
		},
		EntryPoint: "example.com/recursion.main",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("expected no error, but got: %+v", r.Error)
		}
	})
}
