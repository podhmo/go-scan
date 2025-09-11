package symgo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestMismatchImportPackageName_InPolicy(t *testing.T) {
	source := `
package main

import (
	"fmt"
	"example.com/myapp/libs/pkg.v2"
)

type S struct {
	Name string
}

func main() {
	s := S{Name: "foo"}
	b, err := pkg.Marshal(s)
	if err != nil {
		// We don't panic, just trace the error to avoid terminating execution,
		// allowing the test to verify the Println call.
		fmt.Println("error", err)
	}
	fmt.Println(string(b), err)
}
`
	libSource := `
package pkg
func Marshal(v any) ([]byte, error) { return nil, nil }
func Unmarshal(data []byte, v any) error { return nil }
`
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":             "module example.com/myapp\ngo 1.21",
		"main.go":            source,
		"libs/pkg.v2/lib.go": libSource,
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

	// For this test, we mock the behavior of pkg.Marshal
	interp.RegisterIntrinsic("example.com/myapp/libs/pkg.v2.Marshal", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		// Return a multi-return with a placeholder for the bytes and a nil for the error.
		bytesPlaceholder := &symgo.SymbolicPlaceholder{Reason: "result of pkg.Marshal"}
		return &symgo.MultiReturn{Values: []symgo.Object{bytesPlaceholder, object.NIL}}
	})

	got := make(map[string]bool)
	interp.RegisterIntrinsic("fmt.Println", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		if len(args) == 0 {
			return nil
		}
		// The argument might be a ReturnValue from another function call
		val := args[0]
		if rv, ok := val.(*object.ReturnValue); ok {
			val = rv.Value
		}

		if sp, ok := val.(*symgo.SymbolicPlaceholder); ok {
			got[sp.Reason] = true
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

	mainFunc, ok := interp.FindObjectInPackage("example.com/myapp", "main")
	if !ok {
		t.Fatal("main function not found")
	}

	_, err = interp.Apply(context.Background(), mainFunc, nil, pkg)
	if err != nil {
		t.Fatalf("Apply main function failed: %v", err)
	}

	if !got["result of conversion to built-in type string"] {
		t.Errorf("expected to see a symbolic placeholder for string conversion, but it was not found in Println calls")
	}
}

func TestMismatchImportPackageName_OutOfPolicy(t *testing.T) {
	source := `
package main

import (
	"example.com/myapp/libs/pkg.v2"
)

func main() {
	// This call should be traced as a placeholder because pkg is out of policy.
	pkg.Unmarshal(nil, nil)

	// This is an undefined identifier. Because the package 'main' is in-policy,
	// this should still raise an "identifier not found" error.
	xyz.DoSomething()
}
`
	libSource := `
package pkg
func Marshal(v any) ([]byte, error) { return nil, nil }
func Unmarshal(data []byte, v any) error { return nil }
`
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":             "module example.com/myapp\ngo 1.21",
		"main.go":            source,
		"libs/pkg.v2/lib.go": libSource,
	})
	defer cleanup()

	scanner, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	// The primary scope is the main module. The pkg.v2 library is implicitly out of scope.
	interp, err := symgo.NewInterpreter(scanner, symgo.WithPrimaryAnalysisScope("example.com/myapp/..."))
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

	mainFunc, ok := interp.FindObjectInPackage("example.com/myapp", "main")
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

	// Set the primary scope to something that does NOT include the helper package.
	// We can use a dummy path, effectively making the default module policy empty.
	interp, err := symgo.NewInterpreter(scanner, symgo.WithPrimaryAnalysisScope("example.com/not-the-helper"))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	pkg, err := interp.Scanner().ScanPackage(context.Background(), filepath.Join(tmpdir, "helper"))
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	doFunc, ok := interp.FindObjectInPackage("example.com/myapp/helper", "Do")
	if !ok {
		// This is expected since the package is not evaluated yet.
		// We need to Eval the file first.
	}

	helperFile := findFile(t, pkg, "lib.go")
	_, err = interp.Eval(context.Background(), helperFile, pkg)
	if err != nil {
		t.Fatalf("Eval helper file failed: %v", err)
	}

	doFunc, ok = interp.FindObjectInPackage("example.com/myapp/helper", "Do")
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
