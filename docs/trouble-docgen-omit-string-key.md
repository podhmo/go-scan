# Trouble Report: Implementing Type-Safe `docgen` Patterns

This document outlines the progress and difficulties encountered while trying to implement a type-safe pattern configuration for the `docgen` tool. The goal was to replace the string-based `Key` field with a function-based `Fn` field in the `minigo`-driven pattern configuration.

## Goal

The objective was to refactor `docgen`'s custom pattern configuration (`patterns.PatternConfig`) to be more type-safe. Instead of users providing a string `Key` for a function or method (e.g., `Key: "my/pkg.MyFunc"`), they should be able to provide the function reference directly (e.g., `Fn: mypkg.MyFunc`).

This involved two major sub-tasks:
1.  Enhancing the `minigo` interpreter to correctly handle typed `nil` values, which is necessary to support method references on nil receivers (e.g., `Fn: (*MyType)(nil).MyMethod`).
2.  Modifying the `docgen` pattern loader to process the new `Fn` field using reflection to derive the function's fully-qualified name.

## Summary of Work Done

Significant progress was made on both fronts before work was halted.

### 1. `minigo` Interpreter Enhancements (Complete and Verified)

The `minigo` interpreter was successfully enhanced to handle typed `nil` values. This was a complex task involving several changes:
-   **`object.TypedNil`**: A new object was created to represent `nil` values that retain type information (e.g., for `var s []int`).
-   **`object.Pointer` Enhancement**: The `Pointer` object was modified to store its `PointerType`, allowing it to know what type it points to even when its element is `nil`.
-   **Evaluator Modifications (`evaluator.go`)**:
    -   `evalGenDecl`: Updated to create `TypedNil` and typed `Pointer` objects for zero-value variable declarations.
    -   `inferTypeOf`: Updated to correctly infer types from `TypedNil` and typed `Pointer` objects. This was crucial for generic function type inference.
    -   `evalSelectorExpr`: Updated to allow method resolution on typed `nil` pointers.
    -   `evalReturnStmt`: Updated to perform implicit interface conversions for return values, ensuring that a concrete typed `nil` pointer becomes a non-nil interface value.
    -   `evalInfixExpression`: The logic for `==` and `!=` was improved to correctly compare various kinds of `nil` values.
    -   Various builtins (`new`, `append`) were updated to handle the new nil types.

**Verification**: All changes were verified by un-skipping and passing the `TestNilBehavior` suite in `minigo/minigo_nil_test.go`. All other `minigo` tests were also confirmed to pass, so the changes are considered robust and complete.

### 2. `docgen` Pattern Loader Enhancements (Implemented, but Untested)

The `docgen` configuration was refactored as planned:
-   **`patterns.PatternConfig`**: The struct was modified to include `Fn any` and an internal `key string`.
-   **`minigo.unmarshal`**: The unmarshalling logic in `minigo.go` was updated to allow `minigo`'s internal function objects to be assigned to Go fields of type `any`.
-   **`loader.go`**: The `convertConfigsToPatterns` function was updated with logic to check for the `Fn` field, use `runtime.FuncForPC` to get the function's name, and correctly format it for use as the internal pattern key.

## The Core Problem: Test Environment Module Resolution

The primary blocker is an issue with Go module resolution when running tests.

**Scenario**:
- The `docgen` tool lives in `examples/docgen/`. It has its own `go.mod`.
- The new test case for the `Fn` pattern lives in `examples/docgen/testdata/fn-patterns/`. This test case is set up as its own Go module with its own `go.mod` file.
- The `fn-patterns/go.mod` file contains a `replace` directive to find the root `go-scan` project: `replace github.com/podhmo/go-scan => ../../../../`.
- The `fn-patterns/patterns.go` script imports a local package `.../fn-patterns/api`.
- The test, `TestDocgen_withFnPatterns` in `main_test.go`, correctly sets the working directory for both the `go-scan` instance and the `minigo` interpreter to the submodule's directory (`testdata/fn-patterns`).

**Failure**:
Despite this setup, which should be correct, the `minigo` interpreter fails to evaluate `patterns.go`. It cannot resolve the import for the local `api` package, resulting in an `undefined: api.API` error. The error message indicates that the scanner is correctly operating within the `fn-patterns` module context, yet it cannot find a package within that same module.

This suggests a fundamental issue with how the `go-scan` locator interacts with nested/temporary Go modules created for testing purposes when invoked from another module's `go test` command. All attempts to work around this (simplifying the test, changing paths) have failed.

## State of the Codebase for Submission

To leave the codebase in a stable state, all changes have been reverted *except* for the verified improvements to the `minigo` interpreter (`minigo/object/object.go` and `minigo/evaluator/evaluator.go`).

- **Reverted**: All changes to `examples/docgen/` have been reverted. `PatternConfig` is back to its original state, as is `loader.go`. The new test files have been deleted.
- **Kept**: The complete, verified implementation of typed `nil` handling in `minigo` remains. This is a valuable, standalone improvement.

## Recommended Next Steps

1.  **Acknowledge the `minigo` Fixes**: The work done in `minigo` is complete and valuable. It should be kept.
2.  **Re-implement the `docgen` Changes**: The changes to `patterns.go`, `loader.go`, and `minigo.go` (the `unmarshal` function) should be re-applied. They are logically sound.
3.  **Solve the Test Problem**: The next developer must focus on solving the test failure.
    - **Hypothesis**: The issue is environmental and specific to how `go test` handles nested modules with `replace` directives.
    - **Suggestion**: Instead of creating a separate module in `testdata`, try creating a test that programmatically constructs the `minigo` script as a string and uses `minigo.WithGlobals` to inject the necessary functions (like `api.SendJSON`) into the interpreter's global scope. This avoids all file-based module resolution issues and tests the `minigo` evaluation and `docgen` loading logic in a more controlled, hermetic way. This seems like the most promising path forward.
4.  Once the test passes, the rest of the original plan (updating documentation, `TODO.md`) can be completed.
