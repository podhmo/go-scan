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

---

## Update by Jules (2025-08-22)

### Summary of Fixes

I have addressed the regressions in the `minigo` package and subsequent build issues in the `docgen` example.

1.  **Fixed Transitive Imports & Generic Inference**:
    *   The root cause of both the transitive import failure and the generic type inference failure was that functions loaded from Go source (`GoSourceFunction`) did not carry the complete import context of their defining package.
    *   **Solution**:
        1.  I added an `FScope *object.FileScope` field to the `object.GoSourceFunction` struct.
        2.  When a package is loaded on-demand, a unified `FileScope` containing all of its imports is created. I updated the evaluator to store this `FScope` in the `GoSourceFunction` object.
        3.  I updated the `applyFunction` logic in `minigo/evaluator/evaluator.go` to use this stored `FScope` when evaluating the body of a `GoSourceFunction`. This ensures that symbols (including imported packages and their transitive dependencies) are resolved correctly.
        4.  I also fixed a bug where the generic type inference logic was being skipped for `GoSourceFunction` objects.

2.  **Fixed `docgen` Build & Unmarshaling Errors**:
    *   Fixing the `minigo` regressions revealed several latent issues in the `examples/docgen` code.
    *   **Solution**:
        1.  I fixed a build error caused by a redeclared variable (`Description`) in `patterns/patterns.go`.
        2.  I fixed another build error caused by a redeclared test helper function (`newTestLogger`) by moving it to a shared `test_helper_test.go` file.
        3.  I fixed a panic caused by an `undefined: minigo.GoSourceFunction` error in `loader.go` by adding the correct import for `minigo/object`.
        4.  I fixed a final runtime panic by teaching the `minigo.Result.As()` unmarshaler how to handle the new `*object.GoSourceFunction` type, allowing it to be correctly placed in the `Fn any` field of the `PatternConfig` struct.

### Current Status: WORK-IN-PROGRESS

After all the above fixes, the `minigo` test suite passes completely. However, one test in the `docgen` suite is still failing:

*   **`TestDocgen_WithFnPatterns_FullAnalysis`**: This is the new integration test for the type-safe patterns feature.

#### Failure Details:

The test fails with a `cmp.Diff`, indicating that the generated OpenAPI specification is almost empty. It is missing the `paths` and `components` sections that should be generated by analyzing the test API.

```
--- FAIL: TestDocgen_WithFnPatterns_FullAnalysis (0.11s)
    integration_test.go:132: OpenAPI spec mismatch (-want +got):
...
```

#### Analysis:

This failure means that the custom analysis pattern for `utils.SendJSON` (defined with the new `Fn` field) is not being successfully matched and applied by the `symgo` symbolic execution engine.

I have verified the following:
*   The `docgen` loader correctly loads the `patterns.go` config file.
*   It correctly creates an `*object.GoSourceFunction` for `utils.SendJSON`.
*   It correctly generates a pattern key from this object (e.g., `my-test-module/utils.SendJSON`).
*   The `symgo` interpreter correctly registers this key and the associated pattern-handling function as an intrinsic.
*   The key generation logic in `symgo` for function calls *appears* to be compatible with the key generated by the loader.

Despite this, the pattern is not being triggered. The reason for this mismatch is still unclear and requires further debugging within the `symgo` evaluator to trace exactly how it resolves the call to `utils.SendJSON` and what key it attempts to look up. The problem seems to be a subtle interaction between the `minigo` object representation, the `docgen` loader, and the `symgo` evaluator.

### Next Steps for Next Agent:

1.  **Debug `symgo`'s `evalSelectorExpr`**: The primary task is to understand why the intrinsic for `my-test-module/utils.SendJSON` is not being matched.
    *   Add detailed logging to `symgo/evaluator/evaluator.go` in the `evalSelectorExpr` function.
    *   Log the `TypeName` of the object being selected from (e.g., the `mux` variable).
    *   Log the exact key being generated for the intrinsic lookup (e.g., `(*net/http.ServeMux).HandleFunc`).
    *   Log the key being generated when `left` is an `*object.Package` (this is the case for `utils.SendJSON`).
2.  **Verify Intrinsics Registration**: Double-check in `docgen/analyzer.go` that the custom patterns from `LoadPatternsFromConfig` are correctly being passed to `buildHandlerIntrinsics` and registered with the `symgo` interpreter.
3.  **Update Golden File**: Once the analysis is working, the golden file `testdata/integration/fn-patterns-full.golden.json` will need to be updated by running the test with the `-update` flag.
