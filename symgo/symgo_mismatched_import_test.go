package symgo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/noerror"
)

func TestMismatchedImportPackageName(t *testing.T) {
	// This test uses a real-world dependency, gopkg.in/yaml.v2, where the
	// package name (`yaml`) is different from the last part of the import
	// path (`v2`). The goal is to ensure that symgo can correctly resolve
	// the package name and analyze code that uses it, both when the package
	// is inside and outside the scan policy.

	mainModuleDir := filepath.Join("testdata", "mismatched-import")
	mainPkgPath := "example.com/mismatched-import"
	ctx := context.Background()

	run := func(t *testing.T, policy symgo.ScanPolicyFunc) {
		s, err := goscan.New(
			goscan.WithWorkDir(mainModuleDir),
			goscan.WithGoModuleResolver(),
		)
		noerror.Must(t, err)

		interp, err := symgo.NewInterpreter(s, symgo.WithScanPolicy(policy))
		noerror.Must(t, err)

		pkg, err := interp.Scanner().ScanPackageByImport(ctx, mainPkgPath)
		noerror.Must(t, err)

		mainFile := FindFile(t, pkg, "main.go")
		_, err = interp.Eval(ctx, mainFile, pkg)
		noerror.Must(t, err)

		// Find the GetNode function in the environment
		getNodeObj, ok := interp.FindObject("GetNode")
		if !ok {
			t.Fatal("GetNode function not found in interpreter environment")
		}
		getNodeFunc, ok := getNodeObj.(*symgo.Function)
		if !ok {
			t.Fatalf("entrypoint 'GetNode' is not a function, but %T", getNodeObj)
		}

		// Symbolically execute GetNode to trigger the lazy-loading and correction logic.
		ret, err := interp.Apply(ctx, getNodeFunc, nil, pkg)
		noerror.Must(t, err)

		retVal, ok := ret.(*object.ReturnValue)
		if !ok {
			t.Fatalf("expected return value to be a *object.ReturnValue, got %T", ret)
		}

		// --- Assertions ---

		// 1. Check that the return value's type is correctly identified.
		typeInfo := retVal.Value.TypeInfo()
		if typeInfo == nil {
			t.Fatal("return value has no type info")
		}
		if want, got := "Node", typeInfo.Name; want != got {
			t.Errorf("expected type name %q, but got %q", want, got)
		}
		if want, got := "gopkg.in/yaml.v2", typeInfo.PkgPath; want != got {
			t.Errorf("expected package path %q, but got %q", want, got)
		}

		// 2. Check that the environment has been corrected.
		// The package should be accessible via its *correct* name, "yaml".
		pkgObj, ok := interp.FindObject("yaml")
		if !ok {
			t.Fatal(`package "yaml" not found in environment after execution`)
		}
		pkgVal, ok := pkgObj.(*object.Package)
		if !ok {
			t.Fatalf(`expected "yaml" to be an *object.Package, but got %T`, pkgObj)
		}
		if want, got := "yaml", pkgVal.Name; want != got {
			t.Errorf("package object's name is incorrect: want %q, got %q", want, got)
		}
		if want, got := "gopkg.in/yaml.v2", pkgVal.Path; want != got {
			t.Errorf("package object's path is incorrect: want %q, got %q", want, got)
		}

		// 3. Check that the incorrect, guessed name ("v2") is not in the environment.
		_, ok = interp.FindObject("v2")
		if ok {
			t.Error(`incorrect package name "v2" should not exist in the environment`)
		}
	}

	t.Run("in-policy: external package is scanned", func(t *testing.T) {
		policy := func(importPath string) bool {
			// Scan both the main module and the yaml package
			return strings.HasPrefix(importPath, mainPkgPath) || strings.HasPrefix(importPath, "gopkg.in/yaml.v2")
		}
		run(t, policy)
	})

	t.Run("out-of-policy: external package is not scanned", func(t *testing.T) {
		policy := func(importPath string) bool {
			// Scan only the main module
			return strings.HasPrefix(importPath, mainPkgPath)
		}
		run(t, policy)
	})
}
