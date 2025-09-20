# Plan: Enhance `symgo` for Type-Narrowed Member Access

## 1. Goal

The objective is to enhance the `symgo` symbolic execution engine to correctly trace method calls and field accesses on variables whose types have been narrowed within a control flow structure. The key is to resolve members of the **concrete type** (e.g., a specific struct's methods), not just members that might be part of the original interface's method set. This applies to two primary Go idioms:

1.  **Type Switch:** In a `switch v := i.(type)` statement, the variable `v` should be recognized as having the specific type of each `case` block, allowing symbolic execution to trace calls like `v.Method()` or access fields like `v.Field`.
2.  **`if-ok` Type Assertion:** In an `if v, ok := i.(T); ok` statement, the variable `v` should be recognized as type `T` within the `if` block, enabling the tracing of member access on `v`.

## 2. Investigation and Current State Analysis

A review of the `symgo` evaluator (`symgo/evaluator/evaluator.go`) and its tests reveals the following:

-   **Core Design Philosophy:** `symgo` is a symbolic tracer, not a standard interpreter. As documented in `docs/analysis-symgo-implementation.md`, it correctly explores all possible branches of control flow statements like `if` and `switch` to discover all potential code paths. This enhancement must adhere to that design.

-   **`evalTypeSwitchStmt`:** The current implementation correctly handles `switch v := i.(type)`. For each `case` clause, it creates a new, scoped environment (`caseEnv`). Within this environment, it defines a new variable `v` and assigns it a `SymbolicPlaceholder` object. This placeholder is correctly imbued with the `TypeInfo` and `FieldType` corresponding to the `case`'s type.

-   **`evalAssignStmt` & `evalIfStmt`:** The evaluator correctly handles the `v, ok := i.(T)` idiom. `evalAssignStmt` creates a `SymbolicPlaceholder` with the type information for `T` and assigns it to `v`. `evalIfStmt` correctly creates a new scope for the `if` block, ensuring the typed variable `v` is properly scoped.

-   **The Gap:** The existing test suite (`symgo/evaluator/evaluator_typeswitch_test.go`) verifies that the type-switched variable has the correct *type name* within each case. However, **it does not contain any tests that perform a method call or field access on the narrowed variable.** This indicates that while the mechanism for creating the typed variable exists, its utility in resolving member access is unverified and likely incomplete. The logic chain from `evalSelectorExpr` -> `evalSymbolicSelection` -> `accessor.findMethodOnType` seems plausible but has not been exercised by tests for this specific scenario.

## 3. Proposed Implementation Plan (TDD Approach)

This plan should be executed by a future engineer to implement the feature. It follows a test-driven development (TDD) methodology.

### Step 1: Create a New Test File

Create a new file `symgo/evaluator/evaluator_if_typeswitch_test.go` to isolate the new tests for this feature.

### Step 2: Add Failing Test for Method Call in Type Switch

Add a test case that defines a custom type with a method, uses a type switch to narrow an interface to that type, and calls the method. Use an intrinsic to verify the call is traced.

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

### Step 3: Add Failing Test for Field Access in `if-ok` Assertion

Add a test case that defines a struct with a field, uses an `if-ok` type assertion, and accesses the field. The field's value could be another function call to make tracing easier to verify.

**Example Test Snippet:**

```go
// In test file
const ifOkFieldAccessSource = `
package main

func get_name() string { return "Alice" }
func inspect(s string) {} // Intrinsic

type User struct {
	Name string
}

func main() {
	var i any = User{Name: get_name()}
	if v, ok := i.(User); ok {
		inspect(v.Name) // This field access should be traced
	}
}
`
// Test logic would register intrinsics for get_name() and inspect()
// and assert that inspect() was called with the result of get_name().
// This test should also fail initially.
```

### Step 4: Add Tests for Pointer Receivers and Other Edge Cases

Expand the test suite to cover:
-   Types with pointer receivers (e.g., `func (g *Greeter) Greet()`).
-   Accessing members on embedded structs within a type assertion.
-   Multiple `case` blocks in a type switch.

### Step 5: Enhance the Evaluator

Modify the `symgo` evaluator to make the tests pass. The likely areas for modification are:

-   **`evalSelectorExpr`**: When evaluating `v.Greet()`, `v` will resolve to a `SymbolicPlaceholder`. The logic must robustly use the `TypeInfo` attached to this placeholder.
-   **`evalSymbolicSelection`**: This helper function will likely be the primary focus. It receives the `SymbolicPlaceholder` and must correctly delegate to the `accessor` to find the method or field.
-   **`accessor`**: Ensure `findMethodOnType` and `findFieldOnType` work correctly when given the `TypeInfo` from a symbolic placeholder. This includes handling both value and pointer receiver methods correctly based on how the variable is defined.

The core of the implementation will be ensuring that the `TypeInfo` stored on the symbolic variable is fully utilized during method and field resolution, successfully connecting the type-narrowed variable to its members.

**Handling Scan Policies:**
The implementation must be robust with respect to `symgo`'s scan policy. The tests should cover the following scenarios:
1.  **Intra-Policy Assertion:** The type assertion occurs in a package that is within the primary analysis scope, and the target type (`T`) is also defined within that scope. This is the baseline case where full source is available.
2.  **Extra-Policy Assertion:** The type assertion occurs in a package within the primary analysis scope, but the target type `T` is defined in an external package that is *not* part of the source-scanned policy. `symgo` should still be able to symbolically trace method calls on the narrowed variable, likely by creating a `SymbolicPlaceholder` for the method's result based on a shallow scan of the external type's definition.

### Step 6: Verify and Finalize

Once all new tests pass and existing tests continue to pass, the feature is complete. The implementation has been successfully guided and verified by the test suite.
