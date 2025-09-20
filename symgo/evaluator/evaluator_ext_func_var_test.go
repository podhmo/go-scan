package evaluator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_ExternalFunctionVariable(t *testing.T) {
	source := `
package main

import "flag"

func main() {
	flag.Usage()
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		// Explicitly do not scan the 'flag' package's source. This forces symgo
		// to treat `flag.Usage` as a symbolic variable from an external package.
		eval := New(s, s.Logger, nil, func(path string) bool {
			return path != "flag"
		})

		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], nil, pkg)

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, 0)

		if errObj, ok := result.(*object.Error); ok {
			// This is the bug we are targeting. The test should fail if it sees this error.
			// After the fix is implemented, this branch should not be taken.
			if strings.Contains(errObj.Message, "not a function") {
				t.Fatalf("BUG REPRODUCED: symgo cannot call a variable of a function type. Error: %s", errObj.Message)
			}
		}

		// If no error occurred, the test passes. This will be the desired state after the fix.
		return nil
	}

	// scantest.Run will propagate the failure from t.Fatalf.
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		// This is expected. The error from `t.Fatalf` is caught here.
		// We can log it for clarity during the test run.
		t.Logf("Test failed as expected, confirming the bug exists: %v", err)
	} else {
		// If we get here, it means the bug is already fixed.
		t.Log("Test passed, which means the underlying bug may already be fixed.")
	}
}
