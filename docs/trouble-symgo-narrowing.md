# Trouble Analysis: Type Narrowing and Tracking in `symgo` Assignments

This document details the ongoing investigation into a complex issue within the `symgo` evaluator related to assignment statements (`=`, `:=`). While the basic evaluation of assignments is implemented, significant regressions in tests reveal a core challenge in correctly tracking type information, particularly when interfaces are involved.

## 1. The Goal: Implement `AssignStmt`

The initial task was to implement evaluation for `ast.AssignStmt` to make basic assignment patterns work within the `symgo` engine. This was driven by failures in newly created integration tests, such as `TestEvalAssignStmt_Simple`:

```go
// From symgo/integration_test/assign_stmt_test.go
func TestEvalAssignStmt_Simple(t *testing.T) {
	source := `
package main
func main() int {
	var x int
	x = 10
	return x
}
`
// ... test logic asserts that the return value is 10 ...
}
```

This simple case, along with short variable declarations (`x := 10`) and tuple assignments (`x, y = f()`), were failing. The initial investigation revealed two primary issues:

1.  **Variable Declaration (`var`) Scope**: The `evalGenDecl` function, which handles `var x int`, was not correctly registering `x` in the current scope. It was using `env.Set` instead of `env.SetLocal`, which could cause it to incorrectly update a variable in an outer scope instead of declaring a new one.
2.  **Assignment (`=`, `:=`) Logic**: A handler for `ast.AssignStmt` was completely missing from the evaluator's main `Eval` switch statement.

The goal was therefore to implement `evalAssignStmt` and fix the related scoping issues to make these fundamental test cases pass, establishing a baseline for assignment functionality.

## 2. The Implementation Journey: A Series of Unforeseen Complexities

The path to implementing this seemingly simple statement revealed deep-seated complexities in the evaluator's design and testing strategy.

### 2.1. Initial Hurdle: Circular Dependencies

The first attempt to write a test for the new `evalAssignStmt` function immediately failed at compile time.
-   **Problem**: `symgo/evaluator/assign_stmt_test.go` needed to import the main `symgo` package to use its interpreter and test helpers. However, `symgo` already depended on `symgo/evaluator`, creating a circular import.
-   **Solution**: The test was moved to a separate `symgo/integration_test` package. This standard Go pattern allows the test to import `symgo` as an external package, breaking the cycle. This also necessitated refactoring the test to use the `symgotest` helper framework, which proved to be a cleaner, more robust way to write `symgo` tests.

### 2.2. Iterative Refinement and Bug Fixes

With the testing framework in place, an initial implementation of `evalAssignStmt` was added. This version handled the basic mechanics of evaluating the right-hand side (RHS) and setting the value in the environment for the left-hand side (LHS).

However, running the full test suite revealed numerous regressions. This began a multi-stage debugging and refinement process.

-   **Bug 1: Mishandling of `ReturnValue`**: Many tests involving function calls on the RHS of an assignment failed. The reason was that `applyFunction` wraps its results in an `*object.ReturnValue`, but `evalAssignStmt` was not unwrapping this object to get the actual value before assignment. The fix was to add logic to unwrap `*object.ReturnValue` objects after any `Eval` call within the assignment logic.

-   **Bug 2: Static vs. Dynamic Type Inference**: A critical and subtle bug emerged. In code like `var i Animal = NewDog()`, the static type of `i` is the `Animal` interface. However, the evaluator was incorrectly inferring the type of `i` from the dynamic type of the RHS (`*Dog`). This caused failures in tests that expected `i` to be treated as an interface.
    -   **Solution**: This required a more complex fix.
        1.  The `object.ReturnValue` was enhanced to carry the *static* `FieldType` from the function's signature.
        2.  `applyFunction` was updated to populate this static type information.
        3.  The `assign` helper was modified to prioritize this static type information when creating a new variable in a `:=` declaration, thus preserving the crucial interface type information.

### 2.3. The Current Challenge: Interface Type Tracking

After several rounds of fixes, the core assignment tests in `assign_stmt_test.go` now pass. However, more advanced tests, particularly `TestEval_InterfaceMethodCall_AcrossControlFlow` and `TestShallowScan_AssignIdentifier_WithUnresolvedInterface`, continue to fail.

The core of the problem lies in the `updateVarOnAssignment` helper function. This function is responsible for a key piece of symbolic analysis: when a value is assigned to an interface variable, it must record the concrete type of that value in the variable's `PossibleTypes` map.

-   **The Problem**: The current logic for determining the "key" for this map (the string representation of the concrete type) is fragile. It struggles to correctly construct the string for complex types, especially pointers to structs (e.g., `*main.Dog`) and types from shallowly-scanned packages.
-   **Example Failure (`TestEval_InterfaceMethodCall_AcrossControlFlow`)**:
    ```go
    var s Speaker // Interface
    if B {
        s = &Dog{}
    } else {
        s = &Cat{}
    }
    s.Speak()
    ```
    The test fails because the `PossibleTypes` map for `s` does not contain both `"*example.com/me.Dog"` and `"*example.com/me.Cat"`. The logic in `updateVarOnAssignment` fails to correctly identify and stringify the concrete types `*Dog` and `*Cat` when they are assigned to the interface `s`.

## 3. Current Status & Next Steps

The implementation of `AssignStmt` is partially complete. The basic mechanics are in place, but the interaction with the type system, especially concerning interfaces and pointers, remains a significant challenge.

The current implementation in `updateVarOnAssignment` is an attempt to solve this by manually constructing a type key. However, this has proven to be brittle.

**Next Steps (Exploration):**

The path forward requires a more robust way to handle type flow. The current hypothesis is that the problem is not just in how the type *key* is created, but in how the *type information itself* is propagated from the value (`val`) to the variable (`v`).

1.  **Re-evaluate `updateVarOnAssignment`**: The current logic is complex. A simpler, more direct approach might be better. Instead of trying to manually construct a `FieldType` and then stringify it, we should focus on ensuring the `object.Variable` (`v`) correctly inherits all necessary type information (`TypeInfo` and `FieldType`) from the assigned `object.Object` (`val`).
2.  **Focus on `evalCompositeLit`**: The problem might originate earlier. When `&Dog{}` is evaluated, the resulting `*object.Pointer` (pointing to an `*object.Instance` or `*object.Struct`) must have its `TypeInfo` set correctly and unambiguously to `Dog`. If the type information is lost at this stage, no amount of logic in the assignment function can recover it. A thorough review of `evalCompositeLit` and how it interacts with `evalStarExpr` (for the `&` operator) is warranted.
3.  **Isolate and Fix**: Continue with the strategy of focusing on one failing test at a time, starting with `TestEval_InterfaceMethodCall_AcrossControlFlow`, as it represents the most fundamental aspect of the current problem.

The implementation remains in an exploratory phase. The goal is to find a robust and maintainable solution for tracking types through assignments, which is a cornerstone of the `symgo` engine's analysis capabilities.