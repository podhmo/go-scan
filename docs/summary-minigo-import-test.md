# Testing `minigo` Scripts with Local Go Imports

When writing tests for tools that use the `minigo` interpreter, a common challenge arises when the `minigo` script itself needs to import Go packages from the test's local context. This is especially true when the test sets up its own Go module (e.g., in a temporary directory or a `testdata` directory) that is nested within the main project's module.

This document outlines the core problem and provides robust, validated solutions using the `scantest` testing library.

## The Problem: Mismatched Module Contexts

The `minigo` interpreter relies on an underlying `go-scan.Scanner` to resolve `import` statements to actual Go packages on the filesystem. The scanner, in turn, uses a `locator` to find the correct `go.mod` file, which defines the module's name and its dependencies (including `replace` directives).

The entire resolution process depends on the scanner being configured with the correct **working directory** (`WorkDir`). If the scanner's `WorkDir` is not set to the root of the module containing the `minigo` script, it will fail to find the correct `go.mod` file and will be unable to resolve local import paths.

This problem frequently occurs in tests where a temporary, nested module is created. The test runner's context is the main project, but the `minigo` script needs to be evaluated within the context of the temporary module.

## The Solution: The `scantest` Library

The `scantest` package is designed to solve this exact problem. Its primary helper, `scantest.Run`, automatically creates and configures a `go-scan.Scanner` with the correct context for your test files. It handles finding the module root and even automatically creates an in-memory "overlay" for `go.mod` files to correctly resolve relative `replace` paths to absolute ones, making tests portable and reliable.

The following examples demonstrate the recommended patterns for testing `minigo` scripts.

### Method 1: Basic Intra-Module Imports

This is the simplest case: a `minigo` script imports a Go package from within the same Go module.

**Scenario:**
- A temporary module `mytest` is created.
- A Go package `mytest/helper` exists within it.
- A `minigo` script `main.mgo` imports `mytest/helper`.

**Implementation (`minigo/minigo_scantest_test.go`):**

```go
func TestMinigo_Scantest_BasicImport(t *testing.T) {
	files := map[string]string{
		"go.mod": "module mytest\n\ngo 1.24\n",
		"helper/helper.go": `package helper

func Greet() string {
	return "hello from helper"
}`,
		"main.mgo": `package main

import "mytest/helper"

var result = helper.Greet()
`,
	}

	// 1. Create the temporary file structure.
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 2. Define the test logic in an action function.
	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		// `s` is the pre-configured scanner from scantest.Run.
		interp, err := NewInterpreter(s)
		if err != nil {
			return err
		}

		mainMgoPath := filepath.Join(dir, "main.mgo")
		source, err := os.ReadFile(mainMgoPath)
		if err != nil {
			return err
		}

		if err := interp.LoadFile("main.mgo", source); err != nil {
			return err
		}

		if _, err := interp.Eval(ctx); err != nil {
			return err
		}

		// 3. Assert the result.
		val, ok := interp.globalEnv.Get("result")
		if !ok {
			return fmt.Errorf("variable 'result' not found")
		}
		got, ok := val.(*object.String)
		if !ok {
			return fmt.Errorf("result is not a string, got=%s", val.Type())
		}
		want := "hello from helper"
		if diff := cmp.Diff(want, got.Value); diff != "" {
			return fmt.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		return nil
	}

	// 4. Execute the test. scantest.Run handles the scanner setup.
	// It correctly identifies `dir` as the module root.
	if _, err := scantest.Run(t, dir, nil, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
```

### Method 2: Nested Module Imports with `replace`

This is the most complex, and most important, scenario. A `minigo` script in a nested module needs to import a package from a parent module.

**Scenario:**
- A root module `my-root` contains a package `rootpkg`.
- A nested module `my-nested-module` exists in a subdirectory.
- The nested module's `go.mod` uses `replace my-root => ../` to find the parent.
- A `minigo` script in the nested module imports `my-root/rootpkg`.

**Implementation (`minigo/minigo_scantest_test.go`):**

```go
func TestMinigo_Scantest_NestedModuleImportWithReplace(t *testing.T) {
	files := map[string]string{
		"go.mod": "module my-root\n\ngo 1.24\n",
		"rootpkg/root.go": `package rootpkg
func GetVersion() string {
	return "v1.0.0"
}`,
		"nested/go.mod": "module my-nested-module\n\ngo 1.24\n\nreplace my-root => ../\n",
		"nested/main.mgo": `package main
import "my-root/rootpkg"
var result = rootpkg.GetVersion()
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	nestedDir := filepath.Join(dir, "nested")

	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		interp, err := NewInterpreter(s)
		if err != nil {
			return err
		}
		mainMgoPath := filepath.Join(nestedDir, "main.mgo")
		source, err := os.ReadFile(mainMgoPath)
		if err != nil {
			return err
		}
		if err := interp.LoadFile("main.mgo", source); err != nil {
			return err
		}
		if _, err := interp.Eval(ctx); err != nil {
			return err
		}

		val, ok := interp.globalEnv.Get("result")
		if !ok {
			return fmt.Errorf("variable 'result' not found")
		}
		got, ok := val.(*object.String)
		if !ok {
			return fmt.Errorf("result is not a string, got=%s", val.Type())
		}
		want := "v1.0.0"
		if diff := cmp.Diff(want, got.Value); diff != "" {
			return fmt.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		return nil
	}

	// The key to this test is configuring scantest.Run correctly.
	// The first argument (`nestedDir`) tells Run to treat the nested directory as the
	// primary context for the test.
	// `scantest.WithModuleRoot(nestedDir)` explicitly tells the scanner to
	// use `nestedDir` as its starting point to find `nested/go.mod`. This ensures
	// the relative `replace` directive is resolved correctly.
	if _, err := scantest.Run(t, nestedDir, nil, action, scantest.WithModuleRoot(nestedDir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
```

## Common Errors and Troubleshooting

- **Error:** `undefined: my-package.MyType (package scan failed: could not find package directory ...)`
  - **Cause:** This is the classic symptom of a misconfigured scanner. The `minigo` interpreter's internal scanner does not have the correct module context and cannot find the import path.
  - **Solution:** Ensure you are using `scantest.Run` as shown in the examples. If you are testing a nested module, you **must** use `scantest.WithModuleRoot()` to point the scanner to the correct subdirectory. Verify that your `go.mod` files and `replace` directives are correct.

- **Error:** `could not read config file: open my-config.go: no such file or directory`
  - **Cause:** The path to the script/config file being loaded is incorrect. This often happens when the test context is not correctly managed.
  - **Solution:** Always construct a full, absolute path to the file you want to load, for example by using `filepath.Join(dir, "my-script.mgo")`, where `dir` is the path returned by `scantest.WriteFiles`.
