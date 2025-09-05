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

## Resolution and Follow-up

The core infinite recursion bug was fixed by introducing a targeted cycle detection mechanism within the `symgo` evaluator's `evalCompositeLit` function. This resolves the timeout observed in the `TestAnalyzeMinigoPackage` integration test.

However, the task is only partially complete. A key follow-up action is to create a more focused, minimal unit test for this specific fix. An attempt to create such a test using invalid Go code (`var V = T{F: &V}`) revealed a separate robustness issue in the evaluator (a `nil pointer dereference` panic).

Therefore, the remaining high-priority task is to design a proper unit test—likely using valid but structurally complex Go code—that can trigger the original bug in a controlled manner, solidifying the fix and preventing future regressions. This task is now tracked in `TODO.md`.

## E2E Test Failure Analysis (Post-Refactor)

After the initial fixes, the `make -C examples/find-orphans e2e` command was run again to perform a full verification. This revealed a new, more subtle bug that was previously masked.

### Symptom: `identifier not found: findModuleRoot`

The e2e test failed, and the output log (`find-orphans.out`) was filled with errors like this:
```
level=ERROR msg="identifier not found: findModuleRoot" in_func=New in_func_pos=/app/goscan.go:454:16
...
level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/find-orphans.main error="symgo runtime error: identifier not found: findModuleRoot\n\t/app/locator/locator.go:78:18 ...
```

This error occurred during the symbolic execution of nearly every `main` package in the workspace. The stack trace consistently showed that the failure happened when `symgo` was evaluating `locator.New`, which in turn calls the unexported function `findModuleRoot` from the same `locator` package.

### Analysis: Incorrect Package Scoping

The problem is not that `symgo` cannot handle unexported functions. A targeted test case (`TestEval_CrossPackageUnexportedFunctionCall`) was created which proved that `symgo` *can* correctly resolve and call an unexported helper function in a different package, provided the packages are scanned and loaded correctly.

The root cause of the e2e failure is a scoping issue during on-demand package loading within `symgo`. When a function from a new package is encountered during symbolic execution (like `locator.New`), `symgo` loads that package. However, the environment (`object.Environment`) for this newly loaded package was being created incorrectly. Instead of being enclosed by the top-level, global environment (which contains built-ins), it was being enclosed by the current, potentially deeply nested, function execution environment.

This meant that when `locator.New` was evaluated, its environment did not contain the other top-level declarations from its own package, such as `findModuleRoot`.

### Analysis: Incorrect Package Scoping (Still Unresolved)

The problem is not that `symgo` cannot handle unexported functions. A targeted test case (`TestEval_CrossPackageUnexportedFunctionCall`) was created which proved that `symgo` *can* correctly resolve and call an unexported helper function in a different package, provided the packages are scanned and loaded correctly.

The root cause of the e2e failure is a scoping issue during on-demand package loading within `symgo`. When a function from a new package is encountered during symbolic execution (like `locator.New`), `symgo` loads that package. However, the environment (`object.Environment`) for this newly loaded package is being created incorrectly. Instead of being enclosed by the top-level, global environment (which contains built-ins), it is being enclosed by the current, potentially deeply nested, function execution environment.

This means that when `locator.New` was evaluated, its environment did not contain the other top-level declarations from its own package, such as `findModuleRoot`.

A previous attempt to fix this by ensuring all package environments are enclosed by a shared `UniverseEnv` was made. However, as of the latest `make -C examples/find-orphans e2e` run, this fix appears to be ineffective or has been reverted. The `identifier not found: findModuleRoot` error persists, indicating that the package-level scope for the `locator` package is not being correctly populated when it is loaded on-demand during the symbolic execution of other packages.
