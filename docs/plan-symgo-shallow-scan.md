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

This approach is favored for its **simplicity and minimal impact on existing code signatures**.

## 4. Discussion of Alternative Approaches

An alternative, more type-safe approach was considered: creating a new `UnresolvedTypeInfo` struct. This was rejected in favor of the simpler `Unresolved` flag approach to reduce implementation complexity and code churn, accepting a trade-off in type safety.

## 5. Implementation Task List (Issue-Based, TDD-Style)

Below is a proposed set of tasks, structured like individual GitHub issues, for implementing the shallow scan feature. The list is ordered to be implemented from top to bottom. It integrates unit testing and regression testing for high-level tools into each refactoring step to immediately identify the source of any potential issues.

---
### **Issue #1: Foundational `go-scan` Changes (Completed)**
*   **Goal:** Provide a mechanism within `go-scan` to represent a type that is intentionally not being resolved from source.
*   **Final Design & Rationale:**
    *   The core `go-scan` library remains **policy-agnostic**. It has no knowledge of `symgo`'s `ScanPolicyFunc`.
    *   The responsibility of deciding *whether* to resolve a type lies entirely with the caller (e.g., the `symgo` evaluator).
    *   To support this, two changes were made to the `scanner` package:
        1.  A new field, `Unresolved bool`, was added to `scanner.TypeInfo`. The zero-value (`false`) correctly indicates that a type created by the scanner from source is "resolved".
        2.  A new helper function, `scanner.NewUnresolvedTypeInfo(pkgPath, name string) *TypeInfo`, was added. This provides a safe and explicit way for callers (`symgo`) to create a placeholder `TypeInfo` for a type that is being skipped by a policy. `symgo` will use this function when its `ScanPolicyFunc` returns `false` for a given import path, instead of calling `fieldType.Resolve()`.
*   **Completed Tasks:**
    1.  Added `Unresolved bool` field to `scanner/models.go`.
    2.  Added `NewUnresolvedTypeInfo()` constructor to `scanner/models.go`.
    3.  Added a unit test for the new constructor in `scanner/models_test.go`.

---
### **Issue #2: Refactor `evalGenDecl` and Validate**
*   **Goal:** Update variable declaration logic to handle unresolved types and validate against regressions.
*   **Depends On:** Issue #1
*   **Tasks:**
    1.  Create the new test file `symgo/evaluator/evaluator_shallow_scan_test.go`.
    2.  **Write a unit test:** Add a test that (a) declares a variable with an out-of-policy type, then (b) performs a trackable operation on an in-policy object.
    3.  **Implement the change:** In `evalGenDecl`, check for `typeInfo.Unresolved` and handle it gracefully.
    4.  **Verify:**
        *   Ensure the unit test passes, proving the scan continued.
        *   Run the full test suites for `docgen` and `find-orphans` to ensure no regressions were introduced.

---
### **Issue #3: Refactor `evalCompositeLit` and Validate**
*   **Goal:** Update composite literal evaluation for unresolved types and validate against regressions.
*   **Depends On:** Issue #2
*   **Tasks:**
    1.  **Write a unit test:** Add a test that (a) creates a composite literal of an out-of-policy type, then (b) performs a subsequent, trackable operation.
    2.  **Implement the change:** In `evalCompositeLit`, check for `typeInfo.Unresolved` and return a `SymbolicPlaceholder`.
    3.  **Verify:**
        *   Ensure the unit test passes, confirming the scan continued successfully.
        *   Run the full test suites for `docgen` and `find-orphans`.

---
*(This pattern of **Test -> Implement -> Verify (Unit + Tooling)** will be repeated for each of the following refactoring tasks)*

### **Issue #4: Refactor `evalStarExpr` & `evalIndexExpr` and Validate**
*   **Goal:** Ensure pointer-dereferencing and indexing correctly handle unresolved types.
*   **Depends On:** Issue #3

### **Issue #5: Refactor Type Assertion Logic and Validate**
*   **Goal:** Update `evalTypeSwitchStmt` and `evalTypeAssertExpr` to handle unresolved types.
*   **Depends On:** Issue #4

### **Issue #6: Refactor `assignIdentifier` and Validate**
*   **Goal:** Update variable assignment logic for unresolved interfaces.
*   **Depends On:** Issue #5

### **Issue #7: Refactor `applyFunction` and Validate**
*   **Goal:** Ensure function return values are correctly typed, even if unresolved.
*   **Depends On:** Issue #6

### **Issue #8: Refactor `findMethodOnType` and Validate**
*   **Goal:** Gracefully handle method lookup on unresolved embedded types.
*   **Depends On:** Issue #7

### **Issue #9: Implement Symbolic Method Call Logic and Validate**
*   **Goal:** Enable the evaluator to trace method calls on unresolved types.
*   **Depends On:** Issue #8

### **Issue #10: Final `find-orphans` Integration Test**
*   **Goal:** Write a final, specific integration test for `find-orphans` to prove it benefits from the new symbolic capabilities.
*   **Depends On:** Issue #9
*   **Task:** Create an integration test for `find-orphans` where a function's *only* usage is through a method call on an unresolved type. Assert that `find-orphans` correctly marks the function as "used" and does not report it as an orphan. This confirms the end-to-end success of the feature.
