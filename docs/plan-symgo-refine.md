# `symgo` Refinement Plan 2: Addressing Analysis Regressions

## Introduction

This document provides a concrete action plan to resolve critical regressions in the `symgo` engine. The previous plan was marked as complete, but recent `e2e` tests for the `find-orphans` tool show that key issues have returned.

The analysis in [`docs/trouble-symgo-refine.md`](./trouble-symgo-refine.md) identifies two primary failure modes:
1.  **[RESOLVED]** Symbolic execution failure when encountering calls to packages outside the primary analysis scope (e.g., the `flag` package).
2.  **[UNRESOLVED]** Infinite recursion when analyzing code that itself uses the `go-scan` or `symgo` libraries.

## Core Problem: Handling of Unscannable Code

The root cause of these issues is `symgo`'s insufficient handling of symbols from packages outside the defined `PrimaryAnalysisScope`. When the evaluator encounters a selector expression for an unscannable package (e.g., `flag.String`), its fallback logic is incorrect, leading to evaluation errors. Similarly, its recursion detection is not robust enough for complex, self-referential analysis.

This plan focuses on making the `symgo` evaluator more resilient and intelligent when dealing with these cases.

## Action Plan

### Task 1: [COMPLETED] Generalize Handling of Unresolved Functions

*   **Goal**: Prevent symbolic execution failures when calling functions in packages that are not scanned from source (e.g., the standard library).
*   **Details**:
    *   The `evalSelectorExpr` and `ResolveFunction` logic was updated to return an `*object.UnresolvedFunction` for any function in an external, unscannable package.
    *   This allows the `applyFunction` logic to gracefully handle these external calls by creating a symbolic result based on the function's signature.
*   **Acceptance Criteria**: The `not a function` errors no longer appear in the `find-orphans` e2e test logs. **(Met)**

### Task 2: [UNRESOLVED] Fix Infinite Recursion Detection

*   **Goal**: Resolve the `infinite recursion detected: New` error that occurs when `find-orphans` analyzes the `minigo` package.
*   **Details**:
    *   The recursion check in `applyFunction` appears to be correct and does not use the call site position.
    *   However, recursion is still detected during self-referential analysis. The root cause is not yet understood and requires further investigation.
*   **Acceptance Criteria**: The `infinite recursion` errors no longer appear in the `find-orphans` e2e test log. **(Not Met)**

### Task 3: Implement a Timeout Flag in `find-orphans` (Future Work)

*   **Goal**: Add a command-line timeout feature to `find-orphans` to facilitate future debugging.
*   **Details**: This task is unchanged. Add a `--timeout` flag that uses `context.WithTimeout`.
*   **Acceptance Criteria**:
    1.  Running `find-orphans --timeout 1ms` exits immediately with a timeout error.
    2.  The flag is documented.
