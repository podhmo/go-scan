package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"math/cmplx"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalComplex(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   object.Object
	}{
		{
			name:   "imaginary literal",
			source: `func main() { 5i }`,
			want:   &object.Complex{Value: 5i},
		},
		{
			name:   "imaginary literal with float",
			source: `func main() { 2.5i }`,
			want:   &object.Complex{Value: 2.5i},
		},
		{
			name:   "integer + imaginary",
			source: `func main() { 10 + 5i }`,
			want:   &object.Complex{Value: 10 + 5i},
		},
		{
			name:   "float + imaginary",
			source: `func main() { 10.5 + 5i }`,
			want:   &object.Complex{Value: 10.5 + 5i},
		},
		{
			name:   "imaginary + imaginary",
			source: `func main() { 10i + 5i }`,
			want:   &object.Complex{Value: 15i},
		},
		{
			name:   "complex + complex",
			source: `func main() { (2 + 3i) + (4 + 5i) }`,
			want:   &object.Complex{Value: 6 + 8i},
		},
		{
			name:   "complex - complex",
			source: `func main() { (4 + 5i) - (2 + 3i) }`,
			want:   &object.Complex{Value: 2 + 2i},
		},
		{
			name:   "complex * complex",
			source: `func main() { (2 + 3i) * (4 + 5i) }`,
			// (2*4 - 3*5) + (2*5 + 3*4)i = (8 - 15) + (10 + 12)i = -7 + 22i
			want: &object.Complex{Value: -7 + 22i},
		},
		{
			name:   "complex / complex",
			source: `func main() { (2 + 3i) / (4 + 5i) }`,
			// ((2*4 + 3*5) + (3*4 - 2*5)i) / (4*4 + 5*5) = (8+15 + (12-10)i) / (16+25) = (23 + 2i) / 41
			want: &object.Complex{Value: (23 + 2i) / 41},
		},
		{
			name:   "integer * imaginary",
			source: `func main() { 2 * 3i }`,
			want:   &object.Complex{Value: 6i},
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
				env := object.NewEnclosedEnvironment(eval.UniverseEnv)
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

				ret, ok := result.(*object.ReturnValue)
				if !ok {
					return fmt.Errorf("expected return value, got %T", result)
				}

				wantComplex, ok := tt.want.(*object.Complex)
				if !ok {
					t.Fatalf("expected want to be *object.Complex, got %T", tt.want)
				}
				gotComplex, ok := ret.Value.(*object.Complex)
				if !ok {
					t.Fatalf("expected return value to be *object.Complex, got %T", ret.Value)
				}

				complexComparer := cmp.Comparer(func(x, y complex128) bool {
					return cmplx.Abs(x-y) < 1e-9
				})
				if diff := cmp.Diff(wantComplex.Value, gotComplex.Value, complexComparer); diff != "" {
					t.Errorf("complex value mismatch (-want +got):\n%s", diff)
				}
				return nil
			}
			if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}
		})
	}
}
