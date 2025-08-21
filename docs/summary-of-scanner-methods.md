# A Guide to Go-Scan's Package Loading APIs

This document provides a guide to using the `go-scan` library, focusing on its different layers for package discovery and parsing. Understanding these layers helps in choosing the right tool for the job.

## Architectural Overview: Three Layers of Scanning

The `go-scan` library is composed of three main components for package loading and analysis:

1.  **`scanner.Scanner` (Low-Level Engine)**: This is the core parsing engine located in the `scanner` package. Its primary job is to take a specific list of Go source files and parse them into a detailed `scanner.PackageInfo` struct. It does not know how to find files or resolve import paths on its own; it simply parses what it is given.

2.  **`goscan.Scanner` (High-Level Facade)**: This is a powerful, user-facing API in the root `goscan` package. It acts as a facade, wrapping the low-level `scanner.Scanner` and combining it with a `locator.Locator`. This high-level scanner manages state (like visited files), caching, and provides a rich set of methods for both discovering and parsing packages. It is the most flexible and powerful tool for complex analysis tasks.

3.  **`resolver.Resolver` (Simple, Cached Loader)**: This is the simplest, highest-level API for package loading. It wraps a `goscan.Scanner` and provides a single method, `Resolve()`, which handles on-demand, concurrent, cached loading of packages.

**Recommendation:**
*   For applications that need a simple way to load and cache full package information by import path, use the **`resolver.Resolver`**. This is the recommended entry point for most common use cases.
*   For advanced use cases requiring fine-grained control over dependency walking, partial scans, or aggressive discovery strategies, use the **`goscan.Scanner`**.
*   The **`scanner.Scanner`** should rarely be used directly.

---

## The `resolver.Resolver` API

The `resolver` is the easiest way to get started. It abstracts away all the complexity of scanning and caching.

**How to use:** Create a `goscan.Scanner`, then use it to initialize the `resolver.Resolver`.

```go
// 1. Create a standard go-scan Scanner, enabling the module resolver.
scanner, err := goscan.New(goscan.WithGoModuleResolver())
if err != nil {
    log.Fatalf("failed to create scanner: %+v", err)
}

// 2. Create a new Resolver from the scanner.
r := resolver.New(scanner)

// 3. Use the resolver to load package info.
// The first call will scan the "fmt" package.
fmtPkg, err := r.Resolve(context.Background(), "fmt")

// The second call for the same package will hit the cache.
fmtPkgFromCache, err := r.Resolve(context.Background(), "fmt")
```

---

## The `goscan.Scanner` API Method Groups

For more advanced tasks, you can use the methods on `goscan.Scanner` directly. They can be divided into two main groups.

### Group 1: Discovery & Dependency Analysis (Lightweight)

These methods are optimized for speed and are used to understand the relationships *between* packages. They typically work by parsing only the `import` statements in source files, avoiding the cost of parsing function bodies and type definitions. They are ideal for building dependency graphs, finding package importers, etc.

**Use Cases:**
*   Creating dependency visualization tools (like `examples/deps-walk`).
*   Finding all packages that are affected by a change in a specific package.
*   Building dependency trees for analysis.

**Methods:**
*   **`ScanPackageImports(ctx, importPath)`**: The fundamental lightweight method. It scans a package and returns only its name and a list of imported package paths.
*   **`Walk(ctx, rootImportPath, visitor)`**: Performs a dependency graph traversal starting from a root package. It uses `ScanPackageImports` at each step and calls a user-provided `Visitor` to process each discovered package.
*   **`FindImporters(ctx, targetImportPath)`**: Scans the entire module to find all packages that directly import the `targetImportPath`.
*   **`FindImportersAggressively(ctx, targetImportPath)`**: A faster version of `FindImporters` that uses `git grep` to quickly identify potential importers before verifying them. Requires `git` to be installed.
*   **`BuildReverseDependencyMap(ctx)`**: Scans the entire module once to build a complete map where keys are import paths and values are lists of packages that import them.

