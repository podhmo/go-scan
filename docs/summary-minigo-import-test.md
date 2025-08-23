# Testing `minigo` Scripts with Local Go Imports

When writing tests for tools that use the `minigo` interpreter, a common challenge arises when the `minigo` script itself needs to import Go packages from the test's local context. This is especially true when the test sets up its own Go module (e.g., in a `testdata` directory) that is nested within the main project's module.

This document outlines the core problem and provides a robust, validated solution using the `scantest` testing library.

## The Problem: Mismatched Module Contexts

The `minigo` interpreter relies on an underlying `go-scan.Scanner` to resolve `import` statements to actual Go packages on the filesystem. The scanner, in turn, uses a `locator` to find the correct `go.mod` file, which defines the module's name and its dependencies (including `replace` directives).

The entire resolution process depends on the scanner being configured with the correct **working directory** (`WorkDir`). If the scanner's `WorkDir` is not set to the root of the module containing the `minigo` script, it will fail to find the correct `go.mod` file and will be unable to resolve local import paths.

This problem frequently occurs in tests where a temporary, nested module is created. The test runner's context is the main project, but the `minigo` script needs to be evaluated within the context of the temporary module.

## The Solution: The `scantest` Library and External Test Data

The recommended approach is to place your test modules in the `testdata` directory and use the `scantest` library to configure the scanner.

- **External Files**: Keeping test data (like `go.mod` files, Go source, and `minigo` scripts) as actual files in `testdata` makes tests cleaner and easier to maintain than defining file content as strings inside the test.
- **`scantest.Run`**: This helper function automatically creates and configures a `go-scan.Scanner` with the correct context for your test. It handles finding the module root and creating an in-memory "overlay" for `go.mod` files to correctly resolve relative `replace` paths to absolute ones, making tests portable and reliable.

The following example from `examples/docgen/integration_test.go` demonstrates how to test a `minigo` script located in a nested module that needs to import packages from parent modules.

### The Test Scenario

- **File Structure**: A test-specific module is located at `examples/docgen/testdata/integration/fn-patterns/`.
- **Imports**: A `minigo` script inside `fn-patterns` needs to import:
    1. A local package from within `fn-patterns` (`.../api`).
    2. A package from the parent `docgen` module (`.../docgen/patterns`).
- **Resolution**: This is achieved with `replace` directives in the `fn-patterns/go.mod` file.

### Test Implementation

The test code itself is very clean. It simply points to the test module's directory and lets `scantest.Run` handle the complex setup.

**`examples/docgen/integration_test.go`:**
```go
func TestDocgen_WithFnPatterns(t *testing.T) {
	// This test reproduces the scenario from docs/trouble-docgen-minigo-import.md.
	// It verifies that docgen can load a minigo configuration script (`patterns.go`)
	// from a nested Go module (`testdata/integration/fn-patterns`), and that this
	// script can successfully import other packages.

	// 1. Define the path to the nested module we want to test.
	moduleDir := filepath.Join("testdata", "integration", "fn-patterns")
	patternsFile := filepath.Join(moduleDir, "patterns.go")

	// 2. Define the test logic in an action function.
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// `s` is the pre-configured scanner from scantest.Run.
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

		// 3. Call the docgen loader, which uses the scanner.
		_, err := LoadPatternsFromConfig(patternsFile, logger, s)
		return err // If err is nil, the import was successful.
	}

	// 4. Use scantest.Run to drive the test.
	// It's crucial to set the module root to `moduleDir` so the scanner
	// finds the correct `go.mod` and correctly resolves the `replace` directives.
	if _, err := scantest.Run(t, moduleDir, nil, action, scantest.WithModuleRoot(moduleDir)); err != nil {
		t.Fatalf("scantest.Run() failed, indicating a failure in loading patterns: %+v", err)
	}
}
```

### Test Data Setup

The key to making this work is the correct setup of the files in `testdata/integration/`.

**`testdata/integration/fn-patterns/go.mod`:**
The `replace` directives are the most critical part. The relative paths must correctly point from this file to the directories of the modules being replaced.

```
module github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns

go 1.24

// Path from fn-patterns -> docgen is 3 levels up.
replace github.com/podhmo/go-scan/examples/docgen => ../../../

// Path from fn-patterns -> go-scan root is 5 levels up.
replace github.com/podhmo/go-scan => ../../../../../
```

**`testdata/integration/fn-patterns/patterns.go`:**
This `minigo` script can now successfully import from both the `docgen` module and its own local `api` package.
```go
package patterns

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api"
)

var Patterns = []patterns.PatternConfig{
	{
		Key:      "github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api.GetFoo",
		Type:     patterns.RequestBody,
		ArgIndex: 1,
	},
}
```

## Common Errors and Troubleshooting

