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

## E2E Test Failure Analysis and Resolution

After fixing the `minigo` analysis bug, the `make -C examples/find-orphans e2e` command was run again. This revealed a new, more subtle bug that was previously masked.

### Symptom 1: `identifier not found: findModuleRoot`

Initially, the e2e test failed with errors like `identifier not found: findModuleRoot`. This happened when `symgo` was symbolically executing `locator.New`, which calls the unexported function `findModuleRoot` from the same package.

This pointed to a package scoping issue. The `symgo` evaluator's `applyFunction` was creating placeholder `object.Package` instances for a function's imports, but it was creating them with a new, empty `object.Environment()`. This environment was not enclosed by the top-level `UniverseEnv`, so when the package was later populated, it lacked access to built-in functions, and its own unexported members were not being resolved correctly across files.

### Symptom 2: `infinite recursion detected: New`

An initial attempt to fix the scoping issue was to remove the placeholder creation logic from `applyFunction` altogether. This correctly forced `evalIdent` to use the central `getOrLoadPackage` function, which uses a proper, cached, and correctly-scoped environment.

This fixed the `identifier not found` error, but uncovered a new problem: the e2e test would now fail with `infinite recursion detected: New`. The symbolic execution of `goscan.New` would lead to a call to `locator.New`, which in turn involves file system operations (`os.Stat`, etc.) to find `go.mod`. The symbolic execution engine, lacking intrinsics for these `os` functions, would attempt to analyze the entire Go standard library, get confused, and end up re-evaluating `goscan.New`.

### Final Resolution: A Two-Part Fix

The final, successful solution involved two key changes:

1.  **Correct Scoping for Placeholders**: Instead of removing the placeholder logic in `applyFunction`, it was corrected. The line `extendedEnv.Set(name, &object.Package{... Env: object.NewEnvironment()})` was changed to `extendedEnv.Set(name, &object.Package{... Env: object.NewEnclosedEnvironment(e.UniverseEnv)})`. This ensured that the placeholder package objects created for imports had a correctly-scoped environment from the beginning, fixing the `identifier not found` issue without altering the evaluator's fundamental logic.

2.  **Robust Test Configuration**: With the scoping fixed, the `infinite recursion` issue in the `find-orphans` e2e test persisted. This was solved by making the tool itself more robust. A `ScanPolicyFunc` was added to `examples/find-orphans/main.go` to prevent the `symgo` interpreter from analyzing packages outside the current workspace (like the standard library). This is the correct architectural approach for a tool that should only be concerned with user code. This change also fixed a regression in the `docgen` tests. A similar fix was applied to a failing unit test (`TestSymgo_IntraPackageConstantResolution`), which needed `goscan.WithGoModuleResolver()` added to its scanner configuration to correctly locate standard library packages during testing.

After applying this combination of fixes, all unit tests and the `find-orphans` e2e test now pass successfully.
