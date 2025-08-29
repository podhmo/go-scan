package symgo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestStackTrace(t *testing.T) {
	t.Run("error in symbolic execution", func(t *testing.T) {
		// Calling a non-function value will cause an error.
		script := `
package main

func errorFunc() {
	var x = 1
	x()
}

func caller() {
	errorFunc()
}

func main() {
	caller()
}
`
		dir, cleanup := scantest.WriteFiles(t, map[string]string{
			"go.mod":  "module example.com/me",
			"main.go": script,
		})
		defer cleanup()

		var errMsg string
		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			eval := evaluator.New(s, nil, nil, nil)
			env := object.NewEnvironment()

			for _, f := range pkg.AstFiles {
				eval.Eval(ctx, f, env, pkg)
			}

			mainFn, ok := env.Get("main")
			if !ok {
				return fmt.Errorf("could not find main function")
			}

			result := eval.Apply(ctx, mainFn, nil, pkg)

			if retVal, ok := result.(*object.ReturnValue); ok {
				if errObj, ok := retVal.Value.(*object.Error); ok {
					errMsg = errObj.Inspect()
				}
			} else if errObj, ok := result.(*object.Error); ok {
				errMsg = errObj.Inspect()
			}

			return nil
		}

		if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}

		if errMsg == "" {
			t.Fatal("Expected an error, but got nil")
		}

		t.Logf("Full error message:\n---\n%s\n---", errMsg)

		expectedToContain := []string{
			"symgo runtime error: not a function: INTEGER",
			"x()",
			"in errorFunc",
			"in caller",
		}

		for _, expected := range expectedToContain {
			if !strings.Contains(errMsg, expected) {
				t.Errorf("error message should contain %q, but it was:\n---\n%s\n---", expected, errMsg)
			}
		}
	})
}
