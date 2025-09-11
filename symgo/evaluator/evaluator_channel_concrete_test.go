package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_MakeChannel(t *testing.T) {
	source := `
package main

func main() {
	ch := make(chan int)
	return ch
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if isError(result) {
			return fmt.Errorf("evaluation failed: %v", result.Inspect())
		}

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected ReturnValue, got %T", result)
		}

		ch, ok := retVal.Value.(*object.Channel)
		if !ok {
			return fmt.Errorf("expected Channel, got %T (%s)", retVal.Value, retVal.Value.Inspect())
		}

		if ch.ChanFieldType == nil {
			return fmt.Errorf("channel field type is nil")
		}
		if !ch.ChanFieldType.IsChan {
			return fmt.Errorf("expected field type to be a channel, but it was not")
		}
		if ch.ChanFieldType.Elem == nil {
			return fmt.Errorf("channel element type is nil")
		}
		if ch.ChanFieldType.Elem.Name != "int" {
			return fmt.Errorf("expected channel element type to be 'int', got %q", ch.ChanFieldType.Elem.Name)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEval_ReceiveFromChannel(t *testing.T) {
	source := `
package main

func main() {
	ch := make(chan string)
	val := <-ch
	return val
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if isError(result) {
			return fmt.Errorf("evaluation failed: %v", result.Inspect())
		}

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected ReturnValue, got %T", result)
		}

		placeholder, ok := retVal.Value.(*object.SymbolicPlaceholder)
		if !ok {
			return fmt.Errorf("expected SymbolicPlaceholder, got %T (%s)", retVal.Value, retVal.Value.Inspect())
		}

		fieldType := placeholder.FieldType()
		if fieldType == nil {
			return fmt.Errorf("placeholder field type is nil")
		}
		if fieldType.Name != "string" {
			return fmt.Errorf("expected placeholder type to be 'string', got %q", fieldType.Name)
		}
		if !fieldType.IsBuiltin {
			return fmt.Errorf("expected placeholder type to be a builtin")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
