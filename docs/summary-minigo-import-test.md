# Testing `minigo` and `symgo` with Local Go Imports

When writing tests for tools that use the `minigo` or `symgo` interpreters, a common challenge arises when the script or Go code being analyzed needs to import Go packages from the test's local context. This is especially true when the test sets up its own Go module (e.g., in a `testdata` directory) that is nested within the main project's module.

This document outlines the core problem and provides a robust, validated solution using the `scantest` testing library and by creating temporary modules on the filesystem.

## The Problem: Mismatched Module Contexts

The `minigo` and `symgo` interpreters rely on an underlying `go-scan.Scanner` to resolve `import` statements and symbols to actual Go packages on the filesystem. The scanner, in turn, uses a `locator` to find the correct `go.mod` file, which defines the module's name and its dependencies (including `replace` directives).

The entire resolution process depends on the scanner being configured with the correct **working directory** (`WorkDir`). If the scanner's `WorkDir` is not set to the root of the module containing the code to be analyzed, it will fail to find the correct `go.mod` file and will be unable to resolve local import paths.

This problem frequently occurs in tests where a temporary, nested module is created. The test runner's context is the main project, but the code needs to be evaluated within the context of the temporary module. Simply using an in-memory file overlay is often insufficient, as the `locator` needs to `stat` real directories to resolve import paths.

## The Solution: The `scantest` Library and Temporary Directories

The recommended approach is to use the `scantest` library's `WriteFiles` helper to create a complete, temporary Go module on the filesystem for each test. This gives the `go-scan` `Locator` a real directory structure to work with, ensuring that module and import paths can be resolved correctly.

The following example from `symgo/evaluator/evaluator_test.go` demonstrates this pattern.

### Test Implementation

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

### Key Takeaways from this Pattern

- **`scantest.WriteFiles`**: This is the preferred way to create a hermetic test environment. It avoids polluting the project's `testdata` directory and ensures tests are fully isolated.
- **`scantest.Run`**: This function is the orchestrator. It creates the `go-scan.Scanner` with the `WorkDir` correctly set to the temporary directory, so you don't have to configure it manually.
- **Action Function**: The core logic of your test goes inside the `action` function, which receives the pre-configured scanner.
- **No In-Memory Overlay Needed (for source files)**: Because the files are real, you don't need to use `goscan.WithOverlay` for the source code, which simplifies the test setup and avoids `locator` issues.

This method combines the robustness of a real filesystem with the convenience and isolation of temporary directories, making it a highly effective way to test tools built with `go-scan` and `symgo`.
