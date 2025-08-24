package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalBinaryExpr(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		mainFunc string
		want     object.Object
	}{
		{
			name:   "string concatenation",
			source: `func main() { "hello" + " " + "world" }`,
			want:   &object.String{Value: "hello world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := fmt.Sprintf("package main\n%s", tt.source)
			files := map[string]string{
				"go.mod":  "module example.com/me",
				"main.go": source,
			}
			dir, cleanup := scantest.WriteFiles(t, files)
			defer cleanup()

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				pkg := pkgs[0]
				eval := New(s, s.Logger, nil, nil)
				env := object.NewEnvironment()
				for _, file := range pkg.AstFiles {
					eval.Eval(ctx, file, env, pkg)
				}

				mainFuncObj, ok := env.Get("main")
				if !ok {
					return fmt.Errorf("main function not found")
				}
				mainFunc := mainFuncObj.(*object.Function)

				result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
				if isError(result) {
					return fmt.Errorf("evaluation failed: %s", result.Inspect())
				}

				// The result of a function call is the result of its last expression.
				// Our function body is a single expression statement, so we expect a ReturnValue.
				ret, ok := result.(*object.ReturnValue)
				if !ok {
					return fmt.Errorf("expected return value, got %T", result)
				}

				if diff := cmp.Diff(tt.want, ret.Value); diff != "" {
					t.Errorf("result mismatch (-want +got):\n%s", diff)
				}
				return nil
			}
			if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}
		})
	}
}
