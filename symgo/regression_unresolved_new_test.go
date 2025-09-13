package symgo_test

import (
	"context"
	"testing"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestRegression_NewOnUnresolvedType(t *testing.T) {
	// This test reproduces the original bug where calling `new()` on a type
	// from an unscanned package would cause a panic when the resulting pointer
	// was dereferenced.

	// 1. Define the source code for the test.
	// `main` imports `ext`, but our scan policy will prevent `ext` from being scanned.
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
import "example.com/me/ext"
func main() {
	// 'p' will be a pointer to an unresolved type.
	p := new(ext.SomeType)
	// Dereferencing 'p' caused the crash.
	_ = *p
}`,
		"ext/ext.go": `
package ext
// SomeType is the type that will remain unresolved.
type SomeType struct {
	Name string
}`,
	}

	// 2. Set up the test directory with the source files.
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 3. Define the test action.
	action := func(ctx context.Context, scannerInstance *scan.Scanner, scannedPackages []*scan.Package) error {
		// The scanner `scannerInstance` is already configured by scantest.Run.
		// We now create the symgo interpreter.
		interp, err := symgo.NewInterpreter(
			scannerInstance,
			// This policy is the key to the test: we only allow the `main` package to be scanned.
			// This ensures that `ext.SomeType` is an unresolved symbol.
			symgo.WithScanPolicy(func(pkgPath string) bool {
				return pkgPath == "example.com/me"
			}),
		)
		if err != nil {
			t.Fatalf("symgo.NewInterpreter() failed: %v", err)
		}

		// Get the main package
		mainPkg := scannedPackages[0]

		// First, evaluate the entire package to populate the environment with function definitions.
		// We create a new environment for this evaluation.
		env := object.NewEnclosedEnvironment(interp.GlobalEnvForTest())
		for _, fileAst := range mainPkg.AstFiles {
			if _, err := interp.EvalWithEnv(ctx, fileAst, env, mainPkg); err != nil {
				t.Fatalf("interp.EvalWithEnv() failed during setup: %v", err)
			}
		}

		// Now, look up the main function from the package's environment.
		// The package environment is managed internally by the evaluator.
		pkgEnv, ok := interp.EvaluatorForTest().PackageEnvForTest(mainPkg.ImportPath)
		if !ok {
			t.Fatalf("package env for %q not found", mainPkg.ImportPath)
		}

		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			t.Fatal("main function not found in package environment")
		}

		// 4. Apply the main function.
		// The assertion is that this does NOT return an error.
		if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkg); err != nil {
			t.Errorf("interp.Apply() failed unexpectedly: %v", err)
		}
		return nil
	}

	// 5. Run the test case.
	// We point it at the `main` package's directory.
	if _, err := scantest.Run(t, nil, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
