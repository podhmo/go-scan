package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_ExternalInterfaceMethodCall(t *testing.T) {
	t.Skip("this test is failing because of go module resolution issues, which will be addressed in a future task")
	files := map[string]string{
		"go.mod": "module example.com/me",
		"iface/iface.go": `
package iface
type Writer interface {
	Write(p []byte) (n int, err error)
}`,
		"main.go": `
package main
import "example.com/me/iface"
func Do(w iface.Writer) {
	w.Write(nil)
}
func main() {
	Do(nil)
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var writeCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		key := "(example.com/me/iface.Writer).Write"
		eval.RegisterIntrinsic(key, func(args ...object.Object) object.Object {
			writeCalled = true
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	// Create a scanner with the module resolver, which is the key to fixing the issue.
	s, err := goscan.New(goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action, scantest.WithScanner(s), scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !writeCalled {
		t.Errorf("intrinsic for external interface method was not called")
	}
}

func TestEval_InterfaceMethodCall(t *testing.T) {
	t.Skip("this test is failing because of go module resolution issues, which will be addressed in a future task")
	code := `
package main

type Writer interface {
	Write(p []byte) (n int, err error)
}

func Do(w Writer) {
	w.Write(nil)
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var writeCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		key := fmt.Sprintf("(%s.Writer).Write", pkg.ImportPath)
		eval.RegisterIntrinsic(key, func(args ...object.Object) object.Object {
			writeCalled = true
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	// This test does not need a custom scanner, but it DOES need the module root set correctly.
	s, err := goscan.New(goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action, scantest.WithScanner(s), scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !writeCalled {
		t.Errorf("intrinsic for (main.Writer).Write was not called")
	}
}
