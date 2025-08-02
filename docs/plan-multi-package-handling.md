> [!NOTE]
> This feature has been implemented.

# Plan for On-Demand, Multi-Package AST Scanning

## 1. Overview

The current implementation of `go-scan` is primarily designed for single-package scanning. To support advanced code generation tools like `examples/convert`, which must operate across package boundaries (e.g., converting a type from a `source` package to a type in a `destination` package), the mechanism for finding type definitions must be enhanced.

This plan outlines an approach to solve this by creating a robust, lazy-loading **AST scanning system**. This is not a full "type resolution system" like in a compiler; rather, its goal is to **grasp partial type information as much as possible, on-demand**. The central `goscan.Scanner` will act as a "unified package context," while the actual scanning of external packages will be deferred until a type definition from that package is explicitly requested. This respects the library's design principle of lightweight, lazy parsing.

## 2. Core Concepts

### 2.1. The Unified Package Context (`goscan.Scanner`)

The main `goscan.Scanner` instance will be treated as the single source of truth. It will maintain a cache of all packages that have been scanned during its lifecycle.

-   **Role**: To manage the global `packageCache` (`map[string]*scanner.PackageInfo`).
-   **Functionality**: All package scanning operations, like `ScanPackageByImport`, will be idempotent. They will first check the cache and only perform a file scan if the package has not been previously loaded.

### 2.2. Lazy Scanning via `FieldType.Resolve()`

This is the cornerstone of the design. The system will automatically scan a new package only when a type definition from within it is needed. The `Resolve()` method name is a practical shorthand for this "on-demand scan and find" operation.

The workflow is as follows:
1.  When the scanner parses a source file, it encounters type identifiers from other packages (e.g., `models.User` in a field definition).
2.  It creates a `scanner.FieldType` struct for this identifier. This struct initially just holds a reference to a type definition that **has not yet been scanned**. It contains the import path (`"path/to/models"`) and the type name (`"User"`), but not the full `TypeInfo`.
3.  The consumer of the library (e.g., the `convert` tool) calls the `Resolve()` method on this `scanner.FieldType`.
4.  The `Resolve()` method triggers the lazy-loading mechanism:
    a. It requests the `PackageInfo` for the required import path from the parent `goscan.Scanner`.
    b. The `goscan.Scanner` checks its central cache. If the package is not present, it locates the package on disk, **scans its AST**, and stores the new `PackageInfo` in the cache.
    c. Once the `PackageInfo` is available, the `Resolve()` method looks up the type name (`"User"`) within that package's `Types` map.
    d. It returns the `TypeInfo` containing the partial information gathered for `models.User`.

This process ensures that no package is scanned until it is absolutely necessary.

### 2.3. Parent-Child Scanner Relationship

To implement lazy scanning, we must establish a clear relationship between the global scanner and the internal, per-package scanner.

-   **`goscan.Scanner` (Parent)**: The public-facing scanner that manages the unified context (the package cache) and orchestrates high-level scanning operations.
-   **`scanner.Scanner` (Child)**: The internal worker responsible for parsing the AST of a *single* package.
    -   Crucially, the `scanner.FieldType` instances created by the child scanner must retain a reference back to the parent `goscan.Scanner`. This allows the `Resolve()` method to access the global cache and trigger new scans.

## 3. Implementation Plan

### 3.1. `goscan.Scanner` and `scanner.Scanner` Modifications

1.  **Pass Parent Scanner Reference**:
    -   The `goscan.Scanner` will pass a reference to itself (`s`) to the internal `scanner.Scanner` when it initiates a scan.
    -   This reference will be stored on the `scanner.Scanner` and subsequently attached to all `scanner.FieldType` instances it creates.

### 3.2. Consumer API Requirements

A key challenge discovered during implementation is that consumers of the `go-scan` library, like the `convert` tool, need to resolve types that are not directly discovered from the AST of a struct field (e.g., type names specified in string literals within annotations).

The scanner automatically creates resolvable `FieldType` instances for types it finds in the source code. However, there is no public API to create a new, resolvable `FieldType` from scratch using a string representation of a type (e.g., `"destination.User"`).

