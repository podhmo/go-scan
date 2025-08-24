# Adjusting Scope Management in `symgo`

This document outlines a challenge in `symgo`'s scope and variable management that requires a strategic decision. The core issue is a conflict between two necessary behaviors for variable assignment (`=`).

## 1. Current Scope Management

`symgo`'s evaluator uses a lexical scoping model implemented with a chain of `object.Environment` structs.

-   **`object.Environment`**: Each environment has a local `store` (a map of name to `object.Object`) and a pointer to an `outer` environment.
-   **Scoping Rules**:
    -   When a new scope is needed (e.g., for a function call or an `if` block), a new `Environment` is created with its `outer` field pointing to the previous environment.
    -   Variable lookup (`env.Get(name)`) searches the current environment's `store` first, then recursively searches the `outer` environment's store.
    -   Variable definition (`:=`) always creates a variable in the local `store` of the current environment.

This model applies to all scoped constructs, including function calls and control-flow blocks.

## 2. The Assignment (`=`) Conflict

The simple assignment operator (`=`) presents a conflict between two essential use cases.

### Case A: In-Place Updates (The Go Way)

In Go, an assignment `x = value` finds the lexically scoped `x` and modifies its value directly. It does *not* create a "shadow" variable. This is critical for functions modifying package-level variables or for simple sequential statements.

The test case `TestMultiValueAssignment` relies on this behavior. It expects a function `main` to modify a package-level variable `x`.

-   **Required Implementation**: `assignIdentifier` should find the environment where `x` was originally defined and modify the `object.Variable` in that environment's `store`.

### Case B: Shadowing for Branch Analysis (The Symbolic Analysis Way)

For a tool like `find-orphans`, we need to analyze the possible states resulting from different control-flow paths. Consider:

```go
var s Speaker // s is in parent scope
if condition {
    s = &Dog{}
} else {
    s = &Cat{}
}
s.Speak()
```

To determine that `s.Speak()` could be a call on either a `*Dog` or a `*Cat`, the evaluator must:
1.  Isolate the `if` and `else` branches.
2.  Record the state of `s` at the end of each branch.
3.  Merge the recorded states after both branches have been evaluated.

This requires that the assignment `s = &Dog{}` inside the `if` block does *not* immediately modify the `s` in the parent scope. Instead, it should create a **shadow copy** of `s` within the `if` block's local environment. This isolates the change. The `mergeEnvironments` function can then inspect the local environments of the branches and merge the results back to the parent `s`.

The test case `TestEval_InterfaceMethodCall_AcrossControlFlow` relies on this behavior.

-   **Required Implementation**: `assignIdentifier` should *always* create a new variable in the current environment's local `store`. If the variable existed in an outer scope, this new local variable effectively "shadows" it for the duration of the current scope.

## 3. Consequences of Exclusive Choices

The core problem is that these two required behaviors are mutually exclusive with a single, simple implementation of assignment.

-   **If we choose the "In-Place Update" model:**
    -   `TestMultiValueAssignment` will **PASS**.
    -   `TestEval_InterfaceMethodCall_AcrossControlFlow` will **FAIL**. The `else` branch will overwrite the variable modification made by the `if` branch, and the final merged state will only contain the type from the last branch executed (`*Cat`).

-   **If we choose the "Shadowing" model (my current implementation):**
    -   `TestMultiValue-Assignment` will **FAIL**. The assignment inside `main` creates a shadow `x` in `main`'s local scope. The package-level `x` is never touched, and the test fails.
    -   `TestEval_InterfaceMethodCall_AcrossControlFlow` will **PASS** (once the full logic is implemented correctly). The branches are isolated, and the merge logic can correctly combine the distinct states of the shadowed variables.

A more sophisticated approach is needed in `assignIdentifier` that can distinguish between these contexts, or the overall evaluation model needs to be adjusted.
