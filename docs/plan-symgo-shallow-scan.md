# Plan: Shallow Scanning for `symgo` Evaluator

This document details the plan to enhance the `symgo` symbolic execution engine with a "shallow scanning" capability. This will make the evaluator more robust and performant when dealing with types from packages outside the defined scan policy.

## 1. Background: The Purpose of the Scan Policy

As detailed in `docs/trouble-symgo-introduce-policy.md`, a `ScanPolicyFunc` was introduced to prevent `symgo` from scanning the entire dependency graph of a project. By default, `symgo` only performs deep, source-level analysis on packages within the immediate workspace.

The purpose of this policy is **efficiency and scalability**. For tools like `find-orphans`, analyzing the source code of every third-party library is unnecessary and computationally expensive. The policy allows the tool to focus only on the user's code, treating external dependencies as opaque "black boxes."

However, this creates a challenge: the evaluator frequently encounters types defined in these external, unscanned packages. The initial implementation handled this by returning `nil` when type resolution failed, which prevented crashes but resulted in a loss of type information. This plan aims to improve upon that by preserving symbolic type information even for unscanned types.

## 2. Type Resolution Points in the Evaluator

A thorough analysis of `symgo/evaluator/evaluator.go` has identified the following key locations where the evaluator attempts to resolve a `scanner.FieldType` into a full `scanner.TypeInfo` by calling `fieldType.Resolve(ctx)`. These are the places that need to be enhanced for shallow scanning.

1.  **`evalGenDecl`**: Resolves the type of an explicit variable declaration (e.g., `var v mypkg.MyType`).
    *   **Purpose**: To attach static type information to the new variable in the environment.

2.  **`evalCompositeLit`**: Resolves the type of a composite literal (e.g., `mypkg.MyStruct{}`).
    *   **Purpose**: To get the `TypeInfo` needed to create a concrete `object.Instance`.

3.  **`evalStarExpr` (Pointer Dereference)**: Resolves the element type of a pointer being dereferenced (e.g., the `T` in `*T`).
    *   **Purpose**: To ensure the resulting symbolic placeholder has the correct underlying type.

4.  **`evalIndexExpr` (Slice/Array Indexing)**: Resolves the element type of a slice or array.
    *   **Purpose**: To correctly type the symbolic placeholder representing the result of the index operation.

5.  **`evalTypeSwitchStmt` & `evalTypeAssertExpr`**: Resolves the type in a `case` clause or a type assertion.
    *   **Purpose**: To correctly type the new variable created within the scope of the `case` or assertion.

6.  **`assignIdentifier`**: Resolves the static type of a variable during assignment.
    *   **Purpose**: To check if the variable was declared as an interface, which affects how concrete types are tracked.

7.  **`applyFunction`**: Resolves the return types of external functions or interface methods.
    *   **Purpose**: To create correctly-typed symbolic placeholders for the return values of functions whose bodies cannot be analyzed.

8.  **`findMethodOnType`**: Resolves the type of an embedded field during method lookup.
    *   **Purpose**: To recursively search for methods in embedded structs.

## 3. Shallow Scanning with Placeholder `TypeInfo`

The core of this plan is to change `scanner.FieldType.Resolve()` so that when the scan policy denies a scan, it returns a **placeholder `TypeInfo`** object instead of `nil`.

This placeholder will not be fully populated but will contain crucial symbolic information:
- The `Name` and `PkgPath` of the type.
- The `Kind` (e.g., `StructKind`, `InterfaceKind`).
- For structs and interfaces, it could contain a list of method *names* and *signatures* extracted without a full scan, if possible.

This allows the evaluator to continue propagating type information symbolically. For example, when a method is called on a variable with a placeholder type, the evaluator can:
1. Check if a method with the given name exists on the placeholder `TypeInfo`.
2. If it exists, return a fake `object.Function` or a `SymbolicPlaceholder` representing the method call. This placeholder can then be configured to return the correct number of result values (also as placeholders), based on the method signature stored in the placeholder `TypeInfo`.

This approach correctly simulates method calls on external types, providing a `boundmethod`-like fake object, without needing to analyze their implementation.

## 4. Test Strategy

The existing test suite validates the "deep scan" path where all types are accessible. To validate the "shallow scan" path, new tests will be added.

