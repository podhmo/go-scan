package evaluator_test

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_FieldAccessOnInstance(t *testing.T) {
	source := `
package main

// MyStruct has a field, not a method, named 'Value'.
type MyStruct struct {
	Value string
}

func process(s *MyStruct) string {
	// The evaluator needs to handle the field access s.Value correctly.
	// Before the fix, it would only look for a method 'Value' and fail.
	return s.Value
}

func main() {
	instance := &MyStruct{Value: "hello"}
	process(instance)
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
		t.Fatalf("scantest.Run() failed with unexpected error: %+v", err)
	}
}
