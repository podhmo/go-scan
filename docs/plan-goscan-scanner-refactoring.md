# Plan: Unify Scanner Pathing Logic and Remove `ScanPackageByPos`

This document outlines a plan to refactor the `goscan.Scanner`. The primary goals are to unify the divergent path-handling logic between `ScanPackage` and `ScanPackageByImport`, and to remove the deprecated `ScanPackageByPos` method. This work is necessary to improve the scanner's maintainability and robustness.

## 1. The Core Problem: Divergent Path Logic

A subtle but critical discrepancy exists between `ScanPackage` (which takes a file path) and `ScanPackageByImport` (which takes an import path). This becomes apparent when scanning packages that are not inside the primary module's directory, such as those included via a `replace` directive in `go.mod`.

- **`ScanPackageByImport` is Robust**: This method correctly handles these "external" packages. It uses a special `isExternalModule` check. If the package's directory is outside the scanner's main root directory, it calls a special low-level parse function (`scanner.ScanFilesWithKnownImportPath`) that is given the correct, canonical import path. This ensures the resulting `PackageInfo` is correct.

- **`ScanPackage` is Fragile**: This method has a more naive workflow. When given a path to an external package, it first correctly determines the canonical import path using the `locator`. However, it then calls the naive low-level parse function (`scanner.ScanFiles`), which does *not* receive the correct import path and tries to recalculate it (incorrectly). `ScanPackage` then "fixes" the problem by overwriting the incorrect import path on the returned object with the correct one it calculated initially. This "calculate-and-correct" workflow is fragile.

- **The `symgo` Test Scenario**: The `cross_main_package_test.go` test works because the `symgotest` harness calls `scanner.Scan("./...")`, which uses the file-path-based `ScanPackage`. The fragile "calculate-and-correct" logic inside `ScanPackage` correctly handles the test's ad-hoc local modules. A naive refactoring of `ScanPackage` could break this by losing the necessary file path context.

The goal of this refactoring is to unify these two methods, ensuring the resulting single implementation is as robust as the two separate ones combined, correctly handling all pathing scenarios (in-module, external, replaced, workspace, etc.).

## 2. The `main` Package Ambiguity

A related task, requested by the user, is to address the ambiguity of `package main`. The user stated that `PackageInfo.Name` must remain `"main"`. However, downstream tools need a way to distinguish between different main packages.

The "key" that disambiguates them is their unique **import path**. The `symgotest` harness demonstrates the correct pattern: it uses the import path to retrieve a specific package's environment, and then looks for the `main` function within that unique context.

The problem is that the logic to do this is currently in the test harness. The user wants this "normalization" to happen within the scanner itself. This means we need to provide a way for a consumer of the scanner to uniquely identify a `main` package. Since we cannot change `PackageInfo.Name`, we must ensure that the `PackageInfo.ImportPath` is always correct and canonical, even for `main` packages, so it can be used as a reliable key. The path-handling improvements are the solution to this.

## 3. Detailed Plan

### Step 1: Unify and Refactor `goscan.Scanner`

The core of the work is to refactor `ScanPackage` and `ScanPackageByImport` into a single, robust implementation.

- **New private method `scan`**: A new private method, `scan(ctx, path string)`, will be created. This method will contain the unified logic.
    - It will determine if `path` is an import path or a file path.
    - It will robustly find the correct `locator` (in workspace mode).
    - It will correctly determine the canonical import path and the absolute directory path, regardless of the input type.
    - It will use the `isExternalModule` check and call the appropriate low-level parser (`ScanFiles` or `ScanFilesWithKnownImportPath`).
    - It will handle all caching.

- **Refactor `ScanPackage`**: Its body will be replaced with a call to `s.scan(ctx, pkgPath)`.

- **Refactor `ScanPackageByImport`**: Its body will be replaced with a call to `s.scan(ctx, importPath)`.

### Step 2: Remove `ScanPackageByPos`

The `ScanPackageByPos` method will be deleted. Its usage in `examples/docgen/analyzer.go` will be refactored to use the newly-robust `ScanPackage`.
1. Get file path from `token.Pos`.
2. Call `ScanPackage` with the file path's directory.

### Step 3: Update Docstrings and Verify

The docstrings for `ScanPackage` and `ScanPackageByImport` will be updated to reflect that they are now simple wrappers around a unified, robust core logic. The full test suite, including the `symgo` tests, will be run to ensure no regressions and that the `cross_main_package_test` continues to pass.

---
*This document will be submitted for review before any code modifications are undertaken.*
