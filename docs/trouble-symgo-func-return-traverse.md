# Trouble: `symgo` fails to traverse calls inside returned functions

This document details the investigation into a bug where `find-orphans` incorrectly flags a function as an orphan when its only usage is within a closure returned by another function.

## Problem Description

The `find-orphans` tool, which relies on the `symgo` symbolic execution engine, should be able to trace function calls through complex scenarios. A case was identified where this analysis fails:

1. A function `GetHandler` is called.
2. `GetHandler` returns another function (a closure).
3. This returned closure is then executed.
4. Inside the closure, a helper function `usedByReturnedFunc` is called.

In this scenario, `find-orphans` incorrectly reports `usedByReturnedFunc` as an orphan, because the analysis does not seem to connect the final call back to the helper function.

### Minimal Reproducible Example

The following code, when analyzed by `find-orphans`, demonstrates the issue.

```go
// main.go
package main
import "example.com/func-return/service"
func main() {
    handler := service.GetHandler()
    handler()
}

// service/service.go
package service

// GetHandler returns a function that uses an internal helper.
func GetHandler() func() {
    return func() {
        // This call is not being detected by the analyzer.
        usedByReturnedFunc()
    }
}

// This function should NOT be an orphan.
func usedByReturnedFunc() {}
```

## Investigation and Failed Attempts

The investigation focused on the `symgo` evaluator (`symgo/evaluator/evaluator.go`) and why the call inside the closure was not being registered by the `defaultIntrinsic` used by `find-orphans`. Several attempts to fix this were made, none of which were successful.

The core of the evaluation logic seems correct:
- The call to `GetHandler` returns an `object.Function` representing the closure.
- This `object.Function` correctly captures its defining environment.
- The subsequent call to `handler()` invokes `applyFunction` on this `object.Function`.
- `applyFunction` then calls `e.Eval()` on the closure's body.
- This *should* trigger `evalCallExpr` for `usedByReturnedFunc`, which in turn *should* trigger the `defaultIntrinsic` and mark the function as used.

The failure suggests a subtle issue in how the environment or usage map is handled during this multi-step evaluation.

### Attempt 1: Eager Scan in `evalReturnStmt`

The first hypothesis was that the analysis needed to be more "eager". The idea was to scan the body of any function literal as soon as it was returned.

**Change:** Modified `evalReturnStmt` to check if the return value is an `*object.Function`. If so, immediately call `e.Eval()` on its body (`fn.Body`) using its captured environment (`fn.Env`).

```go
// in evalReturnStmt
if fn, ok := val.(*object.Function); ok {
    if fn.Body != nil {
        e.Eval(ctx, fn.Body, fn.Env, fn.Package)
    }
}
```

**Result:** This did not fix the issue. The test still failed. This is the most confusing result, as it implies that simply evaluating the body of the closure is not enough to register the usage, which points to a deeper issue.

### Attempt 2: Eager Scan in `evalCallExpr`

A variation of the first attempt was to scan the function, not when it was returned, but immediately after the function call that produced it completed.

**Change:** Modified `evalCallExpr` so that after `applyFunction` returns, it checks if the `ReturnValue` contains an `*object.Function`. If so, it calls `scanFunctionLiteral` on it. `scanFunctionLiteral` is the helper function used to scan function arguments.

```go
// in evalCallExpr, after result := e.applyFunction(...)
if ret, ok := result.(*object.ReturnValue); ok {
    if fn, ok := ret.Value.(*object.Function); ok {
        e.scanFunctionLiteral(ctx, fn)
    }
}
```

**Result:** This also failed. It seems that whether the scan is done at the return site or the call site, the usage is not registered.

## Conclusion and Next Steps

The problem is non-trivial and likely stems from a subtle bug in the evaluator's state management. The "lazy" evaluation path, where the closure is executed by `applyFunction`, seems logically correct but fails in practice. The "eager" evaluation attempts, which try to scan the closure's body ahead of time, also fail.

For the next developer, the following steps are recommended:

1.  **Instrument the Evaluator:** Standard logging has proven difficult. It would be beneficial to add a dedicated tracer to the evaluator that can be enabled via an option. This tracer should log:
    -   Entry and exit of key functions (`evalCallExpr`, `applyFunction`, `evalIdent`).
    -   The name of the function being evaluated or applied.
    -   The current environment scope (perhaps by logging the names of variables defined in the local scope).
    -   When the `defaultIntrinsic` is triggered and what function it is triggered for.

2.  **Focus on `applyFunction`:** The crucial moment is when `handler()` is called in the test case. A breakpoint or detailed trace inside `applyFunction` when it's handling the closure is needed. The key questions are:
    -   Is the `fn.Env` (the closure's captured environment) correct? Does it contain the `service` package scope?
    -   When `e.Eval(ctx, fn.Body, ...)` is called, does the subsequent call to `evalIdent("usedByReturnedFunc")` succeed?
    -   If it succeeds, does the `object.Function` it returns have the correct `Def` (`*scanner.FunctionInfo`)? The `defaultIntrinsic` in `find-orphans` relies on this definition to update its usage map.

3.  **Verify the `defaultIntrinsic`:** The intrinsic itself is a closure that captures the `analyzer` and its `usageMap`. While it seems unlikely to be the issue, it's worth verifying that the `analyzer.usageMap` being modified inside the intrinsic is the same instance that is used for the final report.

This issue highlights a weakness in the symbolic evaluator's ability to handle closures and returned functions, and resolving it will significantly improve the accuracy of `find-orphans`.

---

## Resolution

The investigation revealed that the root cause was not in the evaluation logic for function calls (`applyFunction`) or function literals (`scanFunctionLiteral`) itself, but in the way the `symgo` evaluator managed package-level environments.

The key issue was that a package's environment (containing all its top-level function and variable declarations) was not being populated correctly or at the right time. When the `GetHandler` function was resolved, its definition was associated with an empty environment for its package (`service`). Consequently, when the closure was created within `GetHandler`, it captured this empty environment. Later, when the closure was executed, it was unable to find `usedByReturnedFunc` within its captured (and empty) environment chain, leading to the `identifier not found` error observed during testing.

The previous developers' attempts at "eager scanning" failed because even though they were correctly forcing an evaluation of the closure's body, they were doing so with the same incomplete environment, so the identifier resolution still failed.

The solution involved two main changes:

1.  **On-Demand Package Environment Population**: A new function, `ensurePackageEnvPopulated`, was added to the evaluator. This function is responsible for populating a package's `object.Package.Env` with all of its top-level declarations (as either full `object.Function` objects or `object.SymbolicPlaceholder` objects, depending on the `scanPolicy`).

2.  **Triggering Population in `evalSelectorExpr`**: The `evalSelectorExpr` function was modified. Now, whenever it resolves a selector on a package (e.g., `service.GetHandler`), it first ensures the package has been scanned from source (if it hasn't been already) and then calls `ensurePackageEnvPopulated`. This guarantees that by the time a function like `GetHandler` is retrieved for execution, its definition is linked to a fully populated environment for its containing package.

With these changes, the closure created by `GetHandler` correctly captures the complete environment of the `service` package. When the closure is later executed, it can successfully resolve `usedByReturnedFunc` within that captured environment, allowing the `defaultIntrinsic` to be called and the usage to be correctly tracked by `find-orphans`.
