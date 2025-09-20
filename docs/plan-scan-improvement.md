> [!NOTE]
> This feature has been implemented.

# Proposal for Improving File and Directory Scanning

This document outlines a proposal to improve the scanning behavior of tools built with the `go-scan` library, such as `examples/derivingjson` and `examples/derivingbind`. The goal is to align the scanning logic with the library's intended support for lazy, on-demand type resolution.

## Current Behavior

Currently, the example tools treat all command-line arguments as file paths, but immediately convert them to directory paths. They then proceed to scan the entire package within that directory using `go-scan.ScanPackageFromFilePath()`. This approach has two main drawbacks:

1.  **Inefficiency**: If a user specifies a single file, the tool scans all `.go` files in its directory, which is unnecessary and inefficient.
2.  **Inconsistency**: The behavior is inconsistent with the library's core feature of lazy type resolution. The library is designed to parse only what is necessary and resolve dependencies as they are encountered.

## Proposed Behavior

The proposed change is to make the scanning behavior dependent on the arguments provided to the command-line interface. The arguments should be processed as follows:

1.  **Group by Package**: First, all file path arguments should be grouped by their parent directory. Each directory represents a package.

2.  **File-based Scanning**: For each package group, the tool should:
    -   Scan **only the specified files** within that package using a single call to `go-scan.ScanFiles()`.
    -   Rely on the `FieldType.Resolve()` method to lazily scan any imported packages for type definitions as needed. This handles single-file and multi-file cases efficiently.

3.  **Directory-based Scanning**: If an argument is a directory, the tool should:
    -   Scan **all `.go` files** within that directory (excluding `_test.go` files) using `go-scan.ScanPackageFromFilePath()`.
    -   This preserves the existing functionality for users who want to process an entire package at once.

This approach provides users with more granular control over the scanning process, leading to better performance and a more intuitive user experience.

## Implementation Details

The `go-scan` library already provides the necessary APIs to implement this behavior. The following changes are recommended for the `main.go` file of the example tools:

### Grouping and Scanning Logic

The `main.go` file of the example tools should be modified to first classify all command-line arguments into two categories: directories to be scanned fully, and files to be grouped by package.

1.  Iterate through the arguments.
2.  If an argument is a directory, add it to a list of directories to be scanned with `ScanPackageFromFilePath()`.
3.  If an argument is a file, add it to a map where keys are directory paths (packages) and values are lists of file paths.
4.  Process the directories and the file groups.

### Example Implementation

Here is a conceptual code snippet illustrating the improved logic:

```go
// In main() function of the example tool

gscn, err := goscan.New(".") // Create scanner once
if err != nil {
    // handle error
    os.Exit(1)
}

filesByPackage := make(map[string][]string)
dirsToScan := []string{}

// 1. Classify arguments
for _, path := range os.Args[1:] {
    stat, err := os.Stat(path)
    if err != nil {
        // handle error
        continue
    }
    if stat.IsDir() {
        dirsToScan = append(dirsToScan, path)
    } else if strings.HasSuffix(path, ".go") {
        pkgDir := filepath.Dir(path)
        filesByPackage[pkgDir] = append(filesByPackage[pkgDir], path)
    }
}

// 2. Process directories
for _, dirPath := range dirsToScan {
    slog.InfoContext(ctx, "Scanning directory", "path", dirPath)
    pkgInfo, err := gscn.ScanPackageFromFilePath(ctx, dirPath)
    if err != nil {
        // handle error
        continue
    }
    // ... proceed with processing the pkgInfo ...
}

// 3. Process file groups
for pkgDir, filePaths := range filesByPackage {
    slog.InfoContext(ctx, "Scanning files in package", "package", pkgDir, "files", filePaths)
    pkgInfo, err := gscn.ScanFiles(ctx, filePaths, pkgDir)
    if err != nil {
        // handle error
        continue
    }
    // ... proceed with processing the pkgInfo ...
}
```

By adopting these changes, the example tools will better demonstrate the power and flexibility of the `go-scan` library, providing a more efficient and logical scanning process.
