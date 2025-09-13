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

		// Evaluate all files to populate the interpreter's internal environments
		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil && !strings.Contains(err.Error(), "undefined_variable") {
				// We expect an error, but only the one we're testing for.
				return fmt.Errorf("initial eval of file %s failed unexpectedly: %w", file.Name.Name, err)
			}
		}

		mainFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
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

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestFeature_CharLiteral(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `package main

func getChar() rune {
	return 'a'
}

func main() {
	_ = getChar()
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var capturedValue object.Object
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}

		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil {
				return err
			}
		}

		getCharFn, ok := interp.FindObjectInPackage(ctx, "example.com/me", "getChar")
		if !ok {
			return fmt.Errorf("getChar function not found")
		}

		result, applyErr := interp.Apply(ctx, getCharFn, []symgo.Object{}, pkg)
		if applyErr != nil {
			return fmt.Errorf("Apply(getChar) failed: %w", applyErr)
		}

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected ReturnValue, got %T", result)
		}
		capturedValue = retVal.Value

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if capturedValue == nil {
		t.Fatalf("did not capture any value")
	}

	intVal, ok := capturedValue.(*object.Integer)
	if !ok {
		t.Fatalf("expected captured value to be *object.Integer, but got %T", capturedValue)
	}

	expected := int64(97) // 'a'
	if intVal.Value != expected {
		t.Errorf("char literal value is wrong\nwant: %d\ngot:  %d", expected, intVal.Value)
	}
}

func TestSymgo_ReturnedFunctionClosure(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/func-return\n\ngo 1.21\n",
		"main.go": `
package main
import "example.com/func-return/service"
func main() {
    handler := service.GetHandler()
    handler()
}`,
		"service/service.go": `
package service

// GetHandler returns a function that uses an internal helper.
func GetHandler() func() {
    return func() {
        // This call is not being detected by the analyzer.
        usedByReturnedFunc()
    }
}

// This function should NOT be an orphan.
func usedByReturnedFunc() {}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	called := make(map[string]bool)
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		var mainPkg *goscan.Package
		for _, p := range pkgs {
			if p.Name == "main" {
				mainPkg = p
				break
			}
		}
		if mainPkg == nil {
			return fmt.Errorf("main package not found in scanned packages")
		}

		interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
		if err != nil {
			return err
		}

		// defaultIntrinsic is the key to tracking usage.
		interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			if len(args) == 0 {
				return nil
			}
			var key string
			switch fn := args[0].(type) {
			case *symgo.Function:
				if fn.Def != nil && fn.Package != nil {
					key = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.Def.Name)
				}
			case *symgo.SymbolicPlaceholder:
				if fn.UnderlyingFunc != nil && fn.Package != nil {
					key = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.UnderlyingFunc.Name)
				}
			}
			if key != "" {
				called[key] = true
			}
			return nil
		})

		// Evaluate all packages to populate the interpreter
		for _, p := range pkgs {
			for _, file := range p.AstFiles {
				if _, err := interp.Eval(ctx, file, p); err != nil {
					return fmt.Errorf("evaluating %s failed: %w", file.Name.Name, err)
				}
			}
		}

		// Find the main function object in the main package's environment.
		mainFuncObj, ok := interp.FindObjectInPackage(ctx, mainPkg.ImportPath, "main")
		if !ok {
			return fmt.Errorf("main function not found")
		}

		// Apply the main function.
		_, applyErr := interp.Apply(ctx, mainFuncObj, nil, mainPkg)
		if applyErr != nil {
			return fmt.Errorf("Apply(main) failed: %w", applyErr)
		}

		// The core of the test: was the nested function call detected?
		if !called["example.com/func-return/service.usedByReturnedFunc"] {
			t.Errorf("expected usedByReturnedFunc to be called, but it was not")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
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

		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil {
				return err
			}
		}

		mainFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
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

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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

		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil {
				return err
			}
		}

		// First, test that calling the function directly returns a MultiReturn
		twoReturnsFn, ok := interp.FindObjectInPackage(ctx, "example.com/me", "twoReturns")
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
		mainFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
		if !ok {
			return fmt.Errorf("main function not found")
		}

		// The main test here is that the Apply call doesn't return an "assignment mismatch" error.
		_, mainApplyErr := interp.Apply(ctx, mainFuncObj, []symgo.Object{}, pkg)
		if mainApplyErr != nil {
			return fmt.Errorf("Apply(main) failed unexpectedly: %w", mainApplyErr)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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

		interp.RegisterIntrinsic("example.com/me.MyPattern", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			patternCalled = true
			return nil
		})

		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil {
				return fmt.Errorf("initial eval of file %s failed: %w", file.Name.Name, err)
			}
		}

		mainFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
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

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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

		// The interpreter should have a default intrinsic for fmt.Sprintf
		// that performs the formatting.

		for _, file := range pkg.AstFiles {
			_, err := interp.Eval(ctx, file, pkg)
			if err != nil {
				return fmt.Errorf("initial eval of file %s failed: %w", file.Name.Name, err)
			}
		}

		runFuncObj, ok := interp.FindObjectInPackage(ctx, "example.com/me", "run")
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
			// If this fails, it's likely because the Sprintf intrinsic is not
			// correctly returning a concrete string object.
			return fmt.Errorf("expected result value to be *object.String, but got %T (%s)", retVal.Value, retVal.Value.Inspect())
		}

		expected := "hello world 42"
		if strVal.Value != expected {
			return fmt.Errorf("Sprintf result is wrong\nwant: %q\ngot:  %q", expected, strVal.Value)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
