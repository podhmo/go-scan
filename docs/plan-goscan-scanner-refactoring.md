# Plan: Unify Scanner Logic & Introduce a Unique Package ID

This document outlines a plan to refactor the `goscan.Scanner`. This refactoring addresses two core issues simultaneously:
1.  Unifying the divergent and fragile path-handling logic between `ScanPackage` and `ScanPackageByImport`.
2.  Introducing a new, unique `ID` field to `scanner.PackageInfo` to definitively solve the ambiguity of `main` packages for downstream tools.

## 1. Background: The Two Core Problems

### Problem 1: The Scanner Method Refactoring Story (Divergent Path Logic)
A subtle but critical discrepancy exists between `goscan.Scanner`'s two main methods: `ScanPackage` (which takes a file path like `./foo`) and `ScanPackageByImport` (which takes a canonical package path like `example.com/module/foo`). This becomes apparent when scanning packages that are not inside the primary module's directory, such as those included via a `go.mod` `replace` directive.

- **`ScanPackageByImport` is Robust**: This method correctly handles these "external" packages. It uses an `isExternalModule` check. If the package's resolved directory is outside the main module root, it calls a special low-level parse function (`scanner.ScanFilesWithKnownImportPath`) that is given the correct, canonical package path. This works.
- **`ScanPackage` is Fragile**: This method has its own "calculate-and-correct" workflow for external paths. It first correctly determines the canonical package path using the `locator`. However, it then calls the naive low-level parse function (`scanner.ScanFiles`), which incorrectly recalculates the path. `ScanPackage` then "fixes" this by overwriting the incorrect path on the returned object with the correct one it calculated initially. This workflow is brittle and a source of potential bugs.

The core of the refactoring is to eliminate this fragile, divergent logic. We must unify the methods so they share a single, robust implementation that handles all pathing scenarios correctly.

### Problem 2: The ID Issuance Story (`main` Package Ambiguity)
Downstream tools like `symgo` need a way to distinguish between different `main` packages. The user has clarified the precise solution required:

- **`PackageInfo.Name` Must Remain `"main"`**: The `Name` field must continue to reflect the `package main` declaration in the source code.
- **A New Unique `ID` is Needed**: To solve the ambiguity, a new field, `ID`, will be added to `scanner.PackageInfo`.
- **The `ID` Generation Rule (The "Key")**: The `ID` serves as a unique key. This key is the result of resolving a potentially relative path (like `./foo`) into a canonical, unique package path, with a special modification for `main` packages.
    - For a normal package (`example.com/foo/bar`), the `ID` is its canonical import path: `"example.com/foo/bar"`.
    - For a `main` package at `example.com/cmd/app`, the `ID` is the canonical import path with a `.main` suffix: `"example.com/cmd/app.main"`.
- **Connecting the Problems**: To generate this `ID` correctly, the scanner *must* first have a robust and consistent way of determining the canonical package path for *any* given input. Fixing the pathing logic from Problem 1 is therefore a prerequisite for correctly implementing the `ID` generation for Problem 2.

## 2. Detailed Plan

### Step 1: Add the `ID` Field to `PackageInfo`
- The `scanner.PackageInfo` struct in `scanner/models.go` will be modified to include the new `ID` field: `ID string`.

### Step 2: Create a Unified, Robust `scan` Method
- A new private method, `scan(ctx, path string)`, will be created in `goscan.go`. This method will contain the complete, unified logic.
- **Path Resolution**: The method must correctly handle various path types as input, including relative file paths (`./foo`) and relative package paths (`mymodule/foo`), distinguishing between them to find the correct canonical package path. It must **use the existing `locator` package's functions** for this and not re-implement this logic, ensuring it works correctly for workspaces and `replace` directives.
- **ID Generation**: After the canonical package path is determined and the package name is found by the low-level parser, the `scan` method will be responsible for generating the new `ID` string according to the rule (`<key>` or `<key>.main`) and populating the `PackageInfo.ID` field.
- **Low-Level Call**: The method will use the `isExternalModule` check to call the correct low-level parser (`ScanFiles` or `ScanFilesWithKnownImportPath`), ensuring the canonical package path is always passed down.

### Step 3: Refactor Public Methods
- `ScanPackage` and `ScanPackageByImport` will be refactored to be simple wrappers around the new private `scan` method.
- The deprecated `ScanPackageByPos` method will be deleted, and its call site in `examples/docgen` will be updated to use the robust `ScanPackage`.

### Step 4: Update Documentation and Verify
- Docstrings for all affected structs and methods will be updated, clarifying the role of the new `ID` field and the behavior of the refactored methods.
- The full test suite will be run. The `symgo` test `cross_main_package_test.go` will be adapted to use the new `ID` for its entry point. With this change, the test should pass, and no regressions should be introduced.

---
*This document will be submitted for review before any code modifications are undertaken.*
