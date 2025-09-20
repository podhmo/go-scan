# Plan: Refactor `goscan.Scanner` Methods and Fix `main` Package Collisions

This document outlines a plan to refactor the main `goscan.Scanner`. The refactoring will unify the package scanning logic and, as a necessary part of this work, introduce a fix for a symbol collision bug that occurs when analyzing projects with multiple `main` packages.

## 1. Goals

- **Unify Scanning Logic**: Make `ScanPackage` a lightweight wrapper around `ScanPackageByImport`. This centralizes the core scanning logic, reducing code duplication and making the system easier to maintain.
- **Fix `main` Package Ambiguity**: Introduce a normalization for `main` package names to resolve a bug where tools like `symgo` cannot distinguish between symbols from different `main` packages in the same workspace.
- **Improve API Clarity**: Remove the `ScanPackageByPos` method. Users can easily replicate this functionality.
- **Update Documentation**: Ensure that the docstrings for `ScanPackage` and `ScanPackageByImport` accurately reflect their new implementations.

## 2. Background & Problem Analysis

### The Refactoring Goal
The `goscan.Scanner` currently has two primary methods for scanning packages: `ScanPackage` (which takes a directory path) and `ScanPackageByImport` (which takes an import path). Their implementations are separate, leading to duplicated logic. The goal is to refactor `ScanPackage` to simply convert its directory path to an import path and call `ScanPackageByImport`, making the latter the single source of truth.

### The Latent Bug
A latent bug exists within the current scanner that affects downstream tools like `symgo`. The `symgo` symbolic execution engine needs to analyze entire applications, which can often include multiple binaries, each with its own `package main`.

The root of the problem lies in how `symgo` caches package information. The `symgo/evaluator/evaluator.go` file contains a `getOrLoadPackage` function that uses a `pkgCache` map, keyed by import path (e.g., `"example.com/pkg_a"`). This function calls `go-scan` to get a `scanner.PackageInfo` and then stores it in an `object.Package`. The `object.Package` struct contains a `Name` field, which is copied directly from `PackageInfo.Name`.

Currently, `go-scan` reports the name for *any* main package as the literal string `"main"`. This means that `symgo` ends up with multiple entries in its `pkgCache` that point to different packages on disk but whose `object.Package` value has the same `Name: "main"`.

This creates ambiguity for any part of the evaluator that needs to resolve a symbol. For example, if a function `main.run` needs to be resolved, `symgo` cannot tell if it belongs to `"example.com/pkg_a"` or `"example.com/pkg_b"`. This is the exact failure scenario that the test in `symgo/integration_test/cross_main_package_test.go` is designed to catch.

Therefore, to complete the refactoring without leaving `symgo` in a broken state, we must first fix this ambiguity in `go-scan`.

## 3. Detailed Plan

### Phase 1: Core Refactoring and Bug Fix

#### Step 1.1: Implement `main` Package Name Normalization

To resolve the symbol ambiguity, we will normalize the names of `main` packages at the source, inside `go-scan`. This change will be made in the low-level `scanner.Scanner`'s `scanGoFiles` method.

- **Logic**: When the scanner determines that the dominant package name for a set of files is `main`, it will not use the literal string `"main"`. Instead, it will use the package's canonical import path (which is passed into `scanGoFiles`) to construct a unique, qualified name in the format: `<import-path>.main`.
- **Example**: A file in `example.com/project/cmd/server` with `package main` will have its `PackageInfo.Name` set to `example.com/project/cmd/server.main`.
- **Impact**: This ensures that every `main` package has a unique name within the scanner's context. When `symgo` receives this `PackageInfo`, its `object.Package.Name` will be unique, resolving the collision issue.

#### Step 1.2: Refactor `ScanPackage` to use `ScanPackageByImport`

With the normalization logic in place, we can now safely unify the scanning methods. The `ScanPackage` method in `goscan.go` will be refactored to perform the following steps:
1.  Take the input directory path (`pkgPath`).
2.  Use `s.locator.PathToImport(pkgPath)` to convert the directory path into a canonical Go import path.
3.  Call `s.ScanPackageByImport(ctx, importPath)` with the resolved import path.
4.  Return the result.

#### Step 1.3: Remove `ScanPackageByPos` and Refactor Usage

The `ScanPackageByPos` method will be deleted from `goscan.Scanner`. The single usage in `examples/docgen/analyzer.go` will be refactored to manually find the directory from the token position and call the newly refactored `ScanPackage`.

#### Step 1.4: Update Docstrings

The docstrings for `ScanPackage` and `ScanPackageByImport` in `goscan.go` will be updated to reflect the changes.

### Phase 2: Integration Testing and Verification

#### Step 2.1: Run Full Test Suite

After the refactoring and normalization are implemented, the entire project's test suite will be run. This includes:
- Unit tests for `goscan`.
- Integration tests in `symgo`, with special attention paid to `cross_main_package_test.go`, which should now pass.
- Tests for all example tools.

#### Step 2.2: Debug and Finalize

Any regressions or test failures will be debugged and fixed. The primary focus will be ensuring that the `main` package normalization does not have unintended side effects. Once all tests pass, the work will be considered complete.

## 4. Risks

- **Impact of `main` Normalization**: While this change is designed to fix a specific bug, altering a fundamental piece of information like the package name could have unforeseen consequences. Any tool that hardcodes a check for `pkg.Name == "main"` may need to be updated. Phase 2 is designed to specifically address this risk by isolating the change and focusing on fixing the fallout.
- **`locator.PathToImport` Accuracy**: The `ScanPackage` refactoring relies on the `locator`'s ability to correctly map a file path to an import path. The investigation has shown this logic to be sound, but it remains a key dependency.

---
*This document will be submitted for review before any code modifications are undertaken.*
