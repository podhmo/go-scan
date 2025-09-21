package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_StructTypeExpression(t *testing.T) {
	source := `
package main

func main() {
	type T struct{ F int }
	var v T
	_ = (struct{ F int })(v)
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		gopkg := pkgs[0]
		pkgEnv := object.NewEnclosedEnvironment(nil)
		evalpkg := &object.Package{
			Name:        gopkg.Name,
			Env:         pkgEnv,
			ScannedInfo: gopkg,
		}
		eval := New(s, s.Logger, nil, nil, WithPackages(map[string]*object.Package{
			gopkg.ImportPath: evalpkg,
		}))

		// Evaluate the file to populate functions etc.
		eval.Eval(ctx, gopkg.AstFiles[gopkg.Files[0]], pkgEnv, gopkg)

		// Get the main function from the package environment
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found in package %s", gopkg.ImportPath)
		}

		// Execute main. We expect this to run without "not implemented" errors.
		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, gopkg, pkgEnv, token.NoPos)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %v", err)
		}
		if ret, ok := result.(*object.ReturnValue); ok {
			if err, ok := ret.Value.(*object.Error); ok {
				return fmt.Errorf("evaluation returned an error: %v", err)
			}
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
