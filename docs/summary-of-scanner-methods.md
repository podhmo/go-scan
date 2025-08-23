# A Summary of Go-Scan's Scanner Methods

This document provides a guide to using the `go-scan` library, focusing on its two primary components: `ModuleWalker` for dependency analysis and `Scanner` for deep code inspection.

## Architectural Overview: `ModuleWalker` vs. `Scanner`

The `go-scan` library is composed of two main high-level components, each designed for a specific task:

1.  **`goscan.ModuleWalker` (Lightweight Dependency Analysis)**: This component is optimized for speed. Its primary purpose is to walk the dependency graph of a Go module by only parsing `import` statements. It's the ideal tool for understanding relationships *between* packages, building dependency trees, or finding all packages that import a specific target.

2.  **`goscan.Scanner` (Heavyweight Code Analysis)**: This is the main engine for deep code inspection. It performs a comprehensive parse of Go source files, building a detailed `PackageInfo` object that includes all top-level declarations: types (structs, interfaces, aliases), functions, methods, constants, and variables. It's the tool to use for code generation, static analysis, and interpreters.

Both components wrap the low-level **`scanner.Scanner`**, which is the core parsing engine that does the file-level work.

**Recommendation:**
*   Use **`ModuleWalker`** when you need to understand package relationships and dependencies.
*   Use **`Scanner`** when you need to understand the code *inside* the packages.

---
## Handling Dependencies

A key feature of `go-scan` is its ability to navigate and parse package dependencies.

### External Modules
By default, `go-scan` only looks for source files within the main module. To make it aware of packages from your Go module cache (`GOMODCACHE`) or `GOROOT`, you must configure it during initialization.

**How to use:** Provide the `goscan.WithGoModuleResolver()` option to a `Scanner` or `goscan.WithModuleWalkerGoModuleResolver()` to a `ModuleWalker`. This configures the underlying `locator` to search in standard Go environment locations for external dependencies.

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
A powerful pattern is to combine the `ModuleWalker` and `Scanner`. You can first explore a project's dependency graph with the fast `ModuleWalker` without incurring the cost of fully parsing every single file. Then, you can use a `Scanner` to perform a deep parse on only the specific packages you are interested in.

**How to use:**
1. Create a `ModuleWalker` to discover packages.
2. Create a `Scanner` to perform on-demand parsing.
3. Use the `ModuleWalker.Walk` method. In your visitor, call `Scanner.ScanPackageByImport` for the packages that need deep analysis.

```go
// 1. Create both a walker and a scanner.
walker, _ := goscan.NewModuleWalker()
scanner, _ := goscan.New()

// MyVisitor holds a reference to the heavyweight scanner.
type MyVisitor struct {
    scanner *goscan.Scanner
}

// 2. The visitor decides whether to perform a full scan.
func (v *MyVisitor) Visit(pkg *goscan.PackageImports) ([]string, error) {
    fmt.Println("Discovered package with ModuleWalker:", pkg.ImportPath)

    // Maybe we only care about fully parsing packages with a certain name.
    if strings.Contains(pkg.ImportPath, "important") {
        // 3. Trigger a full, heavyweight parse for just this one package.
        fullInfo, err := v.scanner.ScanPackageByImport(context.Background(), pkg.ImportPath)
        // ... do something with fullInfo ...
    }

    // Return the imports to continue the walk.
    return pkg.Imports, nil
}

// 4. Start the walk.
visitor := &MyVisitor{scanner: scanner}
err = walker.Walk(ctx, "github.com/your/module/pkg/a", visitor)

```

### Manual Overrides for External Types
For types that cannot be parsed (e.g., those defined in C files and exposed via CGo, or complex types you wish to mock), you can provide a manual definition to a `Scanner`.

**How to use:** Use the `goscan.WithExternalTypeOverrides()` option when creating a `Scanner`.

---

## Method Groups

The methods in the `go-scan` library can be divided into two main groups, corresponding to the two high-level components.

### Group 1: Discovery & Dependency Analysis (`goscan.ModuleWalker`)

These methods are optimized for speed and are used to understand the relationships *between* packages. They typically work by parsing only the `import` statements in source files, avoiding the cost of parsing function bodies and type definitions.

