package minigo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/scantest"
)

func TestInterpreter_WithReplaceDirective_PointingOutsideModule(t *testing.T) {
	// This test simulates the exact scenario from `examples/docgen`.
	// A nested module uses a `replace` directive to point to the parent module.
	// With the fixes in `goscan.go`, this should now succeed.

	// 1. Define the nested module structure.
	files := map[string]string{
		"parent/go.mod": `
module example.com/parent
go 1.21
`,
		"parent/lib/lib.go": `
package lib
const Message = "Hello from parent lib"
`,
		"parent/child/go.mod": `
module example.com/child
go 1.21
replace example.com/parent => ../
`,
		"parent/child/main.go": `
package main
import "example.com/parent/lib"
var Result = lib.Message
`,
	}

	// 2. Write the files to a temporary directory.
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 3. Change into the child module's directory. This is crucial.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %+v", err)
	}
	if err := os.Chdir(filepath.Join(dir, "parent", "child")); err != nil {
		t.Fatalf("failed to chdir to temp dir: %+v", err)
	}
	defer os.Chdir(wd)

	// 4. Set up the interpreter and evaluate the script.
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	scriptContent := files["parent/child/main.go"]
	_, err = interp.EvalString(scriptContent)

	if err != nil {
		t.Fatalf("Eval() was expected to succeed, but it failed: %+v", err)
	}

	// 5. Assert that the script ran correctly and the result is as expected.
	resultObj, ok := interp.GlobalEnvForTest().Get("Result")
	if !ok {
		t.Fatalf("could not find 'Result' variable in interpreter environment")
	}

	var result string
	res := minigo.Result{Value: resultObj}
	if err := res.As(&result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	expected := "Hello from parent lib"
	if result != expected {
		t.Errorf("unexpected result from script:\n  want: %q\n  got:  %q", expected, result)
	}
}

// IsImportError is a helper to check if an error is an import error.
func IsImportError(err error) bool {
	e := err.Error()
	// The expected error from go-scan not finding the package.
	return strings.Contains(e, "no required module provides") || strings.Contains(e, "cannot find package")
}
