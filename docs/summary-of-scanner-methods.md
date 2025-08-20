# A Summary of Go-Scan's Scanner Methods

This document provides a guide to using the `go-scan` scanner, focusing on its methods, use cases, and how to handle common scenarios like external dependencies.

## Architectural Overview: `goscan.Scanner` vs. `scanner.Scanner`

The `go-scan` library is composed of two main scanning components:

1.  **`scanner.Scanner` (Low-Level)**: This is the core parsing engine located in the `scanner` package. Its primary job is to take a specific list of Go source files and parse them into a detailed `scanner.PackageInfo` struct. It does not know how to find files or resolve import paths on its own; it simply parses what it is given.

2.  **`goscan.Scanner` (High-Level Facade)**: This is the main, user-facing API located in the root `goscan` package. It acts as a facade, wrapping the low-level `scanner.Scanner` and combining it with a `locator.Locator`. This high-level scanner manages state (like visited files), caching, and provides a rich set of methods for both discovering and parsing packages.

**Recommendation:** Users should almost always use the high-level `goscan.Scanner`. It provides the necessary orchestration to handle real-world codebases with complex dependencies.

---

## Handling Dependencies

A key feature of `go-scan` is its ability to navigate and parse package dependencies.

### External Modules
By default, the scanner only looks for source files within the main module. To make it aware of packages from your Go module cache (`GOMODCACHE`) or `GOROOT`, you must configure it during initialization.

**How to use:** Provide the `goscan.WithGoModuleResolver()` option when creating the scanner. This configures the underlying `locator` to search in standard Go environment locations for external dependencies.

```go
// This scanner can now resolve import paths like "fmt" or "rsc.io/quote".
s, err := goscan.New(
    goscan.WithGoModuleResolver(),
)
if err != nil {
    // handle error
}

// This call will succeed because the resolver can find the "fmt" package in GOROOT.
pkgInfo, err := s.ScanPackageByImport(context.Background(), "fmt")
```

### Delayed (Lazy) Loading of Imports
Sometimes, you need to explore a project's dependency graph without incurring the cost of fully parsing every single file. For example, you might just want to build a dependency tree diagram.

The `go-scan` library supports this "delayed loading" pattern by separating dependency discovery from full parsing. You can first discover all the import paths using lightweight methods, and then decide which packages to fully parse later.

**How to use:** Use methods from the "Discovery & Dependency Analysis" group, such as `goscan.Scanner.Walk`, which only scans import statements.

```go
// The visitor will receive a lightweight PackageImports object for each dependency.
err := s.Walk(ctx, "github.com/your/module/pkg/a", &MyVisitor{})

// Inside the visitor, you can decide whether to perform a full scan.
func (v *MyVisitor) Visit(pkg *goscan.PackageImports) ([]string, error) {
    fmt.Println("Discovered package:", pkg.ImportPath)

    // Maybe we only care about fully parsing packages with a certain name.
    if strings.Contains(pkg.ImportPath, "important") {
        // Now we trigger a full, heavyweight parse for just this one package.
        fullInfo, err := v.scanner.ScanPackageByImport(context.Background(), pkg.ImportPath)
        // ... do something with fullInfo ...
    }

    // Return the imports to continue the walk.
    return pkg.Imports, nil
}
```

### Manual Overrides for External Types
For types that cannot be parsed (e.g., those defined in C files and exposed via CGo, or complex types you wish to mock), you can provide a manual definition.

**How to use:** Use the `goscan.WithExternalTypeOverrides()` option.

---

## Method Groups

The methods on `goscan.Scanner` can be divided into two main groups, based on their use case and performance profile.

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
*   Interpreters and symbolic execution engines (like `minigo` and `symgo`).

**Methods:**
*   **`ScanPackageByImport(ctx, importPath)`**: The main workhorse for this group. It takes an import path, uses the locator to find the package's directory, and performs a full parse on its files. Results are cached for efficiency.
    ```go
    // Get full details for the "net/http" package.
    pkg, err := scanner.ScanPackageByImport(ctx, "net/http")
    if err == nil {
        for _, t := range pkg.Types {
            fmt.Println("Found type:", t.Name)
        }
    }
    ```
*   **`ScanFiles(ctx, filePaths)`**: Performs a full parse on a specific list of files. All files must belong to the same package. This is useful for tools that operate on a subset of files.
*   **`ScanPackage(ctx, pkgPath)`**: Similar to `ScanPackageByImport`, but takes a file system directory path instead of an import path.
*   **`FindSymbolDefinitionLocation(ctx, symbolFullName)`**: Finds the exact file path where a symbol (e.g., `"fmt.Println"`) is defined. This may trigger a full scan of the package if it hasn't been scanned already.
*   **`ResolveType(ctx, fieldType)`**: A lower-level utility that resolves a `FieldType` into a `TypeInfo`, performing recursive resolution if necessary. This is used after an initial scan to dig deeper into type structures.
*   **`TypeInfoFromExpr(ctx, ...)`**: A helper to parse an `ast.Expr` into a `FieldType`, useful for dynamic type analysis.
*   **`ListExportedSymbols(ctx, pkgPath)`**: Scans a package and returns a simple list of its exported symbol names.
*   **`FindSymbolInPackage(ctx, importPath, symbolName)`**: Scans files in a package one-by-one until a specific symbol is found. This can be more efficient than `ScanPackageByImport` if you only need one symbol from a large package.

---

## Usage by Package

This section summarizes which scanner methods are used by the key commands and examples in this repository, illustrating the patterns described above.

*   **`examples/deps-walk`**:
    *   **Summary**: A dependency graph visualization tool. It is a classic example of a **Group 1 (Lightweight)** user.
    *   **Methods Used**: `Walk`, `FindImportersAggressively`, `BuildReverseDependencyMap`. It relies entirely on the lightweight `ScanPackageImports` (called by `Walk`) to discover dependencies without parsing full source code.

*   **`minigo`**:
    *   **Summary**: A Go interpreter. It is a primary example of a **Group 2 (Heavyweight)** user. The interpreter needs full type information to evaluate code correctly.
    *   **Methods Used**: `ScanPackageByImport` is the key method, called by the evaluator whenever it encounters an `import` statement. It needs the full `PackageInfo` to access the types, functions, and constants of the imported package.

*   **`symgo`**:
    *   **Summary**: A symbolic execution engine. Like `minigo`, it is a **Group 2 (Heavyweight)** user. It needs to understand the precise structure of types and functions to perform its analysis.
    *   **Methods Used**: Interestingly, `symgo` is architected to use the low-level `scanner.Scanner` directly. Its evaluator calls `ScanPackageByImport`, `BuildImportLookup`, and `TypeInfoFromExpr` to get the detailed information it needs. This demonstrates the same *need* for heavyweight analysis, even with a slightly different architecture.

*   **`examples/convert`**, **`examples/derivingjson`**, **`examples/derivingbind`**, **`examples/deriving-all`**:
    *   **Summary**: These are all code generation tools. They are canonical examples of **Group 2 (Heavyweight)** users.
    *   **Methods Used**: They follow a common pattern:
        1.  Use `ScanPackageByImport` or `ScanFiles` to get an initial, complete `PackageInfo` of the target package.
        2.  Iterate through the `Types` and `Fields` of the `PackageInfo`.
        3.  Use `ResolveType` (often by calling `field.Type.Resolve()`) to get full details about field types, especially those from other packages.
        4.  Call `ScanPackageByImport` recursively if they need to analyze an imported package (e.g., to check if a type implements a certain interface).
