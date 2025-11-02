# Troubleshooting: `goinspect` Fails on Standard Library Packages

## Problem

The `goinspect` tool failed to analyze standard library packages like `net/http` or `errors`. When run with a standard library package as an argument, it produced an error indicating it could not find the package on the file system.

```bash
$ goinspect --pkg net/http
# Expected: Output call graph for net/http
# Actual: Error: failed to scan pattern "net/http": could not stat pattern "net/http" ... no such file or directory
```

This indicated that `goinspect` was incorrectly treating the import path as a relative file path.

## Investigation

1.  **Initial Hypothesis (Incorrect):** My first assumption was that the underlying `goscan` library was missing a feature to resolve standard library packages, likely by not invoking `go list`.

2.  **Correction & Deeper Analysis:** I was reminded that `AGENTS.md` explicitly forbids the use of `go list` to avoid creating a dependency on the Go command-line tool. This forced a more careful review of the `goscan` library's architecture.

3.  **Root Cause Analysis:** The investigation revealed two distinct issues:
    *   **Configuration Issue in `goinspect`:** The `goinspect` tool's `main.go` was initializing the `goscan.Scanner` without the `goscan.WithGoModuleResolver()` option. This option is crucial as it configures the scanner's `locator` to understand the Go module environment, including the location of `GOROOT` (for standard library) and the module cache (for external dependencies), without shelling out to `go list`. Without it, the scanner defaulted to a basic file-system-only resolver.
    *   **Brittleness in `goscan.Scanner.Scan`:** The `goscan.Scanner.Scan` method had a logic flaw. For patterns that did not contain `...`, it would *only* attempt to resolve the pattern as a file system path. If `os.Stat` failed, it would immediately return an error, without attempting to treat the pattern as a potential import path. This made the library less robust.

## Solution

A two-part fix was implemented:

1.  **Fix `goinspect`:** The call to `goscan.New` in `tools/goinspect/main.go` was updated to include the `goscan.WithGoModuleResolver()` option.

    ```go
    // tools/goinspect/main.go
    s, err := goscan.New(
        goscan.WithLogger(logger),
        goscan.WithGoModuleResolver(), // This was added
    )
    ```

2.  **Fix `goscan` Library:** The `goscan.Scanner.Scan` method in `goscan.go` was refactored. It now attempts to resolve a pattern as a file path first. If that fails (i.e., `os.Stat` returns an error), it gracefully falls back to treating the pattern as an import path and attempts to resolve it using `ScanPackageFromImportPath`.

    ```go
    // goscan.go (simplified logic)
    info, statErr := os.Stat(absPath)
    if statErr == nil { // Path exists on filesystem
        // ... scan as file or directory
    } else { // Path does not exist, assume it's an import path
        pkg, err = s.ScanPackageFromImportPath(ctx, pattern)
    }
    ```

## Outcome

With these changes, `goinspect` can now correctly analyze standard library packages. The underlying `goscan` library is also more robust, as it can transparently handle both file paths and import paths in its main `Scan` method. All existing tests pass, and a new test case for the `errors` package confirms the fix.