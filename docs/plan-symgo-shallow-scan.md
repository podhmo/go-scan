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

This section provides a concrete, step-by-step task list for implementing the shallow scan feature using the `Unresolved` flag approach.

---
### **Issue #1: Modify `go-scan` to Support Unresolved `TypeInfo`**

*   **Goal:** Update the core `go-scan` package to mark types from out-of-policy packages.
*   **Tasks:**
    1.  In `scanner/models.go`, add the field `Unresolved bool` to the `scanner.TypeInfo` struct.
    2.  Modify `scanner.FieldType.Resolve()`. If the `ScanPolicyFunc` returns `false`, it should now return a `*scanner.TypeInfo` instance with `Unresolved: true`, ensuring `Name` and `PkgPath` are populated.

---
### **Issue #2: Refactor the 8 Evaluator Resolution Points**

*   **Goal:** Adapt the `symgo` evaluator to check the `Unresolved` flag and handle these types gracefully.
*   **Task:** Perform a targeted refactoring of the **8 key resolution points** identified in Section 2. At each location, after calling `Resolve()`, add a check: `if typeInfo != nil && typeInfo.Unresolved`. If the flag is true, implement the logic to handle the unresolved type, which typically involves creating or propagating an `object.SymbolicPlaceholder`.

---
### **Issue #3: Implement Symbolic Method Call Handling**

*   **Goal:** Enable the evaluator to trace method calls on unresolved types.
*   **Tasks:**
    1.  Modify `findMethodOnType`. If the receiver's `TypeInfo` has `Unresolved: true`, the function should not attempt a real method lookup. Instead, it should immediately return a "fake" `object.Function` that stores the method name.
    2.  Update `applyFunction` to handle these fake functions. When called, it should return a single `SymbolicPlaceholder` to represent the unknown result(s) of the symbolic call.

---
### **Issue #4: Create a Comprehensive Test Suite**

*   **Goal:** Verify the correctness of the new `Unresolved` flag logic and ensure no regressions.
*   **Tasks:**
    1.  Create a new test file: `symgo/evaluator/evaluator_shallow_scan_test.go`.
    2.  **Split tests into two main categories:**
        *   **Deep Scan Tests:** Confirm that analysis of fully visible code is still correct (`Unresolved` flag is `false`).
        *   **Shallow Scan Tests:** Use a restrictive scan policy to force the `Unresolved` flag to be set. Assert that calling methods on types with `Unresolved: true` does not crash and returns a placeholder.

---
### **Issue #5: Validate and Harden Tooling (`docgen` & `find-orphans`)**

*   **Goal:** Ensure that high-level tools continue to function correctly.
*   **Tasks:**
    1.  **`docgen`:** Confirm its scan policy is correct and run its test suite to ensure golden files are unchanged.
    2.  **`find-orphans`:** Create an integration test where a function is used via a method on an unresolved type. Assert that `find-orphans` can successfully trace this symbolic call and does not report the function as an orphan.
