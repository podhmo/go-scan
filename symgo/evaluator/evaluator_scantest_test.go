package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalCallExprOnFunction_WithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": `
package main
func add(a, b int) int { return a + b }
func main() { add(1, 2) }
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, but got %d", len(pkgs))
		}
		pkg := pkgs[0]

		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return fmt.Errorf("ScannerForSymgo() failed: %w", err)
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()
		for _, file := range pkg.AstFiles {
			eval.Eval(file, env, pkg)
		}

		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in environment")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not an object.Function, got %T", mainFuncObj)
		}

		eval.applyFunction(mainFunc, []object.Object{}, pkg)
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalCallExprOnIntrinsic_WithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": `
package main
import "fmt"
func main() { fmt.Println("hello") }
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var got string
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		eval.RegisterIntrinsic("fmt.Println", func(args ...object.Object) object.Object {
			if len(args) > 0 {
				if s, ok := args[0].(*object.String); ok {
					got = s.Value
				}
			}
			return &object.SymbolicPlaceholder{}
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		eval.applyFunction(mainFunc, []object.Object{}, pkg)

		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if want := "hello"; got != want {
		t.Errorf("intrinsic not called correctly, want %q, got %q", want, got)
	}
}

func TestEvalCallExprOnInstanceMethod_WithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": `
package main
import "net/http"
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", nil)
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var gotPattern string
	handleFuncCalled := false

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		const serveMuxTypeName = "net/http.ServeMux"
		eval.RegisterIntrinsic("net/http.NewServeMux", func(args ...object.Object) object.Object {
			return &object.Instance{TypeName: serveMuxTypeName}
		})

		eval.RegisterIntrinsic(fmt.Sprintf("(*%s).HandleFunc", serveMuxTypeName), func(args ...object.Object) object.Object {
			handleFuncCalled = true
			if len(args) > 1 { // self, pattern, handler
				if s, ok := args[1].(*object.String); ok {
					gotPattern = s.Value
				}
			}
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		eval.applyFunction(mainFunc, []object.Object{}, pkg)
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if !handleFuncCalled {
		t.Fatal("HandleFunc intrinsic was not called")
	}
	if want := "/"; gotPattern != want {
		t.Errorf("HandleFunc pattern wrong, want %q, got %q", want, gotPattern)
	}
}
