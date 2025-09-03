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
			ShouldFail:    true,
			ExpectedError: "infinite recursion detected",
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
			ShouldFail:    true,
			ExpectedError: "infinite recursion detected",
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
