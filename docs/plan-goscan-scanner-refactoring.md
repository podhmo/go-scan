# Plan: Unify Scanner Logic & Introduce a Unique Package ID

This document outlines a plan to refactor the `goscan.Scanner`. This refactoring addresses two core issues:
1.  Unifying the divergent and fragile path-handling logic between `ScanPackage` and `ScanPackageByImport`.
2.  Introducing a new, unique `ID` field to `scanner.PackageInfo` to definitively solve the ambiguity of `main` packages for downstream tools like `symgo`.

## 1. Background: The Two Core Problems

### Problem 1: Divergent Path-Handling Logic
A subtle but critical discrepancy exists between `ScanPackage` (which takes a file path like `./foo`) and `ScanPackageByImport` (which takes a canonical package path like `example.com/module/foo`). This becomes apparent when scanning packages that are not inside the primary module's directory, such as those included via a `go.mod` `replace` directive.

- **`ScanPackageByImport`'s Method**: This method is robust. It uses an `isExternalModule` check. If a package's directory is outside the main module root, it calls a special low-level parse function (`scanner.ScanFilesWithKnownImportPath`) that is given the correct, canonical package path. This works.
- **`ScanPackage`'s Method**: This method is fragile. It has its own "calculate-and-correct" workflow for external paths that is brittle and duplicates logic that should be centralized.
- **The Risk**: A naive refactoring of `ScanPackage` to use `ScanPackageByImport` would break the existing behavior for certain file-path-based scans, as the context of the original file path can be lost, causing the `isExternalModule` check to fail. The goal is to unify these methods so they share a single, robust implementation that handles all cases correctly.

### Problem 2: `main` Package Ambiguity
Downstream tools like `symgo` need a way to distinguish between different `main` packages. The user has clarified the precise solution required:

- **`PackageInfo.Name` Must Remain `"main"`**: The `Name` field must continue to reflect the `package main` declaration in the source code.
- **A New Unique `ID` is Needed**: To solve the ambiguity, a new field, `ID`, will be added to `scanner.PackageInfo`.
- **The `ID` Generation Rule**: The `ID` serves as a unique key.
    - For a normal package (`example.com/foo/bar`), the `ID` is its canonical import path: `"example.com/foo/bar"`.
    - For a `main` package at `example.com/cmd/app`, the `ID` is the canonical import path with a `.main` suffix: `"example.com/cmd/app.main"`.
- **Consumer's Role**: This allows a tool like `symgo` to use this `ID` as a unique key for its internal data structures, resolving the collision problem without altering the package's fundamental name.

## 2. Detailed Plan

### Step 1: Add the `ID` Field to `PackageInfo`
- The `scanner.PackageInfo` struct in `scanner/models.go` will be modified to include the new `ID` field: `ID string`.

### Step 2: Create a Unified, Robust `scan` Method
- A new private method, `scan(ctx, path string)`, will be created in `goscan.go`. This method will contain the complete, unified logic.
- **Path Resolution**: The method must correctly handle various path types as input, including relative file paths (`./foo`) and relative package paths (`mymodule/foo`), distinguishing between them to find the correct canonical package path. It must **use the existing `locator` package's functions** for this and not re-implement this logic.
- **ID Generation**: After the canonical package path is determined and the package name is found by the low-level parser, the `scan` method will be responsible for generating the new `ID` string according to the rule (`<path>` or `<path>.main`) and populating the `PackageInfo.ID` field.
- **Low-Level Call**: The method will use the `isExternalModule` check to call the correct low-level parser (`ScanFiles` or `ScanFilesWithKnownImportPath`), ensuring the canonical package path is always passed down.

### Step 3: Refactor Public Methods
- `ScanPackage` and `ScanPackageByImport` will be refactored to be simple wrappers around the new private `scan` method.
- The deprecated `ScanPackageByPos` method will be deleted, and its call site in `examples/docgen` will be updated to use the robust `ScanPackage`.

### Step 4: Update Documentation and Verify
- Docstrings for all affected structs and methods will be updated, clarifying the role of the new `ID` field and the behavior of the refactored methods.
- The full test suite will be run. The `symgo` test `cross_main_package_test.go` will be adapted to use the new `ID` for its entry point. With this change, the test should pass, and no regressions should be introduced.

---
*This document will be submitted for review before any code modifications are undertaken.*
