# Plan: Refactor `goscan.Scanner` Methods

This document outlines a plan to refactor the main `goscan.Scanner` to streamline its API, improve internal consistency, and address challenges related to package name resolution in tools like `symgo`.

## 1. Goals

- **Unify Scanning Logic**: Make `ScanPackage` a lightweight wrapper around `ScanPackageByImport`. This centralizes the core scanning logic in one method, reducing code duplication and making the system easier to maintain.
- **Improve API Clarity**: Remove the `ScanPackageByPos` method. Users who need this functionality can replicate it easily by finding the file path from the position and then using `ScanPackage`.
- **Enhance `symgo` Robustness**: Proactively address an issue where `symgo`'s tests fail when analyzing projects with multiple `main` packages. This will be solved by normalizing `main` package names internally.
- **Update Documentation**: Ensure that the docstrings for `ScanPackage` and `ScanPackageByImport` accurately reflect their new implementations.

## 2. Background

The `goscan.Scanner` currently has three similar methods for scanning packages:
- `ScanPackageByImport(ctx, importPath)`: The primary and most robust method. It takes a Go import path, uses a `locator` to find the corresponding directory, and performs a full scan.
- `ScanPackage(ctx, pkgPath)`: Takes a filesystem directory path. It currently contains its own logic for scanning and for deriving an import path from the directory path.
- `ScanPackageByPos(ctx, pos)`: A convenience method that finds a file from a `token.Pos`, gets its directory, and calls `ScanPackage`.

This redundancy creates maintenance overhead. The most significant issue is that `ScanPackage`'s logic is separate from `ScanPackageByImport`, leading to potential inconsistencies.

Furthermore, the `symgo` symbolic execution engine relies heavily on `go-scan`. When analyzing projects with multiple `main` packages (a common pattern in `cmd` directories), `symgo` can get confused because the package name "main" is not unique.

## 3. Detailed Plan

### Step 1: Refactor `ScanPackage` to use `ScanPackageByImport`

The `ScanPackage` method will be refactored to perform the following steps:
1.  Take the input directory path (`pkgPath`).
2.  Use the `locator.PathToImport(pkgPath)` method to convert the directory path into a canonical Go import path. The logic for this already exists in `locator` and parts of the current `ScanPackage` implementation.
3.  Call `s.ScanPackageByImport(ctx, importPath)` with the resolved import path.
4.  Return the result of the `ScanPackageByImport` call.

This change makes `ScanPackageByImport` the single source of truth for package scanning.

### Step 2: Normalize `main` Package Names

A key part of the `symgo` issue is that the `PackageInfo.Name` for multiple packages can be "main". To resolve this, we will introduce a normalization step.

When a package is scanned, if its name is `main`, the `PackageInfo.Name` will be updated to a unique, qualified name in the format `<import-path>.main`. For example, a package at `example.com/my-project/cmd/server` with `package main` will have its `PackageInfo.Name` set to `example.com/my-project/cmd/server.main`.

This normalization will happen within the low-level `scanner.Scanner`'s `scanGoFiles` method, as this is where the dominant package name is determined. This ensures that any tool consuming `PackageInfo` (including `symgo`) will receive the normalized name.

### Step 3: Remove `ScanPackageByPos`

The `ScanPackageByPos` method will be deleted from `goscan.Scanner`.

The single usage of this method in `examples/docgen/analyzer.go` will be refactored. The calling code will be modified to:
1.  Get the file path from the `token.Pos`: `file := a.Scanner.Fset().File(handler.Decl.Pos())`.
2.  Get the directory from the file path: `pkgDir := filepath.Dir(file.Name())`.
3.  Call the refactored `ScanPackage`: `pkg, err := a.Scanner.ScanPackage(ctx, pkgDir)`.

This is a straightforward change that makes the dependency explicit in the calling code.

### Step 4: Update Docstrings

The docstrings for `ScanPackage` and `ScanPackageByImport` in `goscan.go` will be updated to reflect the changes:
- `ScanPackage`: The docstring will state that it resolves the given directory path to an import path and then calls `ScanPackageByImport`.
- `ScanPackageByImport`: The docstring will be clarified to emphasize that it is the primary, canonical method for scanning packages.

## 4. Risks and Concerns

- **`locator.PathToImport` Accuracy**: The success of the `ScanPackage` refactoring depends on `locator.PathToImport` reliably converting file paths to import paths, especially for packages that might be outside the main module root (e.g., via `replace` directives in `go.mod`). This functionality needs to be solid.
- **Impact of `main` Normalization**: Changing `PackageInfo.Name` from `main` to `<path>.main` is a significant change. While it's expected to fix the `symgo` issue, it might have unforeseen consequences for other tools or tests that expect the name to be exactly "main". A thorough test run will be critical to identify and fix any such issues.
- **Cache Consistency**: The refactoring must not introduce inconsistencies into the `packageCache`. Since both methods will now funnel through `ScanPackageByImport`, which has its own caching logic based on import paths, the cache should remain consistent. We must ensure the file-path-to-import-path conversion is canonical to avoid cache fragmentation.

## 5. Timeline

This work will be completed in a single phase. Once this planning document is approved, the implementation will begin. The work will be submitted for review before any code changes are made.

**Note:** As per the user's request, this document will be submitted before any code modifications are undertaken.
