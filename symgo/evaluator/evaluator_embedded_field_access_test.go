package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_EmbeddedFieldAccess(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m",
		"base/base.go": `
package base

import "io"

type Response struct {
	Body io.ReadCloser
	StatusCode int
}
`,
		"main.go": `
package main

import (
	"example.com/m/base"
	"io"
)

type TestResponse struct {
	base.Response
}

func GetBody(resp *TestResponse) io.Reader {
	return resp.Body
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		if mainPkg.Name != "main" {
			for _, p := range pkgs {
				t.Logf("found package: %s", p.Name)
			}
			return fmt.Errorf("expected main package, but got %s", mainPkg.Name)
		}

		eval := New(s, s.Logger, nil, nil)

		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/m")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/m'")
		}
		getBody, ok := pkgEnv.Get("GetBody")
		if !ok {
			return fmt.Errorf("GetBody function not found")
		}
		getBodyFunc, ok := getBody.(*object.Function)
		if !ok {
			return fmt.Errorf("GetBody is not a function, got %T", getBody)
		}

		// Create a symbolic TestResponse to pass to the function
		// We don't need a concrete value, just a symbolic one.
		// The evaluator should be able to resolve `resp.Body` symbolically.
		result := eval.Apply(ctx, getBodyFunc, nil, mainPkg, pkgEnv)

		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %w", err)
		}

		ret, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected return value from Apply, but got %T", result)
		}

		if err, ok := ret.Value.(*object.Error); ok {
			return fmt.Errorf("evaluation returned an error: %w", err)
		}

		// The result should be a symbolic placeholder for `io.Reader`
		// because we can't know the concrete value.
		// The important thing is that it doesn't return an error.
		if _, ok := ret.Value.(*object.SymbolicPlaceholder); !ok {
			t.Logf("return value was: %#v", ret.Value)
			return fmt.Errorf("expected return value to be a SymbolicPlaceholder, but got %T", ret.Value)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
