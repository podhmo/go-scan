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

To create a more controlled testing environment, a new test (`TestCrossPackageUnexportedResolution`) was added to `symgo/symgo_scope_test.go`. This test simulated a cross-package call and checked for symbol resolution. The test used a package-level variable (`var count = 0`) to control a recursive call. This led to a critical discovery when the test failed with:

```
identifier not found: count
```

This revealed a fundamental bug: **package-level `var` declarations from imported packages are not being evaluated at all.**

The function `ensurePackageEnvPopulated` in `symgo/evaluator/evaluator.go` is responsible for populating the environment for imported packages on-demand. A close inspection showed that it correctly handles `func` and `const` declarations, but completely ignores `var` declarations. This is the true root cause of the resolution failures seen in both the test and the `find-orphans` tool.

### Clarification: Unexported Function Resolution Works

To confirm that the `var` issue was the true root cause, a more minimal version of the test (`TestCrossPackageUnexportedResolution_Minimal`) was created. This version removed the package-level variable and the recursion, testing only the cross-package call to an unexported function.

**This minimal test passed.**

This result is significant because it proves that the initial hypothesis—a general failure in resolving unexported functions across packages—was incorrect. The symbolic execution engine *can* correctly resolve and execute unexported functions from other packages, provided no unevaluated package-level state (like `var`s) is involved.

Therefore, the failure of `find-orphans` to resolve `processPackage` is not a simple scoping bug but a side effect of the same underlying problem: the environment for the `parser` package is not correctly populated with all its necessary components due to the failure to handle `var` declarations, which likely leads to an unstable state that prevents subsequent lookups from succeeding.

### The Final Roadblock: Missing AST Information

An attempt was made to fix `ensurePackageEnvPopulated` by adding logic to evaluate `var` declarations. This immediately hit a roadblock: the `scanner.VariableInfo` struct, which is provided by the `go-scan/scanner` dependency, does not store the necessary `*ast.GenDecl` node required to re-evaluate the variable declaration. It only contains a more generic `ast.Node`, which is likely the `*ast.ValueSpec` for the variable.

This means a complete fix requires changes to the `scanner` package itself to expose the full `*ast.GenDecl` node. The current plan is to modify `scanner.VariableInfo` and the scanner logic to include this information, and then use it in `symgo` to correctly populate package environments. This documentation is being updated to record these findings before proceeding with the cross-package modification.
