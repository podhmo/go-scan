package symgo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

const mismatchTestGoMod = `
module example.com/myapp
go 1.21
require gopkg.in/yaml.v2 v2.4.0
`

func TestMismatchImportPackageName_InPolicy(t *testing.T) {
	source := `
package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
)

type S struct {
	Name string
}

func main() {
	s := S{Name: "foo"}
	b, err := yaml.Marshal(s)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b), err)
}
`
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  mismatchTestGoMod,
		"main.go": source,
	})
	defer cleanup()

	scanner, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	interp, err := symgo.NewInterpreter(scanner)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	// For this test, we mock the behavior of yaml.Marshal
	interp.RegisterIntrinsic("gopkg.in/yaml.v2.Marshal", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		// Return a multi-return with a placeholder for the bytes and a nil for the error.
		bytesPlaceholder := &symgo.SymbolicPlaceholder{Reason: "result of yaml.Marshal"}
		return &symgo.MultiReturn{Values: []symgo.Object{bytesPlaceholder, object.NIL}}
	})

	var got []string
	interp.RegisterIntrinsic("fmt.Println", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		if len(args) > 0 {
			if sp, ok := args[0].(*symgo.SymbolicPlaceholder); ok && sp.Reason == "result of conversion to built-in type string" {
				got = append(got, "<string conversion of symbolic bytes>")
			}
		}
		return nil
	})

	pkg, err := interp.Scanner().ScanPackage(context.Background(), tmpdir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	mainFile := findFile(t, pkg, "main.go")
	_, err = interp.Eval(context.Background(), mainFile, pkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	mainFunc, ok := interp.FindObject("main")
	if !ok {
		t.Fatal("main function not found")
	}

	_, err = interp.Apply(context.Background(), mainFunc, nil, pkg)
	if err != nil {
		t.Fatalf("Apply main function failed: %v", err)
	}

	want := []string{
		"<string conversion of symbolic bytes>",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestMismatchImportPackageName_OutOfPolicy(t *testing.T) {
	source := `
package main

import (
	"gopkg.in/yaml.v2"
)

func main() {
	// This call should be traced as a placeholder because yaml is out of policy.
	yaml.Unmarshal(nil, nil)

	// This is an undefined identifier. Because the package 'main' is in-policy,
	// this should still raise an "identifier not found" error.
	xyz.DoSomething()
}
`
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  mismatchTestGoMod,
		"main.go": source,
	})
	defer cleanup()

	scanner, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	policy := func(importPath string) bool {
		return !strings.HasPrefix(importPath, "gopkg.in/yaml.v2")
	}
	interp, err := symgo.NewInterpreter(scanner, symgo.WithScanPolicy(policy))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	pkg, err := interp.Scanner().ScanPackage(context.Background(), tmpdir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	mainFile := findFile(t, pkg, "main.go")
	_, err = interp.Eval(context.Background(), mainFile, pkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	mainFunc, ok := interp.FindObject("main")
	if !ok {
		t.Fatal("main function not found")
	}

	_, err = interp.Apply(context.Background(), mainFunc, nil, pkg)

	// We expect an error here because xyz is undefined in an in-policy package.
	if err == nil {
		t.Fatal("expected an error for undefined identifier 'xyz', but got nil")
	}
	if !strings.Contains(err.Error(), "identifier not found: xyz") {
		t.Errorf("expected error to be about 'identifier not found: xyz', but got: %v", err)
	}
}

func TestMismatchImportPackageName_UndefinedIdentifier_OutOfPolicy(t *testing.T) {
	source := `
package helper

func Do() {
	// this identifier is undefined in this package, which is out of policy
	xyz.DoSomething()
}
`
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":        "module example.com/myapp\ngo 1.21",
		"helper/lib.go": source,
	})
	defer cleanup()

	scanner, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	// Policy: do not scan the 'helper' package.
	policy := func(importPath string) bool {
		return !strings.HasPrefix(importPath, "example.com/myapp/helper")
	}
	interp, err := symgo.NewInterpreter(scanner, symgo.WithScanPolicy(policy))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	pkg, err := interp.Scanner().ScanPackage(context.Background(), filepath.Join(tmpdir, "helper"))
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	doFunc, ok := interp.FindObject("Do")
	if !ok {
		// This is expected since the package is not evaluated yet.
		// We need to Eval the file first.
	}

	helperFile := findFile(t, pkg, "lib.go")
	_, err = interp.Eval(context.Background(), helperFile, pkg)
	if err != nil {
		t.Fatalf("Eval helper file failed: %v", err)
	}

	doFunc, ok = interp.FindObject("Do")
	if !ok {
		t.Fatal("Do function not found")
	}

	_, err = interp.Apply(context.Background(), doFunc, nil, pkg)
	// THIS is the key assertion. We expect NO error because the undefined identifier
	// was found in a package that is outside our scan policy.
	if err != nil {
		t.Fatalf("Apply should have succeeded by creating a placeholder, but got error: %v", err)
	}
}
