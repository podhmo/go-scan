package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalBinaryExpr_StringConcatenation(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "literals",
			code:     `func run() string { return "hello" + " " + "world" }`,
			expected: "hello world",
		},
		{
			name: "variables",
			code: `func run() string {
				x := "hello"
				y := "world"
				return x + " " + y
			}`,
			expected: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"go.mod":  "module example.com/me",
				"main.go": fmt.Sprintf("package main\n%s", tt.code),
			}

			dir, cleanup := scantest.WriteFiles(t, files)
			defer cleanup()

			var finalResult object.Object
			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				pkg := pkgs[0]
				internalScanner, err := s.ScannerForSymgo()
				if err != nil {
					return err
				}
				eval := New(internalScanner, s.Logger)
				env := object.NewEnvironment()

				for _, file := range pkg.AstFiles {
					eval.Eval(file, env, pkg)
				}

				runFuncObj, ok := env.Get("run")
				if !ok {
					return fmt.Errorf("run function not found")
				}
				runFunc := runFuncObj.(*object.Function)

				finalResult = eval.applyFunction(runFunc, []object.Object{}, pkg)
				return nil
			}

			if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}

			if finalResult == nil {
				t.Fatal("finalResult was not set")
			}

			result, ok := finalResult.(*object.String)
			if !ok {
				t.Fatalf("expected result to be *object.String, but got %T (%s)", finalResult, finalResult.Inspect())
			}

			if result.Value != tt.expected {
				t.Errorf("string concatenation result is wrong, want %q, got %q", tt.expected, result.Value)
			}
		})
	}
}
