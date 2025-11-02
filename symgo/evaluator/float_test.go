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

func TestEvalFloat(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   object.Object
	}{
		// Comparisons
		{"1.5 == 1.5", "func main() { 1.5 == 1.5 }", object.TRUE},
		{"1.5 != 1.5", "func main() { 1.5 != 1.5 }", object.FALSE},
		{"1.5 == 2.5", "func main() { 1.5 == 2.5 }", object.FALSE},
		{"1.5 != 2.5", "func main() { 1.5 != 2.5 }", object.TRUE},
		{"1.5 < 2.5", "func main() { 1.5 < 2.5 }", object.TRUE},
		{"1.5 <= 2.5", "func main() { 1.5 <= 2.5 }", object.TRUE},
		{"1.5 <= 1.5", "func main() { 1.5 <= 1.5 }", object.TRUE},
		{"2.5 > 1.5", "func main() { 2.5 > 1.5 }", object.TRUE},
		{"2.5 >= 1.5", "func main() { 2.5 >= 1.5 }", object.TRUE},
		{"1.5 >= 1.5", "func main() { 1.5 >= 1.5 }", object.TRUE},

		// Arithmetic
		{"1.5 + 2.0", "func main() { 1.5 + 2.0 }", &object.Float{Value: 3.5}},
		{"3.5 - 1.5", "func main() { 3.5 - 1.5 }", &object.Float{Value: 2.0}},
		{"2.0 * 2.5", "func main() { 2.0 * 2.5 }", &object.Float{Value: 5.0}},
		{"5.0 / 2.0", "func main() { 5.0 / 2.0 }", &object.Float{Value: 2.5}},
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
				for _, file := range pkg.AstFiles {
					eval.Eval(ctx, file, nil, pkg)
				}

				pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
				if !ok {
					return fmt.Errorf("could not get package env for 'example.com/me'")
				}
				mainFuncObj, ok := pkgEnv.Get("main")
				if !ok {
					return fmt.Errorf("main function not found")
				}
				mainFunc := mainFuncObj.(*object.Function)

				result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
				if isError(result) {
					return fmt.Errorf("evaluation failed: %s", result.Inspect())
				}

				ret, ok := result.(*object.ReturnValue)
				if !ok {
					return fmt.Errorf("expected return value, got %T", result)
				}

				if wantFloat, ok := tt.want.(*object.Float); ok {
					gotFloat, ok := ret.Value.(*object.Float)
					if !ok {
						t.Fatalf("expected return value to be *object.Float, got %T", ret.Value)
					}
					if diff := cmp.Diff(wantFloat.Value, gotFloat.Value); diff != "" {
						t.Errorf("float value mismatch (-want +got):\n%s", diff)
					}
				} else if wantBool, ok := tt.want.(*object.Boolean); ok {
					gotBool, ok := ret.Value.(*object.Boolean)
					if !ok {
						t.Fatalf("expected return value to be *object.Boolean, got %T", ret.Value)
					}
					if diff := cmp.Diff(wantBool.Value, gotBool.Value); diff != "" {
						t.Errorf("boolean value mismatch (-want +got):\n%s", diff)
					}
				} else {
					t.Fatalf("unexpected want type %T", tt.want)
				}

				return nil
			}
			if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}
		})
	}
}
