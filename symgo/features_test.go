package symgo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// lookupFunc is a test helper to find a function by name in a scanned package.
func lookupFunc(pkg *goscan.Package, name string) (*goscan.FunctionInfo, error) {
	for _, f := range pkg.Functions {
		if f.Name == name {
			return f, nil
		}
	}
	return nil, fmt.Errorf("function %q not found in package %s", name, pkg.Name)
}

func TestFeature_ErrorWithPosition(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func main() {
	x := undefined_variable
}`, // error is on line 4
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		// Evaluate all files to populate the environment
		for _, file := range pkg.AstFiles {
			_, err := interp.EvalWithEnv(ctx, file, env, pkg)
			if err != nil && !strings.Contains(err.Error(), "undefined_variable") {
				// We expect an error, but only the one we're testing for.
				return fmt.Errorf("initial eval of file %s failed unexpectedly: %w", file.Name.Name, err)
			}
		}

		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		mainFunc, ok := mainFuncObj.(*symgo.Function)
		if !ok {
			return fmt.Errorf("main is not a *symgo.Function, but %T", mainFuncObj)
		}

		// Apply the function to trigger the evaluation of its body
		_, evalErr := interp.Apply(ctx, mainFunc, []symgo.Object{}, pkg)
		if evalErr == nil {
			t.Fatal("expected an error, but got nil")
		}

		// Check if the error message contains the correct position and message.
		// Note: The exact column can vary, so we check for the file and line.
		expectedPosition := "main.go:4:"
		expectedMessage := "identifier not found: undefined_variable"

		if !strings.Contains(evalErr.Error(), expectedPosition) {
			return fmt.Errorf("error message does not contain expected position\nwant_substr: %q\ngot:         %q", expectedPosition, evalErr.Error())
		}
		if !strings.Contains(evalErr.Error(), expectedMessage) {
			return fmt.Errorf("error message does not contain expected message\nwant_substr: %q\ngot:         %q", expectedMessage, evalErr.Error())
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestBuiltin_Panic(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func main() {
	panic("test message")
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		for _, file := range pkg.AstFiles {
			_, err := interp.EvalWithEnv(ctx, file, env, pkg)
			if err != nil {
				return err
			}
		}

		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}

		_, evalErr := interp.Apply(ctx, mainFuncObj, []symgo.Object{}, pkg)
		if evalErr == nil {
			return fmt.Errorf("expected a panic error, but got nil")
		}

		expectedMsg := "panic: test message"
		if !strings.Contains(evalErr.Error(), expectedMsg) {
			return fmt.Errorf("error message mismatch\nwant_substr: %q\ngot:         %q", expectedMsg, evalErr.Error())
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestMultiValueAssignment(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func twoReturns() (int, string) {
	return 42, "hello"
}

func main() {
	x, y := twoReturns()
	_ = x
	_ = y
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		for _, file := range pkg.AstFiles {
			_, err := interp.EvalWithEnv(ctx, file, env, pkg)
			if err != nil {
				return err
			}
		}

		// First, test that calling the function directly returns a MultiReturn
		twoReturnsFn, ok := env.Get("twoReturns")
		if !ok {
			return fmt.Errorf("twoReturns function not found")
		}

		result, applyErr := interp.Apply(ctx, twoReturnsFn, []symgo.Object{}, pkg)
		if applyErr != nil {
			return fmt.Errorf("Apply(twoReturns) failed: %w", applyErr)
		}

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected ReturnValue, got %T", result)
		}
		multiRet, ok := retVal.Value.(*object.MultiReturn)
		if !ok {
			return fmt.Errorf("expected inner value to be MultiReturn, got %T", retVal.Value)
		}
		if len(multiRet.Values) != 2 {
			return fmt.Errorf("expected 2 return values, got %d", len(multiRet.Values))
		}

		// Now, test the assignment by running main
		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}

		// Since variables are created in a nested environment, we can't easily access them.
		// The main test here is that the Apply call doesn't return an "assignment mismatch" error.
		_, mainApplyErr := interp.Apply(ctx, mainFuncObj, []symgo.Object{}, pkg)
		if mainApplyErr != nil {
			return fmt.Errorf("Apply(main) failed unexpectedly: %w", mainApplyErr)
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestFeature_IfElseEvaluation(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func MyPattern() {}

func main() {
	x := 1
	if x > 0 {
		// do nothing
	} else {
		MyPattern()
	}
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var patternCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		interp.RegisterIntrinsic("example.com/me.MyPattern", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			patternCalled = true
			return nil
		})

		for _, file := range pkg.AstFiles {
			_, err := interp.EvalWithEnv(ctx, file, env, pkg)
			if err != nil {
				return fmt.Errorf("initial eval of file %s failed: %w", file.Name.Name, err)
			}
		}

		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		mainFunc, ok := mainFuncObj.(*symgo.Function)
		if !ok {
			return fmt.Errorf("main is not a *symgo.Function, but %T", mainFuncObj)
		}

		_, evalErr := interp.Apply(ctx, mainFunc, []symgo.Object{}, pkg)
		if evalErr != nil {
			return fmt.Errorf("unexpected error during apply: %w", evalErr)
		}

		if !patternCalled {
			return fmt.Errorf("pattern in else block was not called")
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestFeature_SprintfIntrinsic(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main
import "fmt"
func run() string {
	name := "world"
	return fmt.Sprintf("hello %s %d", name, 42)
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}
		env := symgo.NewEnclosedEnvironment(nil)

		for _, file := range pkg.AstFiles {
			_, err := interp.EvalWithEnv(ctx, file, env, pkg)
			if err != nil {
				return fmt.Errorf("initial eval of file %s failed: %w", file.Name.Name, err)
			}
		}

		runFuncObj, ok := env.Get("run")
		if !ok {
			return fmt.Errorf("run function not found")
		}
		runFunc, ok := runFuncObj.(*symgo.Function)
		if !ok {
			return fmt.Errorf("run is not a *symgo.Function, but %T", runFuncObj)
		}

		result, evalErr := interp.Apply(ctx, runFunc, []symgo.Object{}, pkg)
		if evalErr != nil {
			return fmt.Errorf("unexpected error during apply: %w", evalErr)
		}

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected result to be *object.ReturnValue, but got %T", result)
		}
		strVal, ok := retVal.Value.(*object.String)
		if !ok {
			return fmt.Errorf("expected result value to be *object.String, but got %T", retVal.Value)
		}

		expected := "hello world 42"
		if strVal.Value != expected {
			return fmt.Errorf("Sprintf result is wrong\nwant: %q\ngot:  %q", expected, strVal.Value)
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
