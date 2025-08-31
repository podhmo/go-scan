# Diagnosing `identifier not found` in `symgo` via Scanner Logic

This document details the investigation into a subtle bug where `symgo`'s symbolic execution would fail with an `identifier not found` error for a package-level constant. The root cause was not in `symgo` itself, but in the upstream `scanner`'s complex logic for handling directories containing files from multiple packages.

## 1. The Original Problem

The user reported an `identifier not found: ContextKeyTesting` error when running `find-orphans --include-tests`. The setup involved a test file (`main_test.go`) in a `main` package that imported a library package (`testlog`) and called a function that used a `const` defined in that library.

The `find-orphans` tool, which uses `symgo`, would fail during the symbolic execution of the test function because the constant from the imported `testlog` package could not be found in its environment.

## 2. Investigation and False Leads

Initial investigation focused on `symgo`'s state management. The logs clearly showed that `symgo` *attempted* to populate the constants for the `testlog` package, yet the constant was missing during execution. This contradiction led to a lengthy debugging process:

1.  **Unit Tests for `symgo`**: Multiple attempts to create a failing test case directly within the `symgo` test suite failed. Even high-fidelity tests that simulated the `find-orphans` execution flow (e.g., walking all dependencies, pre-loading all files with `interp.Eval`) could not reproduce the error. This suggested the issue was not in `symgo`'s core logic but in the data it was receiving from the scanner.

2.  **Unit Tests for `goscan`**: A direct test of `goscan.ScanPackageByImport` on the library package passed, showing that the scanner *could* correctly parse the constants when a package was scanned in isolation.

3.  **End-to-End `find-orphans` Execution**: The only reliable way to reproduce the error was to build `find-orphans` with `go install` and run it on the user's exact file structure. This proved the bug was real but highly dependent on the full application's execution flow.

The breakthrough came from analyzing the `scanner.scanGoFiles` function, which contained complex, stateful logic to determine a "dominant package name" when a directory scan included files from multiple packages (e.g., `time` and `main` from the standard library's test files). This logic included a rule to silently ignore `package main` files if another package was already considered dominant.

This lenient, silent-dropping behavior was identified as the likely source of state corruption that led to an incomplete `PackageInfo` object being cached and passed to `symgo`.

## 3. The Fix: Stricter Scanner Logic

To fix the bug, the fragile "dominant package" logic in `scanner/scanner.go` was removed and replaced with a simpler, stricter rule.

**Old Logic:**
```go
// ...
} else if dominantPackageName != "main" && currentPackageName == "main" {
    // The real package is `dominantPackageName`. Ignore the `main` file.
    continue
} // ... more complex rules
```

**New Logic:**
```go
// ...
} else if currentPackageName != dominantPackageName {
    baseDominant := strings.TrimSuffix(dominantPackageName, "_test")
    baseCurrent := strings.TrimSuffix(currentPackageName, "_test")

    if baseDominant != baseCurrent {
        return nil, fmt.Errorf("mismatched package names: %s and %s in directory %s", dominantPackageName, currentPackageName, pkgDirPath)
    }

    // Logic to prefer `pkg` over `pkg_test` as dominant name remains.
    if strings.HasSuffix(dominantPackageName, "_test") && !strings.HasSuffix(currentPackageName, "_test") {
        dominantPackageName = currentPackageName
    }
}
// All files that passed the check are now added to the list.
parsedFiles = append(parsedFiles, result.fileAst)
filePathsForDominantPkg = append(filePathsForDominantPkg, result.filePath)
```

The new logic is much simpler:
- It only allows a package and its `_test` variant to co-exist in a scan.
- Any other mismatch (e.g., `package time` and `package main`) results in an immediate error instead of being silently ignored.

## 4. Regressions and Verification

This stricter logic is more correct and predictable, but it introduces a breaking change for tests that relied on the old, lenient behavior.

### 4.1. New Failing Test: `TestScanStdlib_SucceedsWithoutOverride`

The test `TestScanStdlib_SucceedsWithoutOverride` in `goscan_stdlib_test.go` now fails with:
`mismatched package names: time and main in directory /usr/local/go/src/time`

This happens because the test scans the entire `time` package directory, which includes test files that are `package main`. The new scanner logic correctly identifies this as an error. To fix this test, it would need to be updated to explicitly list the files to scan, excluding the `package main` test files.

### 4.2. New Passing Regression Test

A new test, `TestScanner_MismatchedPackageMainAndOther`, was added to `scanner/scanner_test.go`. It creates a directory with files for `package main` and `package another` and asserts that scanning them together now correctly produces a "mismatched package names" error. This test fails with the old logic but passes with the new, stricter logic, cementing the fix.
