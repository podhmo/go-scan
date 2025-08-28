# Plan: Shallow Scanning for `symgo` Evaluator

This document details the plan to enhance the `symgo` symbolic execution engine with a "shallow scanning" capability. This will make the evaluator more robust and performant when dealing with types from packages outside the defined scan policy.

## 1. Background: The Purpose of the Scan Policy

As detailed in `docs/trouble-symgo-introduce-policy.md`, a `ScanPolicyFunc` was introduced to prevent `symgo` from scanning the entire dependency graph of a project. By default, `symgo` only performs deep, source-level analysis on packages within the immediate workspace.

The purpose of this policy is **efficiency and scalability**. For tools like `find-orphans`, analyzing the source code of every third-party library is unnecessary and computationally expensive. The policy allows the tool to focus only on the user's code, treating external dependencies as opaque "black boxes."

However, this creates a challenge: the evaluator frequently encounters types defined in these external, unscanned packages. The initial implementation handled this by returning `nil` when type resolution failed, which prevented crashes but resulted in a loss of type information. This plan aims to improve upon that by preserving symbolic type information even for unscanned types.

## 2. Type Resolution Points in the Evaluator

A thorough analysis of `symgo/evaluator/evaluator.go` has identified the following key locations where the evaluator attempts to resolve a `scanner.FieldType` into a full `scanner.TypeInfo` by calling `fieldType.Resolve(ctx)`. These are the places that need to be enhanced for shallow scanning.

1.  **`evalGenDecl`**: Resolves the type of an explicit variable declaration.
2.  **`evalCompositeLit`**: Resolves the type of a composite literal.
3.  **`evalStarExpr`**: Resolves the element type of a pointer being dereferenced.
4.  **`evalIndexExpr`**: Resolves the element type of a slice or array.
5.  **`evalTypeSwitchStmt` & `evalTypeAssertExpr`**: Resolves the type in a `case` clause or a type assertion.
6.  **`assignIdentifier`**: Resolves the static type of a variable during assignment.
7.  **`applyFunction`**: Resolves the return types of external functions or interface methods.
8.  **`findMethodOnType`**: Resolves the type of an embedded field during method lookup.

## 3. Chosen Implementation Strategy: The `Unresolved` Flag

To handle types from unscanned packages, the chosen approach is to add a boolean flag to the existing `scanner.TypeInfo` struct:

```go
// In scanner/models.go
type TypeInfo struct {
    // ... existing fields
    Unresolved bool
}
```

When `scanner.FieldType.Resolve()` is called for a type in a package that is disallowed by the `ScanPolicyFunc`, instead of returning `nil`, it will return a `*TypeInfo` object with `Unresolved: true`. This object will have minimal information populated (e.g., `Name` and `PkgPath`), but most other fields (like `StructInfo` or `Methods`) will be empty.

At each of the 8 resolution points in the evaluator, the code will be modified to check this flag: `if typeInfo != nil && typeInfo.Unresolved`. If true, the evaluator will treat the type symbolically (e.g., by creating a `SymbolicPlaceholder`) rather than attempting to access its detailed information.

This approach is favored for its **simplicity and minimal impact on existing code signatures**, prioritizing a less complex implementation.

## 4. Discussion of Alternative Approaches

An alternative, more type-safe approach was considered: creating a new `UnresolvedTypeInfo` struct and a `SymbolicType` interface that both `TypeInfo` and `UnresolvedTypeInfo` would implement.

*   **Pro:** This would have made the distinction between resolved and unresolved types explicit in the type system, preventing accidental access to empty fields.
*   **Con:** This would have required changing many function signatures across the evaluator to accept the `SymbolicType` interface, leading to more complex code and greater implementation effort.

Given the trade-offs, the simpler `Unresolved` flag approach was selected as it achieves the primary goal with significantly less code churn.


## 5. Implementation Task List (Issue-Based, TDD-Style)

Below is a proposed set of tasks, structured like individual GitHub issues, for implementing the shallow scan feature. The list is ordered to be implemented from top to bottom, and it integrates testing into each refactoring step.

---
### **Issue #1: Foundational `go-scan` Changes**
*   **Goal:** Update the core `scanner.TypeInfo` struct and `Resolve()` method to support marking types as unresolved.
*   **Tasks:**
    1.  In `scanner/models.go`, add the field `Unresolved bool` to the `scanner.TypeInfo` struct.
    2.  Modify `scanner.FieldType.Resolve()`. When the `ScanPolicyFunc` returns `false`, it should return a `*scanner.TypeInfo` instance where `Unresolved` is set to `true`, and essential fields like `Name` and `PkgPath` are populated.

