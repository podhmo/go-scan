# Trouble Analysis: Refactoring `AssignStmt` and the Challenge of Implicit Type Tracking

This document details the investigation into a complex issue within the `symgo` evaluator related to assignment statements (`=`, `:=`). The task was not to implement assignment from scratch, but rather to refactor the existing logic. This refactoring effort inadvertently caused significant regressions, revealing the critical and subtle nature of type tracking in the symbolic engine.

## 1. The Original Goal: Refactor and Clarify Assignment Logic

Before this task, many assignment-related tests, such as `TestEval_InterfaceMethodCall_AcrossControlFlow`, were passing. This indicates that the evaluator *could* handle complex assignments. However, the logic was likely scattered across different parts of the `Eval` function (e.g., in `evalIdent`, `evalCallExpr`, etc.) rather than being centralized in a single `evalAssignStmt` handler.

The motivation for the refactoring was to:
-   **Centralize Logic**: Create a single `evalAssignStmt` function to handle all forms of assignment (`=`, `:=`).
-   **Improve Readability**: Make the code easier to understand and maintain by having a clear, dedicated place for assignment evaluation.
-   **Explicitly Handle Edge Cases**: Add clear logic for multi-value returns, type assertions, and map indexing on the RHS of an assignment, which were previously handled implicitly.

The initial task was triggered by the need to implement a core `AssignStmt` handler to provide a foundation for these improvements. The initial tests, like `TestEvalAssignStmt_Simple`, were created to validate this new, centralized handler.

```go
// From symgo/integration_test/assign_stmt_test.go
// This test was created to validate the new, centralized assignment logic.
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

## 2. The Refactoring and the Unforeseen Regressions

The refactoring began by creating a new `evaluator_stmt.go` file and directing all `ast.AssignStmt` nodes to a new `evalAssignStmt` function. This immediately broke a wide range of tests, revealing that the previous, distributed logic had handled many subtle cases implicitly.

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

## 4. Iterative Fixes and Current Status

Several attempts were made to fix this, each revealing a deeper layer of the problem:

-   **Fixing `var` Scope**: Corrected `evalGenDecl` to use `env.SetLocal`, which fixed simple variable declarations but not the interface assignment issue.
-   **Unwrapping `ReturnValue`**: Ensured that values returned from functions were unwrapped, which fixed direct assignments from function calls but not the type tracking.
-   **Improving `updateVarOnAssignment`**: The latest fixes have focused on making this helper function smarter. It now explicitly checks if a variable is an interface and then attempts to deduce the concrete type of the value being assigned (handling pointers correctly) and records a string representation of that type in the `PossibleTypes` map.

**Current Status:**
-   The `assign_stmt_test.go` tests, which cover basic assignment forms, are now passing.
-   `TestEval_InterfaceMethodCall_AcrossControlFlow` is also passing, indicating the refined `updateVarOnAssignment` logic is correctly tracking concrete pointer types.
-   However, `TestShallowScan_AssignIdentifier_WithUnresolvedInterface` still fails. This suggests that the current type-tracking logic, while improved, is not yet robust enough to handle cases where the concrete type being assigned is itself a symbolic placeholder from a shallow-scanned (unresolved) package.

## 5. Conclusion & Next Steps

The refactoring, while initially causing significant disruption, has successfully centralized the assignment logic and made the once-implicit requirements of the type system explicit. This has ultimately improved the maintainability and clarity of the code.

The primary remaining task is to enhance `updateVarOnAssignment` to correctly handle assignments where the RHS is an unresolved type. This will likely involve improving how type information is extracted from `*object.SymbolicPlaceholder` objects, ensuring that even partially available type information is correctly tracked.