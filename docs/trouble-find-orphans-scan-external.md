# Troubleshooting: `symgo` Triggers Scan of External Packages

This document details an issue where tools built on `go-scan`'s symbolic execution engine (`symgo`), such as `find-orphans`, would unintentionally trigger a source-code scan of packages outside the main workspace. This includes both the Go standard library and third-party modules from the module cache, and would often lead to panics.

## Symptom

When running an analysis, a panic could occur. The stack trace would originate from the `scanner.scanGoFiles` function, and the file paths involved would point to locations within `GOROOT` or the Go module cache.

```
panic: /opt/homebrew/Cellar/go/1.24.3/libexec/src/encoding/json

goroutine 1 [running]:
github.com/podhmo/go-scan/scanner.(*Scanner).scanGoFiles(...)
...
github.com/podhmo/go-scan/scanner.(*FieldType).Resolve(...)
...
github.com/podhmo/go-scan/symgo/evaluator.(*Evaluator).evalGenDecl(...)
...
main.run(...)
```

## Root Cause Analysis

The behavior, while seeming like a scanner bug, was actually triggered by the `symgo` engine's design and its interaction with the scanner.

1.  **Symbolic Execution**: During analysis, `symgo` would encounter an expression involving a type from an external package (e.g., a variable of type `json.Encoder`).
2.  **Type Resolution Trigger**: To understand the type, `symgo` would call the `Resolve()` method on the `scanner.FieldType` representing `json.Encoder`. This happened deep within the evaluator, for example when processing a general declaration (`evalGenDecl`).
3.  **Unintentional Scanner Invocation**: The `FieldType.Resolve` method is designed to provide a full type definition by scanning the source package if necessary. It would therefore invoke the main `goscan.Scanner` instance, asking it to scan the external package (`encoding/json`).
4.  **Incorrect Context**: The `goscan.Scanner` would correctly locate the external package's source files in `GOROOT` or the module cache. However, the internal `scanner.Scanner` instance it used for parsing was configured only with the *user's main module* context. Trying to parse files from `GOROOT` using the main module's root directory led to a context mismatch and a panic.

The core issue was a philosophical one: `symgo` was not intended to perform deep analysis of external packages. The desired behavior was for it to treat such types as opaque placeholders. However, by calling `Resolve()`, it was unintentionally asking the scanner to do a deep, source-level scan, which the scanner was not configured to handle for out-of-workspace packages.

## Solution

The fix was to align the scanner's behavior with `symgo`'s intended design. The `goscan.Scanner.ScanPackageByImport` function was modified to act as a guard, preventing `symgo`'s resolution requests from ever reaching the file-parsing logic for external packages.

1.  **Workspace Check**: The function now begins by checking if the requested package's directory is located within any of the defined workspace modules.
2.  **Block External Scans**: If the package is determined to be external (i.e., not in any workspace module), the function immediately returns a minimal, empty `scanner.PackageInfo` struct.
3.  **Force Placeholder Behavior**: By returning an empty package, the scanner signals to `symgo` that no type definitions are available. `symgo` then correctly falls back to treating the external type as an opaque, symbolic placeholder, which was the original intent.

This change prevents the panic and aligns the library's behavior with its design goals, without requiring complex changes to the `symgo` engine itself.

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

// If not in the workspace, do not scan its source files. Return minimal info.
if !isWorkspaceModule {
    slog.DebugContext(ctx, "ScanPackageByImport detected external package, returning minimal info", "importPath", importPath, "path", pkgDirAbs)
    pkgInfo := &scanner.PackageInfo{
        // ... minimal fields ...
    }
    // ... cache and return pkgInfo ...
    return pkgInfo, nil
}

// ... continue with normal scanning ONLY for workspace packages ...
```
