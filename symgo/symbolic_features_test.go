package symgo_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestSymbolic_IfElse(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func IfBlock() {}
func ElseBlock() {}

func main() {
	x := 1 // In symbolic execution, this value doesn't matter.
	if x > 0 {
		IfBlock()
	} else {
		ElseBlock()
	}
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var ifCalled, elseCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		interp.RegisterIntrinsic("example.com/me.IfBlock", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			ifCalled = true
			return nil
		})
		interp.RegisterIntrinsic("example.com/me.ElseBlock", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			elseCalled = true
			return nil
		})

		mainFunc, err := lookupFunc(pkg, "main")
		if err != nil {
			return err
		}

		_, evalErr := interp.EvalWithEnv(ctx, mainFunc.AstDecl.Body, env, pkg)
		if evalErr != nil {
			return fmt.Errorf("unexpected error during eval: %w", evalErr)
		}

		if !ifCalled {
			return fmt.Errorf("if block was not called")
		}
		if !elseCalled {
			return fmt.Errorf("else block was not called")
		}
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestSymbolic_For(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func ForBody() {}

func main() {
	// Symbolic execution should unroll this once.
	for i := 0; i < 10; i++ {
		ForBody()
	}
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var callCount int
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		interp.RegisterIntrinsic("example.com/me.ForBody", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			callCount++
			return nil
		})

		mainFunc, err := lookupFunc(pkg, "main")
		if err != nil {
			return err
		}

		_, evalErr := interp.EvalWithEnv(ctx, mainFunc.AstDecl.Body, env, pkg)
		if evalErr != nil {
			return fmt.Errorf("unexpected error during eval: %w", evalErr)
		}

		if callCount != 1 {
			return fmt.Errorf("for loop body should be called once in symbolic execution, but was called %d times", callCount)
		}
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestSymbolic_Switch(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func CaseA() {}
func CaseB() {}
func DefaultCase() {}

func main() {
	v := "a" // This value doesn't matter for symbolic execution.
	switch v {
	case "a":
		CaseA()
	case "b":
		CaseB()
	default:
		DefaultCase()
	}
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var aCalled, bCalled, defaultCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		interp.RegisterIntrinsic("example.com/me.CaseA", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			aCalled = true
			return nil
		})
		interp.RegisterIntrinsic("example.com/me.CaseB", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			bCalled = true
			return nil
		})
		interp.RegisterIntrinsic("example.com/me.DefaultCase", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			defaultCalled = true
			return nil
		})

		mainFunc, err := lookupFunc(pkg, "main")
		if err != nil {
			return err
		}

		_, evalErr := interp.EvalWithEnv(ctx, mainFunc.AstDecl.Body, env, pkg)
		if evalErr != nil {
			return fmt.Errorf("unexpected error during eval: %w", evalErr)
		}

		if !aCalled {
			return fmt.Errorf("case 'a' was not called")
		}
		if !bCalled {
			return fmt.Errorf("case 'b' was not called")
		}
		if !defaultCalled {
			return fmt.Errorf("default case was not called")
		}
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
