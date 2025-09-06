# `symgo` Refinement Plan 2: Addressing Analysis Regressions

## Introduction

This document provides a concrete action plan to resolve critical regressions in the `symgo` engine. The previous plan was marked as complete, but recent `e2e` tests for the `find-orphans` tool show that key issues have returned.

The analysis in [`docs/trouble-symgo-refine2.md`](./trouble-symgo-refine2.md) identifies two primary failure modes:
1.  **Symbolic execution failure** when encountering calls to standard library packages that are not part of the primary analysis scope (e.g., the `flag` package).
2.  **Infinite recursion** when analyzing code that itself uses the `go-scan` or `symgo` libraries.

## Core Problem: Handling of Unscannable Code

The root cause of these issues is `symgo`'s insufficient handling of function calls to packages outside the defined `PrimaryAnalysisScope`. The `ScanPolicy` mechanism correctly prevents the engine from reading the source code of these packages, but the engine currently lacks a robust fallback. When it encounters a call to a function like `flag.String()`, it has no intrinsic model of its behavior and cannot proceed, causing the analysis of that entrypoint to fail.

## New Action Plan

The previous plan focused on perfecting the scope definition. This new plan focuses on making the `symgo` evaluator more resilient and intelligent when dealing with code outside its scope.

### Task 1: Implement Intrinsics for the `flag` package

*   **Goal**: Prevent symbolic execution failures in `main` packages that use standard command-line flags.
*   **Details**:
    *   Implement intrinsic handlers within `symgo` for the most common functions in the `flag` package (`flag.String`, `flag.Bool`, `flag.Var`, etc.).
    *   These intrinsics will not replicate the full behavior. Instead, they will return a `SymbolicPlaceholder` of the correct type (e.g., a symbolic `*string` for `flag.String`).
    *   This will allow the symbolic execution to continue past the flag definitions in `main` functions, enabling tools like `find-orphans` to analyze them correctly.
*   **Acceptance Criteria**: The `not a function: INTEGER` errors no longer appear in the `find-orphans` e2e test logs.

### Task 2: Investigate and Fix Infinite Recursion

*   **Goal**: Resolve the `infinite recursion detected: New` error that occurs when `find-orphans` analyzes the `minigo` package.
*   **Details**:
    *   The existing cycle detection in `symgo` is clearly insufficient for this complex, self-referential case.
    *   The investigation will focus on enhancing the call stack tracking within the `symgo` evaluator.
    *   The fix may involve creating a more robust cycle detection mechanism that can identify when the evaluator is re-entering the same function with a similar or identical context, even if it's not a simple direct recursion.
*   **Acceptance Criteria**: The `infinite recursion` errors no longer appear in the `find-orphans` e2e test log. The e2e test for `find-orphans` runs to completion without errors.

### Task 3: (If Necessary) Broader Symbolic Models

*   **Goal**: Create a more general mechanism for handling unscannable functions.
*   **Details**: If adding intrinsics on a case-by-case basis proves too brittle, a more general solution will be investigated. This could involve having `symgo` automatically return a `SymbolicPlaceholder` for *any* function call it cannot resolve, based on the function's signature.
*   **Acceptance Criteria**: This is an exploratory task, to be undertaken if Task 1 and 2 do not yield a stable e2e test.

### Task 4: Implement a Timeout Flag in `find-orphans` (Future Work)

*   **Goal**: Add a command-line timeout feature to `find-orphans` to facilitate future debugging.
*   **Details**: This task is unchanged. Add a `--timeout` flag that uses `context.WithTimeout`.
*   **Acceptance Criteria**:
    1.  Running `find-orphans --timeout 1ms` exits immediately with a timeout error.
    2.  The flag is documented.
