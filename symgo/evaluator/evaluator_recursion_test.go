package evaluator_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestServeError(t *testing.T) {
	t.Run("it should not recurse infinitely", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

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

		files := map[string]string{
			"go.mod":  "module myapp",
			"main.go": code,
		}
		dir, cleanup := scantest.WriteFiles(t, files)
		defer cleanup()

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			interp, err := symgo.NewInterpreter(s)
			if err != nil {
				return fmt.Errorf("NewInterpreter failed: %w", err)
			}

			var called bool
			interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
				fn, ok := args[0].(*object.Function)
				if !ok {
					return nil
				}
				if fn.Name != nil && fn.Name.Name == "ServeError" {
					called = true
				}
				return nil
			})

			mainPkg := pkgs[0]
			// First, Eval the whole package to define all symbols.
			for _, fileAst := range mainPkg.AstFiles {
				if _, err := interp.Eval(ctx, fileAst, mainPkg); err != nil {
					return err
				}
			}

			// Then, find the main function and Apply it.
			mainFn, ok := interp.FindObject("main")
			if !ok {
				return fmt.Errorf("main function not found")
			}
			fn, ok := mainFn.(*object.Function)
			if !ok {
				return fmt.Errorf("main is not a function")
			}

			if _, err := interp.Apply(ctx, fn, []object.Object{}, mainPkg); err != nil {
				return err
			}

			if !called {
				t.Error("ServeError was not called")
			}
			return nil
		}

		s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
		if err != nil {
			t.Fatalf("goscan.New failed: %v", err)
		}

		if _, err := scantest.Run(t, ctx, dir, []string{"."}, action, scantest.WithScanner(s)); err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") {
				t.Fatal("interpreter timed out, infinite recursion suspected")
			}
			t.Fatalf("scantest.Run failed: %v", err)
		}
	})
}

func TestRecursion_method(t *testing.T) {
	cases := []struct {
		Name          string
		Code          string
		ShouldFail    bool
		ExpectedError string
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
			ShouldFail:    false,
			ExpectedError: "",
		},
		{
			Name: "actual infinite recursion in method",
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
			ShouldFail:    false,
			ExpectedError: "",
		},
		{
			Name: "no-arg function recursion",
			Code: `
package main

func Recur() {
	Recur()
}

func main() {
	Recur()
}
`,
			ShouldFail:    false,
			ExpectedError: "",
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
			ShouldFail:    false, // With the new bounded logic, this should not error or time out.
			ExpectedError: "",
		},
		{
			Name: "recursive method call on a field, mimicking e.outer.Get()",
			Code: `
package main

type Node struct {
    Name  string
    Outer *Node
}

// Get mimics the recursive structure that caused the bug.
func (n *Node) Get(name string) {
    if n.Outer != nil {
        n.Outer.Get(name) // Recursive call on a field
    }
}

func main() {
    root := &Node{Name: "root"}
    child := &Node{Name: "child", Outer: root}
    // Create a cycle for the analysis to find.
    root.Outer = child
    child.Get("foo")
}
`,
			ShouldFail:    false,
			ExpectedError: "",
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			files := map[string]string{
				"go.mod":  "module myapp",
				"main.go": tt.Code,
			}
			dir, cleanup := scantest.WriteFiles(t, files)
			defer cleanup()

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				interp, err := symgo.NewInterpreter(s)
				if err != nil {
					return fmt.Errorf("NewInterpreter failed: %w", err)
				}

				mainPkg := pkgs[0]
				// First, Eval the whole package to define all symbols.
				for _, fileAst := range mainPkg.AstFiles {
					if _, err := interp.Eval(ctx, fileAst, mainPkg); err != nil {
						return err
					}
				}

				// Then, find the main function and Apply it.
				mainFnObj, ok := interp.FindObject("main")
				if !ok {
					return fmt.Errorf("main function not found")
				}
				mainFn, ok := mainFnObj.(*object.Function)
				if !ok {
					return fmt.Errorf("main is not a function")
				}

				_, err = interp.Apply(ctx, mainFn, []object.Object{}, mainPkg)
				return err
			}

			s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
			if err != nil {
				t.Fatalf("goscan.New failed: %v", err)
			}

			_, err = scantest.Run(t, ctx, dir, []string{"."}, action, scantest.WithScanner(s))

			if tt.ShouldFail {
				if err == nil {
					t.Fatalf("expected an error, but got none")
				}
				if !strings.Contains(err.Error(), tt.ExpectedError) {
					t.Fatalf("expected error to contain %q, but got %q", tt.ExpectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, but got %v", err)
				}
			}
		})
	}
}

func TestEval_CompositeLiteral_RecursiveVar(t *testing.T) {
	// This test case uses invalid Go code (`var V = T{F: &V}`).
	// The Go compiler would reject this. The goal of this test is to ensure
	// that the symbolic evaluator is robust enough to handle such a case
	// without panicking, by correctly detecting the evaluation cycle.
	files := map[string]string{
		"go.mod": "module example.com/m",
		"main.go": `
package main

type T struct {
	F *T
}

var V = T{F: &V}

func main() {
	_ = V
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// We expect pkgs to be non-empty, but possibly with errors.
		if len(pkgs) == 0 {
			t.Log("packages.Load returned no packages, which is unexpected but safe.")
			return nil
		}

		mainPkg := pkgs[0]
		// The scanner might not populate a pkg if loading fails, but we proceed
		// as the goal is to test the evaluator's robustness.
		if mainPkg == nil {
			t.Log("Main package was nil, which can happen with invalid code.")
			return nil
		}

		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return fmt.Errorf("NewInterpreter failed: %w", err)
		}

		// The core of the test: evaluating the file should not panic.
		// The cycle detection in `evalCompositeLit` should prevent infinite recursion.
		// We wrap this in a recover to be explicit about the test's intent.
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("The evaluator panicked! recover: %v", r)
			}
		}()

		for _, file := range mainPkg.AstFiles {
			// interp.Eval is the public API
			if _, err := interp.Eval(ctx, file, mainPkg); err != nil {
				// We might get an "identifier not found" error here, which is fine.
				// The key is that it shouldn't be an infinite recursion panic.
				t.Logf("Interpreter returned an expected error: %v", err)
			}
		}
		return nil
	}

	// We don't check the error from scantest.Run because we expect it to fail
	// at the `packages.Load` level. The real test is the `action` function above.
	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New failed: %v", err)
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithScanner(s)); err != nil {
		// The scantest might fail if packages.Load fails. This is okay.
		// The main check is the `recover` in the action.
		t.Logf("scantest.Run returned an error as expected: %v", err)
	}
}
