package symgo_test

import (
	"context"
	"go/ast"
	"go/parser"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// common test helper for this package
func findFile(t *testing.T, pkg *goscan.Package, filename string) *ast.File {
	t.Helper()
	for path, f := range pkg.AstFiles {
		if strings.HasSuffix(path, filename) {
			return f
		}
	}
	t.Fatalf("file %q not found in package %q", filename, pkg.Name)
	return nil
}

// findFunc is a test helper to find a function by its name in a package.
func findFunc(t *testing.T, pkg *goscan.Package, name string) *scanner.FunctionInfo {
	t.Helper()
	for _, f := range pkg.Functions {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("function %q not found in package %q", name, pkg.ImportPath)
	return nil
}

func TestNewInterpreter(t *testing.T) {
	t.Run("nil scanner", func(t *testing.T) {
		_, err := symgo.NewInterpreter(nil)
		if err == nil {
			t.Error("expected an error for nil scanner, but got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		dir, cleanup := scantest.WriteFiles(t, map[string]string{
			"go.mod": "module mymodule",
		})
		defer cleanup()

		s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
		if err != nil {
			t.Fatalf("goscan.New() failed: %+v", err)
		}

		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			t.Errorf("NewInterpreter() failed: %+v", err)
		}
		if interp == nil {
			t.Error("expected interpreter to be non-nil")
		}
	})
}

func TestInterpreter_Eval_Simple(t *testing.T) {
	source := `
package main
import "fmt"
func main() {
	fmt.Println("hello")
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(t.Context(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	// We need to evaluate the file first to process imports.
	_, err = interp.Eval(t.Context(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Evaluate an expression that uses an imported package.
	node, err := parser.ParseExpr(`fmt.Println`)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %+v", err)
	}

	// Now evaluate the expression
	result, err := interp.Eval(t.Context(), node, pkg)
	if err != nil {
		t.Fatalf("interp.Eval(expr) failed: %+v", err)
	}

	_, ok := result.(*object.UnresolvedType)
	if !ok {
		t.Errorf("Expected an UnresolvedType for an external function, but got %T", result)
	}
}

func TestInterpreter_RegisterIntrinsic(t *testing.T) {
	// Test that a registered intrinsic function is correctly called during evaluation.
	source := `
package main
import "fmt"
func main() {
	fmt.Println("hello")
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}
	pkgs, err := s.Scan(t.Context(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	// Simplified intrinsic handler
	expectedResult := &object.String{Value: "Intrinsic was called!"}
	handler := func(ctx context.Context, interp *symgo.Interpreter, args []object.Object) object.Object {
		return expectedResult
	}
	interp.RegisterIntrinsic("fmt.Println", handler)

	// We need to evaluate the file first to process imports.
	_, err = interp.Eval(t.Context(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Evaluate an expression that calls the intrinsic
	node, err := parser.ParseExpr(`fmt.Println("hello")`)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %+v", err)
	}

	// Now evaluate the call expression
	result, err := interp.Eval(t.Context(), node, pkg)
	if err != nil {
		t.Fatalf("interp.Eval(expr) failed: %+v", err)
	}

	if diff := cmp.Diff(expectedResult, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestBlockStatement_executesAllStatements(t *testing.T) {
	// This test checks for a specific bug where a function call used as a statement
	// would return a ReturnValue object, causing evalBlockStatement to terminate
	// prematurely and skip subsequent statements.
	source := `
package main

func log(msg string) string {
	// This function returns a value, which is key to reproducing the bug.
	return msg
}

func main() {
	log("call 1")
	log("call 2")
	log("call 3")
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(t.Context(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	var callLog []string
	// The key for the intrinsic is the fully qualified package path + function name.
	// For package main in a module named "mymodule", the path is "mymodule".
	interp.RegisterIntrinsic("mymodule.log", func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) > 0 {
			if str, ok := args[0].(*object.String); ok {
				callLog = append(callLog, str.Value)
			}
		}
		// Return a string, which will get wrapped in a ReturnValue
		return &object.String{Value: "dummy return"}
	})

	// Evaluate the file to load symbols
	_, err = interp.Eval(t.Context(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Find and apply the main function
	mainFn, ok := interp.FindObjectInPackage(t.Context(), "mymodule", "main")
	if !ok {
		t.Fatal("could not find main function")
	}
	interp.Apply(t.Context(), mainFn, nil, pkg)

	// Verify that all log calls were made
	expected := []string{"call 1", "call 2", "call 3"}
	if diff := cmp.Diff(expected, callLog); diff != "" {
		t.Errorf("call log mismatch (-want +got):\n%s", diff)
	}
}

func TestNakedReturn_AssignedToVar(t *testing.T) {
	source := `
package main

// This function has a named return type but a naked return.
// It should return the zero value for the type, which is nil for a pointer.
func myFunc() *int {
	return
}

func main() {
	// The assignment to x is what might trigger the panic.
	x := myFunc()
	_ = x // use x to prevent compiler error
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(t.Context(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	// We need a default intrinsic to prevent "not a function" errors
	// for unresolved functions if any.
	interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		return &object.SymbolicPlaceholder{Reason: "default intrinsic"}
	})

	// Evaluate the file to load symbols
	_, err = interp.Eval(t.Context(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Find and apply the main function
	mainFn, ok := interp.FindObjectInPackage(t.Context(), "mymodule", "main")
	if !ok {
		t.Fatal("could not find main function")
	}

	// This call should not panic.
	_, err = interp.Apply(t.Context(), mainFn, nil, pkg)
	if err != nil {
		t.Errorf("Apply() failed: %+v", err)
	}

	// Optional: Check if the variable 'x' exists in the environment and its value is nil
	// This part is more complex as we need to get the final environment.
	// For now, just ensuring it doesn't panic is enough.
}

func TestEntryPoint_WithMissingArguments(t *testing.T) {
	source := `
package main

type MyInterface interface {
	DoSomething() string
}

func MyFunction(p MyInterface) {
	if p == nil {
		return
	}
	_ = p.DoSomething()
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(t.Context(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	// Evaluate the file to load symbols
	_, err = interp.Eval(t.Context(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Find the entry point function
	mainFn, ok := interp.FindObjectInPackage(t.Context(), "mymodule", "MyFunction")
	if !ok {
		t.Fatal("could not find MyFunction function")
	}

	// Apply the function without providing arguments.
	// This should NOT fail with "identifier not found: p".
	// Before the fix, it will fail. After the fix, it should pass.
	_, err = interp.Apply(t.Context(), mainFn, []symgo.Object{}, pkg)
	if err != nil {
		t.Errorf("Apply() failed with unexpected error: %+v", err)
	}
}

func TestEntryPoint_VariadicInterface(t *testing.T) {
	source := `
package main

import "log"

// This function signature is similar to a logger func.
func MyLogf(f string, args ...interface{}) {
	if f == "" {
		log.Println("f is empty")
	}
	_ = args
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(t.Context(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	_, err = interp.Eval(t.Context(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	mainFn, ok := interp.FindObjectInPackage(t.Context(), "mymodule", "MyLogf")
	if !ok {
		t.Fatal("could not find MyLogf function")
	}

	// Call the function with one concrete argument and no variadic arguments.
	// This ensures the logic correctly handles creating an empty slice for
	// the `...interface{}` parameter.
	_, err = interp.Apply(t.Context(), mainFn, []symgo.Object{
		&object.String{Value: "hello"},
	}, pkg)

	if err != nil {
		t.Errorf("Apply() failed with unexpected error: %+v", err)
	}
}

func TestEntryPoint_Variadic_WithMissingArguments(t *testing.T) {
	source := `
package main

func MyVariadicFunc(a, b int, c ...string) {
	_ = a
	_ = b
	_ = c
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(t.Context(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	_, err = interp.Eval(t.Context(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	mainFn, ok := interp.FindObjectInPackage(t.Context(), "mymodule", "MyVariadicFunc")
	if !ok {
		t.Fatal("could not find MyVariadicFunc function")
	}

	// Call the function with only one argument. The function has two regular
	// parameters before the variadic one. This should cause a panic with the
	// buggy implementation.
	_, err = interp.Apply(t.Context(), mainFn, []symgo.Object{
		&object.Integer{Value: 1},
	}, pkg)

	if err != nil {
		t.Errorf("Apply() failed with unexpected error: %+v", err)
	}
}
