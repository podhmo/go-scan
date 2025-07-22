# Proposal for Improving File and Directory Scanning

This document outlines a proposal to improve the scanning behavior of tools built with the `go-scan` library, such as `examples/derivingjson` and `examples/derivingbind`. The goal is to align the scanning logic with the library's intended support for lazy, on-demand type resolution.

## Current Behavior

Currently, the example tools treat all command-line arguments as file paths, but immediately convert them to directory paths. They then proceed to scan the entire package within that directory using `go-scan.ScanPackage()`. This approach has two main drawbacks:

1.  **Inefficiency**: If a user specifies a single file, the tool scans all `.go` files in its directory, which is unnecessary and inefficient.
2.  **Inconsistency**: The behavior is inconsistent with the library's core feature of lazy type resolution. The library is designed to parse only what is necessary and resolve dependencies as they are encountered.

## Proposed Behavior

The proposed change is to make the scanning behavior dependent on the type of path provided as a command-line argument:

1.  **File-based Scanning**: When a user provides a path to a single `.go` file, the tool should:
    -   Scan **only that file** using the `go-scan.ScanFiles()` function.
    -   Rely on the `FieldType.Resolve()` method to lazily scan any imported packages for type definitions as needed.

2.  **Directory-based Scanning**: When a user provides a path to a directory, the tool should:
    -   Scan **all `.go` files** within that directory (excluding `_test.go` files).
    -   This preserves the existing functionality for users who want to process an entire package at once.

This dual approach provides users with more granular control over the scanning process, leading to better performance and a more intuitive user experience.

## Implementation Details

The `go-scan` library already provides the necessary APIs to implement this behavior. The following changes are recommended for the `main.go` file of the example tools:

### Use `go-scan.ScanFiles()` for Single Files

When a file path is detected, the tool should call `ScanFiles()` with a single-element slice containing that file's path.

### Continue Using `go-scan.ScanPackage()` for Directories

When a directory path is detected, the tool should continue to use `ScanPackage()` as it does now.

### Example Implementation

Here is a conceptual code snippet illustrating how the argument parsing and scanning logic in `main.go` could be modified:

```go
// In main() function of the example tool

for _, path := range os.Args[1:] {
    stat, err := os.Stat(path)
    if err != nil {
        // handle error
        continue
    }

    var pkgInfo *scanner.PackageInfo
    var scanErr error

    gscn, err := goscan.New(".") // Assuming scanner is created
    if err != nil {
        // handle error
        continue
    }

    if stat.IsDir() {
        // If it's a directory, scan the whole package
        slog.InfoContext(ctx, "Scanning directory", "path", path)
        pkgInfo, scanErr = gscn.ScanPackage(ctx, path)
    } else if strings.HasSuffix(path, ".go") {
        // If it's a .go file, scan only that file
        slog.InfoContext(ctx, "Scanning file", "path", path)
        pkgDir := filepath.Dir(path)
        // Note: The second argument to ScanFiles is the package directory path,
        // which is needed for context.
        pkgInfo, scanErr = gscn.ScanFiles(ctx, []string{path}, pkgDir)
    } else {
        // handle non-go files or other errors
        continue
    }

    if scanErr != nil {
        // handle scanning error
        continue
    }

    // ... proceed with processing the pkgInfo ...
}
```

By adopting these changes, the example tools will better demonstrate the power and flexibility of the `go-scan` library, providing a more efficient and logical scanning process.
