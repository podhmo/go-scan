package evaluator_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestMapIndexAssignment(t *testing.T) {
	var getValueCalled bool

	// Create a temporary directory with the files.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module myapp\ngo 1.22",
		"main.go": `
package main

func getValue() string {
	return "world"
}

func main() {
	m := make(map[string]string)
	m["hello"] = getValue()
}`,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		interp.RegisterIntrinsic("myapp.getValue", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			getValueCalled = true
			return &object.String{Value: "world"}
		})

		mainFile, err := lookupFile(pkg, "main.go")
		if err != nil {
			return err
		}

		if _, err := interp.Eval(ctx, mainFile, pkg); err != nil {
			return err
		}

		mainFn, ok := interp.FindObject("main")
		if !ok {
			t.Fatal("main func not found")
		}

		// Run main(). This should now succeed.
		_, err = interp.Apply(ctx, mainFn, nil, pkg)
		if err != nil {
			return fmt.Errorf("Apply failed unexpectedly: %w", err)
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}

	if !getValueCalled {
		t.Errorf("intrinsic for getValue was not called")
	}
}

func TestSliceIndexAssignment(t *testing.T) {
	var getValueCalled bool

	// Create a temporary directory with the files.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module myapp\ngo 1.22",
		"main.go": `
package main

func getValue() string {
	return "world"
}

func main() {
	s := make([]string, 1)
	s[0] = getValue()
}`,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		interp.RegisterIntrinsic("myapp.getValue", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			getValueCalled = true
			return &object.String{Value: "world"}
		})

		mainFile, err := lookupFile(pkg, "main.go")
		if err != nil {
			return err
		}

		if _, err := interp.Eval(ctx, mainFile, pkg); err != nil {
			return err
		}

		mainFn, ok := interp.FindObject("main")
		if !ok {
			t.Fatal("main func not found")
		}

		// Run main(). This should now succeed.
		_, err = interp.Apply(ctx, mainFn, nil, pkg)
		if err != nil {
			return fmt.Errorf("Apply failed unexpectedly: %w", err)
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}

	if !getValueCalled {
		t.Errorf("intrinsic for getValue was not called")
	}
}
