# Trouble with symgo: Unexported Function Resolution Across Packages

This document details a bug in the `symgo` symbolic execution engine where it fails to resolve unexported functions when the call originates from a different package, leading to incorrect "identifier not found" errors and, consequently, false positives in tools like `find-orphans`.

## Problem Description

When running the `find-orphans` tool on the `examples/convert` directory, the function `github.com/podhmo/go-scan/examples/convert.formatCode` is incorrectly reported as an orphan.

This is a symptom of a deeper issue. The symbolic execution fails during the analysis of the `convert` example. The log shows the following error:

```
level=ERROR msg="identifier not found: processPackage" in_func=Parse in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/convert/main.go:121:15
```

The call chain leading to the error is:
1.  `main.run()` (in `examples/convert/main.go`) calls `parser.Parse()`.
2.  `parser.Parse()` (in `examples/convert/parser/parser.go`) immediately calls `processPackage()`, which is an **unexported** function within the same `parser` package.

The error message is revealing:
-   `identifier not found: processPackage`: The engine cannot find the function.
-   `in_func=Parse`: The error occurs while executing `Parse`.
-   `in_func_pos=.../main.go:121:15`: The position reported is the call site of `parser.Parse()` in the `main` package, not the call site of `processPackage()` inside the `parser` package.

This indicates that when `symgo`'s evaluator is executing the body of `parser.Parse`, it is incorrectly using the scope or environment of the *calling* package (`main`) instead of the *callee's* package (`parser`). Since `processPackage` is unexported, it is not visible in the `main` package's scope, leading to the failure.

## How to Reproduce

1.  Build the `find-orphans` tool.
    ```bash
    go build -o /tmp/find-orphans ./examples/find-orphans
    ```

2.  Run `find-orphans` on the `examples/convert` directory.
    ```bash
    (cd examples/convert && /tmp/find-orphans -v ./ )
    ```

You will observe the "identifier not found" error for `processPackage` and a subsequent incorrect orphan report for `formatCode`.

## Root Cause Analysis

The root cause lies in `symgo/evaluator/evaluator.go`, specifically in how the evaluation context (the `pkg *scanner.PackageInfo` parameter) is propagated during recursive calls to the `Eval` function.

While `applyFunction` correctly identifies the callee's package (`fn.Package`) and uses it to evaluate the function's body, there appears to be a flaw in how the `pkg` context is maintained through the chain of `eval*` function calls. When `evalCallExpr` is invoked for an unexported function like `processPackage` from inside the `parser` package, it seems to be doing so with the `pkg` context of the original `main` package still active.

When `evalIdent` is then called to resolve `processPackage`, it receives the incorrect package context. It looks for the unexported function in the wrong package, fails to find it, and throws the error.

The fix requires ensuring that the `pkg *scanner.PackageInfo` parameter in the `Eval` function and all its subsidiary `eval*` methods always reflects the package of the code currently being executed. The `pkg` context must be correctly updated when `applyFunction` begins evaluating a function from a different package. The current implementation in `applyFunction` seems to attempt this, but a subtle flaw is causing the context to be lost or incorrect during the subsequent evaluation of the function body's expressions.

A suspicious code block in `applyFunction` manually adds imports to the function's environment. This logic is likely flawed, redundant, and may interfere with the correct lexical scoping provided by the enclosing package environment, `fn.Env`. Removing or correcting this logic is a probable path to a solution.

## Deeper Investigation and New Findings

Further investigation revealed that the initial hypothesis, while plausible, was not the root cause. The problem is more fundamental, related to how package-level environments are populated for imported packages.

### Initial Fix Attempts and Failures

The investigation started by focusing on the suspicious block in `applyFunction` that re-populates import information. Two fixes were attempted:

1.  **Removing the Block:** The entire block was commented out. This did not resolve the issue.
2.  **Using the Package Cache:** The block was modified to use the evaluator's package cache (`getOrLoadPackage`) instead of creating new, empty package objects. This also failed to fix the bug.

These failures, confirmed with a more reliable and isolated test case, indicated that the problem lay elsewhere.

### Discovery: Package-Level Variables are Not Evaluated

To create a more controlled testing environment, a new test (`TestCrossPackageUnexportedResolution`) was added to `symgo/symgo_scope_test.go`. This test simulated a cross-package call and checked for symbol resolution. A key modification to this test, which involved a recursive call using a package-level variable (`var count = 0`), led to a critical discovery. The test failed with:

```
identifier not found: count
```

This revealed a new, more fundamental bug: **package-level `var` declarations from imported packages are not being evaluated at all.**

The function `ensurePackageEnvPopulated` in `symgo/evaluator/evaluator.go` is responsible for populating the environment for imported packages on-demand. A close inspection showed that it correctly handles `func` and `const` declarations, but completely ignores `var` declarations. This is the true root cause of the resolution failures.

### The Final Roadblock: Missing AST Information

