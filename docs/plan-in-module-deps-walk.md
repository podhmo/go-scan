# Plan: In-Module Dependency Walker

This document outlines a plan to create a tool for visualizing the dependency graph of specific packages within a Go module. This tool will be built upon the existing `go-scan` library, and this document also details the new features required in `go-scan` to support this tool efficiently.

## 1. Motivation and Goal

The primary goal is to create a developer utility that can generate a dependency graph for a targeted subset of packages within the current Go module. While tools exist to visualize the dependencies of an entire project, they often produce graphs that are too large and noisy to be useful for understanding a specific component's architecture.

This tool will provide a focused view, helping developers answer questions like, "What are the immediate dependencies of my `api` package?" or "How does the `models` package connect to the rest of the application within two hops?"

The tool will be a command-line application that uses the `go-scan` library as its engine.

A secondary use case for this dependency walking mechanism is to programmatically collect a set of target packages for other tools. For example, a code generator that operates on marker comments (like `derivingjson`) could use the walker to find all packages within a certain part of the module, and then run its more intensive scanning process only on that collected set.

## 2. Core Features of the Visualization Tool

The command-line tool will support the following features:

### 2.1. Hop Count Limiting

The user will be able to specify the maximum number of hops (degrees of separation) to render in the dependency graph from the starting package.

-   **Example:** `go-deps-walk --start-pkg=./api --hops=1` would show only the packages that `./api` directly imports.

### 2.2. Package Exclusion

The user will be able to provide a list of package import patterns to ignore or exclude from the graph. This is useful for hiding ubiquitous dependencies like logging, configuration, or common utility packages to de-clutter the output. The patterns should support glob-style matching for more flexibility.

-   **Example:** `go-deps-walk --start-pkg=./api --ignore="github.com/my-org/core/*,*.test"`

### 2.3. Output Format

The tool will output the graph in standard formats. Initially, it will support:
- **DOT**: For rendering with tools like Graphviz.
- **Mermaid**: For easy embedding in Markdown documents (e.g., on GitHub).

### 2.4. Dependency Scope (In-Module vs. Full)

By default, the tool will only traverse dependencies that are part of the current Go module. This is the most common use case, as it focuses on the internal architecture of the project without getting cluttered by standard library or third-party dependencies.

The user can include external dependencies in the graph by using a specific flag.

-   **Default:** Only packages within the current Go module are included.
-   **Flag:** `go-deps-walk --start-pkg=./api --full` will include all dependencies, including those from the standard library and external modules.

## 3. Analysis of the `go-scan` Library

To build this tool, we must first analyze the capabilities of the existing `go-scan` library.

### 3.1. Current Dependency Resolution Mechanism

The `go-scan` library is built on a foundation of parsing Go files directly using `go/ast`, deliberately avoiding `go/packages`. Its dependency resolution is "lazy" and works as follows:

1.  A call to `goscan.Scanner.ScanPackageByImport()` triggers a scan of a package.
2.  The `locator` finds the package's directory on disk.
3.  The `scanner` parses all the `.go` files in that directory into a full AST.
4.  The scanner extracts type, function, and constant information. When it encounters a type from another package (e.g., `anotherpkg.MyType`), it creates a `FieldType` struct containing the import path of that package (`"github.com/my-org/anotherpkg"`).
5.  The dependency is not immediately parsed. Only when a user of the library calls `FieldType.Resolve()` does `go-scan` recursively call `ScanPackageByImport()` on the dependency's import path.

### 3.2. Suitability for Dependency Walking

This architecture can be used to build the dependency graph. The walker tool would start with a package, call `ScanPackageByImport`, and inspect the `ast.File.Imports` list for each parsed file to find its direct dependencies. It would then recursively call `ScanPackageByImport` on those dependencies.

However, there is a major performance issue: `ScanPackageByImport` **always performs a full AST parse**. For building a dependency graph, where we only need the `import` statements, this is highly inefficient.

## 4. Gap Analysis: Missing Features in `go-scan`

To support the dependency walker tool efficiently and cleanly, the `go-scan` library itself needs to be extended. The following two features are missing.