---
### **Issue #2: Refactor `evalGenDecl` and Add Test**
*   **Goal:** Update variable declaration logic to handle unresolved types and verify scan continuation.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  Create `symgo/evaluator/evaluator_shallow_scan_test.go`.
    2.  **Write a failing test:** Add a test that (a) declares a variable with an out-of-policy type, then (b) performs a trackable operation on an in-policy object.
    3.  **Implement the change:** In `evalGenDecl`, check for `typeInfo.Unresolved` and handle it gracefully.
    4.  **Verify:** The test must pass, proving that handling the unresolved type did not halt the scan and the subsequent operation was analyzed.

---
### **Issue #3: Refactor `evalCompositeLit` and Add Test**
*   **Goal:** Update composite literal evaluation for unresolved types and verify scan continuation.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  **Write a failing test:** Add a test that (a) creates a composite literal of an out-of-policy type, then (b) performs a subsequent, trackable operation.
    2.  **Implement the change:** In `evalCompositeLit`, check for `typeInfo.Unresolved` and return a `SymbolicPlaceholder`.
    3.  **Verify:** The test must pass, confirming the scan continued successfully.

---
### **Issue #4: Refactor `evalStarExpr` & `evalIndexExpr` and Add Tests**
*   **Goal:** Ensure pointer-dereferencing and indexing correctly propagate unresolved types and do not halt scanning.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  **Write failing tests:** For both pointer dereferencing and slice indexing, create a test that operates on an out-of-policy type and then performs a subsequent trackable action.
    2.  **Implement the changes:** Update both `evalStarExpr` and `evalIndexExpr` to check for the `Unresolved` flag and propagate the unresolved type to the resulting placeholder.
    3.  **Verify:** Ensure the tests pass, proving the scan continued in both cases.

---
### **Issue #5: Refactor Type Assertion Logic and Add Test**
*   **Goal:** Update `evalTypeSwitchStmt` and `evalTypeAssertExpr` to handle unresolved types and verify scan continuation.
*   **Depends On:** Issue #1
*   **Task:**
    1.  **Write failing tests:** Add tests for type switches and assertions using an out-of-policy type, followed by a trackable operation.
    2.  **Implement the change:** In both functions, check for `typeInfo.Unresolved` and ensure the new variable `v` is created as a symbolic placeholder.
    3.  **Verify:** The tests must pass, proving the scan continued.

---
### **Issue #6: Refactor `assignIdentifier` and Add Test**
*   **Goal:** Update variable assignment logic for unresolved interfaces and verify scan continuation.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  **Write a failing test:** Add a test that assigns a value to a variable whose type is an out-of-policy interface, followed by a trackable operation.
    2.  **Implement the change:** In `assignIdentifier`, ensure the interface check works correctly for types where `Unresolved: true`.
    3.  **Verify:** The test must pass, proving the scan continued.

---
### **Issue #7: Refactor `applyFunction` and Add Test**
*   **Goal:** Ensure function return values are correctly typed, even if unresolved, and do not halt scanning.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  **Write a failing test:** Add a test that calls an external function returning an out-of-policy type, then performs a subsequent trackable operation.
    2.  **Implement the change:** In `applyFunction`, when creating placeholders for return values, check `typeInfo.Unresolved` and tag the placeholder accordingly.
    3.  **Verify:** The test must pass, proving the scan continued.

---
### **Issue #8: Refactor `findMethodOnType` and Add Test**
*   **Goal:** Gracefully handle method lookup on unresolved embedded types and verify scan continuation.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  **Write a failing test:** Add a test with a struct that embeds an out-of-policy type, call a method on it, and follow with a trackable operation.
    2.  **Implement the change:** In `findMethodOnType`, if an embedded field's type has `Unresolved: true`, gracefully stop that recursive search branch.
    3.  **Verify:** The test must pass, proving the scan continued.

---
### **Issue #9: Implement Symbolic Method Call Logic and Add Test**
*   **Goal:** Enable the evaluator to trace method calls on unresolved types and verify scan continuation.
*   **Depends On:** Issues #1-8
*   **Tasks:**
    1.  **Write a failing test:** Call a method on an object with an unresolved type, then perform a trackable operation. Assert the method call returns a placeholder and the subsequent operation is still analyzed.
    2.  **Implement the change:** In `findMethodOnType`, if the receiver's `TypeInfo` has `Unresolved: true`, return a "fake" `object.Function`. Update `applyFunction` to handle this fake function.
    3.  **Verify:** The test must pass.

---
### **Issue #10: Final Integration Testing and Tooling Validation**
*   **Goal:** Ensure the complete feature is robust and doesn't break high-level tools.
*   **Depends On:** Issues #1-9
*   **Tasks:**
    1.  **Deep Scan Regression:** Write a final test using a permissive policy to ensure no existing logic was broken.
    2.  **Tooling Validation (`docgen`):** Confirm `docgen`'s scan policy is correct and run its entire test suite to ensure no golden files have changed.
    3.  **Tooling Validation (`find-orphans`):** Create an integration test where a function's only usage is via a method call on an unresolved type. Assert that `find-orphans` correctly marks the function as "used".
