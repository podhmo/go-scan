package evaluator_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"fmt"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestServeError(t *testing.T) {
	t.Run("it should not recurse infinitely", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
			"go.mod": "module myapp",
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
