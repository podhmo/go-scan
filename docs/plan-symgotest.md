# `symgotest`: A Test Helper Package for `symgo`

## 1. Introduction & Motivation

The `symgo` symbolic execution engine and its underlying `evaluator` require a fair amount of setup for testing. A typical test involves:
1.  Writing Go source code to a temporary file.
2.  Creating a `go.mod` file to define a module.
3.  Initializing a `goscan.Scanner` with the correct module paths.
4.  Scanning the temporary package.
5.  Creating a `symgo.Interpreter`.
6.  Registering mock "intrinsic" functions to handle external calls.
7.  Evaluating the code or applying a specific function.
8.  Asserting the result.

This leads to verbose tests with a lot of repeated boilerplate, obscuring the actual intent of the test. The goal of the `symgotest` package is to abstract away this boilerplate, providing a simple, fluent API for writing clear and concise tests for `symgo`.

This package is designed to replace the manual `scantest.Run` pattern with a higher-level `Runner` that is specifically tailored for `symgo`.

## 2. Core API: The `Runner`

The main entry point to the package is the `symgotest.Runner`. It provides a fluent interface to configure and run a symbolic execution test on a snippet of Go code.

### `NewRunner(t *testing.T, source string) *Runner`

This function creates a new test runner.

-   `t`: The `*testing.T` instance for the current test.
-   `source`: A string containing the Go source code for the test. The runner automatically ensures the code is part of a `main` package, so you do not need to write `package main` in your source string.

**Example:**
```go
source := `
func add(a, b int) int {
    return a + b
}
`
runner := symgotest.NewRunner(t, source)
```

### `WithSetup(setupFunc func(interp *symgo.Interpreter)) *Runner`

This method allows you to perform custom configuration on the `symgo.Interpreter` before it executes any code. Its primary use case is to register intrinsic functions for mocking dependencies or tracking calls.

-   `setupFunc`: A function that receives the `*symgo.Interpreter` instance.

**Example:**
```go
var wasCalled bool
setup := func(interp *symgo.Interpreter) {
    // The module name is fixed as "example.com/symgotest/module"
    interp.RegisterIntrinsic("example.com/symgotest/module.myintrinsic",
        func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
            wasCalled = true
            return object.NIL
        },
    )
}

runner.WithSetup(setup)
```

### `Apply(funcName string, args ...object.Object) object.Object`

This is the main execution method. It runs the entire test lifecycle (file creation, scanning, interpretation) and applies the specified function from your source code.

-   `funcName`: The name of the function to execute (e.g., `"main"`).
-   `args`: A variadic list of `object.Object` arguments to pass to the function.

It returns the `object.Object` that results from the function call. This is often an `*object.ReturnValue` or an `*object.Error`.

**Example:**
```go
// Continuing from above...
result := runner.Apply("add", &object.Integer{Value: 5}, &object.Integer{Value: 10})

// Now, assert the result...
```

## 3. Assertion Helpers

To simplify test validation and avoid a dependency on assertion libraries like `testify`, `symgotest` provides a set of simple, standalone assertion helpers. All helpers take `*testing.T` as their first argument and fail the test with a descriptive message if the assertion fails.

-   `AssertSuccess(t, obj)`: Fails if `obj` is an `*object.Error`.
-   `AssertError(t, obj, contains)`: Fails if `obj` is not an `*object.Error`. If `contains` is not empty, it also checks that the error message includes the substring.
-   `AssertInteger(t, obj, expected)`: Fails if `obj` is not an `*object.Integer` with the `expected` value.
-   `AssertString(t, obj, expected)`: Fails if `obj` is not an `*object.String` with the `expected` value.
-   `AssertSymbolicNil(t, obj)`: Fails if `obj` is not the special `object.NIL`.
-   `AssertPlaceholder(t, obj)`: Fails if `obj` is not an `*object.SymbolicPlaceholder`.
-   `AssertEqual(t, want, got)`: A generic helper that uses `github.com/google/go-cmp/cmp` to compare any two values, failing the test if there is a difference.

## 4. Usage Example: Before and After

To illustrate the benefit of `symgotest`, here is a comparison of a test written manually with `scantest` and the same test written with the new `symgotest.Runner`.

### Before `symgotest`

```go
func TestExecution_Manual(t *testing.T) {
	source := `
package main

func main() {
	doSomethingImportant()
}
func doSomethingImportant() {}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	var intrinsicCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		interp.RegisterIntrinsic("mymodule.doSomethingImportant",
            func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
			    intrinsicCalled = true
			    return nil
		    },
        )

		mainFile := pkgs[0].AstFiles[filepath.Join(dir, "main.go")]
		if _, err := interp.Eval(ctx, mainFile, pkgs[0]); err != nil {
			// handle error
		}

		mainFn, ok := interp.FindObjectInPackage(ctx, "mymodule", "main")
		if !ok {
			return fmt.Errorf("main not found")
		}

		_, err = interp.Apply(ctx, mainFn, nil, pkgs[0])
		if err != nil {
			return err
		}

		if !intrinsicCalled {
			return fmt.Errorf("expected intrinsic to be called")
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
```

### After `symgotest`

```go
func TestExecution_WithSymgoTest(t *testing.T) {
	source := `
func main() {
	doSomethingImportant()
}
func doSomethingImportant() {}
`
	var intrinsicCalled bool
	setup := func(interp *symgo.Interpreter) {
		interp.RegisterIntrinsic("example.com/symgotest/module.doSomethingImportant",
            func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
			    intrinsicCalled = true
			    return object.NIL
		    },
        )
	}

	runner := symgotest.NewRunner(t, source).WithSetup(setup)
	result := runner.Apply("main")

	symgotest.AssertSuccess(t, result)
	symgotest.AssertEqual(t, true, intrinsicCalled)
}
```

The "After" version is significantly shorter, easier to read, and focuses on the core logic of the test: the source code, the setup, and the assertion.
