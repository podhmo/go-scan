# Plan: Refactor `goscan.Scanner` Methods

This document outlines a plan to refactor the main `goscan.Scanner` to streamline its API, improve internal consistency, and resolve a symbol collision issue that occurs when analyzing applications with multiple `main` packages.

## 1. Goals

- **Unify Scanning Logic**: Make `ScanPackage` a lightweight wrapper around `ScanPackageByImport`. This centralizes the core scanning logic in one method, reducing code duplication and making the system easier to maintain.
- **Improve API Clarity**: Remove the `ScanPackageByPos` method. Users who need this functionality can replicate it easily by finding the file path from the position and then using `ScanPackage`.
- **Resolve `main` Package Symbol Collisions**: Fix a critical bug where tools like `symgo` cannot distinguish between functions from different `main` packages in the same workspace. This will be solved by normalizing `main` package names internally to make them unique.
- **Update Documentation**: Ensure that the docstrings for `ScanPackage` and `ScanPackageByImport` accurately reflect their new implementations.

## 2. Background

The `goscan.Scanner` currently has several methods for scanning packages, leading to some redundancy. The most significant issue this plan addresses, however, is a fundamental problem for analysis tools like `symgo` when they encounter projects with multiple `main` packages (e.g., a project with multiple binaries in a `cmd/` directory).

The problem, identified and reproduced in `symgo/integration_test/cross_main_package_test.go`, is a **symbol collision**. `symgo` builds an internal model of the program, likely using a map to store information about discovered functions, keyed by a combination of package name and function name (e.g., `main.run`). When `go-scan` analyzes two different packages that are both `package main`, it reports the package name as "main" for both. This causes a key collision in `symgo`'s internal map: the information for `main.run` from the first package is overwritten by the information for `main.run` from the second. This leads to incorrect call graph analysis and test failures.

This refactoring will solve this problem by ensuring that `go-scan` provides a unique, unambiguous name for every package.

## 3. Detailed Plan

### Step 1: Refactor `ScanPackage` to use `ScanPackageByImport`

The `ScanPackage` method will be refactored to perform the following steps:
1.  Take the input directory path (`pkgPath`).
2.  Use the `locator.PathToImport(pkgPath)` method to convert the directory path into a canonical Go import path.
3.  Call `s.ScanPackageByImport(ctx, importPath)` with the resolved import path.
4.  Return the result of the `ScanPackageByImport` call.

### Step 2: Normalize `main` Package Names

To resolve the symbol collision issue, we will introduce a normalization step inside `go-scan`.

When a package is scanned, if its name is determined to be `main`, the `PackageInfo.Name` will be updated to a unique, qualified name in the format `<import-path>.main`. For example, a package at import path `example.com/my-project/cmd/server` with `package main` will have its `PackageInfo.Name` set to `example.com/my-project/cmd/server.main`.

This normalization will happen within the low-level `scanner.Scanner`'s `scanGoFiles` method, as this is where the dominant package name is determined. This ensures that any tool consuming `PackageInfo` (including `symgo`) will receive the normalized, unambiguous name, thus preventing symbol collisions.

### Step 3: Remove `ScanPackageByPos` and Refactor Usage

The `ScanPackageByPos` method will be deleted from `goscan.Scanner`. The single usage in `examples/docgen/analyzer.go` will be refactored to manually resolve the position to a directory and call `ScanPackage`.

### Step 4: Update Docstrings

The docstrings for `ScanPackage` and `ScanPackageByImport` in `goscan.go` will be updated to reflect the changes.

## 4. Phased Implementation

The work will be broken down into distinct phases:

- **Phase 1: Core `goscan` Refactoring.**
    - Implement Step 1 (Refactor `ScanPackage`).
    - Implement Step 3 (Remove `ScanPackageByPos` and refactor `docgen`).
    - Implement Step 4 (Update Docstrings).
    - Run tests within the `go-scan` module to verify these changes.

- **Phase 2: `main` Package Normalization and `symgo` Integration.**
    - Implement Step 2 (Normalize `main` package names).
    - Run the full test suite, including `symgo` integration tests, to validate the fix and check for regressions.

- **Phase 3: Final Verification and Submission.**
    - Perform a final review and submit the completed work.

## 5. Risks and Concerns

- **`locator.PathToImport` Accuracy**: The `ScanPackage` refactoring relies on the `locator` correctly converting file paths to import paths.
- **Impact of `main` Normalization**: Changing `PackageInfo.Name` is a significant change. Other tools or tests besides `symgo` might be unexpectedly affected. Phase 2 is designed to isolate and address this risk.
- **Cache Consistency**: The refactoring must not introduce inconsistencies into the `packageCache`. Using the canonical import path as the key should maintain consistency.
