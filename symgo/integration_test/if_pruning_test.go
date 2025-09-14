package integration_test

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"
	"os"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestIfPruning_ShouldNotPanicOnDeadBranch(t *testing.T) {
	source := `
package main

func AlwaysNilError() error {
	return nil
}

func MyFunction() {
	err := AlwaysNilError()
	if err != nil {
		panic(err)
	}
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module a.b/c",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, got %d", len(pkgs))
		}
		mainPkg := pkgs[0]

		logLevel := new(slog.LevelVar)
		logLevel.Set(slog.LevelDebug)
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(logger))
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		// Load the package
		for _, fileAst := range mainPkg.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, mainPkg); err != nil {
				return fmt.Errorf("initial load failed: %w", err)
			}
		}

		// Find the function to analyze
		pkgEnv, ok := interp.PackageEnvForTest(mainPkg.ImportPath)
		if !ok {
			return fmt.Errorf("could not find package env for %s", mainPkg.ImportPath)
		}
		fnObj, ok := pkgEnv.Get("MyFunction")
		if !ok {
			return fmt.Errorf("could not find MyFunction in package env")
		}
		fn, ok := fnObj.(*object.Function)
		if !ok {
			return fmt.Errorf("MyFunction is not an object.Function, but %T", fnObj)
		}

		// Analyze the function
		dummyCall := &ast.CallExpr{
			Fun: fn.Name, // Use the function's own name identifier
		}
		result := interp.ApplyFunction(ctx, dummyCall, fn, nil, nil)

		if err, isErr := result.(*object.Error); isErr {
			return fmt.Errorf("Analysis of MyFunction failed unexpectedly: %s", err.Inspect())
		}
		return nil
	}

	_, err := scantest.Run(t, context.Background(), dir, []string{"./..."}, action)
	if err != nil {
		t.Fatal(err)
	}
}