- **Error:** `could not find package directory ...`
  - **Cause:** This almost always means the `go-scan.Scanner` has the wrong module context. The most common reasons are:
      1. Not using `scantest.WithModuleRoot()` to point to the correct nested module directory.
      2. The relative paths in your `replace` directives are wrong. Carefully count the `../` segments needed to get from your `go.mod` file to the root of the module you are replacing.
  - **Solution:** Use the working example above as a template. Double-check your `replace` paths. Add debug logging to see which directory the scanner is using as its `WorkDir`.

---

## Alternative: Self-Contained Tests with In-Memory Files

For smaller, more focused unit tests, creating physical files in `testdata` can be cumbersome. The `scantest` package offers another powerful function, `scantest.WriteFiles`, which allows you to define a complete, isolated test module as a set of in-memory strings.

This approach is ideal for testing specific features of a tool without needing any external file dependencies.

### Example: Verifying `docgen` Key Generation

This example shows how to test `docgen`'s ability to create a matching key from a type-safe `Fn` reference.

**1. Define Test Module as String Constants**

Instead of creating physical files, define their content inside your test file.

```go
// in my_test.go
package main

const testGoMod = `
module my-test-module

go 1.21

// The replace directive is crucial. The path should be relative to where
// the temporary test directory will be created inside the project.
replace github.com/podhmo/go-scan => "../../../"
`

const testFooGo = `
package foo
type Foo struct{}
func (f *Foo) Bar() {}
`

const testPatternsGo = `
//go:build minigo
package main
import "my-test-module/foo"
// ... (rest of patterns.go) ...
`
```

**2. Use `scantest.WriteFiles` and `scantest.Run`**

The test function orchestrates the setup and execution.

```go
// in my_test.go
func TestKeyFromFnWithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod":      testGoMod,
		"foo/foo.go":  testFooGo,
		"patterns.go": testPatternsGo,
	}

	// 1. `scantest.WriteFiles` creates a temp directory with the file layout.
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 2. The action function contains the core test logic.
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// `s` is a scanner pre-configured by scantest.Run to use the temp dir.
		// ... your test logic here ...
		// e.g., call your tool's main logic with the scanner
		return nil
	}

	// 3. `scantest.Run` handles the setup and execution.
	// It finds the go.mod in `dir`, processes the replace directive,
	// and provides a correctly configured scanner to the action.
	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}
}
```

This method combines the robustness of `replace` directives with the convenience of self-contained, in-memory test definitions, making it a highly effective way to test tools built with `go-scan`.

---

## Addendum: Testing `symgo` with `scantest`

The patterns described above apply equally well to testing the `symgo` symbolic execution engine, which is built on top of `go-scan`. The key is to ensure the `symgo.Interpreter` receives a `goscan.Scanner` that is correctly configured for the test's module context.

The `scantest.Run` function is the ideal way to achieve this.

### Example: Testing a Method Call in `symgo`

This example from `symgo/evaluator/evaluator_test.go` shows how to test that the evaluator can correctly identify and dispatch a method call on a struct.

**`symgo/evaluator/evaluator_test.go`:**
```go
func TestEvalCallExpr_MethodCallOnStruct(t *testing.T) {
	source := `
package main

type Greeter struct {}
func (g *Greeter) Greet() string { return "hello" }

func main() {
	g := &Greeter{}
	g.Greet()
}
`
	// 1. Define the test module's file layout.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var greetCalled bool

	// 2. Define the test logic in an action function.
	// scantest.Run will provide a correctly configured scanner.
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil) // Create the evaluator with the test scanner.

		// 3. Set up test-specific behavior, like an intrinsic to track a method call.
		eval.RegisterIntrinsic("(*example.com/me.Greeter).Greet", func(args ...object.Object) object.Object {
			greetCalled = true
			return &object.String{Value: "intrinsic hello"}
		})

		// 4. Run the evaluation logic.
		env := object.NewEnvironment()
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg) // Evaluate file to define symbols.
		mainFunc, _ := env.Get("main")
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos) // Execute main.

		// 5. Assert the outcome.
		if !greetCalled {
			return fmt.Errorf("Greet method was not called")
		}
		return nil
	}

	// 6. Use scantest.Run to drive the test.
	// It automatically finds the go.mod in `dir` and configures the scanner.
	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
```

### Key Takeaways for `symgo` Testing

- **Isolate Failures**: Creating small, focused tests like this for the `symgo` evaluator is critical for debugging. When a complex tool like `find-orphans` fails, these small tests can prove whether the underlying engine or the tool's usage of it is the problem.
- **Use Intrinsics for Mocks**: The intrinsic system is the perfect mechanism for mocking functions and methods to verify that they were called during symbolic execution.
- **`scantest` is Essential**: For any test involving cross-package resolution or method calls (which requires the scanner to find type definitions), using `scantest.Run` to create a temporary module on the filesystem is the most reliable approach.