To solve this, the following modifications to the core library are necessary:
-   **Export `FieldType` Fields:** The internal fields required for resolution (`resolver`, `fullImportPath`, `typeName`) must be exported (`Resolver`, `FullImportPath`, `TypeName`). This allows a consumer to manually construct a `FieldType`, populate it with the necessary information (the resolver from the parent `goscan.Scanner` and the parsed type details), and then call the public `ResolveType` method on the scanner.

This change is critical for enabling tools that derive behavior from metadata outside of the Go's strong type system, such as from comments or struct tags.

### 3.3. Refactoring `examples/convert`

The `convert` tool will be refactored to demonstrate the power and simplicity of the new approach by relying entirely on the recursive `Resolve()` method.

### 3.3. `generator` Improvements

The `generator` will use the `TypeInfo.PkgPath` to correctly qualify type names from different packages, leveraging the `ImportManager`. Since `Resolve()` provides the necessary `TypeInfo`, the generator has all the information it needs.

### 3.4. Example Use Case: Nested Cross-Package Scanning

To make the process concrete, consider how `examples/convert` would handle a nested conversion.

**The Types:**

```go
// in package: path/to/source
package source
import "path/to/models"

// @derivingconvert(destination.User)
type User struct {
    ID      string
    Profile models.Profile // From another package
}

// in package: path/to/models
package models
type Profile struct {
    Name string
    Age  int
}

// in package: path/to/destination
package destination
import "path/to/dmodels"

type User struct {
    ID      string
    Profile dmodels.Profile // From yet another package
}

// in package: path/to/dmodels
package dmodels
type Profile struct {
    Name string
    Age  int
}
```

**The Process:**

1.  **Initial Scan**: The `convert` tool starts by scanning the `source` package. It finds `source.User` and its `@derivingconvert(destination.User)` annotation.

2.  **First `Resolve()` Call**: To understand the conversion target, the tool calls `Resolve()` on the type `destination.User`. This triggers the first on-demand scan, loading the `destination` package. Now, the tool has the top-level `TypeInfo` for both `source.User` and `destination.User`.

3.  **Field-by-Field Analysis**: The generator starts comparing the fields of `source.User` and `destination.User`. It sees the `Profile` field in both.

4.  **Recursive `Resolve()` Calls**: To generate code for the `Profile` field, the generator needs to understand the structure of `models.Profile` and `dmodels.Profile`.
    -   It calls `Resolve()` on the `source.User.Profile` field's type (`models.Profile`). This triggers a **second** on-demand scan, loading the `models` package.
    -   It then calls `Resolve()` on the `destination.User.Profile` field's type (`dmodels.Profile`). This triggers a **third** on-demand scan, loading the `dmodels` package.

5.  **Code Generation**: Now that the `TypeInfo` for both `models.Profile` and `dmodels.Profile` has been loaded, the generator can see they are compatible structs. It can then generate a recursive call to a helper function, like `convertModelsProfileToDmodelsProfile()`, to handle the nested conversion.

This recursive, on-demand scanning is the key to handling complex, multi-package structures in a clean and efficient way.

## 4. Testing Plan

-   **Test Harness Enhancements (`scantest`)**:
    -   To prevent the historical problem of tests failing in temporary directories, `scantest` must be enhanced. It should, by default, automatically search parent directories to locate the project's `go.mod` file and use it as the module root.
    -   It should also provide an option to explicitly specify a module root path, to override the default behavior when needed.
-   **Unit Tests for `go-scan`**:
    -   Create a test `TestFieldType_Resolve_CrossPackage` that verifies `FieldType.Resolve()` can successfully scan and find a type definition from a separate, uncached package.
    -   Add a test to ensure that calling `Resolve()` multiple times on the same type only triggers one scan.
    -   Add a test for the nested scanning scenario described in the use case above.
-   **Integration Tests for `examples/convert`**:
    -   Update the integration tests to use the enhanced `scantest` harness.
    -   The tests must cover a scenario where the `source` and `destination` types reside in different packages, including nested structs from other packages.
    -   Verify that the generated code compiles and includes all necessary `import` statements.
    -   Assert that the generated code performs the conversion correctly.
