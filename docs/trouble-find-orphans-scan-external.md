# Trouble: `find-orphans` Scans Packages Outside the Workspace

## Symptom

When running the `find-orphans` tool on a project, it was observed that the underlying `go-scan` library was reading files from packages outside the specified workspace (e.g., from the Go module cache). This could lead to unnecessary file I/O and, in environments without network access, potential failures when trying to download modules.

The intended behavior for `find-orphans` is to perform its analysis strictly within the packages defined by the user's workspace (e.g., the modules in `go.work` or the main module).

## Root Cause

The root cause was traced to the symbolic execution engine, `symgo`. When the `symgo` evaluator encountered a selector expression for a symbol in a package it had not yet seen (e.g., `extpkg.SomeFunc`), it would unconditionally ask the `go-scan` library to scan that package by its import path (`e.scanner.ScanPackageByImport(ctx, val.Path)`).

The `go-scan` instance used by `find-orphans` was configured with `WithGoModuleResolver()`, which allows it to fetch packages from the module cache. This meant that any mention of an external package in the code being analyzed would trigger a scan of that package's source files, even if it was not part of the defined workspace. This behavior, while useful for some tools that need type information from external dependencies, was incorrect for `find-orphans`.

The core issue was that the check to determine if a package was "scannable" or part of the workspace was performed *after* the scan had already occurred.

## Solution

The solution was to modify `symgo`'s evaluator to respect the workspace boundaries *before* attempting to scan a new package.

1.  **Refactored `isScannablePackage`:** The helper function `isScannablePackage` in `symgo/evaluator/evaluator.go` was refactored to take a string `importPath` instead of a `*scanner.PackageInfo`. This allows the check to be performed before any package information is loaded. The function's logic, which correctly identifies packages within the workspace (i.e., belonging to a configured module or explicitly included via `WithExtraPackages`), was preserved.

2.  **Pre-emptive Check in Evaluator:** The call to `isScannablePackage` was moved inside `evalSelectorExpr` to occur *before* the call to `e.scanner.ScanPackageByImport()`.

    ```go
    // in symgo/evaluator/evaluator.go, inside evalSelectorExpr
    case *object.Package:
        // ...
        if val.ScannedInfo == nil {
            if e.scanner == nil {
                return e.newError(...)
            }

            // Perform the check BEFORE scanning.
            if !e.isScannablePackage(val.Path, pkg) {
                // If the package is not part of the workspace, do not scan it.
                // Return a generic placeholder for the symbol.
                placeholder := &object.SymbolicPlaceholder{
                    Reason:  fmt.Sprintf("external symbol %s.%s", val.Name, n.Sel.Name),
                    Package: nil,
                }
                val.Env.Set(n.Sel.Name, placeholder)
                return placeholder
            }

            // If it IS a scannable package, proceed with the scan.
            pkgInfo, err := e.scanner.ScanPackageByImport(ctx, val.Path)
            // ...
            val.ScannedInfo = pkgInfo
        }
        // ...
    ```

3.  **Updated Tests:** Since this change in behavior is fundamental, several tests in the `symgo` package had to be updated. Tests that previously relied on the implicit scanning of external packages to get type information for constants or function return values were modified to assert that they now receive an untyped `SymbolicPlaceholder`, reflecting the new, stricter evaluation strategy.

This change ensures that `symgo`'s behavior is more aligned with the principle of least privilege. It will not perform file I/O for packages outside the defined workspace unless explicitly instructed to do so via the `WithExtraPackages` option, resolving the bug in `find-orphans` while maintaining flexibility for other tools.
