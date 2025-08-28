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
