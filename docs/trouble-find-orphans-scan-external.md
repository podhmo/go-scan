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

The bug was addressed with a single, robust fix in `goscan.Scanner.ScanPackageByImport`.

The function now determines if a requested package is part of the defined workspace (which can include multiple modules) at the beginning of the function. If the package's resolved directory is not found within any of the workspace module roots, it is considered "external". This applies to both standard library packages (in `GOROOT`) and third-party dependencies (in the module cache).

For any such external package, the scanner immediately returns a minimal, empty `PackageInfo` struct without attempting to read or parse its source files. This prevents the panic by ensuring the scanner never operates on files outside its configured workspace context. It also correctly allows the symbolic execution engine to treat all external types as opaque, which is sufficient for its analysis needs.

This approach is more general and correctly handles all out-of-workspace packages, not just the standard library.

```go
// In goscan.Scanner.ScanPackageByImport...

// Determine if the package is part of the workspace.
isWorkspaceModule := false
if s.IsWorkspace() {
    for _, modRoot := range s.ModuleRoots() {
        if strings.HasPrefix(pkgDirAbs, modRoot) {
            isWorkspaceModule = true
            break
        }
    }
} else {
    isWorkspaceModule = strings.HasPrefix(pkgDirAbs, s.RootDir())
}

// If not in the workspace, do not scan its source files.
if !isWorkspaceModule {
    slog.DebugContext(ctx, "ScanPackageByImport detected external package, returning minimal info", "importPath", importPath, "path", pkgDirAbs)
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

// ... continue with normal scanning for workspace packages ...
```
