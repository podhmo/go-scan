# Trouble Analysis: The `AssignStmt` Refactoring and the Design Conflict with the Symbolic Tracer

This document details the investigation into a complex series of regressions within the `symgo` evaluator. The work did not originate as a simple bug fix, but as a strategic refactoring aimed at a larger goal: implementing support for type-narrowing constructs. This attempt, however, revealed a fundamental conflict between a naive, interpreter-like implementation of assignment and the core design principles of `symgo` as a symbolic tracer.

## 1. The Core Design Principle: `symgo` is a Tracer, Not an Interpreter

As established in `docs/analysis-symgo-implementation.md`, the `symgo` engine is intentionally not a standard interpreter. Its primary purpose is to trace potential code paths to build a call graph. Two key principles from the analysis document are relevant here:

1.  **Exploration of All Branches**: The engine evaluates *both* the `then` and `else` blocks of an `if` statement to discover what *could* happen in any branch.
2.  **Additive State for Interfaces**: To support this, the evaluator uses an "additive update" mechanism. When a value is assigned to an interface variable, it **adds** the concrete type of that value to a `PossibleTypes` set on the variable object, rather than simply overwriting the previous state.

## 2. The Initial Goal and the Refactoring "Trial"

The initial motivation for this task was to begin implementing support for type-narrowing constructs like `if-ok` and `type-switch`, as outlined in the project's `TODO.md`.

As an exploratory first step, a strategic decision was made to refactor the existing, scattered logic for assignments into a single, centralized `evalAssignStmt` function. The hypothesis was that a clean, unified assignment handler would make it easier and safer to subsequently add the new type-narrowing logic.

## 3. The Conflict: A Naive Implementation vs. a Symbolic Design

The initial, centralized `evalAssignStmt` I implemented was fundamentally flawed because it behaved like a standard interpreter: it simply overwrote the value of a variable. This directly conflicted with the "additive state" principle required by the tracer.

This conflict was immediately exposed when running the test suite. A key test, `TestEval_InterfaceMethodCall_AcrossControlFlow`, which had been passing under the old, implicit logic, began to fail.

**The Failing Test Case - A Clear Illustration of the Conflict:**
```go
func main() {
    var s Speaker // s is statically typed as the Speaker interface.
    if someCondition {
        s = &Dog{} // Path 1: s can be a *Dog
    } else {
        s = &Cat{} // Path 2: s can be a *Cat
    }
    s.Speak() // The evaluator must know that s could be a *Dog OR a *Cat.
}
```
This test failed because my new, interpreter-like assignment logic would evaluate one branch (e.g., the `if`), assign `&Dog{}` to `s`, and then evaluate the second branch (`else`), which would simply **overwrite** the state of `s` with `&Cat{}`. The information that `s` could also have been a `*Dog` was lost. The `PossibleTypes` map for `s` ended up with only one entry instead of the required two.

The regression was not a simple bug; it was a fundamental design violation. The task immediately pivoted from "refactoring for a new feature" to "re-aligning the assignment logic with the core principles of the symbolic tracer."

## 4. The Solution: Re-aligning with the Tracer Design

The subsequent iterative fixes were a process of making the assignment logic "tracer-aware."

1.  **`updateVarOnAssignment`**: This helper function became the focal point. Its logic was rewritten to implement the "additive update" principle. Instead of just setting `v.Value`, it now inspects the variable `v`. If `v` is an interface, it determines the concrete type of the value being assigned (`val`) and adds a key representing that type (e.g., `"*example.com/me.Dog"`) to the `v.PossibleTypes` map.
2.  **Scoping and Type Preservation**: Other related bugs were fixed along the way. `evalGenDecl` was corrected to use `env.SetLocal` to respect lexical scope. Logic was also added to preserve the static type of an interface when using `:=` on a function call that returns an interface, preventing the variable's type from being incorrectly narrowed to the concrete returned type at compile time.

## 5. Current Status

This refactoring journey, though difficult, has ultimately been successful.
-   The assignment logic is now centralized and more explicit.
-   Most importantly, it is now correctly aligned with `symgo`'s design as a symbolic tracer. `TestEval_InterfaceMethodCall_AcrossControlFlow` and the other core assignment tests now pass.
-   Some regressions remain, particularly `TestShallowScan_AssignIdentifier_WithUnresolvedInterface`. This indicates that the type-tracking logic, while much improved, is not yet robust enough to handle cases where the concrete type being assigned is itself a symbolic placeholder from a shallow-scanned (unresolved) package. This is a known limitation and is tracked in `TODO.md`.

The original goal of implementing type-narrowing can now be pursued on this more robust and correctly designed foundation.