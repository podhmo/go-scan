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

## 5. Test Strategy

The test strategy remains the same, but the focus of the new tests will be on the `Unresolved` flag.

-   **Test Splitting**:
    -   **Deep Scan Tests (Existing & New):** Verify that `Unresolved` is `false` for all types within the scan policy and that the evaluator's behavior is unchanged.
    -   **Shallow Scan Tests (New):** In a new file (`symgo/evaluator/evaluator_shallow_scan_test.go`), use a restrictive `ScanPolicyFunc` to force `Resolve()` to return types with `Unresolved: true`. These tests will assert that the evaluator correctly handles these types at all 8 resolution points without crashing.

## 6. Implementation Task List (Issue-Based)

Below is a proposed set of tasks, structured like individual GitHub issues, for implementing the shallow scan feature. This task list uses the simpler **`Unresolved` flag** strategy and breaks down the work into granular, actionable steps.

---
### **Issue #1: Foundational `go-scan` Changes**
*   **Goal:** Update the core `scanner.TypeInfo` struct and `Resolve()` method to support marking types as unresolved.
*   **Tasks:**
    1.  In `scanner/models.go`, add the field `Unresolved bool` to the `scanner.TypeInfo` struct.
    2.  Modify `scanner.FieldType.Resolve()`. When the `ScanPolicyFunc` returns `false` for a package, it should return a `*scanner.TypeInfo` instance where `Unresolved` is set to `true`, and essential fields like `Name` and `PkgPath` are populated.

---
### **Issue #2: Refactor `evalGenDecl`**
*   **Goal:** Update variable declaration logic to handle unresolved types.
*   **Depends On:** Issue #1
*   **Task:** In `evalGenDecl`, after resolving a variable's type, check if `typeInfo.Unresolved` is `true`. If so, ensure the `object.Variable` is created correctly with this unresolved type information without causing errors.

---
### **Issue #3: Refactor `evalCompositeLit`**
*   **Goal:** Update composite literal evaluation to handle unresolved types.
*   **Depends On:** Issue #1
*   **Task:** In `evalCompositeLit`, after resolving the literal's type, check if `typeInfo.Unresolved` is `true`. If so, the function must return an `object.SymbolicPlaceholder` (tagged with the unresolved `TypeInfo`) instead of attempting to create an `object.Instance`.

---
### **Issue #4: Refactor `evalStarExpr` and `evalIndexExpr`**
*   **Goal:** Ensure pointer-dereferencing and indexing operations correctly propagate unresolved types.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  In `evalStarExpr`, when dereferencing a pointer, if the pointer's element type is unresolved (`elemType.Unresolved == true`), ensure the resulting placeholder is correctly tagged with this unresolved type.
    2.  In `evalIndexExpr`, if a slice's element type is unresolved, ensure the placeholder for the indexed element is correctly tagged with this unresolved type.

---
### **Issue #5: Refactor `evalTypeSwitchStmt` and `evalTypeAssertExpr`**
*   **Goal:** Update type assertion logic to handle unresolved types.
*   **Depends On:** Issue #1
*   **Task:** In both `evalTypeSwitchStmt` and `evalTypeAssertExpr`, when resolving the type `T` in an expression like `v := i.(T)`, check if the resulting `typeInfo.Unresolved` is `true`. If so, ensure the variable `v` is correctly created as a symbolic placeholder tagged with the unresolved `TypeInfo`.

---
### **Issue #6: Refactor `assignIdentifier`**
*   **Goal:** Update variable assignment logic to correctly handle unresolved interface types.
*   **Depends On:** Issue #1
*   **Task:** In `assignIdentifier`, the logic that checks if a variable is an interface must be updated. It should correctly identify a type as an interface even if its `typeInfo.Unresolved` flag is `true` (e.g., by checking a `Kind` field that is populated even for unresolved types).

---
### **Issue #7: Refactor `applyFunction`**
*   **Goal:** Ensure that return values from external or interface functions are correctly typed, even if unresolved.
*   **Depends On:** Issue #1
*   **Task:** In `applyFunction`, when creating placeholders for the return values of a function, check if the resolved return `typeInfo.Unresolved` is `true`. If so, ensure the resulting `SymbolicPlaceholder` is correctly tagged with that unresolved `TypeInfo`.

---
### **Issue #8: Refactor `findMethodOnType`**
*   **Goal:** Enable method lookup on unresolved embedded types.
*   **Depends On:** Issue #1
*   **Task:** In `findMethodOnType`, when recursively searching through embedded fields, if an embedded field resolves to a `typeInfo` with `Unresolved: true`, the logic should gracefully stop the recursive search for that branch instead of causing an error.

---
### **Issue #9: Implement Symbolic Method Call Logic**
*   **Goal:** Enable the evaluator to trace method calls on unresolved types.
*   **Depends On:** Issues #1-8
*   **Tasks:**
    1.  Modify `findMethodOnType` (or a related function). If the receiver's `TypeInfo` has `Unresolved: true`, it should immediately return a "fake" `object.Function`.
    2.  This fake function will store the method name. Update `applyFunction` to handle it by returning a single `SymbolicPlaceholder` to represent the unknown result(s) of the symbolic call.

---
### **Issue #10: Create Test Suite for Shallow Scanning**
*   **Goal:** Verify the correctness of the `Unresolved` flag logic and ensure it does not introduce regressions.
*   **Depends On:** Issues #1-9
*   **Tasks:**
    1.  Create `symgo/evaluator/evaluator_shallow_scan_test.go`.
    2.  **Split tests:**
        *   **Deep Scan Tests:** Confirm that the `Unresolved` flag is `false` for all normally resolved types and that no existing behavior is broken.
        *   **Shallow Scan Tests:** Use a restrictive scan policy. For each of the 8 refactored locations, write a targeted test to assert that an `Unresolved: true` type is handled correctly and results in a symbolic placeholder.

---
### **Issue #11: Validate and Harden Tooling (`docgen` & `find-orphans`)**
*   **Goal:** Ensure high-level tools are not broken and can leverage the new capabilities.
*   **Depends On:** Issues #1-10
*   **Tasks:**
    1.  **`docgen`:** Confirm its scan policy is correct and run its test suite to ensure golden files are unchanged.
    2.  **`find-orphans`:** Add an integration test where a function is only used via a method on an unresolved type, and assert it is not reported as an orphan.
