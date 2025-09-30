# `symgo` Robustness Report: Identifier Resolution in Test Code

This document details the analysis and resolution of a critical identifier resolution issue within the `symgo` symbolic execution engine, discovered when analyzing test files.

## 1. Problem Description

When running `symgo`-based tools like `find-orphans` with test file analysis enabled (`--include-tests`), the evaluator would fail with numerous `identifier not found` errors.

**Symptom:**

The analysis logs would show many errors similar to the following, originating from various test files across the project:

```
level=ERROR msg="identifier not found: sampleAPIPath" in_func=TestDocgen exec_pos=...
level=ERROR msg="symbolic execution failed for entry point" function=...TestDocgen error="symgo runtime error: identifier not found: sampleAPIPath..."
```

These errors prevented `symgo` from correctly analyzing any test functions that used locally defined constants, significantly limiting its utility for whole-program analysis that includes test code.

## 2. Root Cause Analysis

The investigation revealed that the issue was specific to how the `symgo` evaluator handled declarations within the scope of a function, particularly when that function was an entry point for analysis (as is common for tests).

1.  **Local Constants in Tests:** Go tests frequently define local constants within the test function body for convenience (e.g., `const sampleAPIPath = "..."`).
2.  **Statement-by-Statement Evaluation:** The `symgo` evaluator processes the statements within a function body sequentially. It does not pre-scan the entire function body to hoist all declarations.
3.  **Missing `const` Handler:** The core of the bug was a critical omission in the `evalGenDecl` function, which is responsible for handling `var`, `type`, and `const` declarations. While it had logic for `var` and `type`, it was completely missing a `case token.CONST`.
4.  **The Crash:** Because the `const` handler was missing, any `const` declaration inside a function was simply ignored. When a later statement in the function attempted to use that constant, the `evalIdent` function would look for it in the current environment, fail to find it, and produce the `identifier not found` error, halting the analysis of that function.

## 3. Solution Implemented

The fix involved implementing the missing logic for constant declarations within the evaluator, making it correctly recognize and process them.

-   **File Modified:** `symgo/evaluator/evaluator.go`
-   **Function Modified:** `evalGenDecl`

A new `case token.CONST` was added to the `switch` statement in `evalGenDecl`. The new logic correctly handles the semantics of constant declarations:

-   It iterates through all constant specifications in a `const (...)` block.
-   It correctly handles value repetition (e.g., `const (a = 1; b; c)` where `b` and `c` inherit the value of `a`).
-   To correctly handle `iota`, it creates a temporary, enclosed environment for each constant's value expression, binding the current `iota` value within it.
-   It uses the evaluator's own `e.Eval` method to evaluate the constant's value expression.
-   Finally, it adds the resulting constant object to the current function's scope using `env.SetLocal()`, ensuring it is available for subsequent statements.

This change ensures that `symgo` correctly processes `const` declarations as it encounters them, making the identifiers available for the rest of the symbolic execution process within that function.

## 4. Verification

The fix was validated through two methods:

1.  **New Unit Test:** A new test, `TestSymgo_LocalConstantResolution`, was added in a new file, `symgo/symgo_local_const_test.go`. This test specifically defines a function with a local constant and asserts that `symgo` can correctly resolve and return its value. This test failed before the fix and passed after, confirming the solution's effectiveness and preventing future regressions.
2.  **End-to-End Test:** The `make -C examples/find-orphans` command was re-run. A review of the generated `find-orphans.out` log confirmed that all `identifier not found` errors related to local constants in test files were successfully eliminated.

While a separate issue causing `not a function: NIL` errors in `minigo` tests remains, the primary bug preventing the analysis of test code with local constants has been resolved.

---

# `symgo` Robustness Report: Selector Expression on Non-Struct Types

This document details the analysis and resolution of a class of errors in the `symgo` symbolic execution engine where a selector expression (e.g., `X.Y`) was used on an object `X` that was not a struct, pointer to a struct, or package.

## 1. Problem Description

When analyzing certain codebases, the `symgo` evaluator would crash with errors indicating that it could not perform a selection on various built-in or fundamental types.

**Symptoms:**

The analysis logs showed errors such as:

-   `expected a package, instance, or pointer on the left side of selector, but got FUNCTION`
-   `undefined method or field: Has for pointer type MAP`
-   `expected a package, instance, or pointer on the left side of selector, but got PANIC`
-   `expected a package, instance, or pointer on the left side of selector, but got SLICE`
-   `expected a package, instance, or pointer on the left side of selector, but got STRING`

These errors occurred in `evalSelectorExpr` within `symgo/evaluator/evaluator.go`. The core issue was that the evaluator's logic for handling `ast.SelectorExpr` was not robust enough. It assumed the left-hand side would always be a type that could contain fields or methods (like a struct or a package). When it encountered other types (functions, maps, slices, strings, etc.), it would halt the analysis by returning an error. This behavior is too brittle for a static analysis tool, which should be able to gracefully handle such cases and continue its analysis.

## 2. Root Cause Analysis

The root cause was an incomplete `switch` statement over the type of the left-hand side object (`left`) in the `evalSelectorExpr` function. The function had explicit cases for `*object.Package`, `*object.Instance` (structs), and pointers to them. However, it lacked handling for many other possible `object.Object` types that can appear as the result of expression evaluation. When `left` was one of these unhandled types, the function would fall through to a default error case, terminating the analysis of the current path.

For a robust symbolic execution engine, the desired behavior in these cases is not to fail, but to assume the access might be valid in some context (perhaps via a yet-to-be-analyzed type conversion or generic) and return a `SymbolicPlaceholder`. This allows the analysis to continue down the code path, which is crucial for tools like `find-orphans` that need to traverse the entire call graph.

## 3. Solution Implemented

The fix involved making `evalSelectorExpr` more robust by adding handlers for the problematic types.

-   **File Modified:** `symgo/evaluator/evaluator.go`
-   **Function Modified:** `evalSelectorExpr`

A new `case` was added to the `switch` statement over the left-hand side object's type. This new case handles `*object.Function`, `*object.Map`, `*object.Slice`, `*object.String`, `*object.Intrinsic`, and `*object.PanicError`. When a selector expression is used on an object of one of these types, the evaluator now logs a debug message and returns a `*object.SymbolicPlaceholder` instead of an error. This prevents the analysis from halting and aligns with `symgo`'s design philosophy of prioritizing robustness.

## 4. Verification

The fix was validated by adding a new test file, `symgo/evaluator/evaluator_selector_robustness_test.go`. This file contains specific test cases for selector expressions on each of the newly supported types. The tests confirm that the evaluator no longer returns an error for these cases and that the analysis can proceed. The entire test suite was run via `make test` to ensure no regressions were introduced.