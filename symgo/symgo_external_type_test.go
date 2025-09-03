package symgo_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestInterpreter_MethodCallOnExternalPointerType(t *testing.T) {
	files := map[string]string{
		"go.mod": `
module mytest
go 1.24
replace github.com/podhmo/go-scan => ../
`,
		"usecase/usecase.go": `
package usecase
import "github.com/podhmo/go-scan/locator"

// UseIt takes a pointer to a type from an external (non-analyzed) package.
func UseIt(l *locator.Locator) {
	// The bug is triggered when calling a method on the symbolic pointer 'l'.
	_, _ = l.PathToImport(".")
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) (retErr error) {
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

		for _, fileAst := range usecasePkg.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, usecasePkg); err != nil {
				return fmt.Errorf("Eval() for declarations failed: %+v", err)
			}
		}

		fnObj, ok := interp.FindObject("UseIt")
		if !ok {
			return fmt.Errorf("could not find function UseIt")
		}
		fn, ok := fnObj.(*object.Function)
		if !ok {
			return fmt.Errorf("object UseIt is not a function, but %T", fnObj)
		}

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
				ResolvedFieldType: &scanner.FieldType{
					IsPointer:  true,
					Definition: locatorType,
				},
			},
		}

		_, applyErr := interp.Apply(ctx, fn, []object.Object{symbolicArg}, usecasePkg)
		if applyErr != nil {
			return fmt.Errorf("Apply() returned error: %+v", applyErr)
		}

		return nil
	}

	scantest.Run(t, context.Background(), dir, nil, action)
}
