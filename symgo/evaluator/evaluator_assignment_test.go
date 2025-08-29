package evaluator_test

import (
	"context"
	"fmt"
	"go/ast"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// lookupFile is a test helper to find a file by name in a scanned package.
func lookupFile(pkg *goscan.Package, name string) (*ast.File, error) {
	for path, f := range pkg.AstFiles {
		if strings.HasSuffix(path, name) {
			return f, nil
		}
	}
	return nil, fmt.Errorf("file %q not found in package %s", name, pkg.Name)
}

func TestMultiValueAssignment(t *testing.T) {
	var intrinsicCalled bool
	var assignedValue symgo.Object

	// Create a temporary directory with the files.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module myapp\ngo 1.22",
		"main.go": `
package main
func myFunc() (string, error) {
	return "", nil
}
var x string
func main() {
	x, _ = myFunc()
}`,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		// Intrinsic for a function that returns two values
		interp.RegisterIntrinsic("myapp.myFunc", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			intrinsicCalled = true
			return &object.MultiReturn{
				Values: []symgo.Object{
					&object.String{Value: "hello"},
					object.NIL,
				},
			}
		})

		mainFile, err := lookupFile(pkg, "main.go")
		if err != nil {
			return err
		}

		// Eval the file to populate global 'x' and function definitions
		_, err = interp.Eval(ctx, mainFile, pkg)
		if err != nil {
			return err
		}

		mainFn, ok := interp.FindObject("main")
		if !ok {
			t.Fatal("main func not found")
		}

		// Run main() which contains the assignment
		_, err = interp.Apply(ctx, mainFn, nil, pkg)
		if err != nil {
			return err
		}

		// After running main, check the value of the assigned variable 'x'
		xVar, ok := interp.FindObject("x")
		if !ok {
			t.Error("variable 'x' was not found in the environment")
		} else {
			// The object found is the *Variable* itself. We need to inspect its value.
			if v, ok := xVar.(*object.Variable); ok {
				assignedValue = v.Value
			} else {
				t.Errorf("x is not a variable, but %T", xVar)
			}
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}

	if !intrinsicCalled {
		t.Errorf("intrinsic for myFunc was not called")
	}
	if str, ok := assignedValue.(*object.String); !ok {
		t.Errorf("assigned value 'x' is not a string, got %T", assignedValue)
	} else if str.Value != "hello" {
		t.Errorf("assigned value is wrong, want 'hello', got %q", str.Value)
	}
}

func TestAssignmentToFieldOfTypeAssertion(t *testing.T) {
	// This test verifies that a complex assignment like `v.(*S).X = 10` is
	// correctly evaluated. The fix involves ensuring that the LHS of the
	// assignment is evaluated, tracing any calls within it.
	var checkCalled bool

	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module myapp\ngo 1.22",
		"main.go": `
package main

type S struct { X int }

func check(v any) {}

func main() {
	var v any = &S{X: 0}
	v.(*S).X = 10
	check(v)
}`,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}

		// This intrinsic will be called at the end of main. We can inspect the
		// state of 'v' when it's called.
		interp.RegisterIntrinsic("myapp.check", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			checkCalled = true
			return nil
		})

		mainFile, err := lookupFile(pkg, "main.go")
		if err != nil {
			return err
		}

		if _, err := interp.Eval(ctx, mainFile, pkg); err != nil {
			return fmt.Errorf("initial eval failed: %w", err)
		}

		mainFn, ok := interp.FindObject("main")
		if !ok {
			return fmt.Errorf("main func not found")
		}

		// Run main(). With the fix, this should no longer error out.
		_, err = interp.Apply(ctx, mainFn, nil, pkg)
		if err != nil {
			return fmt.Errorf("Apply failed unexpectedly: %w", err)
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}

	if !checkCalled {
		t.Fatalf("the check() function was not called, indicating evaluation did not complete")
	}
}
