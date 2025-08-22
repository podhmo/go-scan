# Minigo Interpreter State after Build Fixes

This document outlines the state of the `minigo` interpreter after a series of changes to fix initial build failures. While the project now builds, several runtime tests are failing.

## Summary of Changes

1.  **Core Build Fixes:**
    *   Resolved a syntax error in `evaluator.go`.
    *   Refactored `object.FileScope` to include a `Filename`, allowing the evaluator to correctly locate source files for method scanning. This required updating all call sites of `object.NewFileScope`.
    *   Implemented evaluation logic for `*object.GoSourceFunction` within `applyFunction` in `evaluator.go`. This involved adding an `FScope` to `GoSourceFunction` to ensure functions loaded from source are evaluated in their correct definitional scope.

2.  **Code Cleanup:**
    *   The `minigo/minigotest` package was identified as obsolete and was causing build failures. It has been removed.
    *   The test file `minigo/minigo_typed_nil_method_test.go`, which depended on the deleted `minigotest` package, has been rewritten to use the modern `goscan.Scanner` with an in-memory file overlay.

## Current Status: Runtime Test Failures

After the above changes, `make test` executes but fails with several runtime errors. The failures can be categorized as follows:

### 1. `cmp` Package Not Found

Tests that evaluate standard library packages like `slices` are failing. The interpreter cannot resolve the `cmp` package, which is a built-in package in Go used for generic constraints like `cmp.Ordered`.

**Error Example:**
```
--- FAIL: TestStdlib_slices (0.01s)
    minigo_stdlib_custom_test.go:307: failed to evaluate script: runtime error: identifier not found: cmp
		/usr/local/go/src/slices/slices.go:57:24:
			func Compare[S ~[]E, E cmp.Ordered](s1, s2 S) int {
```

### 2. Transitive Import Resolution Failure

Tests involving a chain of imports (e.g., `main` imports `pkga`, which imports `pkgb`) are failing. The evaluator successfully resolves `pkga` but fails to find symbols from `pkgb` when they are used within `pkga`.

**Error Example:**
```
--- FAIL: TestTransitiveImport (0.00s)
    minigo_transitive_import_test.go:30: eval failed: runtime error: identifier not found: pkgb
		/app/minigo/testdata/pkga/pkga.go:6:22:
			return "A says: " + pkgb.FuncB()
```
This suggests a problem with how the interpreter manages environments and scopes for nested package imports.

### 3. In-Memory Module Resolution Failure

The rewritten `TestTypedNilMethodValue` test, which uses a scanner overlay to create an in-memory module, is failing. The interpreter cannot find the `my/api` package that is defined in the overlay.

**Error Example:**
```
--- FAIL: TestTypedNilMethodValue (0.01s)
    minigo_typed_nil_method_test.go:71: minigo execution failed: runtime error: undefined: api.API (package scan failed: could not get unscanned files for package my/api: could not find package directory for "my/api" (tried as path and import path): import path "my/api" could not be resolved. Current module is "my" (root: /tmp/TestTypedNilMethodValue...))
```
This points to an issue with how the scanner or interpreter is configured to handle in-memory modules and overlays.
