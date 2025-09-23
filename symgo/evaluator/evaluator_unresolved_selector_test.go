package evaluator_test

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_SelectorOnUnresolvedType_Direct(t *testing.T) {
	source := `
package main

import "example.com/unscannable"

func main() {
	// Here, 'unscannable.Foo' is a selector on an out-of-policy package.
	// The evaluator resolves this to an UnresolvedType object.
	// Then we try to access a field '.Bar' on it. This should be handled gracefully.
	_ = unscannable.Foo.Bar
}
`
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		eval := evaluator.New(s, nil, nil, func(path string) bool {
			return path != "example.com/unscannable"
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

		// After the fix, this should pass without error.
		result := eval.Apply(ctx, mainFunc, nil, mainPkg, pkgObj.Env)

		if err, ok := result.(*object.Error); ok {
			t.Fatalf("evaluation failed unexpectedly: %v", err)
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
