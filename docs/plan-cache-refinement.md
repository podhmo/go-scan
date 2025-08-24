# Plan: Cache Refinement for `go-scan`

**Status**: Proposed

## 1. Context

`go-scan`'s core scanning functionality is designed to be efficient by caching parsed data and avoiding redundant work. However, while implementing the `...` wildcard feature, a fundamental issue in the current caching strategy was exposed.

A test for the `symgo` symbolic execution engine began to fail with a `nil pointer dereference`. The root cause was that the scanner was returning a "shallow" `PackageInfo` object from its cache—one that contained import information but lacked the full Abstract Syntax Trees (ASTs). The `symgo` engine, which requires a "deep" scan with full ASTs, received this shallow object and panicked when it tried to access the missing ASTs.

This incident reveals an inconsistency in how cached data is stored and retrieved, leading to unpredictable behavior for different consumers of the scanner. This plan outlines the problem in detail and proposes a solution to make the caching mechanism more robust and predictable.

## 2. Current Caching Mechanism

The `goscan.Scanner` currently employs two primary caching mechanisms to optimize performance:

1.  **Package Cache (`packageCache`)**: An in-memory map of type `map[string]*Package`.
    -   **Key**: The import path of a package (e.g., `"fmt"`).
    -   **Value**: A `*goscan.Package` (`*scanner.PackageInfo`) object containing the information scanned for that package.
    -   **Problem**: The `PackageInfo` object is mutable and its completeness is inconsistent. A shallow scan (e.g., for dependency analysis) might populate it with only import data, while a deep scan (e.g., for `symgo`) requires it to be populated with full `AstFiles`. The cache stores whatever was generated last, without context of its "depth".

2.  **Visited Files (`visitedFiles`)**: An in-memory set of type `map[string]struct{}`.
    -   **Key**: The absolute file path of a Go source file.
    -   **Function**: This set tracks which files have already been parsed. Core scanning methods like `ScanPackage` and `ScanFiles` check this set to avoid re-parsing the content of a file that has already been processed in any capacity.
    -   **Problem**: This is the core of the issue. A file can be marked as "visited" during a shallow scan where its AST was not needed and therefore not stored. When a subsequent request needs the AST from that same file (a deep scan), the `visitedFiles` check prevents the file from being re-parsed, and the AST is never loaded.

This creates a race condition based on the order of operations. If the first operation on a package is a shallow scan, the cache becomes "poisoned" with an incomplete `PackageInfo`, which then causes failures in subsequent deep scan operations.

## 3. Proposed Solution

The goal is to create a caching system where the results of a scan are predictable and consumers can reliably get the level of detail they need (e.g., with or without ASTs) without introducing side effects for other consumers.

### Proposal: Explicit Scan Modes and Layered Caching (Recommended)

This proposal introduces the concept of "scan depth" directly into the API and caching logic.

#### a. Introduce `ScanMode`

We will define an enumeration to represent the desired depth of a scan.

```go
type ScanMode int

const (
    // ScanModeImportsOnly retrieves only the package name and import declarations.
    // This is the fastest mode, suitable for dependency graph analysis.
    ScanModeImportsOnly ScanMode = iota

    // ScanModeFull parses the entire source code, including function bodies and expressions,
    // populating the full AST. This is required for tools like `symgo`.
    ScanModeFull
)
```

#### b. Update Caching Logic

The caching mechanism will be updated to be aware of `ScanMode`.

1.  **`packageCache`**: The cache will now store different versions of a `PackageInfo` based on the mode. The simplest way is to change the key or have a nested map.
    -   `packageCache map[string]map[ScanMode]*Package`
    -   When a scan is requested for `(path, mode)`, the cache is checked for that specific `(path, mode)` entry.
    -   If a `ScanModeFull` is requested and only a `ScanModeImportsOnly` entry exists, the scanner will perform a full scan and store a new, separate entry for `ScanModeFull`.

2.  **`visitedFiles`**: This set will be removed. The `packageCache` itself becomes the source of truth for what has been scanned and at what depth. A request to scan a package will first check the cache for the desired `ScanMode`. If a sufficient entry exists, it's returned immediately. If not, a new scan is performed.

#### c. Update Scanner APIs

Public-facing and key internal methods will be updated to accept a `ScanMode`.

-   `func (s *Scanner) Scan(patterns ...string) ([]*Package, error)` will be changed to `func (s *Scanner) Scan(mode ScanMode, patterns ...string) ([]*Package, error)`.
-   Methods like `ScanPackage` and `ScanPackageByImport` will also accept the `ScanMode`.
-   The default mode for legacy calls or simple tools could be `ScanModeFull` to ensure correctness, or `ScanModeImportsOnly` for performance, to be decided. For `symgo`, `ScanModeFull` would be explicitly requested.

### 4. Implementation Plan

1.  **Define `ScanMode`**: Add the `ScanMode` enum in `scanner/models.go` or a similar central location.
2.  **Refactor `Scanner.packageCache`**: Change its type to `map[string]map[ScanMode]*Package` and update all access patterns.
3.  **Remove `Scanner.visitedFiles`**: Remove the field and all associated logic that checks it. The decision to parse will now be based solely on the state of the `packageCache`.
4.  **Update Method Signatures**:
    -   Add the `mode ScanMode` parameter to `Scan`, `ScanPackage`, `ScanPackageByImport`, `ScanFiles`, etc.
    -   Update the internal logic of these methods to check the cache using the mode, and to perform the correct depth of parsing based on the mode. For example, `ScanPackageByImport` would no longer need complex logic to decide what to parse; it would be dictated by the `mode` and the cache state.
5.  **Update Call Sites**:
    -   Trace all usages of the modified methods and provide the correct `ScanMode`.
    -   The `symgo` tests will be modified to call `s.Scan(goscan.ScanModeFull, dir)`.
    -   `ModuleWalker`, which only needs imports, will be refactored to always request `ScanModeImportsOnly`.
6.  **Testing**: Add new tests specifically for the caching logic to verify:
    -   A `ScanModeFull` request after a `ScanModeImportsOnly` request correctly triggers a new parse and returns a complete `PackageInfo` with ASTs.
    -   A `ScanModeImportsOnly` request after a `ScanModeFull` request correctly returns the existing (and sufficient) full data from the cache without re-parsing.

## 5. Conclusion

Adopting an explicit `ScanMode` makes the scanner's behavior predictable and robust. It resolves the fundamental issue of cache inconsistency and provides a clear contract to consumers of the library. While it involves a significant refactoring, it will lead to a more maintainable and reliable system in the long run. The temporary fix applied to the `symgo` tests should be reverted once this plan is implemented.
