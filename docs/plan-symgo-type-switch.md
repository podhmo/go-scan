# Plan: Enhance `symgo` for `type-switch` Member Access

## 1. Status

-   **`if-ok` Assertion (`v, ok := i.(T)`)**: **Completed**. Support for this construct was implemented by adding a `Clone() Object` method to the `object.Object` interface. In `evalAssignStmt`, the evaluator now clones the underlying concrete object held by the interface, preserving its state (e.g., field values) in the new variable `v`. This allows for correct member access on the narrowed type.
-   **`type-switch` Statement (`switch v := i.(type)`)**: **To Be Implemented**. This document outlines the remaining work to support member access within `type-switch` statements.

## 2. Goal

The remaining objective is to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a `switch v := i.(type)` statement. The key is to resolve members of the **concrete type** of each `case` block, not just members that might be part of the original interface's method set.

## 3. Investigation and Current State Analysis

A review of the `symgo` evaluator (`symgo/evaluator/evaluator.go`) reveals:

-   **`evalTypeSwitchStmt`:** The current implementation correctly handles the `switch v := i.(type)` syntax. For each `case` clause, it creates a new, scoped environment (`caseEnv`). Within this environment, it defines a new variable `v` and assigns it a `SymbolicPlaceholder` object. This placeholder is correctly imbued with the `TypeInfo` and `FieldType` corresponding to the `case`'s type.
-   **The Gap:** The current implementation assigns a *new* `SymbolicPlaceholder` to the case variable `v`, which does not carry the *value* of the original object being switched on. This means that while the type is correct, any state (like field values) is lost. To trace member access correctly, the evaluator must, similar to the `if-ok` implementation, clone the original object's value for each matching case.

## 4. Proposed Implementation Plan (TDD Approach)

This plan should be executed by a future engineer to implement the feature.

### Step 1: Add Failing Test for Method Call in Type Switch

Add a test case to a new file (`symgo/evaluator/type_switch_access_test.go`) that defines a custom type with a method, uses a type switch to narrow an interface to that type, and calls the method. Use an intrinsic to verify the call is traced.

**Example Test Snippet:**

```go
// In test file
const typeSwitchMethodSource = `
package main

type Greeter struct { Name string }
func (g Greeter) Greet() { inspect(g.Name) }

func inspect(s string) {} // Intrinsic

func main() {
	var i any = Greeter{Name: "World"}
	switch v := i.(type) {
	case Greeter:
		v.Greet() // This method call should be traced
	case int:
		// Other case
	}
}
`
// Test logic would register an intrinsic for inspect() and
// assert that it was called with the value "World".
// This test should fail initially.
```

### Step 2: Enhance the Evaluator (`evalTypeSwitchStmt`)

Modify `symgo/evaluator/evaluator_eval_type_switch_stmt.go` to make the test pass.

1.  **Get Original Value**: At the beginning of `evalTypeSwitchStmt`, evaluate the expression being switched on (e.g., `i` in `i.(type)`) to get the underlying `object.Object`.
2.  **Clone in `case`**: Inside the loop over `case` clauses, when creating the new variable `v`, instead of creating a new `SymbolicPlaceholder`, **clone** the original object obtained in step 1.
3.  **Set Type Info**: On the cloned object, set the `TypeInfo` and `FieldType` to match the `case`'s type. This ensures the variable `v` has both the correct value *and* the correct narrowed type for that specific scope.
4.  **Handle `default`**: In the `default` case, the variable `v` should be a clone of the original object with its original type.

This approach mirrors the successful implementation for `if-ok` assertions and leverages the existing `Clone()` functionality.

### Step 3: Verify and Finalize

Once all new tests pass and existing tests continue to pass, the feature is complete.