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

## Part 3: Recommended Solutions

The root cause of the `deriving-all` bug is an architectural limitation in the low-level scanner (`scanner/scanner.go`). Its single-pass AST traversal cannot robustly handle cases where identifiers are used before they are declared within a single file. Two possible solutions were identified.

### Solution A: Multi-Pass Logic in the Low-Level Scanner

This approach involves refactoring `scanner/scanner.go` to use a multi-pass system. My attempt at this was incomplete due to its complexity, but it remains a valid path.

-   **Task 1: Modify Data Structures (`scanner/models.go`)**: Modify `ConstantInfo` and `VariableInfo` to temporarily store `ast.Expr` for their types, as they cannot be fully resolved in the first pass.
-   **Task 2: Implement Multi-Pass Logic in `scanGoFiles` (`scanner/scanner.go`)**:
    -   **Pass 1 (Declarations):** Loop through all declarations to create placeholder `TypeInfo` and `FunctionInfo` objects. This populates a complete symbol table in `PackageInfo`.
    -   **Pass 2 (Details & Resolution):** Iterate through the collected symbols and call helper functions (`fillTypeInfoDetails`, `fillFuncInfoDetails`) to parse the full details (fields, signatures, etc.), using the complete symbol table for resolution.
-   **Task 3: Fix Regressions**: The architectural change will break existing unit tests, which will need to be updated.
-   **Task 4: Final Verification**: Run all tests, including the `deriving-all` e2e test.

### Solution B: Multi-Pass Logic in the High-Level Scanner (Superior Architecture)

This alternative was proposed by the user and is architecturally superior. It promotes better separation of concerns.

-   **Concept**: Keep the low-level `scanner.Scanner` as a simple, single-pass AST walker. Move the complexity and orchestration of the multi-pass analysis to the high-level `goscan.Scanner`, specifically within the `ScanPackageByImport` method.
-   **Mechanism**:
    1.  `goscan.ScanPackageByImport` would first call the low-level `scanner.scanGoFiles` to get a raw, "declarations-only" `PackageInfo`.
    2.  `goscan.ScanPackageByImport` would then orchestrate a second pass over the declarations in the `PackageInfo`. It would call new, specialized functions on the low-level scanner (e.g., `FillStructDetails(TypeInfo, PackageInfo)`) to populate the details for each symbol.
    3.  During this second pass, the low-level scanner would have access to the complete symbol table from the first pass, allowing for correct resolution.
-   **Benefits**: This design is cleaner. The low-level `scanner` is a "dumb" but fast AST walker. The high-level `goscan` package is the "smart" orchestrator that handles complex logic like caching and multi-pass analysis. This makes the system easier to reason about and maintain. `ScanPackage` could remain a simple wrapper around the single-pass scanner, while `ScanPackageByImport` becomes the robust, multi-pass entry point.

## Appendix: Analysis of `ScanPackage` vs. `ScanPackageByImport`

-   **Shared Logic**: Both `goscan.ScanPackage` and `goscan.ScanPackageByImport` are high-level methods that ultimately use the same core parsing engine in `scanner/scanner.go`.
-   **Key Differences**: The primary difference is their input and intended use. `ScanPackage` takes a directory path and is (in its original design) a simpler, incremental scanner. `ScanPackageByImport` takes a Go import path and is the more robust, cache-aware method intended for full package resolution.
-   **Impact of Refactoring (Solution B)**: If Solution B were implemented, the multi-pass logic would be contained within `ScanPackageByImport`. This would fix the bug while respecting the intended distinction between the two methods. `ScanPackage` could remain a simple, single-pass function, preserving its existing behavior for consumers that might rely on it (like `symgo`). This is the most surgical and least disruptive approach.
