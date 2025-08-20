package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

// lookupFunction is a test helper to find a function by name in a package.
func lookupFunction(pkg *scanner.PackageInfo, name string) *scanner.FunctionInfo {
	for _, f := range pkg.Functions {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func TestEval_SliceLiteral(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/slice-test\n",
		"main.go": `
package main

type User struct {
	ID   int
	Name string
}

func main() {
	_ = []User{}
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, but got %d", len(pkgs))
		}
		pkg := pkgs[0]

		// 2. Get the AST for the composite literal expression.
		mainFunc := lookupFunction(pkg, "main")
		if mainFunc == nil || mainFunc.AstDecl.Body == nil || len(mainFunc.AstDecl.Body.List) == 0 {
			t.Fatal("main function with assignment not found in test code")
		}
		assignStmt, ok := mainFunc.AstDecl.Body.List[0].(*ast.AssignStmt)
		if !ok {
			t.Fatalf("expected first statement in main to be an assignment, got %T", mainFunc.AstDecl.Body.List[0])
		}
		compositeLit := assignStmt.Rhs[0]

		// 3. Setup evaluator.
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return fmt.Errorf("failed to get internal scanner: %w", err)
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		// 4. Evaluate the composite literal expression.
		obj := eval.Eval(compositeLit, env, pkg)

		// 5. Assert the result.
		slice, ok := obj.(*object.Slice)
		if !ok {
			t.Fatalf("Eval() returned wrong type. want=*object.Slice, got=%T (%+v)", obj, obj)
		}

		// Check the FieldType of the slice
		ft := slice.FieldType
		if ft == nil {
			t.Fatal("Slice.FieldType is nil")
		}
		if !ft.IsSlice {
			t.Error("FieldType.IsSlice is false, want true")
		}
		if ft.Elem == nil {
			t.Fatal("FieldType.Elem is nil")
		}
		if ft.Elem.TypeName != "User" {
			t.Errorf("Slice element type name is wrong. want=%q, got=%q", "User", ft.Elem.TypeName)
		}
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
