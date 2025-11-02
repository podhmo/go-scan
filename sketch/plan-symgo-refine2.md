# `symgo` Refinement Plan 2: Addressing Analysis Regressions

## Introduction

This document provides a concrete action plan to resolve critical regressions in the `symgo` engine. The previous plan was marked as complete, but recent `e2e` tests for the `find-orphans` tool show that key issues have returned.

The analysis in [`sketch/trouble-symgo-refine2.md`](./trouble-symgo-refine2.md) identifies two primary failure modes:
1.  **Symbolic execution failure** when encountering calls to packages outside the primary analysis scope (e.g., the `flag` package).
2.  **Infinite recursion** when analyzing code that itself uses the `go-scan` or `symgo` libraries.

## Core Problem: Handling of Unscannable Code

The root cause of these issues is `symgo`'s insufficient handling of symbols from packages outside the defined `PrimaryAnalysisScope`. When the evaluator encounters a selector expression for an unscannable package (e.g., `flag.String`), its fallback logic is incorrect, leading to evaluation errors. Similarly, its recursion detection is not robust enough for complex, self-referential analysis.

This plan focuses on making the `symgo` evaluator more resilient and intelligent when dealing with these cases.

## New Action Plan

### Task 1: Generalize Handling of Unresolved Functions

*   **Goal**: Prevent symbolic execution failures when calling functions in packages that are not scanned from source (e.g., the standard library).
*   **Details**:
    *   The current `evalSelectorExpr` logic incorrectly assumes any unknown identifier in an external package is a type, returning an `object.Type`. This causes `not a function` errors when the identifier is actually a function (like `flag.String`).
    *   The fix is to change this fallback behavior. When a symbol is not found as a known function, variable, constant, or type in an external package, the evaluator should return an `*object.UnresolvedFunction`.
    *   The existing logic in `applyFunction` can handle `*object.UnresolvedFunction` by attempting to find the function's signature and returning a symbolic placeholder for its result. This provides a general mechanism to gracefully handle any external function call without needing specific intrinsics.
*   **Acceptance Criteria**: The `not a function` errors no longer appear in the `find-orphans` e2e test logs.

### Task 2: Fix Infinite Recursion Detection

*   **Goal**: Resolve the `infinite recursion detected: New` error that occurs when `find-orphans` analyzes the `minigo` package.
*   **Details**:
    *   The current recursion check in `applyFunction` is too strict. It requires the function definition, receiver, and **call site position** to all be identical.
    *   In complex analysis scenarios (e.g., `A` calls `B` which calls `A` from a different line), the call site position check prevents the recursion from being detected.
    *   The fix is to remove the call site position (`frame.Pos == callPos`) from the check. The detection will now trigger if the same function definition is re-entered on the same receiver within the same call stack, regardless of the call's location.
*   **Acceptance Criteria**: The `infinite recursion` errors no longer appear in the `find-orphans` e2e test log. The e2e test for `find-orphans` runs to completion without errors.

### Task 3: Implement a Timeout Flag in `find-orphans` (Future Work)

*   **Goal**: Add a command-line timeout feature to `find-orphans` to facilitate future debugging.
*   **Details**: This task is unchanged. Add a `--timeout` flag that uses `context.WithTimeout`.
*   **Acceptance Criteria**:
    1.  Running `find-orphans --timeout 1ms` exits immediately with a timeout error.
    2.  The flag is documented.
