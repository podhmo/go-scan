# Troubleshooting: `find-orphans` Scans External Packages

This document details a bug where the `find-orphans` tool, and by extension the `go-scan` library, would attempt to scan the source code of packages outside the main workspace, including the Go standard library. This would lead to panics or incorrect behavior.

## Symptom

When running a tool built on `go-scan`'s symbolic execution engine (`symgo`), such as `find-orphans`, a panic could occur. The stack trace would originate from the `scanner.scanGoFiles` function, and the file paths involved would point to locations within the Go installation's `GOROOT` (e.g., `/usr/local/go/src/encoding/json`) or the Go module cache.

A log message might show the scanner attempting to process a package using its absolute file path instead of its import path:

```
level=INFO source=.../scanner.go:228 msg="## scan go files" package=/opt/homebrew/Cellar/go/1.24.3/libexec/src/encoding/json size=8
panic: /opt/homebrew/Cellar/go/1.24.3/libexec/src/encoding/json
```

## Root Cause Analysis

The issue stemmed from a chain of events triggered during symbolic execution:

1.  **Type Resolution**: The `symgo` engine would encounter a type from an external package (e.g., `json.Encoder` from `encoding/json`). To understand this type, it would try to resolve its definition.
2.  **Scanner Invocation**: This resolution request was passed to the main `goscan.Scanner` instance via the `FieldType.Resolve` method. The scanner was asked to get information about the `encoding/json` package.
3.  **Package Location**: The `goscan.Scanner` uses its `locator` to find the directory for the requested import path. The locator correctly found the source code for `encoding/json` inside the system's `GOROOT`.
4.  **Incorrect Context**: The core of the problem was that `go-scan` used a single internal `scanner.Scanner` instance that was initialized with the *user's main module* context (its root directory and module path).
5.  **The Panic**: When this internal scanner was then asked to parse the Go files found in `GOROOT`, it was operating outside its configured environment. This mismatch in file paths versus the expected module root led to incorrect behavior and ultimately the panic.

A related secondary issue was an overly simplistic check for "external" modules (`isExternalModule := !strings.HasPrefix(pkgDirAbs, s.RootDir())`). This check did not correctly handle multi-module workspaces or modules using `replace` directives, causing workspace-local packages to sometimes be treated as external.

## Solution

The bug was addressed with a two-part fix in `goscan.go`:

1.  **Standard Library Special Case**: The `goscan.Scanner.ScanPackageByImport` function was modified to explicitly check if a requested package resides within the `GOROOT`. If it does, the scanner now immediately returns a minimal, empty `PackageInfo` struct. This stops the scanner from ever attempting to parse the source files of the standard library, preventing the panic. This allows `symgo` to treat standard library types as opaque, which is sufficient for its analysis.

    ```go
    // In goscan.Scanner.ScanPackageByImport...
    goRoot := runtime.GOROOT()
    if goRoot != "" && strings.HasPrefix(pkgDirAbs, filepath.Join(goRoot, "src")) {
        slog.DebugContext(ctx, "ScanPackageByImport detected stdlib package, returning minimal info", "importPath", importPath)
        pkgInfo := &scanner.PackageInfo{
            Path:       pkgDirAbs,
            ImportPath: importPath,
            Name:       filepath.Base(importPath),
            Fset:       s.fset,
            Files:      []string{},
            Types:      []*scanner.TypeInfo{},
        }
        // ... cache and return pkgInfo ...
        return pkgInfo, nil
    }
    ```

2.  **Improved External Module Detection**: The check for whether a package is external to the workspace was made more robust. Instead of only checking against the primary module's root directory, it now checks against the root directories of *all* modules known to the scanner in a workspace context. This ensures that packages from other modules in the same workspace are correctly identified as internal.
