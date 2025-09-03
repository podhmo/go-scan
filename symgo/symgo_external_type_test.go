package symgo_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestInterpreter_MethodCallOnExternalPointerType(t *testing.T) {
	// This is the definitive test to reproduce the "invalid indirect" error.
	// The bug occurs when symbolically executing a function that takes a pointer
	// to an external type and then calls a method on that pointer.

	files := map[string]string{
		"go.mod": `
module mytest
go 1.24
replace github.com/podhmo/go-scan => ../
`,
		// The `locator` package is real, but will be treated as external.
		// We don't need to provide its source here because of the `replace` directive.

		// The `usecase` package is in the primary analysis scope.
		"usecase/usecase.go": `
package usecase
import "github.com/podhmo/go-scan/locator"

// UseIt takes a pointer to a type from an external (non-analyzed) package.
func UseIt(l *locator.Locator) {
	// The bug is triggered when calling a method on the symbolic pointer `l`.
	// The interpreter incorrectly resolves `l` as a pointer to a function,
	// causing an "invalid indirect" error when trying to dispatch the method call.
	_, _ = l.PathToImport(".")
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) (retErr error) {
		// Set the primary analysis scope to our in-memory package.
		// The `locator` package will be treated as external.
		interp, err := symgo.NewInterpreter(s, symgo.WithPrimaryAnalysisScope("mytest/usecase"))
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		usecasePkg, err := s.ScanPackageByImport(ctx, "mytest/usecase")
		if err != nil {
			return fmt.Errorf("failed to scan usecase package: %w", err)
		}

		defer func() {
			if r := recover(); r != nil {
				retErr = fmt.Errorf("evaluation panicked: %v", r)
			}
		}()

		// Load declarations from the usecase package.
		for _, fileAst := range usecasePkg.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, usecasePkg); err != nil {
				return fmt.Errorf("Eval() for declarations failed: %+v", err)
			}
		}

		// Find the function to execute.
		fnObj, ok := interp.FindObject("UseIt")
		if !ok {
			return fmt.Errorf("could not find function UseIt")
		}
		fn, ok := fnObj.(*object.Function)
		if !ok {
			return fmt.Errorf("object UseIt is not a function, but %T", fnObj)
		}

		// Create a symbolic argument for the `*locator.Locator` parameter.
		// This simulates the state when `symgo` starts analyzing a function
		// without a concrete value for its parameters.
		locatorPkg, err := s.ScanPackageByImport(ctx, "github.com/podhmo/go-scan/locator")
		if err != nil {
			return fmt.Errorf("failed to scan locator package for setup: %w", err)
		}
		locatorType := locatorPkg.Lookup("Locator")
		if locatorType == nil {
			return fmt.Errorf("could not find TypeInfo for locator.Locator")
		}

		symbolicArg := &object.SymbolicPlaceholder{
			Reason: "symbolic *locator.Locator for test",
			BaseObject: object.BaseObject{
				ResolvedTypeInfo: locatorType,
				ResolvedFieldType: &goscan.FieldType{
					IsPointer:  true,
					Definition: locatorType,
				},
			},
		}

		// Apply the function with the symbolic argument. This should fail.
		_, applyErr := interp.Apply(ctx, fn, []object.Object{symbolicArg}, usecasePkg)
		if applyErr != nil {
			return fmt.Errorf("Apply() returned error: %+v", applyErr)
		}

		return nil // Success!
	}

	// scantest.Run will automatically fail the test `t` if the action function returns a non-nil error.
	// Before the fix, we expect the action to return an error (from the panic or evalErr).
	// After the fix, we expect the action to return nil.
	scantest.Run(t, context.Background(), dir, nil, action)
}
