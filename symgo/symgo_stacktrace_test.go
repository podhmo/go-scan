package symgo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
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
			interp, err := symgo.NewInterpreter(s)
			if err != nil {
				return err
			}

			for _, f := range pkg.AstFiles {
				interp.Eval(ctx, f, pkg)
			}

			mainFn, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
			if !ok {
				return fmt.Errorf("could not find main function")
			}

			result, err := interp.Apply(ctx, mainFn, nil, pkg)
			if err != nil {
				errMsg = err.Error()
			} else if result != nil {
				if errObj, ok := result.(*object.Error); ok {
					errMsg = errObj.Inspect()
				}
			}

			return nil
		}

		if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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
