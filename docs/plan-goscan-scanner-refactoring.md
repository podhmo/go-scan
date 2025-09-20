# Plan: Refactor `goscan.Scanner` for Workspace and Pathing Robustness

This document outlines a plan to refactor the `goscan.Scanner`. The primary goals are to make the `ScanPackage` method correctly handle complex scenarios involving multi-module workspaces and `replace` directives, and to introduce a normalization for `main` package names to prevent symbol collision bugs in downstream tools.

## 1. Background & Problem Analysis

### The Path-Handling Discrepancy
A subtle but critical discrepancy exists between `ScanPackage` (which takes a file path) and `ScanPackageByImport` (which takes an import path). This discrepancy becomes apparent when scanning packages that are not inside the primary module's directory, such as those included via a `replace` directive in `go.mod` or those in a multi-module workspace.

- **`ScanPackageByImport` is Robust**: This method correctly handles these "external" packages. It uses a special `isExternalModule` check. If the package's directory is outside the scanner's main root directory, it calls a special low-level parse function (`scanner.ScanFilesWithKnownImportPath`) that is given the correct, canonical import path. This ensures the resulting `PackageInfo` is correct.

- **`ScanPackage` is Fragile**: This method has a more naive workflow. When given a path to an external package, it first correctly determines the canonical import path using the `locator`. However, it then calls the naive low-level parse function (`scanner.ScanFiles`), which does *not* receive the correct import path and tries to recalculate it (incorrectly). `ScanPackage` then "fixes" the problem by overwriting the incorrect import path on the returned object with the correct one it calculated initially.

This "calculate-and-correct" workflow is fragile and a source of potential bugs.

### The Refactoring Challenge
The user's request is to refactor `ScanPackage` to use `ScanPackageByImport`. As the user correctly pointed out, a naive refactoring would break the currently "solved" (albeit fragile) behavior for `replace`d packages.

- **Why it Breaks**: A naive refactoring would call `locator.PathToImport()` to get the import path, then pass that to `ScanPackageByImport`. However, this loses the crucial context that the *original call was path-based*. The `ScanPackageByImport` function's `isExternalModule` check might not trigger correctly without this context, leading it to call the naive `scanner.ScanFiles` and resulting in an incorrect `PackageInfo`.

The core task is to refactor `ScanPackage` to be as robust as `ScanPackageByImport` without losing its path-based-call context, and then unify the logic.

### The `main` Package Collision
A related, mandatory task is to fix the ambiguity of `package main`. When `go-scan` scans multiple `main` packages, it returns `PackageInfo.Name` as `"main"` for all of them. This causes symbol collisions in tools like `symgo`. The `symgo/integration_test/cross_main_package_test.go` test will fail until this is fixed. The solution is to normalize the package name as part of this refactoring effort.

## 2. Detailed Plan

### Phase 1: Implement Prerequisite Fixes

#### Step 1.1: Implement `main` Package Name Normalization
This is a defensive measure to prevent regressions and fix the `symgo` test. The change will be in `scanner/scanner.go`.

- **Logic**: In the low-level `scanGoFiles` method, after the dominant package name is determined to be `"main"`, it will be immediately qualified using the canonical import path that is passed into the function. The name will be changed to the format `<import-path>.main`.
- **Example**: A package with import path `example.com/project/cmd/server` will have its `PackageInfo.Name` set to `example.com/project/cmd/server.main`.

### Phase 2: Unify Scanner Methods and Verify

#### Step 2.1: Refactor `ScanPackage`
The `ScanPackage` method in `goscan.go` will be rewritten to be a robust wrapper around `ScanPackageByImport`.

- **Logic**:
  1. Take the input directory path (`pkgPath`).
  2. In workspace mode, iterate through all `s.locators` to find the one whose `RootDir` contains `pkgPath`. In non-workspace mode, just use `s.locator`.
  3. Use the *correct* locator to convert the `pkgPath` to its canonical `importPath` (`correctLocator.PathToImport(pkgPath)`).
  4. Call `s.ScanPackageByImport(ctx, resolvedImportPath)`.
- **Impact**: This makes `ScanPackage` fully workspace-aware and robustly handle `replace` directives, just like `ScanPackageByImport`. The fragile "calculate-and-correct" logic is eliminated.

#### Step 2.2: Remove `ScanPackageByPos`
The `ScanPackageByPos` method will be deleted. Its usage in `examples/docgen/analyzer.go` will be refactored to use the newly-fixed `ScanPackage`.

#### Step 2.3: Update Docstrings
The docstrings for `ScanPackage` and `ScanPackageByImport` will be updated.

#### Step 2.4: Full Integration Test
Run the entire test suite, including `symgo` tests. The `cross_main_package_test.go` should now pass. All other tests should continue to pass. Any regressions will be fixed.

## 3. Risks
- **Workspace Logic Interaction**: The new logic in `ScanPackage` for finding the correct locator for a file path must be robust.
- **`main` Normalization Side Effects**: Changing the package name could have side effects. A full test run is essential.

---
*This document will be submitted for review before any code modifications are undertaken.*
