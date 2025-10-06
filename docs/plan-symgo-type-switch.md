# Plan: Enhance `symgo` for `type-switch` Member Access

## 1. Status: Completed

This document describes the implementation of support for member access (fields and methods) on variables whose types are narrowed by `if-ok` assertions and `type-switch` statements.

-   **`if-ok` Assertion (`v, ok := i.(T)`)**: **Completed**.
-   **`type-switch` Statement (`switch v := i.(type)`)**: **Completed**.

## 2. Goal

The objective was to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed. This is crucial for analyzing code that uses common Go idioms for handling interface values.

## 3. Final Implementation Details

The implementation for `if-ok` and `type-switch` required different strategies to correctly model the behavior of Go and the goals of a symbolic tracer.

### `if-ok` Assertions

For `v, ok := i.(T)` assertions, the implementation focuses on the "ok" path. In this path, the new variable `v` is not just a placeholder; it's a clone of the concrete object held by the interface `i`.

-   **Mechanism**: The `evalAssignStmt` function handles this. It evaluates the object `i`, `Clone()`s its underlying value, and then assigns this clone to `v`.
-   **Type Safety**: The clone's static type information is updated to `T`, allowing the evaluator to correctly resolve members of `T` when `v` is used within the `if` block.
-   **State Preservation**: Crucially, cloning preserves the state (e.g., field values) of the original object, enabling state-dependent analysis.

### `type-switch` Statements

For `switch v := i.(type)`, the goal of a symbolic **tracer** is different from that of an interpreter. An interpreter would only execute the single matching `case`. A tracer, however, must explore **all possible `case` branches hypothetically**.

-   **Mechanism**: The `evalTypeSwitchStmt` function was updated to support this tracer behavior. When analyzing a function that receives a symbolic interface, the evaluator does not know its concrete type. Therefore, for each `case T:` in the switch, it creates a **new, symbolic instance of type `T`** and assigns it to `v`.
-   **Hypothetical Exploration**: This approach allows the evaluator to trace the code path inside every `case` block, assuming `v` is of that block's type. This ensures that method calls like `v.Greet()` or `v.Bark()` are traced in all branches, leading to a complete call graph.
-   **`default` Case**: In the `default` branch, the variable `v` receives a clone of the original object `i` with its original type, as no type narrowing occurs.

This distinction between cloning a value (for a known path) and creating a new symbolic instance (for a hypothetical path) is key to `symgo`'s power as a static analysis tool.

## 4. Verification

The implementation was validated with a new test file, `symgo/evaluator/type_switch_access_test.go`. The final test, `TestTypeNarrowing_TypeSwitchTracerBehavior`, confirms that when a function containing a `type-switch` is called with a symbolic interface, the evaluator successfully traces execution into all `case` branches, proving the tracer-centric implementation is working correctly.