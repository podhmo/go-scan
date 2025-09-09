# Plan: Implement Bounded Recursion in `symgo`

This document outlines the plan to fix a hang in the `find-orphans` tool by implementing a bounded recursion mechanism in the `symgo` symbolic execution engine.

## Problem Description

The `find-orphans` tool hangs when analyzing code with deep or infinite recursion. An investigation revealed that this is caused by an inconsistent "bounded analysis" strategy in `symgo`.

- The engine correctly bounds `for` loops by unrolling them a fixed number of times (typically once).
- However, it does not apply a similar bound to recursive function calls.

This inconsistency causes the analysis of certain files (e.g., `parser.go` in the `convert` example) to become impractically long, appearing as a hang.

## Implementation Plan

The goal is to make the analysis of recursive functions consistent with the analysis of loops by limiting the depth of recursive call chains.

1.  **Introduce Recursion Depth Tracking**:
    -   Add a `recursionDepth` counter to the `symgo.Context` struct in `symgo/evaluator/evaluator.go`.
    -   This counter will be incremented at the beginning of `applyFunction` and decremented at the end.

2.  **Implement the Recursion Bound**:
    -   In `applyFunction`, before evaluating the function body, check if `recursionDepth` exceeds a predefined limit (e.g., a constant `maxRecursionDepth` set to a small integer like 3 or 5).
    -   If the limit is exceeded, `applyFunction` will immediately return a symbolic placeholder (`object.SymbolicPlaceholder`) representing the function's return value, instead of proceeding with the evaluation. This stops the recursion for the purpose of analysis.

3.  **Refine Existing Recursion Detection**:
    -   The current recursion detection (using `ctx.IsRecursiveCall`) is designed to detect simple, state-less infinite loops.
    -   This mechanism should be preserved but will now work in concert with the new depth limit. The depth limit acts as a fail-safe for all forms of recursion, while the existing check can quickly terminate simpler cases.

4.  **Testing and Validation**:
    -   Create a specific test case in `symgo/evaluator/evaluator_recursion_test.go` that fails without the fix (i.e., it hangs or times out). The test should involve a deeply recursive function.
    -   Ensure all existing tests in `./symgo/...` continue to pass.
    -   Run the `find-orphans` tool on the problematic `examples/convert` directory and verify that it completes without hanging. A timeout can be used with the `go test` command to verify this (e.g., `go test -timeout 30s ./...`).
