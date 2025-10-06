# Trouble Analysis: The `AssignStmt` Refactoring and Its Unforeseen Consequences

This document details the investigation into a complex series of regressions within the `symgo` evaluator. The work did not originate as a simple bug fix, but as a strategic, exploratory refactoring aimed at a larger goal: **implementing support for type-narrowing constructs**. This attempt, however, revealed critical, implicit assumptions in the existing evaluation logic, leading to widespread test failures that needed to be addressed.

## 1. The Original Goal: Supporting Type-Narrowing

The initial motivation for this task was to begin implementing the features described in `TODO.md` under "Enhance Type-Narrowed Member Access." Specifically, the goal was to enable `symgo` to understand code patterns like:

**`if-ok` type assertion:**
```go
if v, ok := i.(T); ok {
    // Inside this block, `v` is known to be of type `T`.
    // The evaluator should be able to resolve method calls on `v` as type `T`.
}
```

**`type switch`:**
```go
switch v := i.(type) {
case T1:
    // `v` is of type `T1`
case T2:
    // `v` is of type `T2`
}
```

Before implementing this new logic, a strategic decision was made to first refactor the existing, scattered logic for assignments into a single, centralized `evalAssignStmt` function. The hypothesis was that a clean, unified assignment handler would make it easier and safer to subsequently add the new type-narrowing logic. This refactoring was an *exploratory trial* within the larger solution space.

## 2. The Refactoring Attempt and the Cascade of Regressions

This refactoring effort, intended as a preparatory cleanup, immediately destabilized the evaluator.

1.  **Initial Implementation**: A new `evaluator_stmt.go` was created, and a basic `evalAssignStmt` handler was added to the evaluator's main `Eval` function.
2.  **Cascading Failures**: Running the test suite revealed that numerous existing tests—which had been passing—were now failing. This was the critical moment of realization: **assignment logic was not missing, but was implicitly and correctly handled by a combination of other evaluation functions.** My attempt to centralize it had broken these fragile, implicit connections.
3.  **Shift in Focus**: The task immediately pivoted from "refactoring for a new feature" to "fixing the regressions caused by the refactoring."

The core of the regressions stemmed from a single, complex problem: **the loss of type information when assigning values to interface variables.**

**Example of a Broken Test (`TestEval_InterfaceMethodCall_AcrossControlFlow`):**
This test, which was previously passing, verifies a critical static analysis capability:
```go
func main() {
    var s Speaker // s is statically typed as the Speaker interface.
    if someCondition {
        s = &Dog{} // s is assigned a concrete type *Dog
    } else {
        s = &Cat{} // s is assigned a different concrete type *Cat
    }
    s.Speak() // The evaluator must know that s could be a *Dog OR a *Cat.
}
```
After the refactoring, this test began to fail. The `PossibleTypes` map for the variable `s` was no longer being populated with both `"*example.com/me.Dog"` and `"*example.com/me.Cat"`.

## 3. The Core Conflict: Static vs. Dynamic Type Tracking

The investigation into this regression revealed a fundamental tension in the evaluator's design:

-   **Interpreter Behavior**: A standard interpreter would assign a concrete value (`&Dog{}`) to `s`. The static type `Speaker` would be used for compile-time checks but discarded at runtime in favor of the concrete type.
-   **Symbolic Tracer Requirement**: For static analysis, `symgo` must do both. It needs to know that the *static type* of `s` is `Speaker` (to resolve the `Speak` method) *and* it needs to accumulate a list of all *possible dynamic types* (`*Dog`, `*Cat`) that could be assigned to `s` across all code paths.

The original, implicit logic handled this correctly. The new, centralized `assign` helper function initially failed because it did not correctly manage this dual-type system.

## 4. Current Status: Recovery and Remaining Challenges

After several iterations of debugging, the refactored assignment logic has been stabilized to the point where most of the regressions are fixed. The core assignment tests and the `TestEval_InterfaceMethodCall_AcrossControlFlow` test are now passing.

This was achieved by making the implicit logic explicit within a new `updateVarOnAssignment` helper function, which now correctly tracks concrete types (including pointers) assigned to interface variables.

However, some regressions remain, notably `TestShallowScan_AssignIdentifier_WithUnresolvedInterface`. This indicates that the type-tracking logic, while improved, is not yet robust enough to handle cases where the concrete type being assigned is itself a symbolic placeholder from a shallow-scanned (unresolved) package.

The original goal of implementing type-narrowing for `if-ok` and `type-switch` has been postponed until this foundational assignment logic is fully stabilized. The exploratory refactoring, while disruptive, has been valuable in forcing a deeper understanding and a more explicit, robust implementation of `symgo`'s core type-tracking mechanism.