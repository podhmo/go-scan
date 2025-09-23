package evaluator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestVariadicFunction(t *testing.T) {
	source := `
package main

func variadicInts(nums ...int) {
	// do nothing
}

func variadicMixed(s string, args ...interface{}) {
	// do nothing
}

func main() {
	variadicInts(1, 2, 3)

	s := []interface{}{"hello", 42}
	variadicMixed("start", s...)
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var calls []string

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		e := New(s, s.Logger, nil, nil)

		e.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
			fnObj := args[0]
			fnArgs := args[1:]

			var name string
			switch fn := fnObj.(type) {
			case *object.Function:
				name = fn.Name.Name
			case *object.SymbolicPlaceholder:
				name = fn.Reason
			default:
				return nil
			}

			var argStrings []string
			for _, arg := range fnArgs {
				argStrings = append(argStrings, arg.Inspect())
			}
			calls = append(calls, fmt.Sprintf("%s(%s)", name, strings.Join(argStrings, ", ")))
			return nil
		})

		for _, astFile := range pkg.AstFiles {
			e.Eval(ctx, astFile, nil, pkg)
		}

		loadedPkg, err := e.GetOrLoadPackageForTest(ctx, "example.com/me")
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not a function object")
		}

		result := e.Apply(ctx, mainFunc, []object.Object{}, pkg, pkgEnv)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("eval failed: %s", err.Inspect())
		}

		expectedCalls := []string{
			`variadicInts(1, 2, 3)`,
			`variadicMixed("start", ...["hello", 42])`,
		}

		if len(calls) != len(expectedCalls) {
			return fmt.Errorf("expected %d calls, but got %d.\ncalls:\n%s", len(expectedCalls), len(calls), strings.Join(calls, "\n"))
		}

		for i := range expectedCalls {
			if calls[i] != expectedCalls[i] {
				return fmt.Errorf("expected call %d to be %q, but got %q", i, expectedCalls[i], calls[i])
			}
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
