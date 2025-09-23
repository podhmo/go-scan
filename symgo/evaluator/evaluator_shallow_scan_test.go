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
import "example.com/me/foreign/lib"

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
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)

		pkg := findPackage(t, pkgs, "example.com/me")
		if len(pkg.AstFiles) == 0 {
			return fmt.Errorf("no ast files found in package")
		}

		for _, file := range pkg.AstFiles {
			if result := evaluator.Eval(ctx, file, nil, pkg); result != nil {
				if _, ok := result.(*object.Error); ok {
					return fmt.Errorf("Eval() returned an error: %v", result.Inspect())
				}
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, pkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		// Check the unresolved variable
		obj, ok := pkgEnv.Get("x")
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
			PkgPath:    "example.com/me/foreign/lib",
			Name:       "ForeignType",
			Unresolved: true,
			Kind:       scanner.UnknownKind,
		}
		if diff := cmp.Diff(want, typeInfo); diff != "" {
			t.Errorf("TypeInfo mismatch (-want +got):\n%s", diff)
		}

		// Check that evaluation continued past the unresolved type.
		if _, ok := pkgEnv.Get("sentinel"); !ok {
			return fmt.Errorf("sentinel variable not found, evaluation may have stopped prematurely")
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestShallowScan_MethodCall_OnUnresolvedType(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

func DoCall() {
	var x lib.ForeignType
	x.DoSomething() // This method call should be symbolic and not cause an error.
	Sentinel()
}

func Sentinel() {}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
		"foreign/lib/lib.go": `
package lib
type ForeignType struct{}
func (f ForeignType) DoSomething() {}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me.Sentinel", func(ctx context.Context, args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		mainPkg := findPackage(t, pkgs, "example.com/me")

		// Evaluate package-level declarations first
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, nil, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		// Now, call the function that performs the calls.
		fnObj, ok := pkgEnv.Get("DoCall")
		if !ok {
			return fmt.Errorf("DoCall function not found")
		}
		fn := fnObj.(*object.Function)

		result := evaluator.Apply(ctx, fn, nil, mainPkg, pkgEnv)
		if result != nil && result.Type() == object.ERROR_OBJ {
			return fmt.Errorf("Apply() returned an error: %s", result.Inspect())
		}

		if !sentinelReached {
			return fmt.Errorf("sentinel was not reached, evaluation may have stopped prematurely")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestShallowScan_ApplyFunction_WithUnresolvedReturn(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

var one lib.ForeignType
var two lib.ForeignType
var err error

func DoCalls() {
	one = lib.GetOne()
	two, err = lib.GetTwo()
	Sentinel()
}

func Sentinel() {}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
		"foreign/lib/lib.go": `
package lib
type ForeignType struct{}
func GetOne() ForeignType { return ForeignType{} }
func GetTwo() (ForeignType, error) { return ForeignType{}, nil }
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me.Sentinel", func(ctx context.Context, args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		mainPkg := findPackage(t, pkgs, "example.com/me")

		// Evaluate package-level declarations first
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, nil, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		// Now, call the function that performs the calls.
		fnObj, ok := pkgEnv.Get("DoCalls")
		if !ok {
			return fmt.Errorf("DoCalls function not found")
		}
		fn := fnObj.(*object.Function)

		if result := evaluator.Apply(ctx, fn, nil, mainPkg, pkgEnv); result != nil && result.Type() == object.ERROR_OBJ {
			return fmt.Errorf("Apply() returned an error: %s", result.Inspect())
		}

		if !sentinelReached {
			return fmt.Errorf("sentinel was not reached, evaluation may have stopped prematurely")
		}

		// --- Assertions ---
		// Check the variable from the single-return function
		oneVar, ok := pkgEnv.Get("one")
		if !ok {
			return fmt.Errorf("variable 'one' not found")
		}
		if _, ok := oneVar.(*object.Variable).Value.(*object.SymbolicPlaceholder); !ok {
			return fmt.Errorf("expected value of 'one' to be SymbolicPlaceholder, got %T", oneVar.(*object.Variable).Value)
		}
		// Check the variable from the multi-return function
		if _, ok := pkgEnv.Get("two"); !ok {
			return fmt.Errorf("variable 'two' not found")
		}

		// Check the error variable from the multi-return function
		if _, ok := pkgEnv.Get("err"); !ok {
			return fmt.Errorf("variable 'err' not found")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestShallowScan_AssignIdentifier_WithUnresolvedInterface(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

// i's type is an interface from an unscanned package.
var i lib.ForeignInterface

func DoAssign() {
	// Assign two different concrete types to the same interface variable.
	c1 := lib.ConcreteType1{}
	i = c1

	c2 := lib.ConcreteType2{}
	i = c2

	Sentinel()
}

func Sentinel() {}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
		"foreign/lib/lib.go": `
package lib
type ForeignInterface interface { Do() }

type ConcreteType1 struct{}
func (c ConcreteType1) Do() {}

type ConcreteType2 struct{}
func (c ConcreteType2) Do() {}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me.Sentinel", func(ctx context.Context, args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		mainPkg := findPackage(t, pkgs, "example.com/me")

		// Evaluate package-level declarations first
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, nil, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		// Verify the initial state of the interface variable 'i'
		iObj, ok := pkgEnv.Get("i")
		if !ok {
			return fmt.Errorf("initial variable 'i' not found")
		}
		iVar, ok := iObj.(*object.Variable)
		if !ok {
			return fmt.Errorf("initial object 'i' is not a variable, but %T", iObj)
		}
		if iVar.TypeInfo() == nil || !iVar.TypeInfo().Unresolved {
			t.Errorf("initial variable 'i' should have an unresolved TypeInfo, but got: %v", iVar.TypeInfo())
		}

		// Now, call the function that performs the assignment.
		fnObj, ok := pkgEnv.Get("DoAssign")
		if !ok {
			return fmt.Errorf("DoAssign function not found")
		}
		fn := fnObj.(*object.Function)

		if result := evaluator.Apply(ctx, fn, nil, mainPkg, pkgEnv); result != nil && result.Type() == object.ERROR_OBJ {
			return fmt.Errorf("Apply() returned an error: %s", result.Inspect())
		}

		if !sentinelReached {
			return fmt.Errorf("sentinel was not reached, evaluation may have stopped prematurely")
		}

		// After assignment, check the state of 'i' again.
		iObj, ok = pkgEnv.Get("i")
		if !ok {
			return fmt.Errorf("variable 'i' not found after assignment")
		}
		iVar, ok = iObj.(*object.Variable)
		if !ok {
			return fmt.Errorf("object 'i' is not a variable after assignment, but %T", iObj)
		}

		// The core of the test: check if BOTH concrete types were tracked.
		if len(iVar.PossibleTypes) != 2 {
			return fmt.Errorf("expected 2 possible concrete types, but got %d", len(iVar.PossibleTypes))
		}

		foundNames := make(map[string]bool)
		for typeString := range iVar.PossibleTypes {
			// This is a simplification; a real test might parse the string.
			// For now, we just check that the expected type names are present.
			if strings.Contains(typeString, "ConcreteType1") {
				foundNames["ConcreteType1"] = true
			}
			if strings.Contains(typeString, "ConcreteType2") {
				foundNames["ConcreteType2"] = true
			}
		}

		if !foundNames["ConcreteType1"] {
			t.Error("did not find ConcreteType1 in possible types")
		}
		if !foundNames["ConcreteType2"] {
			t.Error("did not find ConcreteType2 in possible types")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvaluator_ShallowScan_TypeSwitch(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

func Sentinel() {}

var result any // Package-level variable to store the result from the case

func MyFunction(v any) {
	switch x := v.(type) {
	case lib.ForeignType:
		result = x // Assign to package-level var
	default:
		// Do nothing
	}
	Sentinel()
}
`
	files := map[string]string{
		"go.mod":             "module example.com/me",
		"main.go":            code,
		"foreign/lib/lib.go": "package lib; type ForeignType struct{}",
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			// Disallow scanning the "foreign" package.
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me.Sentinel", func(ctx context.Context, args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		mainPkg := findPackage(t, pkgs, "example.com/me")

		// Evaluate the package to populate top-level decls.
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, nil, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		fnObj, ok := pkgEnv.Get("MyFunction")
		if !ok {
			return fmt.Errorf("MyFunction function not found")
		}
		fn := fnObj.(*object.Function)

		arg := &object.SymbolicPlaceholder{Reason: "test argument"}
		argFieldType := &scanner.FieldType{
			PkgName:        "lib",
			TypeName:       "ForeignType",
			FullImportPath: "example.com/me/foreign/lib",
		}
		arg.SetFieldType(argFieldType)

		result := evaluator.Apply(ctx, fn, []object.Object{arg}, mainPkg, pkgEnv)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("Apply() returned an error: %s", err.Inspect())
		}

		if !sentinelReached {
			return fmt.Errorf("sentinel was not reached")
		}

		// Check the type of the 'result' variable.
		resultVar, ok := pkgEnv.Get("result")
		if !ok {
			return fmt.Errorf("variable 'result' not found")
		}
		v, ok := resultVar.(*object.Variable)
		if !ok {
			return fmt.Errorf("object 'result' is not a variable, but %T", v)
		}
		placeholder, ok := v.Value.(*object.SymbolicPlaceholder)
		if !ok {
			return fmt.Errorf("expected value of 'result' to be a SymbolicPlaceholder, got %T (%s)", v.Value, v.Value.Inspect())
		}

		wantUnresolvedType := &scanner.TypeInfo{
			PkgPath:    "example.com/me/foreign/lib",
			Name:       "ForeignType",
			Unresolved: true,
			Kind:       scanner.InterfaceKind, // Kind is inferred from the type switch.
		}
		if diff := cmp.Diff(wantUnresolvedType, placeholder.TypeInfo()); diff != "" {
			t.Errorf("result placeholder TypeInfo mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvaluator_ShallowScan_TypeAssert(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

func Sentinel() {}

// This variable will hold the result of the type assertion.
var result lib.ForeignType

func MyFunction(v any) {
	result = v.(lib.ForeignType)
	Sentinel()
}
`
	files := map[string]string{
		"go.mod":             "module example.com/me",
		"main.go":            code,
		"foreign/lib/lib.go": "package lib; type ForeignType struct{}",
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me.Sentinel", func(ctx context.Context, args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		mainPkg := findPackage(t, pkgs, "example.com/me")

		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, nil, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		fnObj, ok := pkgEnv.Get("MyFunction")
		if !ok {
			return fmt.Errorf("MyFunction function not found")
		}
		fn := fnObj.(*object.Function)

		arg := &object.SymbolicPlaceholder{Reason: "test argument"}
		result := evaluator.Apply(ctx, fn, []object.Object{arg}, mainPkg, pkgEnv)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("Apply() returned an error: %s", err.Inspect())
		}

		if !sentinelReached {
			return fmt.Errorf("sentinel was not reached, evaluation may have stopped prematurely")
		}

		// Also check the type of the result variable.
		resultVar, ok := pkgEnv.Get("result")
		if !ok {
			return fmt.Errorf("variable 'result' not found")
		}
		v, ok := resultVar.(*object.Variable)
		if !ok {
			return fmt.Errorf("object 'result' is not a variable, but %T", v)
		}
		placeholder, ok := v.Value.(*object.SymbolicPlaceholder)
		if !ok {
			return fmt.Errorf("expected value of 'result' to be a SymbolicPlaceholder, got %T", v.Value)
		}

		wantUnresolvedType := &scanner.TypeInfo{
			PkgPath:    "example.com/me/foreign/lib",
			Name:       "ForeignType",
			Unresolved: true,
			Kind:       scanner.InterfaceKind, // Kind is inferred from the type assertion.
		}
		if diff := cmp.Diff(wantUnresolvedType, placeholder.TypeInfo()); diff != "" {
			t.Errorf("result placeholder TypeInfo mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestShallowScan_CompositeLit_WithUnresolvedType(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

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
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		pkg := findPackage(t, pkgs, "example.com/me")
		for _, file := range pkg.AstFiles {
			evaluator.Eval(ctx, file, nil, pkg)
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, pkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		// Check the unresolved variable
		obj, ok := pkgEnv.Get("x")
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
		if diff := cmp.Diff("example.com/me/foreign/lib", fieldType.FullImportPath); diff != "" {
			return fmt.Errorf("FieldType.FullImportPath mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("ForeignType", fieldType.TypeName); diff != "" {
			return fmt.Errorf("FieldType.TypeName mismatch (-want +got):\n%s", diff)
		}

		// Check that evaluation continued past the unresolved type.
		if _, ok := pkgEnv.Get("sentinel"); !ok {
			return fmt.Errorf("sentinel variable not found, evaluation may have stopped prematurely")
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

// findPackage is a test helper to find a package by its import path.
func findPackage(t *testing.T, pkgs []*goscan.Package, path string) *goscan.Package {
	t.Helper()
	for _, pkg := range pkgs {
		if pkg.ImportPath == path {
			return pkg
		}
	}
	t.Fatalf("package %q not found in any of the loaded packages", path)
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

func TestShallowScan_FindMethodOnUnresolvedEmbeddedType(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

type MyStruct struct {
	lib.ForeignType // Embedded unresolved type
}

func DoCall() {
	var s MyStruct
	s.ForeignMethod() // This method call should not be found, and should not crash.
	Sentinel()
}

func Sentinel() {}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
		"foreign/lib/lib.go": `
package lib
type ForeignType struct{}
func (f ForeignType) ForeignMethod() {}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me.Sentinel", func(ctx context.Context, args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		mainPkg := findPackage(t, pkgs, "example.com/me")

		// Evaluate package-level declarations first
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, nil, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		// Now, call the function that performs the calls.
		fnObj, ok := pkgEnv.Get("DoCall")
		if !ok {
			return fmt.Errorf("DoCall function not found")
		}
		fn := fnObj.(*object.Function)

		result := evaluator.Apply(ctx, fn, nil, mainPkg, pkgEnv)
		if result != nil && result.Type() == object.ERROR_OBJ {
			t.Fatalf("Apply() returned an error, but it should have succeeded symbolically: %s", result.Inspect())
		}

		// Since the method call is now handled symbolically, the Sentinel() function
		// in the test code SHOULD be reached.
		if !sentinelReached {
			t.Errorf("sentinel was not reached, evaluation may have stopped prematurely")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
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
		"go.mod":         "module example.com/me",
		"mypkg/code.go":  mypkg_code,
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
		evaluator.RegisterIntrinsic("example.com/me/mypkg.Sentinel", func(ctx context.Context, args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		// First, evaluate the whole package to populate top-level declarations.
		mainPkg := findPackage(t, pkgs, "example.com/me/mypkg")
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, nil, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		loadedPkg, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
		if err != nil {
			return fmt.Errorf("failed to get loaded package: %w", err)
		}
		pkgEnv := loadedPkg.Env

		// Now, find the function and call it.
		fnObj, ok := pkgEnv.Get("MyFunction")
		if !ok {
			return fmt.Errorf("MyFunction not found")
		}
		fn := fnObj.(*object.Function)

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

		result := evaluator.Apply(ctx, fn, []object.Object{arg1, arg2}, mainPkg, pkgEnv)
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
			Kind:       scanner.UnknownKind,
		}

		// Check the value from the pointer dereference
		pValObj, ok := pkgEnv.Get("P_val")
		if !ok {
			return fmt.Errorf("variable 'P_val' not found")
		}
		pValVar := pValObj.(*object.Variable)
		pValPlaceholder := pValVar.Value.(*object.SymbolicPlaceholder)

		if diff := cmp.Diff(wantUnresolvedType, pValPlaceholder.TypeInfo()); diff != "" {
			t.Errorf("P_val placeholder TypeInfo mismatch (-want +got):\n%s", diff)
		}

		// Check the value from the slice index
		sValObj, ok := pkgEnv.Get("S_val")
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

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