**How to use:** Create a `ModuleWalker` instance using `goscan.NewModuleWalker()`.

```go
// This walker can now be used to explore the module's dependency graph.
walker, err := goscan.NewModuleWalker(
    goscan.WithModuleWalkerGoModuleResolver(), // Optional: to include external modules
)
if err != nil {
    // handle error
}
```

**Use Cases:**
*   Creating dependency visualization tools (like `examples/deps-walk`).
*   Finding all packages that are affected by a change in a specific package.
*   Building dependency trees for analysis.

**Methods:**
*   **`Walk(ctx, rootImportPath, visitor)`**: The primary method for this group. It performs a dependency graph traversal starting from a root package and calls a user-provided `Visitor` to process each discovered package.
*   **`ScanPackageImports(ctx, importPath)`**: The fundamental lightweight method used by `Walk`. It scans a package and returns only its name and a list of imported package paths.
*   **`FindImporters(ctx, targetImportPath)`**: Scans the entire module to find all packages that directly import the `targetImportPath`.
*   **`FindImportersAggressively(ctx, targetImportPath)`**: A faster version of `FindImporters` that uses `git grep` to quickly identify potential importers before verifying them. Requires `git` to be installed.
*   **`BuildReverseDependencyMap(ctx)`**: Scans the entire module once to build a complete map where keys are import paths and values are lists of packages that import them.


### Group 2: Full Parsing & Code Analysis (`goscan.Scanner`)

These methods perform a comprehensive parse of Go source files. They are used for any tool that needs to understand the actual code within a package.

**How to use:** Create a `Scanner` instance using `goscan.New()`.

```go
// This scanner can now resolve import paths and perform deep parsing.
scanner, err := goscan.New(
    goscan.WithGoModuleResolver(),
)
if err != nil {
    // handle error
}
```

**Use Cases:**
*   Code generation.
*   Static analysis tools and linters.
*   Interpreters and symbolic execution engines (like `minigo` and `symgo`).

**Methods:**
*   **`ScanPackageByImport(ctx, importPath)`**: The main workhorse for this group. It takes an import path, uses a locator to find the package's directory, and performs a full parse on its files. Results are cached for efficiency.
    ```go
    // Get full details for the "net/http" package.
    pkg, err := scanner.ScanPackageByImport(ctx, "net/http")
    if err == nil {
        for _, t := range pkg.Types {
            fmt.Println("Found type:", t.Name)
        }
    }
    ```
*   **`ScanFiles(ctx, filePaths)`**: Performs a full parse on a specific list of files. All files must belong to the same package.
*   **`ScanPackage(ctx, pkgPath)`**: Similar to `ScanPackageByImport`, but takes a file system directory path instead of an import path.
*   **`FindSymbolDefinitionLocation(ctx, symbolFullName)`**: Finds the exact file path where a symbol (e.g., `"fmt.Println"`) is defined.
*   **`ResolveType(ctx, fieldType)`**: A lower-level utility that resolves a `FieldType` into a `TypeInfo`, performing recursive resolution if necessary.
*   **`TypeInfoFromExpr(ctx, ...)`**: A helper to parse an `ast.Expr` into a `FieldType`.
*   **`ListExportedSymbols(ctx, pkgPath)`**: Scans a package and returns a simple list of its exported symbol names.
*   **`FindSymbolInPackage(ctx, importPath, symbolName)`**: Scans files in a package one-by-one until a specific symbol is found.

---

## Usage by Package

This section summarizes which component is used by the key commands and examples in this repository.

*   **`examples/deps-walk`**:
    *   **Summary**: A dependency graph visualization tool. It is a classic example of a **`ModuleWalker`** user.
    *   **Methods Used**: `Walk`, `FindImportersAggressively`, `BuildReverseDependencyMap`. It relies entirely on the lightweight `ScanPackageImports` (called by `Walk`) to discover dependencies without parsing full source code.

*   **`minigo`**, **`symgo`**, **`examples/convert`**, **`examples/derivingjson`**, etc.:
    *   **Summary**: These are all code generation tools or interpreters. They are canonical examples of **`Scanner`** users.
    *   **Methods Used**: They use `ScanPackageByImport`, `ScanFiles`, and `ResolveType` to get detailed information about the code within packages.
