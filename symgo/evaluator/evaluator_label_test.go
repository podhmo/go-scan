package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"

	goscan "github.com/podhmo/go-scan"
)

func TestEvaluator_LabeledStatement(t *testing.T) {
	source := `
package main

func trace(s string) {}

func main() {
OuterLoop:
	for i := 0; i < 2; i++ {
		trace("outer_loop")
		for j := 0; j < 2; j++ {
			trace("inner_loop")
			if i == 0 && j == 1 {
				break OuterLoop
			}
		}
		trace("after_inner_loop")
	}
	trace("after_outer_loop")
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var calls []string

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) == 0 {
			return fmt.Errorf("no packages were scanned")
		}
		pkg := pkgs[0]

		tracer := object.TracerFunc(func(ev object.TraceEvent) {
			if call, ok := ev.Node.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "trace" {
					if len(call.Args) > 0 {
						if lit, ok := call.Args[0].(*ast.BasicLit); ok {
							// Unquote the string literal
							val := strings.Trim(lit.Value, `"`)
							calls = append(calls, val)
						}
					}
				}
			}
		})

		eval := New(s, s.Logger, tracer, nil)

		for _, f := range pkg.AstFiles {
			eval.Eval(ctx, f, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in package environment")
		}

		result := eval.Apply(ctx, mainFuncObj, nil, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("eval failed: %s", err.Error())
		}

		expectedCalls := []string{
			"outer_loop",
			"inner_loop",
			"after_outer_loop",
		}

		if want, got := strings.Join(expectedCalls, ","), strings.Join(calls, ","); want != got {
			return fmt.Errorf("mismatched calls:\nwant: %s\ngot : %s", want, got)
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
