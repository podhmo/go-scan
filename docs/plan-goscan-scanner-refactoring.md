# Plan: Unify Scanner Pathing Logic and Remove `ScanPackageByPos`

This document outlines a plan to refactor the `goscan.Scanner`. The primary goals are to unify the divergent path-handling logic between `ScanPackage` and `ScanPackageByImport`, and to remove the deprecated `ScanPackageByPos` method. This work is necessary to improve the scanner's maintainability and robustness.

## 1. The Core Problem: Divergent Path Logic

A subtle but critical discrepancy exists between `ScanPackage` (which takes a file path like `./foo`) and `ScanPackageByImport` (which takes a canonical package key like `example.com/module/foo`). This becomes apparent when scanning packages that are not inside the primary module's directory, such as those included via a `replace` directive in `go.mod`.

- **`ScanPackageByImport` is Robust**: This method works with a canonical package key. It correctly handles complex scenarios by using a special `isExternalModule` check. If the package's directory is outside the scanner's main root directory, it calls a special low-level parse function (`scanner.ScanFilesWithKnownImportPath`) that is given the correct key. This ensures the resulting `PackageInfo` is correct.

- **`ScanPackage` is Fragile**: This method takes a raw file path. It contains a fragile "calculate-and-correct" workflow. It first correctly determines the canonical package key using the `locator`. However, it then calls a naive low-level parse function (`scanner.ScanFiles`) which incorrectly recalculates the key. `ScanPackage` then "fixes" this by overwriting the incorrect key on the returned object with the correct one it calculated initially.

- **The `symgo` Test Scenario**: The `cross_main_package_test.go` test works because the `symgotest` harness calls `scanner.Scan("./...")`, which uses the file-path-based `ScanPackage`. The fragile logic inside `ScanPackage` correctly handles the test's ad-hoc local modules. A naive refactoring of `ScanPackage` could break this by losing the necessary file path context.

The goal is to unify these two methods, ensuring the resulting single implementation is robust and that the correct **canonical package key** (the unique, fully-resolved package path calculated from an input like `./foo`) is used throughout.

## 2. The `main` Package Ambiguity

A related task is to address the ambiguity of `package main`. The user has clarified that **`PackageInfo.Name` must remain `"main"`**.

The "key" that disambiguates different `main` packages is their unique, canonical package path (which this document refers to as the "key"). The `symgotest` harness demonstrates the correct pattern: it uses this key to retrieve a specific package's environment, and then looks for the `main` function within that unique context.

The problem is that the logic to do this reliably is split across the scanner and the test harness. The refactoring must ensure that the scanner consistently produces a correct and canonical key in `PackageInfo.ImportPath` for all packages, including `main` packages, so that consumers like `symgo` can rely on it for disambiguation.

## 3. Detailed Plan

### Step 1: Unify and Refactor `goscan.Scanner`

The core of the work is to refactor `ScanPackage` and `ScanPackageByImport` into a single, robust implementation.

- **New private method `scan`**: A new private method, `scan(ctx, path string)`, will be created. This method will contain the unified logic.
    - It will determine if the input `path` is a canonical package key or a file path.
    - It will robustly find the correct `locator` (in workspace mode).
    - It will correctly determine the canonical package key (`PackageInfo.ImportPath`) and the absolute directory path, regardless of the input type.
    - It will use the `isExternalModule` check and call the appropriate low-level parser (`ScanFiles` or `ScanFilesWithKnownImportPath`), always providing the correct canonical key.
    - It will handle all caching, using the canonical package key.

- **Refactor `ScanPackage`**: Its body will be replaced with a call to `s.scan(ctx, pkgPath)`.

- **Refactor `ScanPackageByImport`**: Its body will be replaced with a call to `s.scan(ctx, importPath)`.

### Step 2: Remove `ScanPackageByPos`

The `ScanPackageByPos` method will be deleted. Its usage in `examples/docgen/analyzer.go` will be refactored to use the newly-robust `ScanPackage`.

### Step 3: Update Docstrings and Verify

The docstrings for `ScanPackage` and `ScanPackageByImport` will be updated to clarify their behavior and use of file paths vs. canonical package keys. The full test suite, including the `symgo` tests, will be run to ensure no regressions and that the `cross_main_package_test` continues to pass.

---
*This document will be submitted for review before any code modifications are undertaken.*
