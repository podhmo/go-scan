# Investigation: `deriving-all` E2E Test Failure and Scanner Inconsistency

This document provides a final, detailed analysis of a bug found in the `deriving-all` example. It explains the evolution of my understanding, the contradictions discovered, and the final, most likely correct, hypothesis.

## Part 1: The Initial Problem

-   **Symptom**: The `make -C examples/deriving-all e2e` command failed with the error `json: cannot unmarshal object into Go struct field Event.data of type models.EventData`.
-   **Analysis**: This error indicated that the code generator failed to produce a custom `UnmarshalJSON` method that could correctly handle the `EventData` interface. This typically happens when the generator cannot identify all the concrete types that implement the interface.

## Part 2: Detailed Analysis of Hypotheses & Contradictions

My investigation proceeded through two main hypotheses. The first was proven incorrect by new information from the user, leading to the second, more accurate one.

### Hypothesis 1: Multi-File Cache Poisoning (Incorrect)

This was my initial mental model for the bug.

-   **Basis for this hypothesis**: My initial reading of `goscan.go` showed that the `goscan.Scanner` has two caching mechanisms (`visitedFiles` and `packageCache`). I saw that `ScanPackage` respected the `visitedFiles` cache and could return a partial `PackageInfo`, which it would then write to the `packageCache`. The `deriving-all` tool calls multiple generators, so I theorized that one part of the process could partially scan a package, "poisoning" the cache with incomplete information that a later part of the process would then incorrectly use.
-   **Why it was wrong**: This entire model was predicated on the interaction between multiple files within a package. The user clarified a critical piece of information I had missed: **the input for the failing e2e test was a single file** (`models.go`). In a single-file scenario, the logic of skipping already-visited files is irrelevant. This invalidated my entire "cache poisoning" theory and all the fixes based on it.

### Hypothesis 2: Single-File Declaration-Order Dependency (Correct)

This hypothesis was formed based on the user's crucial feedback that the input was a single file.

-   **Basis for this hypothesis**: If the input is a single file, the problem must be internal to how that one file is processed. I re-analyzed `scanner/scanner.go` (the low-level scanner) and focused on the `scanGoFiles` function. I observed that it processes the file's declarations (`Decls`) sequentially, in the order they appear in the source code.
-   **Why the current code cannot handle this**: The `deriving-all` use case involves an interface field (`EventData`). To correctly generate code for this, the scanner must know which structs implement this interface. In `models.go`, the `Event` struct (which *uses* the interface) is defined before the methods (`func (UserCreated) isEventData()`) that actually satisfy the interface.
    Because the current implementation is **single-pass**, when it processes the `Event` struct declaration, it has not yet processed the method declarations that appear later in the file. Therefore, at that moment, the `PackageInfo` object being built is incomplete. Any attempt to resolve the `EventData` interface and find its implementers at this stage is doomed to fail.
-   **Why a refactoring is necessary**: A single-pass scanner is fundamentally unable to solve this problem reliably. The only architecturally sound solution is a **multi-pass approach** within the scanner. My final attempt was to implement this.

## Part 3: Conclusion & Recommended Path Forward

The root cause of the `deriving-all` bug is an architectural limitation in the low-level scanner (`scanner/scanner.go`). Its single-pass AST traversal cannot robustly handle cases where identifiers are used before they are declared within a single file.

The correct solution is to refactor `scanner/scanner.go` to use a multi-pass system. My attempt at this refactoring was incomplete due to the complexity and resulting test regressions, but it remains the correct path forward.

### Recommended Task List for Refactoring
Here is a concrete task list to implement the necessary multi-pass scanner architecture:

1.  **Modify Data Structures for Multi-Pass (`scanner/models.go`)**:
    *   Modify `ConstantInfo` and `VariableInfo` structs.
    *   Change the `Type` field from `*FieldType` to `TypeExpr ast.Expr` to temporarily store the type's AST node during the first pass, as it cannot be fully resolved yet.
    *   Alternatively, add a new `TypeExpr ast.Expr` field and populate the existing `Type *FieldType` field during the second pass.

2.  **Implement Multi-Pass Logic in `scanGoFiles` (`scanner/scanner.go`)**:
    *   **Pass 1: Declarations & Placeholders**: Loop through all declarations in all files of the package.
        *   For `type` and `func` declarations, create basic, placeholder `TypeInfo` and `FunctionInfo` objects (containing just name, path, etc.). Do not resolve details like struct fields or function signatures yet.
        *   Populate `PackageInfo.Types` and `PackageInfo.Functions` with these placeholders. This creates a complete symbol table for the package.
        *   For `const` and `var` declarations, collect their raw AST expressions into the `ConstantInfo`/`VariableInfo` structs without trying to evaluate them.
    *   **Pass 2: Detail Population & Resolution**:
        *   Iterate through the `TypeInfo` list created in Pass 1. For each `TypeInfo`, now parse its full details (struct fields, interface methods, underlying alias types) by calling `s.fillTypeInfoDetails`.
        *   Iterate through the `FunctionInfo` list. For each, parse its full signature (parameters, results, receiver) by calling `s.fillFuncInfoDetails`. Because Pass 1 is complete, all type lookups within the same package will now succeed.
    *   **Pass 3: Constant Evaluation**:
        *   Iterate through the `ConstantInfo` list and evaluate the constant values. This pass runs after all type information is available, though it may require further refinement to handle typed constants correctly.

3.  **Fix Unit Test Regressions**:
    *   The architectural change in the scanner will likely break existing unit tests in `goscan_test.go`, `minigo_enum_test.go`, `symgo` tests, etc.
    *   A dedicated effort is required to systematically fix these tests by updating them to work with the new, more correct scanner behavior.

4.  **Final Verification**:
    *   After all unit tests are passing, run `make -C examples/deriving-all e2e` to confirm that the multi-pass refactoring has fixed the original bug.
    *   Run the full `make test` suite one last time.

This document serves as a record of this investigation and a guide for the necessary future work.
