# `symgo` Stabilisation Continuation Plan

This document outlines the progress made in stabilizing the `symgo` test suite and details the remaining issues and a proposed plan to address them. This was created because the development environment became unstable, preventing further code modifications.

## Summary of Work and Achievements

The primary goal was to fix all failing tests in the `./symgo/...` packages. Significant progress was made on two fronts:

1.  **Method Recursion (`TestRecursion_method`)**: The timeout failure in the linked-list traversal test was successfully fixed.
    *   **Root Cause**: The evaluator was "stateless" regarding struct fields. When evaluating a composite literal like `Node{ Next: &anotherNode }`, it did not store the association between the `Next` field and the `&anotherNode` value. This caused an infinite loop during recursive method calls (`n.Next.Traverse()`) because the evaluator could not resolve that `n.Next` pointed to a different node with a terminal state.
    *   **Solution**:
        1.  Added a `Fields map[string]object.Object` to the `object.Instance` struct.
        2.  Modified `evalCompositeLit` to populate this `Fields` map when evaluating struct literals, storing the evaluated objects for each field.
        3.  Modified `evalSelectorExpr` to first check this `Fields` map when resolving a field access on an instance or a pointer to an instance. This allows the evaluator to retrieve the concrete object associated with the field, correctly tracking state changes.

2.  **Function Recursion (`TestServeError`)**: The test failing due to a false-positive "infinite recursion" error was fixed.
    *   **Root Cause**: The recursion detector for regular functions was a simple depth counter, which incorrectly flagged legitimate recursive calls that used different arguments (e.g., `ServeError(..., err.Errors[0])`).
    *   **Solution**:
        1.  Added a `recursionCheck map[string]map[string]int` to the `Evaluator`.
        2.  Replaced the depth counter in `applyFunction` with logic that generates a key based on the `Inspect()` value of the function's arguments.
        3.  An infinite loop is now only flagged if the same function is called with the identical argument key more than once.
        4.  A `defer` statement was used to correctly decrement the count for a given call upon returning, allowing for complex, non-infinite call patterns.

## Remaining Issues and Blockers

### 1. Regression: Usage Tracking in Composite Literals (`TestEval_FunctionInCompositeLiteral`)

The fix for method recursion, while successful, introduced a regression. The test, which verifies that functions passed as values in a struct literal are tracked as "used", now fails.

*   **Symptom**: The test's `defaultIntrinsic`, which populates a `usedFunctions` map, is no longer being triggered for functions inside the struct literal initializer.
*   **Analysis**: This is a subtle side effect of the changes to `evalCompositeLit`. Although the new implementation still calls `e.Eval()` on the field values, something about the evaluation order or context appears to have changed, breaking the usage tracking mechanism. The exact cause could not be determined due to the tooling issues described below.

### 2. Unresolved: Interface Method Dispatch

A significant cluster of tests related to interface method calls continue to fail.
*   **Failing Tests**: `TestInterfaceBinding`, `TestEval_ExternalInterfaceMethodCall`, `TestDefaultIntrinsic_InterfaceMethodCall`, etc.
*   **Symptom**: The common pattern is a failure to resolve a method call on an interface variable to the method on the underlying concrete type. For example, `TestInterfaceBinding` expects a call to `w.WriteString(...)` (where `w` is an `io.Writer`) to resolve to the intrinsic for `(*bytes.Buffer).WriteString`, but this does not happen.
*   **Hypothesis**: The evaluator is not correctly tracking or using the concrete type information when an assignment like `var w io.Writer = new(bytes.Buffer)` occurs. The `assignIdentifier` function contains logic related to a `PossibleTypes` map on `object.Variable`, which is likely intended for this purpose but appears to be incomplete or used incorrectly in `evalSelectorExpr`.

### Blocker: Tooling Failure

All further progress was blocked by a persistent failure in the `replace_with_git_merge_diff` tool. The tool repeatedly failed to find the specified search blocks in `symgo/evaluator/evaluator.go`, even immediately after the file was read and the blocks were confirmed to be present. This prevented the implementation of the fixes described below.

## Proposed Next Steps

If the tooling were functional, the following plan would be executed:

1.  **Fix Composite Literal Regression**: Carefully revert and re-apply the changes to `evalCompositeLit` to ensure that both state tracking (`Fields` map) and usage tracking (calling the `defaultIntrinsic`) work correctly. This would involve ensuring that every value expression in a literal is passed through `e.Eval()` in the correct context.

2.  **Fix Interface Dispatch**:
    *   Focus on `evalSelectorExpr` for a receiver whose type is an interface.
    *   The logic should be enhanced to check the `PossibleTypes` map of the variable.
    *   If a concrete type is known, the method lookup should be performed on that concrete type. This would allow the correct intrinsic or function body to be found and executed.
    *   This would likely fix the entire cluster of interface-related test failures.

3.  **Address Anonymous Type Failures**: The failures in `TestSymgo_AnonymousTypes` are likely a subset of the interface dispatch problem and would be re-evaluated after the general fix is in place.
