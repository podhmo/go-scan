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
*   Use **both together** for advanced tools that need to efficiently combine dependency analysis with deep code inspection.

---
## Handling Dependencies & Advanced Usage

### External Modules
By default, `go-scan` only looks for source files within the main module. To make it aware of packages from your Go module cache (`GOMODCACHE`) or `GOROOT`, you must configure it during initialization.

**How to use:** Provide the `goscan.WithGoModuleResolver()` option to a `Scanner` or `goscan.WithModuleWalkerGoModuleResolver()` to a `ModuleWalker`.

### Combining `ModuleWalker` and `Scanner` with a Shared Config

For advanced tools that need to perform both fast dependency discovery and on-demand deep parsing, the recommended approach is to create a `ModuleWalker` and a `Scanner` that share the same underlying components. This is achieved using a shared `goscan.Config` object.

Sharing a config ensures:
1.  **Efficiency**: The `locator`'s cache for module lookups is shared, avoiding redundant work.
2.  **Consistency**: Both components use the same `*token.FileSet`, which is crucial for correlating source code position information between the results of the walker and the scanner.

**How to use:**
1. Create a `goscan.Config` with shared components like a `*token.FileSet` and `*locator.Locator`.
2. Create a `ModuleWalker` and a `Scanner` by passing the same config object to them using the `With...Config()` option.
3. Use the `ModuleWalker.Walk` method. In your visitor, call `Scanner.ScanPackageByImport` for the packages that need deep analysis. This powerful pattern is sometimes called "Delayed" or "Lazy" loading.

```go
import (
    "go/token"
    "github.com/podhmo/go-scan"
    "github.com/podhmo/go-scan/locator"
)

// 1. Create shared components and a config.
fset := token.NewFileSet()
loc, err := locator.New("./")
// ... handle error ...
config := &goscan.Config{
    Fset:    fset,
    Locator: loc,
}

// 2. Create both a walker and a scanner with the same config.
walker, err := goscan.NewModuleWalker(goscan.WithModuleWalkerConfig(config))
// ... handle error ...
scanner, err := goscan.New(goscan.WithConfig(config))
// ... handle error ...

// MyVisitor holds a reference to the heavyweight scanner.
type MyVisitor struct {
    scanner *goscan.Scanner
}

// 3. The visitor decides whether to perform a full scan.
func (v *MyVisitor) Visit(pkg *goscan.PackageImports) ([]string, error) {
    fmt.Println("Discovered package with ModuleWalker:", pkg.ImportPath)

    if strings.Contains(pkg.ImportPath, "important") {
        // 4. Trigger a full, heavyweight parse for just this one package.
        fullInfo, err := v.scanner.ScanPackageByImport(context.Background(), pkg.ImportPath)
        // ... do something with fullInfo ...
    }
    return pkg.Imports, nil // Continue the walk
}

// 5. Start the walk.
visitor := &MyVisitor{scanner: scanner}
err = walker.Walk(ctx, "github.com/your/module/pkg/a", visitor)
```

### Manual Overrides for External Types
For types that cannot be parsed (e.g., those defined in C files and exposed via CGo, or complex types you wish to mock), you can provide a manual definition to a `Scanner`.

**How to use:** Use the `goscan.WithExternalTypeOverrides()` option when creating a `Scanner`.

---

## Method Groups

### `goscan.ModuleWalker` Methods (Discovery & Dependency Analysis)

These methods are optimized for speed and are used to understand the relationships *between* packages.

**Methods:**
*   **`Walk(ctx, rootImportPath, visitor)`**: The primary method for this group. It performs a dependency graph traversal starting from a root package and calls a user-provided `Visitor` to process each discovered package.
*   **`ScanPackageImports(ctx, importPath)`**: The fundamental lightweight method used by `Walk`. It scans a package and returns only its name and a list of imported package paths.
*   **`FindImporters(ctx, targetImportPath)`**: Scans the entire module to find all packages that directly import the `targetImportPath`.
*   **`FindImportersAggressively(ctx, targetImportPath)`**: A faster version of `FindImporters` that uses `git grep` to quickly identify potential importers before verifying them. Requires `git` to be installed.
*   **`BuildReverseDependencyMap(ctx)`**: Scans the entire module once to build a complete map where keys are import paths and values are lists of packages that import them.


### `goscan.Scanner` Methods (Full Parsing & Code Analysis)

These methods perform a comprehensive parse of Go source files. They are used for any tool that needs to understand the actual code within a package.

**Methods:**
*   **`ScanPackageByImport(ctx, importPath)`**: The main workhorse for this group. It takes an import path, uses a locator to find the package's directory, and performs a full parse on its files.
*   **`ScanFiles(ctx, filePaths)`**: Performs a full parse on a specific list of files.
*   **`ScanPackage(ctx, pkgPath)`**: Similar to `ScanPackageByImport`, but takes a file system directory path instead of an import path.
*   **`FindSymbolDefinitionLocation(ctx, symbolFullName)`**: Finds the exact file path where a symbol (e.g., `"fmt.Println"`) is defined.
*   **`ResolveType(ctx, fieldType)`**: A lower-level utility that resolves a `FieldType` into a `TypeInfo`, performing recursive resolution if necessary.
*   **`TypeInfoFromExpr(ctx, ...)`**: A helper to parse an `ast.Expr` into a `FieldType`.
*   **`ListExportedSymbols(ctx, pkgPath)`**: Scans a package and returns a simple list of its exported symbol names.
*   **`FindSymbolInPackage(ctx, importPath, symbolName)`**: Scans files in a package one-by-one until a specific symbol is found.

---

## Usage by Package

*   **`examples/deps-walk`**: A dependency graph visualization tool. It is a classic example of a **`ModuleWalker`** user.
*   **`minigo`**, **`symgo`**, **`examples/convert`**, etc.: Code generation tools or interpreters. They are canonical examples of **`Scanner`** users.