### 4.1. A Lightweight, "Imports-Only" Scanning Mode

The most critical missing piece is an efficient way to get a package's imports without parsing every file in its entirety.

**Proposed Solution:**

1.  **New `scanner` Method:** Create a new method in `scanner/scanner.go`: `ScanImportsOnly(ctx, filePaths)`. This method will use `parser.ParseFile` with the `parser.ImportsOnly` flag. It will return the package name and a slice of import paths.

2.  **New `goscan` Struct:** Create a new lightweight struct `goscan.PackageImports` to hold this minimal information.
    ```go
    package goscan

    // PackageImports holds the minimal information about a package's direct imports.
    type PackageImports struct {
        Name       string
        ImportPath string
        Imports    []string
    }
    ```

3.  **New `goscan` Method:** Create a new public method `goscan.Scanner.ScanPackageImports(ctx, importPath)`. This method will orchestrate the process, using the `locator` to find the package and the new `scanner.ScanImportsOnly` to parse it. It should also have its own in-memory cache to avoid re-processing packages during a single walk.

### 4.2. A Generic Graph Traversal Utility

Every tool that needs to walk the dependency graph will have to re-implement the same traversal logic (a queue/recursion loop and a "visited" map). This is boilerplate that the `go-scan` library can provide.

**Proposed Solution:**

1.  **New `Visitor` Interface:** Define a `Visitor` interface that allows a tool to inject its own logic into the traversal process.
    ```go

    package goscan

    // Visitor defines the interface for operations to be performed at each node
    // during a dependency graph walk.
    type Visitor interface {
        // Visit is called for each package discovered during the walk.
        // It can inspect the package's imports and return the list of
        // imports that the walker should follow next. Returning an empty
        // slice stops the traversal from that node.
        Visit(pkg *PackageImports) (importsToFollow []string, err error)
    }
    ```

2.  **New `Walk` Method:** Create a new public method `goscan.Scanner.Walk(ctx, rootImportPath, visitor)`. This function will perform a breadth-first or depth-first search of the dependency graph. It will handle the queue, manage the `visited` set, and call the new `ScanPackageImports` method to get dependencies. At each package, it will invoke the `visitor.Visit` method, giving the calling tool control over the process, including implementing hop limits and ignore lists.

## 5. Further Considerations

### 5.1. Reverse Dependencies (Importers)

The proposed `Walk` utility follows `import` statements, which is a directed graph. A common related task is to find all packages that *import* a given package (i.e., finding "importers" or "reverse dependencies"). This is a much harder problem as it requires searching the entire codebase for import declarations, rather than just following a direct graph.

This is out of scope for the initial tool, but a potential future enhancement could involve:
- **Text-based Search**: Integrating a fast search tool like `ripgrep` to `grep` for import paths across the module.
- **Building an Index**: A more advanced solution could involve a separate tool that builds a full import index of the module. The `go-scan` symbol cache could potentially be extended for this purpose.

### 5.2. Dependency Granularity

The current plan focuses on package-level dependencies. However, dependencies can also be analyzed at a more granular, file-level. For example, `file_A.go` might import package `P`, while `file_B.go` in the same package does not.

The proposed `goscan.PackageImports` struct could be extended to support this:

```go
// Extended PackageImports struct
type PackageImports struct {
    Name       string
    ImportPath string
    // Imports is the aggregated list of all imports in the package
    Imports []string
    // FileImports provides a file-by-file breakdown
    FileImports map[string][]string // map[filePath][]importPath
}
```
The visualization tool could then have a flag (`--granularity=file`) to render the more detailed graph. This would be a valuable feature for understanding coupling within a package.

## 6. Conclusion

By adding an efficient **imports-only scanning mode** and a **reusable graph walking utility** to the `go-scan` library, we can build the desired dependency visualization tool cleanly and efficiently. The further considerations of reverse dependencies and file-level granularity should be kept in mind for future iterations. The implementation should proceed by first adding these core features to the `go-scan` library, then building the command-line tool on top of that enhanced foundation.