### Group 2: Full Parsing & Code Analysis (Heavyweight)

These methods perform a comprehensive parse of Go source files. They build a detailed `PackageInfo` object that includes all top-level declarations: types (structs, interfaces, aliases), functions, methods, constants, and variables. This is necessary for any tool that needs to understand the actual code within a package.

**Use Cases:**
*   Code generation.
*   Static analysis tools and linters.
*   Interpreters and symbolic execution engines.

**Methods:**
*   **`ScanPackageByImport(ctx, importPath)`**: The main workhorse for this group. It takes an import path, uses the locator to find the package's directory, and performs a full parse on its files. Results are cached for efficiency. **Note:** For most applications, using `resolver.Resolve()` is a simpler way to access this functionality.
*   **`ScanFiles(ctx, filePaths)`**: Performs a full parse on a specific list of files. All files must belong to the same package. This is useful for tools that operate on a subset of files.
*   **`ScanPackage(ctx, pkgPath)`**: Similar to `ScanPackageByImport`, but takes a file system directory path instead of an import path.
*   **`FindSymbolDefinitionLocation(ctx, symbolFullName)`**: Finds the exact file path where a symbol (e.g., `"fmt.Println"`) is defined. This may trigger a full scan of the package if it hasn't been scanned already.
*   **`ResolveType(ctx, fieldType)`**: A lower-level utility that resolves a `FieldType` into a `TypeInfo`, performing recursive resolution if necessary. This is used after an initial scan to dig deeper into type structures.
*   **`TypeInfoFromExpr(ctx, ...)`**: A helper to parse an `ast.Expr` into a `FieldType`, useful for dynamic type analysis.
*   **`ListExportedSymbols(ctx, pkgPath)`**: Scans a package and returns a simple list of its exported symbol names.
*   **`FindSymbolInPackage(ctx, importPath, symbolName)`**: Scans files in a package one-by-one until a specific symbol is found. This can be more efficient than `ScanPackageByImport` if you only need one symbol from a large package, but it is a more complex API. The `resolver.Resolver` provides a simpler, unified loading pattern.

---

## Usage by Package

This section summarizes which scanner methods are used by the key commands and examples in this repository, illustrating the patterns described above.

*   **`examples/deps-walk`**:
    *   **Summary**: A dependency graph visualization tool. It is a classic example of a **Group 1 (Lightweight)** user.
    *   **Methods Used**: `Walk`, `FindImportersAggressively`, `BuildReverseDependencyMap`. It relies entirely on the lightweight `ScanPackageImports` (called by `Walk`) to discover dependencies without parsing full source code.

*   **`minigo` & `symgo`**:
    *   **Summary**: A Go interpreter and a symbolic execution engine. They are primary examples of **Group 2 (Heavyweight)** users.
    *   **Architecture**: Both engines now use the new **`resolver.Resolver`** to handle on-demand, lazy loading of imported packages. When their respective evaluators encounter an import, they use `resolver.Resolve()` to get the complete `PackageInfo`. This unified approach simplifies the code and ensures consistent behavior. The resolver, in turn, uses the heavyweight `ScanPackageByImport` method under the hood to perform the actual scanning and caching. This architecture provides the engines with a consistent, complete view of each package needed for resolving transitive dependencies during evaluation.

*   **`examples/convert`**, **`examples/derivingjson`**, **`examples/derivingbind`**, **`examples/deriving-all`**:
    *   **Summary**: These are all code generation tools. They are canonical examples of **Group 2 (Heavyweight)** users.
    *   **Methods Used**: They follow a common pattern:
        1.  Use `ScanPackageByImport` or `ScanFiles` to get an initial, complete `PackageInfo` of the target package.
        2.  Iterate through the `Types` and `Fields` of the `PackageInfo`.
        3.  Use `ResolveType` (often by calling `field.Type.Resolve()`) to get full details about field types, especially those from other packages.
        4.  Call `ScanPackageByImport` recursively if they need to analyze an imported package (e.g., to check if a type implements a certain interface).
