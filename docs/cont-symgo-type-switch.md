# Continuation: Enhancing `symgo` for Type-Narrowed Member Access

This document outlines the plan to fix `symgo`'s handling of type assertions and type switches.

## Goal

The goal is to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a `switch v := i.(type)` statement or an `if v, ok := i.(T); ok` type assertion.

## Current State

The work is starting from a clean repository state, with the exception of this document and a new test file, `symgo/evaluator/evaluator_if_typeswitch_test.go`, which contains three failing tests that define the required behavior.

## Core Problem

Previous attempts failed because of two related bugs:
1.  `evalCompositeLit`: When evaluating a struct literal (e.g., `User{Name: "Alice"}`), the evaluator was correctly determining the type of the literal but was discarding the evaluated field values (`"Alice"`). The resulting `*object.Instance` only contained type information, not the state of the fields.
2.  `resolver.ResolveSymbolicField`: This function was a stub. When asked to resolve a field access like `v.Name`, it would always return a new placeholder object instead of attempting to look up the value from the receiver.

These two bugs combined meant that even when a type assertion was correctly identified, the concrete values of the fields were lost, making it impossible to trace them.

## The Plan

The fix involves a new, more precise strategy:

1.  **Fix `evalCompositeLit` in `evaluator.go`**:
    *   The function will be modified to differentiate between struct literals and other composite literals (maps, slices).
    *   For struct literals, it will evaluate each field's value and store the result in the `State` map of the `*object.Instance` it creates. This ensures the instance carries its state.
    *   The logic for maps and slices will remain unchanged to prevent regressions.

2.  **Move and Fix `resolveSymbolicField`**:
    *   The method `ResolveSymbolicField` will be moved from `resolver.go` to `evaluator.go` and renamed to `resolveSymbolicField` to break a circular dependency that would be created by giving the resolver access to the evaluator's `forceEval` method.
    *   The call sites in `evaluator.go` will be updated to call `e.resolveSymbolicField`.
    *   The new `e.resolveSymbolicField` method will be implemented to:
        *   Check if the receiver is an `*object.Instance` (or a pointer to one).
        *   If so, look up the requested field name in the instance's `State` map.
        *   If the value is found, return it (after running it through `forceEval` to handle lazy variables).
        *   If the field is not in the state map, or if the receiver is not an instance, fall back to the old behavior of returning a new placeholder.

3.  **Fix `evalSelectorExpr` in `evaluator.go`**:
    *   This function will be refactored into a wrapper (`evalSelectorExpr`) and a core logic function (`evalSelectorExprForObject`).
    *   The wrapper will be responsible for detecting if the expression being selected upon (e.g., `v` in `v.DoA()`) is a placeholder that resulted from a type assertion (i.e., its `Original` field is not nil).
    *   If it is, it will perform a **type compatibility check**: does the concrete type of the `Original` object match the narrowed type of the placeholder?
    *   If the types do **not** match, it means we are in a symbolic path that is impossible at runtime (e.g., evaluating `case B:` for an object of type `A`). The wrapper will prune this path by returning a generic placeholder.
    *   If the types **do** match, the wrapper will "unwrap" the placeholder and call `evalSelectorExprForObject` with the concrete `Original` object, allowing the method/field access to succeed.

4.  **Add `Original` field**: The `object.SymbolicPlaceholder` struct in `symgo/object/object.go` will be modified to include an `Original object.Object` field.

5.  **Populate `Original` field**: `evalTypeSwitchStmt` and `evalAssignStmt` in `evaluator.go` will be modified to populate this `Original` field when they create placeholders for type-narrowed variables.

This comprehensive plan addresses the root causes of the bugs and should lead to a successful implementation.
