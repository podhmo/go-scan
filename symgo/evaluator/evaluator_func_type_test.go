package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"

	goscan "github.com/podhmo/go-scan"
)

func TestEval_FuncType(t *testing.T) {
	source := `
package main

var myFunc func(int) string
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		// Find the 'myFunc' declaration
		var varDecl *ast.GenDecl
		for _, file := range pkg.AstFiles {
			for _, decl := range file.Decls {
				if gd, ok := decl.(*ast.GenDecl); ok {
					if vs, ok := gd.Specs[0].(*ast.ValueSpec); ok {
						if vs.Names[0].Name == "myFunc" {
							varDecl = gd
							break
						}
					}
				}
			}
			if varDecl != nil {
				break
			}
		}

		if varDecl == nil {
			return fmt.Errorf("variable declaration for 'myFunc' not found")
		}

		// The type of the declaration is what we want to test
		funcTypeExpr := varDecl.Specs[0].(*ast.ValueSpec).Type

		// Evaluate the FuncType expression
		result := eval.Eval(ctx, funcTypeExpr, env, pkg)

		// Check if the result is a symbolic placeholder, not an error
		if _, ok := result.(*object.SymbolicPlaceholder); !ok {
			if err, isErr := result.(*object.Error); isErr {
				return fmt.Errorf("evaluation of FuncType resulted in an error: %s", err.Message)
			}
			return fmt.Errorf("expected evaluation of FuncType to be a SymbolicPlaceholder, but got %T", result)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
