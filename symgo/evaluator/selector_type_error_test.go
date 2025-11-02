package evaluator

import (
	"context"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestSelectorTypeError(t *testing.T) {
	tests := []struct {
		name                   string
		input                  string
		expectedReasonContains string
	}{
		{
			name: "selector on slice",
			input: `
package main
func main() {
	s := []int{1, 2, 3}
	_ = s.foo
}`,
			expectedReasonContains: "invalid selector on SLICE value",
		},
		{
			name: "selector on integer",
			input: `
package main
func main() {
	i := 10
	_ = i.foo
}`,
			expectedReasonContains: "invalid selector on INTEGER value",
		},
		{
			name: "selector on string",
			input: `
package main
func main() {
	s := "hello"
	_ = s.foo
}`,
			expectedReasonContains: "invalid selector on STRING value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				mainPkg := pkgs[0]
				eval := New(s, s.Logger, nil, nil)
				env := object.NewEnclosedEnvironment(eval.UniverseEnv)

				// 1. Evaluate the file to populate the environment.
				eval.Eval(ctx, mainPkg.AstFiles[mainPkg.Files[0]], env, mainPkg)

				// 2. Look up the main function.
				pkgEnv, ok := eval.PackageEnvForTest(mainPkg.ImportPath)
				if !ok {
					t.Fatalf("package env not found for %q", mainPkg.ImportPath)
				}
				mainFunc, ok := pkgEnv.Get("main")
				if !ok {
					t.Fatalf("function 'main' not found")
				}

				// 3. Apply the main function.
				result := eval.Apply(ctx, mainFunc, []object.Object{}, mainPkg)

				// 4. Check that the result is a ReturnValue wrapping a SymbolicPlaceholder.
				retVal, ok := result.(*object.ReturnValue)
				if !ok {
					t.Fatalf("expected result to be *object.ReturnValue, but got %T (%+v)", result, result)
				}
				placeholder, ok := retVal.Value.(*object.SymbolicPlaceholder)
				if !ok {
					t.Fatalf("expected return value to be *object.SymbolicPlaceholder, but got %T (%+v)", retVal.Value, retVal.Value)
				}
				if !strings.Contains(placeholder.Reason, tt.expectedReasonContains) {
					t.Errorf("expected placeholder reason to contain %q, but got %q", tt.expectedReasonContains, placeholder.Reason)
				}

				return nil
			}

			dir, cleanup := scantest.WriteFiles(t, map[string]string{
				"go.mod":  "module example.com/me",
				"main.go": tt.input,
			})
			defer cleanup()

			if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
				t.Fatalf("scantest.Run() failed unexpectedly: %v", err)
			}
		})
	}
}
