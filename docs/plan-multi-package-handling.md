# Refined Plan for Multi-Package Type Resolution

## 1. Overview

The current implementation of `go-scan` is primarily designed for single-package scanning. To support advanced code generation tools like `examples/convert`, which must operate across package boundaries (e.g., converting a type from a `source` package to a type in a `destination` package), the core type resolution mechanism must be enhanced.

This plan outlines a refined approach to solve this problem by introducing a robust, lazy-loading type resolution system. The central `goscan.Scanner` will act as a "unified package context," while the actual scanning of external packages will be deferred until a type from that package is explicitly requested. This respects the library's design principle of lazy, on-demand parsing and simplifies the API for consumers.

## 2. Core Concepts

### 2.1. The Unified Package Context (`goscan.Scanner`)

The main `goscan.Scanner` instance will be treated as the single source of truth. It will maintain a cache of all packages that have been scanned during its lifecycle.

-   **Role**: To manage the global `packageCache` (`map[string]*scanner.PackageInfo`).
-   **Functionality**: All package scanning operations, like `ScanPackageByImport`, will be idempotent. They will first check the cache and only perform a file scan if the package has not been previously loaded.

### 2.2. Lazy Resolution via `FieldType.Resolve()`

This is the cornerstone of the new design. Instead of pre-scanning all possible dependencies, the system will automatically scan a new package only when a type within it is being resolved.

The workflow is as follows:
1.  When the scanner parses a source file, it encounters type identifiers from other packages (e.g., `models.User` in a field definition).
2.  It creates a `scanner.FieldType` struct for this identifier. This struct is initially **unresolved**. It contains the import path (`"path/to/models"`) and the type name (`"User"`), but not the full `TypeInfo`.
3.  The consumer of the library (e.g., the `convert` tool) calls the `Resolve()` method on this `scanner.FieldType`.
4.  The `Resolve()` method triggers the lazy-loading mechanism:
    a. It requests the full `PackageInfo` for the required import path from the parent `goscan.Scanner`.
    b. The `goscan.Scanner` checks its central cache. If the package is not present, it locates the package on disk, scans its files, and stores the new `PackageInfo` in the cache.
    c. Once the `PackageInfo` is available, the `Resolve()` method looks up the type name (`"User"`) within that package's `Types` map.
    d. It returns the complete `TypeInfo` for `models.User`.

This process ensures that no package is scanned until it is absolutely necessary.

### 2.3. Parent-Child Scanner Relationship

To implement lazy resolution, we must establish a clear relationship between the global scanner and the internal, per-package scanner.

-   **`goscan.Scanner` (Parent)**: The public-facing scanner that manages the unified context (the package cache) and orchestrates high-level scanning operations.
-   **`scanner.Scanner` (Child)**: The internal worker responsible for parsing the AST of a *single* package. It should not be aware of other packages.
    -   Crucially, the `scanner.FieldType` instances created by the child scanner must retain a reference back to the parent `goscan.Scanner`. This allows the `Resolve()` method to access the global cache and trigger new scans.

### 2.4. Handling Annotations and Markers

A key requirement for code generators is to read annotations (e.g., `// @deriving...`) on resolved types. The lazy-loading model seamlessly supports this. When `FieldType.Resolve()` is called for a type in an external package, that package's source files are fully parsed. The resulting `TypeInfo` will contain all associated documentation and comments, making annotations available for inspection immediately after resolution.

## 3. Implementation Plan

### 3.1. `goscan.Scanner` and `scanner.Scanner` Modifications

1.  **Pass Parent Scanner Reference**:
    -   The `goscan.Scanner` will pass a reference to itself (`s`) to the internal `scanner.Scanner` when it initiates a scan.
    -   This reference will be stored on the `scanner.Scanner` and subsequently attached to all `scanner.FieldType` instances it creates.

    ```go
    // In goscan.go
    func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string) (*scanner.PackageInfo, error) {
        // ...
        // The internal scanner now receives a reference to the parent `goscan.Scanner`.
        pkgInfo, err := s.scanner.ScanFiles(ctx, filesToParse, pkgDirAbs, s)
        // ...
    }

    // In scanner/scanner.go
    // The FieldType needs a way to call back to the parent scanner to resolve itself.
    type FieldType struct {
        // ... existing fields
        parent *goscan.Scanner // Reference to the top-level scanner
    }

    // The Resolve method will use this parent reference.
    func (ft *FieldType) Resolve() (*TypeInfo, error) {
        if ft.IsResolved() {
            return ft.resolved, nil
        }
        if ft.parent == nil {
            return nil, errors.New("cannot resolve type: parent scanner is not available")
        }

        // Use the parent to lazily scan the package and find the type.
        pkgInfo, err := ft.parent.ScanPackageByImport(context.Background(), ft.PkgPath)
        if err != nil {
            return nil, fmt.Errorf("failed to scan package %q for type %q: %w", ft.PkgPath, ft.Name, err)
        }

        // Find the specific type definition within the newly scanned package.
        def, ok := pkgInfo.Types[ft.Name]
        if !ok {
            return nil, fmt.Errorf("type %q not found in package %q", ft.Name, ft.PkgPath)
        }
        ft.resolved = def
        return def, nil
    }
    ```

### 3.2. Refactoring `examples/convert`

The `convert` tool will be refactored to demonstrate the power and simplicity of the new approach.

1.  **Simplify `main.go`**:
    -   The `main` function will only be responsible for scanning the initial source package specified by the `-pkg` flag. It will no longer need to manually pre-scan destination packages.

2.  **Enhance `parser/parser.go`**:
    -   The `Parse` function will no longer need to manually trigger scans.
    -   When it parses an annotation like `@derivingconvert(pkg.DstType)`, it will resolve the `DstType`'s `TypeInfo` by simply calling the `Resolve()` method on its `FieldType`. The lazy-loading mechanism will handle the scanning of `pkg` automatically.

    ```go
    // in examples/convert/parser/parser.go
    func Parse(ctx context.Context, s *goscan.Scanner, scannedPkg *scanner.PackageInfo) (*Info, error) {
        // ...
        for _, t := range scannedPkg.Types {
            // After parsing the annotation and finding the destination type identifier...
            // e.g., we have a `FieldType` for the destination type.

            // Simply resolve it. The scanner handles the lazy-loading.
            dstTypeInfo, err := destinationFieldType.Resolve()
            if err != nil {
                return nil, fmt.Errorf("failed to resolve destination type: %w", err)
            }

            // Now dstTypeInfo is fully populated, including its documentation and fields.
            // We can inspect it for further annotations or properties.
        }
        // ...
    }
    ```

### 3.3. `generator` Improvements

The `generator` will use the `TypeInfo.PkgPath` to correctly qualify type names from different packages, leveraging the `ImportManager` as originally planned. Since `Resolve()` provides the full `TypeInfo`, the generator has all the information it needs.

## 4. Testing Plan

-   **Unit Tests for `go-scan`**:
    -   Create a test `TestFieldType_Resolve_CrossPackage` that verifies `FieldType.Resolve()` can successfully scan and resolve a type from a separate, uncached package.
    -   Add a test to ensure that calling `Resolve()` multiple times on the same type only triggers one scan.
-   **Integration Tests for `examples/convert`**:
    -   Update the integration tests to use a scenario where the `source` and `destination` types reside in different packages.
    -   Verify that the generated code compiles and includes the correct `import` statement for the destination package.
    -   Assert that the generated code performs the conversion correctly.
