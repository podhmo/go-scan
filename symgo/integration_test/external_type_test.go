package integration_test

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestExternalTypePointerDereference(t *testing.T) {
	source := `
package main

import "log/slog"

func main() {
	// 1. new() is called with an external type.
	//    - evalSelectorExpr for "slog.Logger" must return an object.Type.
	//    - BuiltinNew must receive this object.Type and return a correctly typed object.Pointer.
	l := new(slog.Logger)

	// 2. The pointer is dereferenced.
	//    - evalStarExpr must handle the object.Pointer returned by new()
	//      without causing an "invalid indirect" error.
	_ = *l
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/hello\ngo 1.21\n",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		pkg := pkgs[0]

		// Eval the file to populate the environment
		if _, err := interp.Eval(ctx, pkg.AstFiles[pkg.Files[0]], pkg); err != nil {
			t.Fatalf("Eval of file failed: %v", err)
		}

		// Find the main function and apply it
		mainFunc, ok := interp.FindObject("main")
		if !ok {
			t.Fatal("main function not found")
		}

		_, err = interp.Apply(ctx, mainFunc, nil, pkg)
		if err != nil {
			t.Errorf("Apply(main) returned an unexpected error: %v", err)
		}
		return nil
	}

	_, err := scantest.Run(t, context.Background(), dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}
}
