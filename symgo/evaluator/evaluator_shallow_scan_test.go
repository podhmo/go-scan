package evaluator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestShallowScan_VarDecl_WithUnresolvedType(t *testing.T) {
	code := `
package main
import "foreign/lib"

// This variable's type is from a package disallowed by the scan policy.
var x lib.ForeignType

// This variable is used to verify that evaluation continued successfully.
var sentinel int
`
	files := map[string]string{
		"go.mod":             "module example.com/me",
		"main.go":            code,
		"foreign/lib/lib.go": "package lib; type ForeignType struct{}",
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		env := object.NewEnvironment()

		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, got %d", len(pkgs))
		}
		pkg := pkgs[0]
		if len(pkg.AstFiles) == 0 {
			return fmt.Errorf("no ast files found in package")
		}

		for _, file := range pkg.AstFiles {
			if result := evaluator.Eval(ctx, file, env, pkg); result != nil {
				if _, ok := result.(*object.Error); ok {
					return fmt.Errorf("Eval() returned an error: %v", result.Inspect())
				}
			}
		}

		// Check the unresolved variable
		obj, ok := env.Get("x")
		if !ok {
			return fmt.Errorf("variable 'x' not found in environment")
		}
		v, ok := obj.(*object.Variable)
		if !ok {
			return fmt.Errorf("object 'x' is not a variable, but %T", obj)
		}
		typeInfo := v.TypeInfo()
		if typeInfo == nil {
			return fmt.Errorf("variable 'x' has no TypeInfo")
		}
		if !typeInfo.Unresolved {
			t.Errorf("expected TypeInfo.Unresolved to be true, but it was false")
		}
		want := &scanner.TypeInfo{
			PkgPath:    "foreign/lib",
			Name:       "ForeignType",
			Unresolved: true,
		}
		if diff := cmp.Diff(want, typeInfo); diff != "" {
			t.Errorf("TypeInfo mismatch (-want +got):\n%s", diff)
		}

		// Check that evaluation continued past the unresolved type.
		if _, ok := env.Get("sentinel"); !ok {
			return fmt.Errorf("sentinel variable not found, evaluation may have stopped prematurely")
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestShallowScan_CompositeLit_WithUnresolvedType(t *testing.T) {
	code := `
package main
import "foreign/lib"

// This variable is initialized with a composite literal of an unresolved type.
var x = lib.ForeignType{}

// This variable is used to verify that evaluation continued successfully.
var sentinel int
`
	files := map[string]string{
		"go.mod":             "module example.com/me",
		"main.go":            code,
		"foreign/lib/lib.go": "package lib; type ForeignType struct{ ID int }",
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		env := object.NewEnvironment()

		pkg := pkgs[0]
		for _, file := range pkg.AstFiles {
			evaluator.Eval(ctx, file, env, pkg)
		}

		// Check the unresolved variable
		obj, ok := env.Get("x")
		if !ok {
			return fmt.Errorf("variable 'x' not found")
		}
		v, ok := obj.(*object.Variable)
		if !ok {
			return fmt.Errorf("object 'x' is not a variable, but %T", obj)
		}
		placeholder, ok := v.Value.(*object.SymbolicPlaceholder)
		if !ok {
			return fmt.Errorf("expected value to be SymbolicPlaceholder, but got %T", v.Value)
		}
		fieldType := placeholder.FieldType()
		if fieldType == nil {
			return fmt.Errorf("placeholder has no FieldType")
		}
		if diff := cmp.Diff("foreign/lib", fieldType.FullImportPath); diff != "" {
			return fmt.Errorf("FieldType.FullImportPath mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("ForeignType", fieldType.TypeName); diff != "" {
			return fmt.Errorf("FieldType.TypeName mismatch (-want +got):\n%s", diff)
		}

		// Check that evaluation continued past the unresolved type.
		if _, ok := env.Get("sentinel"); !ok {
			return fmt.Errorf("sentinel variable not found, evaluation may have stopped prematurely")
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
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

// findFunc is a test helper to find a function object by its name in a package.
func findFunc(t *testing.T, pkg *goscan.Package, name string) *object.Function {
	t.Helper()
	for _, f := range pkg.Functions {
		if f.Name == name {
			return &object.Function{
				Name:       f.AstDecl.Name,
				Parameters: f.AstDecl.Type.Params,
				Body:       f.AstDecl.Body,
				Decl:       f.AstDecl,
				Package:    pkg,
				Def:        f,
				// Env is intentionally nil, will be set by evaluator
			}
		}
	}
	t.Fatalf("function %q not found in package %q", name, pkg.ImportPath)
	return nil
}

func TestShallowScan_StarAndIndexExpr(t *testing.T) {
	mypkg_code := `
package mypkg
import "example.com/me/extpkg"

// These will be assigned the results of the shallow scan expressions.
var P_val extpkg.ExternalType
var S_val extpkg.ExternalType

func MyFunction(p *extpkg.ExternalType, s []extpkg.ExternalType) {
	P_val = *p
	S_val = s[0]
	Sentinel(1)
}
`
	files := map[string]string{
		"go.mod":          "module example.com/me",
		"mypkg/code.go":   mypkg_code,
		"extpkg/code.go": "package extpkg; type ExternalType struct{}",
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return path != "example.com/me/extpkg"
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me/mypkg.Sentinel", func(args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		// First, evaluate the whole package to populate top-level declarations.
		mainPkg := findPackage(t, pkgs, "example.com/me/mypkg")
		env := object.NewEnvironment()
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, env, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		// Now, find the function and call it.
		fn := findFunc(t, mainPkg, "MyFunction")
		fn.Env = env // Use the package-level environment.

		extTypeField := &scanner.FieldType{
			PkgName:        "extpkg",
			TypeName:       "ExternalType",
			FullImportPath: "example.com/me/extpkg",
		}
		pointerToExtType := &scanner.FieldType{
			IsPointer: true,
			Elem:      extTypeField,
		}
		sliceOfExtType := &scanner.FieldType{
			IsSlice: true,
			Elem:    extTypeField,
		}

		arg1 := &object.SymbolicPlaceholder{Reason: "p"}
		arg1.SetFieldType(pointerToExtType)
		arg2 := &object.SymbolicPlaceholder{Reason: "s"}
		arg2.SetFieldType(sliceOfExtType)

		result := evaluator.Apply(ctx, fn, []object.Object{arg1, arg2}, mainPkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("Apply() returned an error: %s", err.Inspect())
		}

		if !sentinelReached {
			return fmt.Errorf("sentinel was not reached, evaluation stopped prematurely")
		}

		// Assertions about the resulting values
		wantUnresolvedType := &scanner.TypeInfo{
			PkgPath:    "example.com/me/extpkg",
			Name:       "ExternalType",
			Unresolved: true,
		}

		// Check the value from the pointer dereference
		pValObj, ok := env.Get("P_val")
		if !ok {
			return fmt.Errorf("variable 'P_val' not found")
		}
		pValVar := pValObj.(*object.Variable)
		pValPlaceholder := pValVar.Value.(*object.SymbolicPlaceholder)

		if diff := cmp.Diff(wantUnresolvedType, pValPlaceholder.TypeInfo()); diff != "" {
			t.Errorf("P_val placeholder TypeInfo mismatch (-want +got):\n%s", diff)
		}

		// Check the value from the slice index
		sValObj, ok := env.Get("S_val")
		if !ok {
			return fmt.Errorf("variable 'S_val' not found")
		}
		sValVar := sValObj.(*object.Variable)
		sValPlaceholder := sValVar.Value.(*object.SymbolicPlaceholder)

		if diff := cmp.Diff(wantUnresolvedType, sValPlaceholder.TypeInfo()); diff != "" {
			t.Errorf("S_val placeholder TypeInfo mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
