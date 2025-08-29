package evaluator_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// writeTestFiles is a helper to create a temporary directory and populate it with files.
func writeTestFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpdir := t.TempDir()

	for name, content := range files {
		path := filepath.Join(tmpdir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll failed for %q: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile failed for %q: %v", path, err)
		}
	}
	return tmpdir
}

// This test confirms that the evaluator correctly handles global variable
// initialization order, even when the `var` declaration appears after the
// `init` function that uses it in the source file.
func TestEval_GlobalVarInitializationOrder(t *testing.T) {
	ctx := context.Background()

	const libSrc = `
package lib
type Label struct { Value string }
func (l *Label) Set(s string) { l.Value = s }
func NewLabel(name string) *Label { return &Label{Value: name} }
`
	// The main.go source is written with the init() function physically appearing
	// before the var declaration. This ensures that we are testing the evaluator's
	// adherence to Go's init order rules, not just file parse order.
	const mainSrc = `
package main
import "example.com/test/lib"
func init() {
	l.Set("hello")
}
var l = lib.NewLabel("app")
func main() {}
`
	const modSrc = "module example.com/test\n"

	tmpdir := writeTestFiles(t, map[string]string{
		"go.mod":     modSrc,
		"lib/lib.go": libSrc,
		"main.go":    mainSrc,
	})

	s, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
	)
	if err != nil {
		t.Fatalf("New() with temp dir failed: %v", err)
	}

	// 1. Setup the interpreter
	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %+v", err)
	}

	// 2. Scan the main package.
	mainPkg, err := s.ScanPackageByImport(ctx, "example.com/test")
	if err != nil {
		t.Fatalf("ScanPackageByImport(example.com/test) failed: %+v", err)
	}

	// 3. Evaluate all files in the package. With the fix, this should succeed.
	for _, file := range mainPkg.AstFiles {
		if _, evalErr := interp.Eval(ctx, file, mainPkg); evalErr != nil {
			t.Fatalf("Eval failed unexpectedly: %v", evalErr)
		}
	}

	// 4. Find the init function object
	obj, ok := interp.FindObject("init")
	if !ok {
		t.Fatalf("could not find 'init' function object in interpreter")
	}
	initFunc, ok := obj.(*object.Function)
	if !ok {
		t.Fatalf("object 'init' is not a function, but %T", obj)
	}

	// 5. Try to apply the init function. With the fix, this should not produce an error.
	result, applyErr := interp.Apply(ctx, initFunc, nil, mainPkg)
	if applyErr != nil {
		t.Fatalf("interp.Apply returned an unexpected error: %v", applyErr)
	}

	// 6. Assert that NO error occurred.
	if result != nil && result.Type() == object.ERROR_OBJ {
		t.Errorf("Apply(init) failed unexpectedly with object: %v", result)
	}

	// 7. As a final check, inspect the variable in the environment to see if it was affected.
	vObj, ok := interp.FindObject("l")
	if !ok {
		t.Fatalf("global variable 'l' not found in interpreter's final state")
	}
	variable, ok := vObj.(*object.Variable)
	if !ok {
		t.Fatalf("object 'l' is not a variable, but %T", vObj)
	}

	// The value of `l` is a pointer to an instance.
	ptr, ok := variable.Value.(*object.Pointer)
	if !ok {
		t.Fatalf("variable 'l' is not a pointer, but %T", variable.Value)
	}
	instance, ok := ptr.Value.(*object.Instance)
	if !ok {
		t.Fatalf("variable 'l' does not point to an instance, but %T", ptr.Value)
	}
	// Note: We cannot easily check the fields of the symbolic instance here,
	// as `l.Set("hello")` modifies state within the symbolic world, which is
	// not directly reflected back to concrete Go values in this test setup.
	// The primary goal is to ensure the call doesn't fail.
	_ = instance // use instance
}