An attempt was made to fix `ensurePackageEnvPopulated` by adding logic to evaluate `var` declarations. This immediately hit a roadblock: the `scanner.VariableInfo` struct, which is provided by the `go-scan/scanner` dependency, does not store the necessary `*ast.GenDecl` node required to re-evaluate the variable declaration. It only contains a more generic `ast.Node`, which is likely the `*ast.ValueSpec` for the variable.

This means a complete fix requires changes to the `scanner` package itself to expose the full `*ast.GenDecl` node. The current plan is to modify `scanner.VariableInfo` and the scanner logic to include this information, and then use it in `symgo` to correctly populate package environments. This documentation is being updated to record these findings before proceeding with the cross-package modification.

## Update: The plot thickens - State loss during recursion

The proposed fix in the previous section was implemented.
1.  `scanner.VariableInfo` was updated to include `*ast.GenDecl`.
2.  `scanner.Scanner` was updated to populate this field.
3.  `symgo.Evaluator.ensurePackageEnvPopulated` was updated to iterate over `Variables` and evaluate their `GenDecl`s to populate the package environment.

This successfully resolved the `identifier not found` error for the `count` variable in `TestCrossPackageUnexportedResolution`.

However, this fix revealed a deeper issue. The test now fails with an `infinite recursion detected` error. This is because the state of the `count` variable is not being persisted across the recursive calls to `getSecretMessage`. The symbolic execution engine believes `count` is always `0`, causing the function to call itself endlessly until the recursion check aborts the execution.

A detailed analysis of the environment (`object.Environment`), variable modification (`evalIncDecStmt`), and function application logic (`applyFunction`) did not reveal an obvious cause for this state loss. The environment correctly encloses outer scopes, and the `IncDec` logic appears to modify the variable's value in place.

This indicates a more subtle bug in how state is managed or propagated during recursive calls in the evaluator. The issue remains unresolved pending further investigation.

## Further Investigation (2025-09-06)

A follow-up investigation was performed to resolve the "infinite recursion detected" error. The findings are documented below.

### Analysis of the "Infinite Recursion" Error

The investigation started by adding detailed logging to key parts of the evaluator in `evaluator.go`:
1.  **`applyFunction`**: To log the value of the `count` variable at the beginning of each call to `getSecretMessage`.
2.  **`evalIncDecStmt`**: To log the value of `count` before and after it was incremented.
3.  **`evalIdent`**: To log the memory address of the `*object.Variable` for `count` whenever it was accessed.

The logs produced a clear, but contradictory, result:
-   **State IS Updated**: The logs definitively showed that on the first call to `getSecretMessage`, the `count` variable (at a specific memory address) was correctly incremented from `0` to `1`.
-   **Failure Cause**: The test failed because the original recursion check in `applyFunction` (`frame.Fn.Def == f.Def`) fired on the second, recursive call to `getSecretMessage`. It aborted the execution *before* the function body was entered, and thus before the updated value of `count` could be checked by the `if count > 0` condition.

This revealed that the hypothesis in the previous document section ("State loss during recursion") was incorrect. The state is managed correctly, but the recursion check is too aggressive for this stateful test case.

### Attempts to Fix

Based on this analysis, several fixes were attempted:

1.  **Relax the Recursion Check**: The check was modified to allow a function to be on the call stack once already, but not twice (`recursionCount > 1`).
    -   **Result**: This failed with the same "infinite recursion detected" error. This result is the core of the unresolved problem. It implies that even with the relaxed check, the second call to `getSecretMessage` still evaluated `count > 0` as false, triggering a third call which was then caught. This contradicts the logging evidence.

2.  **Modify `evalIfStmt` Control Flow**: A hypothesis was formed that `evalIfStmt` was swallowing the `ReturnValue` from the `if` block's body. The logic was changed to propagate `ReturnValue` objects.
    -   **Result**: This caused the test run to hang indefinitely, likely by creating a genuine infinite loop. This approach seems to violate the intended symbolic nature of the evaluator (which often needs to evaluate all paths, not stop on the first `return`). This change was reverted.

### Unresolved Contradiction and Future Direction

The central issue remains a deep contradiction:
-   **Evidence A (Logs):** The `count` variable's underlying integer object is successfully modified in place.
-   **Evidence B (Test Behavior):** The program behaves as if the `count` variable was *not* modified on the subsequent recursive call.

This suggests an extremely subtle bug in how environments are being created, cached, or referenced during recursive evaluation. Despite logs showing the same memory addresses for the environment object (`fn.Env`) across calls, it's possible that a stale version of the environment or the variable within it is being resolved and used by the second call.

The relevant files for the next investigation should be `evaluator.go`, `resolver.go`, and `accessor.go`, with a focus on the interaction between `applyFunction`, `extendFunctionEnv`, and `getOrLoadPackage`. The bug is likely not in the `object.Environment` implementation itself, but in how instances of it are managed by the evaluator during the call lifecycle.
