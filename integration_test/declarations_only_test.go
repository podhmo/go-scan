package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
)

func TestDeclarationsOnly(t *testing.T) {
	mainCode := `
package main
import "example.com/me/foreign/lib"
var x lib.ForeignType
func main() {
	lib.DoSomething()
}
`
	foreignCode := `
package lib
type ForeignType struct {
	ID string
}
func DoSomething() {
	// This call should NOT be seen by symgo if declarations-only is working.
	ShouldNotBeCalled()
}
func ShouldNotBeCalled() {}
`
	files := map[string]string{
		"go.mod":             "module example.com/me",
		"main.go":            mainCode,
		"foreign/lib/lib.go": foreignCode,
	}

	dir, cleanup := writeTestFiles(t, files)
	defer cleanup()

	var shouldNotBeCalledReached bool

	// Since this is a true integration test, we set up the scanner
	// and symgo interpreter as a user of the library would.
	scanOpts := []goscan.ScannerOption{
		goscan.WithWorkDir(dir),
		goscan.WithDeclarationsOnlyPackages([]string{"example.com/me/foreign/lib"}),
	}
	s, err := goscan.New(scanOpts...)
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	pkgs, err := s.Scan(context.Background(), "./...")
	if err != nil {
		t.Fatalf("s.Scan() failed: %v", err)
	}

	// Action part of the test
	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("could not create symgo interpreter: %v", err)
	}

	interp.RegisterIntrinsic("example.com/me/foreign/lib.ShouldNotBeCalled", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		shouldNotBeCalledReached = true
		return nil
	})

	mainPkg := findPackage(t, pkgs, "example.com/me")
	for _, file := range mainPkg.AstFiles {
		if _, err := interp.Eval(context.Background(), file, mainPkg); err != nil {
			t.Fatalf("Eval() returned an error: %v", err)
		}
	}

	varX, ok := interp.FindObject("x")
	if !ok {
		t.Fatal("variable 'x' not found in symgo environment")
	}
	xVar, ok := varX.(*symgo.Variable)
	if !ok {
		t.Fatalf("object 'x' is not a symgo variable, but %T", varX)
	}

	typeInfo := xVar.ResolvedTypeInfo
	if typeInfo == nil || typeInfo.Unresolved {
		t.Fatalf("type of 'x' should be resolved, but got: %v", typeInfo)
	}
	wantName := "example.com/me/foreign/lib.ForeignType"
	gotName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if gotName != wantName {
		t.Fatalf("type name mismatch, want %q, got %q", wantName, gotName)
	}

	mainFuncObj, ok := interp.FindObject("main")
	if !ok {
		t.Fatal("main function not found")
	}
	mainFunc, ok := mainFuncObj.(*symgo.Function)
	if !ok {
		t.Fatalf("main is not a function object")
	}

	if _, err := interp.Apply(context.Background(), mainFunc, nil, mainPkg); err != nil {
		t.Fatalf("Apply(main) returned an error: %v", err)
	}

	if shouldNotBeCalledReached {
		t.Error("intrinsic for ShouldNotBeCalled was reached, but it should have been ignored")
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

func writeTestFiles(t *testing.T, files map[string]string) (string, func()) {
	t.Helper()
	tmpdir, err := os.MkdirTemp("", "goscan_integration_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	for name, content := range files {
		path := filepath.Join(tmpdir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create parent dir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", name, err)
		}
	}

	return tmpdir, func() { os.RemoveAll(tmpdir) }
}
