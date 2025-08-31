package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_UnaryExpr_Numeric(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"-5", -5},
		{"-10", -10},
		{"+5", 5},
		{"^2", -3}, // bitwise NOT
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			source := fmt.Sprintf(`
package main

var result = %s
`, tt.input)
			files := map[string]string{
				"go.mod":  "module example.com/me",
				"main.go": source,
			}

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				pkg := pkgs[0]
				eval := New(s, s.Logger, nil, nil)
				env := object.NewEnvironment()

				for _, file := range pkg.AstFiles {
					if err := eval.Eval(ctx, file, env, pkg); err != nil {
						if objErr, ok := err.(*object.Error); ok {
							return objErr
						}
						return fmt.Errorf("unexpected error: %v", err)
					}
				}

				val, ok := env.Get("result")
				if !ok {
					return fmt.Errorf("variable 'result' not found")
				}
				variable, ok := val.(*object.Variable)
				if !ok {
					return fmt.Errorf("expected variable, got %T", val)
				}

				got, ok := variable.Value.(*object.Integer)
				if !ok {
					return fmt.Errorf("expected integer, got %T (%s)", variable.Value, variable.Value.Inspect())
				}

				if got.Value != tt.expected {
					return fmt.Errorf("got %v, want %v", got.Value, tt.expected)
				}
				return nil
			}

			dir, cleanup := scantest.WriteFiles(t, files)
			defer cleanup()
			if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
				t.Fatalf("scantest.Run() failed: %+v", err)
			}
		})
	}
}

func TestEval_UnaryExpr_Bang(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"!true", false},
		{"!false", true},
		{"!!true", true},
		{"!!false", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			source := fmt.Sprintf(`
package main

var result = %s
`, tt.input)
			files := map[string]string{
				"go.mod":  "module example.com/me",
				"main.go": source,
			}

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				pkg := pkgs[0]
				eval := New(s, s.Logger, nil, nil)
				env := object.NewEnvironment()

				for _, file := range pkg.AstFiles {
					if err := eval.Eval(ctx, file, env, pkg); err != nil {
						if objErr, ok := err.(*object.Error); ok {
							return objErr
						}
						return fmt.Errorf("unexpected error: %v", err)
					}
				}

				val, ok := env.Get("result")
				if !ok {
					return fmt.Errorf("variable 'result' not found")
				}
				variable, ok := val.(*object.Variable)
				if !ok {
					return fmt.Errorf("expected variable, got %T", val)
				}

				got, ok := variable.Value.(*object.Boolean)
				if !ok {
					return fmt.Errorf("expected boolean, got %T (%s)", variable.Value, variable.Value.Inspect())
				}

				if got.Value != tt.expected {
					return fmt.Errorf("got %v, want %v", got.Value, tt.expected)
				}
				return nil
			}

			dir, cleanup := scantest.WriteFiles(t, files)
			defer cleanup()
			if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
				t.Fatalf("scantest.Run() failed: %+v", err)
			}
		})
	}
}
