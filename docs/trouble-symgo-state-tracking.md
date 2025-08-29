# [RESOLVED] State Tracking Issue in `symgo`: Global Variables and Method Calls

## Overview

The `symgo` symbolic execution engine previously had an issue with tracking type information across state changes, specifically when the result of a function call is assigned to a global variable and methods are subsequently called on that variable. This issue has been resolved.

This could cause tools like `find-orphans` to incorrectly report methods as "orphans," as well as the factory functions that create the objects on which those methods are called.

## Problem Details

The root cause lay in how the `symgo` evaluator handled the evaluation of variables.

1.  **Function Call and Variable Assignment**:
    Given the following code:
    ```go
    // a.go
    var instance = factory.New() // New() returns *MyType
    ```
    `symgo` correctly evaluated the call to `factory.New()` and marked `New()` itself as "used."

2.  **Loss of Type Information**:
    The `*MyType` object returned by `New()` was assigned to the global variable `instance`. At this point, the `symgo` `Variable` object did not fully retain the detailed type information (`*MyType` in this case) from the value it was assigned (`*object.Instance`).

3.  **Failed Method Call Resolution**:
    The problem became apparent when the `instance` variable was used later in another function (e.g., `init()`).
    ```go
    // a.go
    func init() {
        instance.DoSomething()
    }
    ```
    When `symgo` evaluated `instance.DoSomething()`, it evaluated the `instance` identifier, but in the process, the precise type information (`*MyType`) that the variable should have was lost. Therefore, it could not find the method named `DoSomething`.

As a result, the `(*MyType).DoSomething` method was considered "unused."

## Resolution (2025-08-29)

The issue was deeper than initially thought and required a significant refactoring of the evaluator's architecture. The fix was multi-faceted and addressed an underlying inconsistency in how variables were treated.

### Part 1: Architectural Refactoring

The core of the problem was an inconsistency in how identifiers were evaluated.
- `evalSelectorExpr` (for `instance.DoSomething`) had special logic to fetch the `*object.Variable` for `instance` directly from the environment.
- However, this only worked for variables in the current scope. For variables in outer scopes, `evalSelectorExpr` would fall back to the generic `e.Eval`, which would call `evalIdent`.
- The `evalIdent` function was designed to return the *value* of a variable (`v.Value`), not the `*object.Variable` container itself.

This meant that `evalSelectorExpr`'s special logic for handling `*object.Variable` was bypassed for any variable not in the immediate scope, causing the type information stored on the variable to be lost.

The fix was to make the evaluator's behavior consistent:
1.  **`evalIdent` Changed**: The `evalIdent` function was modified to *always* return the raw object from the environment. For a variable, this is the `*object.Variable` itself.
2.  **Evaluator Functions Updated**: All functions in the evaluator that consume expression results (e.g., `evalBinaryExpr`, `evalCallExpr`, `evalReturnStmt`) were updated to handle receiving a `*object.Variable`. They now "unwrap" the variable to get its `Value` before proceeding.
3.  **`evalSelectorExpr` Simplified**: With the above change, the special-case logic in `evalSelectorExpr` was no longer needed and was removed. It now consistently calls `e.Eval` and receives the `*object.Variable`.

### Part 2: Fixing Type Propagation

The architectural refactoring exposed the final bug. When a variable was initialized from a function call, the evaluator was not correctly unwrapping the result.

- `e.Eval` on a `CallExpr` returns an `*object.ReturnValue`.
- In `evalGenDecl` (for `var instance = New()`), the code was assigning the `*object.ReturnValue` directly as the `Value` of the new `*object.Variable`.
- A `ReturnValue` object has no type information, so the `Variable` was not inheriting the type from the actual result of the function call (an `*object.Pointer`).

The final fix was to add logic in `evalGenDecl` to check for and unwrap the `*object.ReturnValue` before assigning the value and propagating its type information to the new variable.

With these changes, the `Variable` for `instance` correctly stores the `FieldType` indicating it's a pointer and the `TypeInfo` for `MyType`, allowing `findMethodOnType` to successfully resolve the `DoSomething` method call. The test case in `symgo/integration_test/global_var_state_test.go` now passes.
