# Plan: Add Unique ID to `PackageInfo` and Unify Scanner Logic

This document outlines a plan to refactor the `goscan.Scanner`. The core of this task is to introduce a new, unique `ID` field to `scanner.PackageInfo` to definitively solve the ambiguity of `main` packages for downstream tools. This change necessitates unifying the path-handling logic in the scanner's public methods.

## 1. The Core Problem & The New Approach

### The Ambiguity of `main`
Downstream tools like `symgo` need a way to distinguish between different `main` packages when analyzing a whole project or workspace. Currently, `go-scan` sets `PackageInfo.Name` to `"main"` for all such packages, creating ambiguity.

### The Solution: A New `ID` Field
Per user direction, we will solve this by introducing a new field to `scanner.PackageInfo`:
```go
type PackageInfo struct {
    // ... existing fields
    ID string // A unique, canonical key for the package.
}
```

This `ID` will serve as the stable, unique identifier for a package. Its generation logic will be:
- For a **normal package**, the `ID` is its canonical import path.
  - e.g., `example.com/foo/bar` -> `ID: "example.com/foo/bar"`
- For a **`main` package**, the `ID` is its canonical import path with a `.main` suffix.
  - e.g., a `main` package at `example.com/cmd/app` -> `ID: "example.com/cmd/app.main"`

Crucially, existing fields will **not** be changed:
- `PackageInfo.Name` will remain `"main"` for main packages.
- `PackageInfo.ImportPath` will remain the canonical import path (without the `.main` suffix).

This provides a clean, explicit key for consumers like `symgo` to use for disambiguation, without altering the meaning of existing fields.

### The Refactoring Necessity
To implement this correctly, the scanner's methods must be able to robustly determine the canonical import path for any given input (be it a file path, relative path, or import path). Currently, `ScanPackage` and `ScanPackageByImport` have different, fragile logic for this. Unifying them is a necessary step to ensure the new `ID` field is always populated correctly.

## 2. Detailed Plan

### Step 1: Add the `ID` field
- Modify the `scanner.PackageInfo` struct in `scanner/models.go` to include the new `ID` field.

### Step 2: Implement ID Generation and Unify Pathing Logic
- **Create a new private method `scan` in `goscan.go`**: This method will contain the unified and robust logic.
    1. It will accept a generic `path` string.
    2. It will determine if the input `path` is a file path or a canonical package path.
    3. It will robustly find the correct `locator` for the path, even in workspace mode.
    4. It will resolve the input to a **canonical import path** and an **absolute directory path**.
    5. It will call the low-level `scanner.scanGoFiles`, passing it the canonical import path.
    6. Inside `scanner.scanGoFiles`, after determining the `packageName`, the new `ID` field will be generated based on the `packageName` and the canonical import path.
- **Refactor `ScanPackage` and `ScanPackageByImport`**: These methods will become simple wrappers around the new private `scan` method.

### Step 3: Remove `ScanPackageByPos`
- The deprecated `ScanPackageByPos` method will be deleted.
- Its usage in `examples/docgen/analyzer.go` will be updated to use the robust `ScanPackage`.

### Step 4: Verification
- Update docstrings for all affected structs and methods.
- Run the full test suite. The `symgo` test `cross_main_package_test.go` will need to be adapted to use the new `ID` for its entry point, and with that change, it should pass. All other tests should continue to pass.

---
*This document will be submitted for review before any code modifications are undertaken.*
