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