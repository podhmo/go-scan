package symgo

import (
	"context"
	"fmt"
	"testing"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestAnonymousTypes_Interface(t *testing.T) {
	source := `
package main

func capture(f func()) {}

type Hasher interface {
	Hex() []byte
}

func run(h Hasher) {
	capture(h.Hex)
}
`
	files := map[string]string{
		"go.mod":  "module main",
		"main.go": source,
	}

	var captured object.Object

	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, got %d", len(pkgs))
		}
		mainPkg := pkgs[0]

		// Create a new evaluator for this test run
		eval := evaluator.New(s, nil, nil, func(path string) bool { return true })

		// Register the intrinsic function that will capture the argument
		eval.RegisterIntrinsic("main.capture", func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) > 0 {
				captured = args[0]
			}
			return object.NIL
		})

		// Find the `run` function to execute
		var runFunc *object.Function
		for _, f := range mainPkg.Functions {
			if f.Name == "run" {
				// The package object passed to getOrResolveFunction needs an environment.
				pkgObj := &object.Package{ScannedInfo: mainPkg, Env: object.NewEnvironment()}
				runFunc = eval.GetOrResolveFunctionForTest(ctx, pkgObj, f).(*object.Function)
				break
			}
		}
		if runFunc == nil {
			return fmt.Errorf("function 'run' not found")
		}

		// Find the Hasher type to create a symbolic argument
		var hType *scan.TypeInfo
		for _, typ := range mainPkg.Types {
			if typ.Name == "Hasher" {
				hType = typ
				break
			}
		}
		if hType == nil {
			return fmt.Errorf("type 'Hasher' not found")
		}

		symbolicHasher := &object.SymbolicPlaceholder{
			Reason: "symbolic hasher",
		}
		symbolicHasher.SetTypeInfo(hType)

		// Apply the function
		eval.Apply(ctx, runFunc, []object.Object{symbolicHasher}, mainPkg, object.NewEnvironment())

		// --- Assertions ---
		if captured == nil {
			return fmt.Errorf("intrinsic 'capture' was not called")
		}

		bm, ok := captured.(*object.BoundMethod)
		if !ok {
			return fmt.Errorf("expected captured object to be a *object.BoundMethod, but got %T", captured)
		}

		if bm.Function == nil {
			return fmt.Errorf("BoundMethod.Function is nil")
		}
		if bm.Function.Def == nil {
			return fmt.Errorf("BoundMethod.Function.Def is nil")
		}
		if bm.Function.Def.Name != "Hex" {
			return fmt.Errorf("expected bound method name to be 'Hex', but got %q", bm.Function.Def.Name)
		}

		v, ok := bm.Receiver.(*object.Variable)
		if !ok {
			return fmt.Errorf("expected receiver to be a *object.Variable, but got %T", bm.Receiver)
		}

		receiver, ok := v.Value.(*object.SymbolicPlaceholder)
		if !ok {
			return fmt.Errorf("expected receiver's value to be a SymbolicPlaceholder, but got %T", v.Value)
		}
		if receiver.Reason != "symbolic hasher" {
			return fmt.Errorf("unexpected receiver reason: %q", receiver.Reason)
		}
		return nil
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Run the test
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
