package symgo_test

import (
	"context"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestWithSymbolicDependencyScope(t *testing.T) {
	ctx := context.Background()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/myapp\ngo 1.21",
		"main.go": `
package main
import "example.com/myapp/lib"
func main() { lib.DoSomething() }
`,
		"lib/lib.go": `
package lib
func DoSomething() {}
`,
	})
	defer cleanup()

	// The scanner is configured without any special options.
	scanner, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	// Create an interpreter with 'lib' in the symbolic dependency scope.
	// This should instruct the underlying scanner to not parse function bodies for this package.
	interp, err := symgo.NewInterpreter(scanner,
		symgo.WithSymbolicDependencyScope("example.com/myapp/lib"),
	)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	// Scan the 'lib' package.
	libPkg, err := interp.Scanner().ScanPackageByImport(ctx, "example.com/myapp/lib")
	if err != nil {
		t.Fatalf("ScanPackageByImport for lib failed: %v", err)
	}

	// Verify that the function body is nil because it was in the symbolic scope.
	doSomethingFunc := findFunc(t, libPkg, "DoSomething")
	if doSomethingFunc.AstDecl.Body != nil {
		t.Errorf("expected function body to be nil for symbolic dependency, but it was not")
	}
}

func TestWithPrimaryAnalysisScope(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"myapp/go.mod": "module example.com/myapp\ngo 1.21\nreplace example.com/lib => ../lib",
		"myapp/main.go": `
package main
import "example.com/lib"
func main() string { return lib.DoSomething() }
`,
		"lib/go.mod": "module example.com/lib\ngo 1.21",
		"lib/lib.go": `
package lib
func DoSomething() string { return "from lib" }
`,
	}
	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// The directory for the scanner should be the main application module.
	appDir := filepath.Join(tmpdir, "myapp")

	t.Run("in-scope", func(t *testing.T) {
		// The primary scope includes the main app and the 'lib' dependency.
		// Calls to 'lib' should be deeply evaluated.
		result := runMainAnalysis(t, ctx, appDir, "example.com/myapp/...", "example.com/lib/...")
		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			t.Fatalf("expected ReturnValue, got %T: %v", result, result.Inspect())
		}
		str, ok := retVal.Value.(*object.String)
		if !ok {
			t.Fatalf("expected String, got %T", retVal.Value)
		}
		if str.Value != "from lib" {
			t.Errorf("want %q, got %q", "from lib", str.Value)
		}
	})

	t.Run("out-of-scope", func(t *testing.T) {
		// The primary scope only includes the main app.
		// Calls to 'lib' should be treated as symbolic placeholders.
		result := runMainAnalysis(t, ctx, appDir, "example.com/myapp/...")
		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			t.Fatalf("expected ReturnValue, got %T: %v", result, result.Inspect())
		}
		if _, ok := retVal.Value.(*object.SymbolicPlaceholder); !ok {
			t.Errorf("expected SymbolicPlaceholder for out-of-scope call, got %T", retVal.Value)
		}
	})
}

// runMainAnalysis is a helper to analyze the main package and return the result of main().
func runMainAnalysis(t *testing.T, ctx context.Context, dir string, primaryScope ...string) object.Object {
	t.Helper()
	scanner, err := goscan.New(
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	interp, err := symgo.NewInterpreter(scanner,
		symgo.WithPrimaryAnalysisScope(primaryScope...),
	)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	mainPkg, err := interp.Scanner().ScanPackageByImport(ctx, "example.com/myapp")
	if err != nil {
		t.Fatalf("ScanPackageByImport for main failed: %v", err)
	}

	mainFile := findFile(t, mainPkg, "main.go")
	_, err = interp.Eval(ctx, mainFile, mainPkg)
	if err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}

	mainFuncObj, ok := interp.FindObject("main")
	if !ok {
		t.Fatal("main function not found")
	}
	mainFunc, ok := mainFuncObj.(*symgo.Function)
	if !ok {
		t.Fatalf("main is not a function, but %T", mainFuncObj)
	}

	result, err := interp.Apply(ctx, mainFunc, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply main function failed: %v", err)
	}
	return result
}
