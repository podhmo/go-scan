# Continuation 2: Enhancing `symgo` for Type-Narrowed Member Access

This document records the continuation of the work to enhance `symgo`'s handling of type assertions and type switches, picking up from `docs/cont-symgo-type-switch.md`.

## Goal

The goal remains the same: to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a `switch v := i.(type)` statement or an `if v, ok := i.(T); ok` type assertion.

## Previous State

The work started from the state defined in `docs/cont-symgo-type-switch.md`, which involved a clean repository with a new test file containing failing tests.

## Work Done

The following steps from the plan in `docs/cont-symgo-type-switch.md` have been successfully implemented and verified to be present in the codebase:

1.  **Added `Original` field to `SymbolicPlaceholder`**: The struct `object.SymbolicPlaceholder` in `symgo/object/object.go` was modified to include an `Original object.Object` field. This field is intended to hold the concrete object that a placeholder represents after a type assertion.

2.  **Populated the `Original` field in type assertions**: The `evalTypeSwitchStmt` and `evalAssignStmt` functions in `symgo/evaluator/evaluator.go` were modified to correctly populate this new `Original` field on the `SymbolicPlaceholder` objects they create for type-narrowed variables.

3.  **Implemented stateful struct literals**: The `evalCompositeLit` function in `symgo/evaluator/evaluator.go` was enhanced. It now correctly evaluates the values of fields in a struct literal and stores them in the `State` map of the `*object.Instance` it creates, ensuring the instance carries its state.

## Failures and Challenges

The primary blocker in this session was the persistent failure of the `replace_with_git_merge_diff` tool when attempting to perform a key refactoring step on the `evalSelectorExpr` function in `symgo/evaluator/evaluator.go`.

-   Multiple attempts were made to refactor the function, both in a single large step and in smaller, incremental steps.
-   These attempts consistently failed, often leaving the `evaluator.go` file in a syntactically incorrect state, which required repeated restoration.
-   This consumed a significant amount of time and prevented the implementation of the core logic required to make the tests pass.

## Next Steps (for the next agent)

The original plan remains sound. The next agent should focus on completing the refactoring of `evalSelectorExpr` and the related logic.

1.  **Move and Fix `resolveSymbolicField`**:
    *   The method `ResolveSymbolicField` should be moved from `symgo/evaluator/resolver.go` to `symgo/evaluator/evaluator.go` and renamed to `resolveSymbolicField`.
    *   The implementation of `resolveSymbolicField` must be updated to first check the `State` map of the receiver (`*object.Instance` or `*object.Pointer` to one) before falling back to creating a new placeholder.

2.  **Refactor `evalSelectorExpr`**:
    *   This is the most critical step. The function `evalSelectorExpr` in `symgo/evaluator/evaluator.go` needs to be refactored into a wrapper and a core logic function (e.g., `evalSelectorExprForObject`).
    *   The new `evalSelectorExpr` wrapper must inspect its `leftObj`. If it's a `SymbolicPlaceholder` with a non-nil `Original` field, it must perform a type compatibility check.
    *   **Type Check**: The check should verify that the concrete type of the `Original` object is compatible with the narrowed type of the `SymbolicPlaceholder`.
    *   **Unwrap or Prune**: If the types are compatible, the wrapper should "unwrap" the placeholder and call the core logic function (`evalSelectorExprForObject`) with the `Original` object. If they are not compatible, it should return a generic placeholder to prune the impossible symbolic path.

3.  **Run Tests and Fix**:
    *   After the refactoring, run `go test -v ./symgo/evaluator/...`.
    *   The tests in `symgo/evaluator/evaluator_if_typeswitch_test.go` should now pass. Debug any remaining issues until they do.
