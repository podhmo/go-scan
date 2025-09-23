package evaluator_test

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_NamedReturnValues(t *testing.T) {
	source := `
package main

// The key here is that 'items' is a named return value, and it's used
// in the call to 'append' before it's explicitly assigned to in that statement.
// Go pre-declares named return values as variables in the function scope.
// The symgo evaluator needs to do the same.
func getItems() (items []string, err error) {
	items = append(items, "one")
	return
}

func main() {
	getItems()
}
`
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		eval := evaluator.New(s, nil, nil, func(path string) bool {
			return true // Scan everything
		})

		mainPkg := pkgs[0]

		pkgObj, err := eval.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return err
		}

		mainFunc, ok := pkgObj.Env.Get("main")
		if !ok {
			t.Fatalf("main function not found")
		}

		// After the fix, this should pass without any errors.
		result := eval.Apply(ctx, mainFunc, nil, mainPkg, pkgObj.Env)
		if err, ok := result.(*object.Error); ok {
			t.Errorf("evaluation failed unexpectedly: %v", err)
		}

		return nil
	}

	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": source,
	})
	defer cleanup()

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
