# `symgo` Refinement 2: Troubleshooting the Analysis of `minigo`

This document details the troubleshooting process for fixing the `symgo` analysis of the `minigo` package, as outlined in `plan-symgo-refine2.md`.

## Initial Analysis (Correction)

The initial problem analysis was flawed. The root cause of the `find-orphans` e2e test timeout was not just a failure to resolve standard library symbols, but a more fundamental issue: `symgo`'s inability to analyze the `minigo` package itself. This package, while part of the same workspace, is complex and uses language features that triggered deep, unhandled recursive loops in `symgo`'s evaluator.

The `identifier not found` errors for `goscan`, `token`, etc., were symptoms of this deeper failure. The analysis would enter an infinite loop before it even had a chance to correctly resolve all symbols.

## Test-Driven Debugging Journey

The strategy was to create a focused integration test to reproduce the `minigo` analysis failure in isolation. This proved to be a difficult task, fraught with environmental issues and incorrect assumptions.

### Iteration 1: Simple File Evaluation

*   **Action**: Create a test that iterates through all `.go` files in the `minigo` package and its sub-packages, calling `interp.Eval()` on each.
*   **Result**: The test passed almost instantly.
*   **Conclusion**: This was not enough to trigger the bug. The failure must be in the evaluation of function *bodies*, not just top-level declarations.

### Iteration 2: Full Function Application

*   **Action**: Revise the test to first load all declarations, then collect all `object.Function` instances from the `minigo` packages, and finally call `interp.ApplyFunction()` on each one.
*   **Result**: The test failed, but not with the expected recursion. It failed because the package objects for `minigo` were not found in the interpreter's state after the initial `Eval` pass.
*   **Conclusion**: The `Eval` function does not populate the interpreter's central package cache in the way that was assumed. Package resolution is lazy and happens on-demand when symbols are accessed.

### Iteration 3: Environment and Build System Hell

*   **Action**: A long series of attempts were made to work around the package resolution issue and fix the test. This involved:
    *   Trying to "prime" the interpreter by evaluating a dummy file that imported all `minigo` packages.
    *   Adding and re-adding test helper methods (`Files()`, `ApplyFunction()`, etc.) to the `symgo` and `evaluator` packages.
    *   Wrestling with a **major environmental issue** where `read_file`, `replace_with_git_merge_diff`, and `run_in_bash_session` would either fail with `file not found` or hang indefinitely.
*   **Result**: This led to a frustrating cycle of build errors, tool failures, and incorrect assumptions. The user provided a critical hint: check `AGENTS.md` and the current working directory (`pwd`).
*   **Conclusion**: The root cause of the tool failures was an unstable CWD. Following the advice in `AGENTS.md` to prefix commands with `cd /app` was a necessary workaround, though long-running `make` commands still tended to hang.

### Iteration 4: A Working, Failing Test

*   **Action**: After resetting the workspace and applying the `cd /app` workaround, a stable test was finally created. The final, successful approach was:
    1.  Create a `goscan.Scanner` manually for the project root.
    2.  Use the scanner to find and parse all packages under `minigo/...`.
    3.  Create a `symgo.Interpreter`.
    4.  "Load" all packages by calling `interp.Eval()` on each file's AST. This populates the necessary file scopes.
    5.  Iterate through the interpreter's *loaded files* (`interp.Files()`).
    6.  For each file belonging to a `minigo` package, iterate through its `*ast.FuncDecl`s.
    7.  For each `funcDecl`, create an `object.Function` and call `interp.ApplyFunction()` on it.
*   **Result**: This test successfully and reliably reproduced the bug, causing the test runner to time out.
*   **Conclusion**: This test-driven approach, despite the difficulties, resulted in a valuable regression test that precisely captures the bug.

## Final Status

The primary goal of creating a failing test case for the `minigo` analysis bug has been achieved. The test, `TestAnalyzeMinigoPackage`, now exists in the codebase. As per the user's instruction, the test is skipped using `t.Skip()` to allow the CI/CD pipeline to pass, with the understanding that the underlying bug will be fixed in a subsequent task. The knowledge gained about the unstable CWD and hanging `make` commands has been documented in `docs/trouble.md`.

## Resolution

The infinite recursion bug was fixed by introducing a targeted cycle detection mechanism within the `symgo` evaluator.

-   **Problem:** The evaluator entered an infinite loop when evaluating a composite literal (`*ast.CompositeLit`) whose fields indirectly triggered the evaluation of the same literal. A general-purpose recursion check in the main `Eval` function was too aggressive and caused regressions by incorrectly flagging valid recursive method calls (like in a linked-list traversal).

-   **Solution:** The general check was removed. Instead, a specific check was added only to the `evalCompositeLit` function. This check uses a map (`evaluationInProgress`) to track `*ast.CompositeLit` nodes currently being evaluated. If a node is encountered a second time before its evaluation completes, it's identified as a cycle. The evaluator then logs a warning and returns a symbolic placeholder, allowing analysis to continue without timing out. This targeted approach successfully resolved the timeout in the `TestAnalyzeMinigoPackage` test without affecting legitimate recursive algorithms.

### Known Issue: Panic on Invalid Recursive `var`

During this investigation, an attempt was made to create a minimal unit test to reproduce the cycle. This test used an invalid, self-referential variable initialization:
```go
package main
type T struct { F *T }
var V = T{F: &V}
```
While the fix for the composite literal recursion works, evaluating this specific code exposed a separate, deeper robustness issue in the `symgo` evaluator, causing a `nil pointer dereference` panic. The root cause of this panic is not yet understood and requires further investigation. A new task has been added to `TODO.md` to track this issue.