-   **Test Splitting**:
    -   **Deep Scan (Existing Tests)**: All tests where the scan policy allows access to all packages will be preserved. They ensure the evaluator works correctly when full type information is available.
    -   **Shallow Scan (New Tests)**: A new test file (`symgo/evaluator/evaluator_shallow_scan_test.go`) will be created. These tests will use a `ScanPolicyFunc` that explicitly *denies` access to specific dependency packages.

-   **New Test Assertions**: The new tests will assert that:
    -   Operations involving out-of-policy types do not cause the evaluator to crash.
    -   Calling a method on an out-of-policy type results in a `SymbolicPlaceholder`.
    -   The placeholders for multi-value returns from out-of-policy functions have the correct number of values.
    -   Method calls on embedded out-of-policy structs are correctly identified.

## 5. Feasibility and Impact on `docgen` and `find-orphans`

### `docgen`
-   **Requirement**: `docgen` needs to perform a deep analysis of the `net/http` package to generate OpenAPI specs.
-   **Impact Assessment**: The shallow scanning mechanism will not negatively impact `docgen` provided its configuration is correct. The `docgen` tool must initialize the `symgo` evaluator with a scan policy that explicitly permits scanning of `net/http` and any other required packages. The shallow scan behavior will then only apply to packages *not* covered by `docgen`'s policy.
-   **Feasibility**: High. The key is ensuring the tool's configuration is correct. The golden file tests for `docgen` will be run to verify that its output remains unchanged.

### `find-orphans`
-   **Requirement**: `find-orphans` needs to identify if functions and methods are used, even if the call site is on an interface or external type. It relies on the symbolic execution not losing track of method calls.
-   **Impact Assessment**: Shallow scanning is critical for the performance of `find-orphans`. The risk is generating false positives (marking a used method as an orphan). The placeholder `TypeInfo` approach mitigates this risk. When the `find-orphans` analysis hook sees a method call on a symbolic object, it can inspect its placeholder `TypeInfo`. If the method name exists on the placeholder, it can be marked as used.
-   **Feasibility**: High. This approach makes the analysis more robust and avoids the need for full dependency scanning.

### Multi-Value Return Handling
-   **Requirement**: The evaluator must correctly handle functions returning multiple values, even if the function itself is in an unscanned package.
-   **Impact Assessment**: The logic in `applyFunction` already iterates through the results of a function signature. The shallow scan enhancement will ensure that when `Resolve()` is called on each return type, it provides a placeholder `TypeInfo`. `applyFunction` will then create a corresponding number of `SymbolicPlaceholder` objects, each tagged with the correct placeholder type.
-   **Feasibility**: High. This extends the existing multi-value return logic to work seamlessly with placeholder types.

## 6. Alternative Approach: A Dedicated `UnresolvedTypeInfo` Type

An alternative to using a "placeholder" `goscan.TypeInfo` is to introduce a new, dedicated type, for example, `scanner.UnresolvedTypeInfo`.

`goscan.TypeInfo` is an existing, complex struct with many fields (like `StructInfo`, `InterfaceInfo`, `Methods`, etc.) that are relevant only after a full source code scan. Using a placeholder `TypeInfo` means these fields would be `nil`, requiring checks throughout the evaluator.

A dedicated `UnresolvedTypeInfo` could be much simpler:

```go
type UnresolvedTypeInfo struct {
    Name    string
    PkgPath string
    // Potentially a list of known method signatures
    Methods []*MethodInfo
}
```

### Pros and Cons

*   **Pro: Type Safety and Clarity.** This approach is more explicit. The type system itself would distinguish between a fully resolved type and a symbolic one. This avoids nullable fields and reduces the chance of errors where code accidentally tries to access, for example, the fields of a placeholder struct.
*   **Con: Broader Code Impact.** The evaluator's functions would need to be updated to handle two different types. This might lead to code duplication or the need for a new common interface that both `TypeInfo` and `UnresolvedTypeInfo` would implement, so they can be passed to the same functions. For example:
    ```go
    type SymbolicType interface {
        TypeName() string
        PackagePath() string
        GetMethod(name string) *MethodInfo
    }
    ```

### Handling Chained Method Calls

This approach would handle chained method calls (`a.b().c()`) effectively.
1.  Assume `a.b()` returns an object of an external, unscanned type.
2.  The symbolic placeholder object for this result would hold an `UnresolvedTypeInfo`.
3.  When `.c()` is called on this placeholder, the evaluator would look for a method named "c" in the `UnresolvedTypeInfo`'s `Methods` list.
4.  If found, it would return a new symbolic placeholder representing the result of the call to `c`, using the return types specified in the method signature.

This is conceptually similar to the placeholder `TypeInfo` approach but provides greater type safety at the cost of broader changes to function signatures within the evaluator. This trade-off is worth considering during implementation.

## 7. Implementation Task List (Issue-Based)

Below is a proposed set of tasks, structured like GitHub issues, for implementing the shallow scan feature. This approach is based on the `UnresolvedTypeInfo` alternative for its type safety.

---

### **Issue #1: Foundational `go-scan` Changes for Shallow Types**

*   **Goal:** Update the core `go-scan` package to support the concept of an unresolved type.
*   **Tasks:**
    1.  In the `scanner` package, define a new `UnresolvedTypeInfo` struct. It should contain basic information like `Name`, `PkgPath`, and potentially a list of method signatures.
    2.  Define a `SymbolicType` interface implemented by both `*scanner.TypeInfo` and the new `*scanner.UnresolvedTypeInfo`. The interface should expose common methods like `PackagePath()`, `TypeName()`, and `Kind()`.
    3.  Change the return signature of `scanner.FieldType.Resolve()` from `(*TypeInfo, error)` to `(SymbolicType, error)`.
    4.  Implement the logic in `Resolve()`: if the scan policy for a package returns `false`, it should create and return an `*UnresolvedTypeInfo`; otherwise, it should return a `*TypeInfo` as it does currently.

---

### **Issue #2: Refactor Evaluator for `SymbolicType` Compatibility**

*   **Goal:** Adapt the `symgo` evaluator to work with the new `SymbolicType` interface, handling both resolved and unresolved types gracefully.
*   **Tasks:**
    1.  Perform a targeted refactoring of the **8 key resolution points** identified in Section 2 (`evalGenDecl`, `evalCompositeLit`, etc.).
    2.  In each location, update variables and function parameters from `*scanner.TypeInfo` to the `SymbolicType` interface.
    3.  Use type switches (`switch t := symbolicType.(type)`) to differentiate logic:
        *   `case *scanner.TypeInfo`: Maintain the existing logic for fully resolved types.
        *   `case *scanner.UnresolvedTypeInfo`: Implement new logic to handle unresolved types, typically by creating or propagating `object.SymbolicPlaceholder` instances.

---

### **Issue #3: Implement Symbolic Method and Function Call Handling**

*   **Goal:** Enable the evaluator to correctly trace method and function calls on unresolved types without analyzing their source code.
*   **Tasks:**
    1.  Modify `findMethodOnType` to operate on the `SymbolicType` interface. If the receiver is an `UnresolvedTypeInfo`, the method lookup should consult its list of method signatures.
    2.  If a method is found on an `UnresolvedTypeInfo`, `findMethodOnType` should return a "fake" `object.Function`. This object will not have a body but will store the method's signature (parameters and return types).
    3.  Update `applyFunction` to handle these fake functions. When called, a fake function will inspect the stored signature and return the correct number of `SymbolicPlaceholder` objects, each representing a return value.

---

### **Issue #4: Create a Comprehensive Test Suite for Shallow Scanning**

*   **Goal:** Verify the correctness of the shallow scanning mechanism and ensure it does not introduce regressions.
*   **Tasks:**
    1.  Create a new test file: `symgo/evaluator/evaluator_shallow_scan_test.go`.
    2.  **Split tests into two main categories:**
        *   **Deep Scan Tests:** These tests will use a permissive scan policy (or the default). They will reuse existing test patterns to confirm that analysis of fully visible code remains correct after the refactoring.
        *   **Shallow Scan Tests:** These tests will configure a *restrictive* scan policy to exclude a mock dependency. They will assert that:
            *   Calling methods on types from the excluded dependency does not crash.
            *   The return values from these calls are correctly modeled as the right number of symbolic placeholders.
            *   Chained method calls on unresolved types are handled correctly.

---

### **Issue #5: Validate and Harden Tooling (`docgen` & `find-orphans`)**

*   **Goal:** Ensure that high-level tools continue to function correctly and can benefit from the new shallow scanning capabilities.
*   **Tasks:**
    1.  **`docgen`:** Review its scan policy configuration to ensure it explicitly includes `net/http`. Run the full `docgen` test suite to verify that its golden file outputs are unchanged by the underlying refactoring.
    2.  **`find-orphans`:** Create a specific integration test for `find-orphans`. In this test, a function's only usage is through a method call on a type from an out-of-policy package. Assert that `find-orphans` correctly marks the target function as "used" and does not report it as an orphan, proving that symbolic calls are tracked correctly.
