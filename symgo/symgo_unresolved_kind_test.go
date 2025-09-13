package symgo_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestSymgo_UnresolvedKindInference(t *testing.T) {
	// 1. Setup test files
	files := map[string]string{
		"go.mod": "module example.com/unresolvedkind",
		"ifaceandstruct/defs.go": `
package ifaceandstruct
type MyStruct struct { Name string }
type MyInterface interface { DoSomething() }
`,
		"main/main.go": `
package main
import "example.com/unresolvedkind/ifaceandstruct"
var VStruct ifaceandstruct.MyStruct
var VInterface ifaceandstruct.MyInterface
func main() {
	s := ifaceandstruct.MyStruct{Name: "test"}
	VStruct = s
	var x any
	i := x.(ifaceandstruct.MyInterface)
	VInterface = i
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	defsPath, err := scantest.ToImportPath(filepath.Join(dir, "ifaceandstruct"))
	if err != nil {
		t.Fatalf("could not resolve import path for defs: %v", err)
	}

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(pkgPath string) bool {
			return pkgPath != defsPath
		}

		i, err := symgo.NewInterpreter(s, symgo.WithScanPolicy(policy))
		if err != nil {
			return fmt.Errorf("NewInterpreter failed: %w", err)
		}

		mainPkg := findPackage(t, pkgs, "example.com/unresolvedkind/main")
		if mainPkg == nil {
			return fmt.Errorf("main package not found")
		}

		// Evaluate the package to populate top-level decls
		if _, err := i.Eval(ctx, mainPkg.AstFiles[mainPkg.Files[0]], mainPkg); err != nil {
			return fmt.Errorf("initial Eval of package failed: %w", err)
		}

		mainFuncObj, ok := i.FindObjectInPackage(ctx, "example.com/unresolvedkind/main", "main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not a function object")
		}

		// Run main
		if _, err := i.Apply(ctx, mainFunc, []object.Object{}, mainPkg); err != nil {
			return fmt.Errorf("Apply(main) failed: %w", err)
		}

		// Assertions
		vStructObj, ok := i.FindObjectInPackage(ctx, "example.com/unresolvedkind/main", "VStruct")
		if !ok {
			return fmt.Errorf("global variable VStruct not found")
		}
		vStruct, ok := vStructObj.(*object.Variable)
		if !ok {
			return fmt.Errorf("VStruct is not a variable, but %T", vStructObj)
		}
		// We check the TypeInfo of the VALUE assigned to the variable,
		// as this is what gets the inferred kind.
		structTypeInfo := vStruct.Value.TypeInfo()
		if structTypeInfo == nil {
			return fmt.Errorf("value of VStruct has no TypeInfo")
		}
		if !structTypeInfo.Unresolved {
			t.Error("MyStruct's TypeInfo should be Unresolved, but it was not")
		}
		if diff := cmp.Diff(scanner.StructKind, structTypeInfo.Kind); diff != "" {
			t.Errorf("VStruct kind mismatch (-want +got):\n%s", diff)
		}

		vIfaceObj, ok := i.FindObjectInPackage(ctx, "example.com/unresolvedkind/main", "VInterface")
		if !ok {
			return fmt.Errorf("global variable VInterface not found")
		}
		vIface, ok := vIfaceObj.(*object.Variable)
		if !ok {
			return fmt.Errorf("VInterface is not a variable, but %T", vIfaceObj)
		}
		// We check the TypeInfo of the VALUE assigned to the variable.
		ifaceTypeInfo := vIface.Value.TypeInfo()
		if ifaceTypeInfo == nil {
			return fmt.Errorf("value of VInterface has no TypeInfo")
		}
		if !ifaceTypeInfo.Unresolved {
			t.Error("MyInterface's TypeInfo should be Unresolved, but it was not")
		}
		if diff := cmp.Diff(scanner.InterfaceKind, ifaceTypeInfo.Kind); diff != "" {
			t.Errorf("VInterface kind mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

// findPackage is a test helper to find a package by its import path.
func findPackage(t *testing.T, pkgs []*goscan.Package, importPath string) *goscan.Package {
	t.Helper()
	for _, p := range pkgs {
		if p.ImportPath == importPath {
			return p
		}
	}
	t.Fatalf("package %q not found", importPath)
	return nil
}
