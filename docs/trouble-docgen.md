# Trouble Report: Type-Safe Docgen Patterns Implementation

This document outlines the state of the "Type-Safe docgen Patterns" task, which is currently in a work-in-progress state. The core feature has been implemented and a new integration test for it passes, but it has introduced regressions in the existing `minigo` test suite.

## Summary of Changes

The goal of this task was to allow users of the `docgen` tool to define analysis patterns using a type-safe function reference (`Fn: mypkg.MyFunc`) instead of a fragile, string-based key.

This was accomplished through the following changes:

1.  **`minigo` Interpreter Enhancement (`minigo/object/object.go`, `minigo/evaluator/evaluator.go`):**
    *   A new `object.GoSourceFunction` type was created. This object represents a Go function loaded from source code and, crucially, stores a reference to the environment (`DefEnv`) in which it was defined.
    *   The evaluator was updated to use this new object. When a `GoSourceFunction` is called from a script, the evaluator now uses the stored `DefEnv` to resolve other symbols (variables, functions, etc.) from the original package. This is the key mechanism that allows the type-safe `Fn` pattern to work.
    *   The package loading logic was also improved to correctly process global `var` declarations, in addition to `const` and `func`.

2.  **`docgen` Refactoring (`examples/docgen/patterns/patterns.go`, `examples/docgen/loader.go`):**
    *   The `patterns.PatternConfig` struct was updated to include a new `Fn any` field.
    *   The pattern loader (`loader.go`) was updated to inspect this `Fn` field. If it contains a `*minigo.GoSourceFunction`, it dynamically constructs the fully-qualified key (e.g., `path/to/pkg.MyFunc`) that the analyzer uses for matching.

3.  **New Integration Test (`examples/docgen/integration_test.go`):**
    *   A new end-to-end test, `TestDocgen_WithFnPatterns_FullAnalysis`, was added.
    *   This test includes a new test module (`testdata/integration/fn-patterns-full`) with a sample API, a helper function, and a `patterns.go` config that uses the new `Fn` field.
    *   It verifies that the `docgen` tool can correctly analyze the API using the type-safe pattern and produce the expected OpenAPI JSON output, which is validated against a golden file.
    *   **This new test passes.**

## Current Problem: `minigo` Regressions

While the new functionality works as expected, the changes to the `minigo` evaluator have caused several existing `minigo` tests to fail.

### Failing Test Output

```
--- FAIL: TestStdlib_slices (0.01s)
    minigo_stdlib_custom_test.go:307: failed to evaluate script: runtime error: wrong number of type arguments. got=0, want=2
		test.mgo:5:10:
--- FAIL: TestStdlib_slices_Sort (0.01s)
    minigo_stdlib_custom_test.go:780: failed to evaluate script: runtime error: wrong number of type arguments. got=0, want=2
		test.mgo:5:9:
--- FAIL: TestTransitiveImport (0.00s)
    minigo_transitive_import_test.go:30: eval failed: runtime error: identifier not found: pkgb
		/app/minigo/testdata/pkga/pkga.go:6:22:
			return "A says: " + pkgb.FuncB()
		main.go:7:10:	in FuncA
		:0:0:	in main

        stderr:
--- FAIL: TestTransitiveImportMultiFilePackage (0.01s)
    minigo_transitive_import_test.go:69: eval failed: runtime error: identifier not found: pkgf
		/app/minigo/testdata/pkge/pkge1.go:6:23:
			return "E1 says: " + pkgf.FuncF()
		/app/minigo/testdata/pkge/pkge2.go:4:23:	in FuncE1
			return "E2 says: " + FuncE1()
		main.go:7:10:	in FuncE2
		:0:0:	in main
```

### Analysis of Failures

1.  **`TestStdlib_slices` & `TestStdlib_slices_Sort`**: The error `wrong number of type arguments. got=0, want=2` strongly suggests that the changes made to support generic functions in the evaluator have broken the type inference for the standard library's generic `slices.Sort` function. The evaluator is failing to automatically infer the type arguments for the slice being sorted.

2.  **`TestTransitiveImport` & `TestTransitiveImportMultiFilePackage`**: The error `identifier not found: pkgb` (and `pkgf`) indicates a problem with symbol resolution across packages. The changes to how imported functions are handled (`GoSourceFunction` and `DefEnv`) seem to have interfered with the ability to resolve symbols from a package that is imported transitively (i.e., a dependency of a dependency).

## Next Steps

The immediate next step for the next agent is to fix these regressions in the `minigo` package. The core logic for the type-safe patterns is sound, but it cannot be merged until the `minigo` test suite is fully passing.

**Recommended approach:**
1.  Focus on the failing tests in `minigo_stdlib_custom_test.go` and `minigo_transitive_import_test.go`.
2.  Debug the `minigo/evaluator/evaluator.go` file to understand why the type inference and symbol resolution are failing in these specific cases.
3.  Modify the evaluator to fix the regressions while ensuring that the new test, `TestDocgen_WithFnPatterns_FullAnalysis`, continues to pass.
4.  Run `cd /app && go test ./...` to confirm that all tests are passing.
