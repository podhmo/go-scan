# Examples

This directory contains example projects that demonstrate the capabilities and usage of the `go-scan` library. Each subdirectory focuses on a specific use-case or feature.

## Available Examples

- [derivingjson](#derivingjson)
- [derivingbind](#derivingbind)
- [convert](#convert)
- [minigo](#minigo)
- [deps-walk](#deps-walk)
- [docgen](#docgen)
- [find-orphans](#find-orphans)

---

## derivingjson

The `derivingjson` example showcases an experimental tool that leverages the `go-scan` library.

**Purpose**: To automatically generate `UnmarshalJSON` methods for Go structs that are structured to represent `oneOf` semantics (similar to JSON Schema's `oneOf`). This is useful when a field can be one of several distinct types, and a discriminator field is used to determine the actual type during unmarshaling.

**Key Features**:
- Uses `go-scan` for type information analysis.
- Targets structs specifically annotated with `@deriving:unmarshal` in their GoDoc comments.
- Identifies a discriminator field (e.g., `Type string \`json:"type"\``) and a target interface field to generate the appropriate unmarshaling logic.
- Searches for concrete types that implement the target interface within the same package.

This example demonstrates how `go-scan` can be used to build tools for advanced code generation tasks based on static analysis of Go source code.

---

## derivingbind

The `derivingbind` example demonstrates a code generator built with `go-scan`.

**Purpose**: To generate a `Bind` method for Go structs. This method is designed to populate the struct's fields from various parts of an HTTP request, including:
- Path parameters
- Query parameters
- Headers
- Cookies
- Request body (JSON)

**Key Features**:
- Uses `go-scan` to analyze struct definitions.
- Processes structs annotated with `@deriving:binding` in their GoDoc comments.
- Determines data binding sources for fields based on a combination of struct-level GoDoc annotations and individual field tags (e.g., `in:"path"`, `query:"name"`, `header:"X-API-Key"`).
- Supports a wide range of Go built-in types for binding, including scalars, pointers to scalars, and slices of scalars.
- Can bind the entire request body to a specific field or map JSON body fields to struct fields if the struct itself is marked with `in:"body"`.

This example illustrates how `go-scan` can facilitate the creation of tools for request binding and similar web framework-related code generation.

---

## convert

The `convert` example is a command-line tool that automates the generation of type conversion functions between Go structs.

**Purpose**: To eliminate the boilerplate involved in writing manual conversion functions for tasks like mapping database models to API DTOs or transforming data between different representations.

**Key Features**:
- Triggers code generation based on a `@derivingconvert` annotation in struct doc comments.
- Provides fine-grained control over field mapping and custom logic using `convert` struct tags.
- Supports global conversion and validation rules.
- Automatically handles nested structs, slices, maps, and pointers.
- Collects and reports multiple errors from a single conversion pass.

This example demonstrates how `go-scan` can be used to build a sophisticated and practical code generation tool that simplifies common development tasks.

---

## minigo

The `minigo` example is a command-line interface for `minigo`, a miniature Go interpreter built to demonstrate and test the `go-scan` library.

**Purpose**: To provide a runnable example of a language interpreter that uses `go-scan` for its core analysis features, such as resolving imported Go packages and their symbols on the fly.

**Key Features**:
-   **Powered by `minigo`**: The actual interpreter logic resides in the `minigo` package in the parent directory, not in the example itself.
-   **Dynamic Go Symbol Resolution**: It leverages `go-scan` to dynamically load information about functions and constants from other Go packages that are `import`ed by a script.
-   **Lazy Loading**: The underlying `minigo` engine uses `go-scan`'s lazy-loading mechanism, meaning imported Go packages are only fully scanned when their symbols are actually referenced in the script.
-   **File-Scoped Imports**: Demonstrates support for both aliased (`import f "fmt"`) and dot (`import . "strings"`) imports, with the scope of the import correctly constrained to the file in which it is declared.

This example showcases how `go-scan` can be used as a backbone for complex language tooling that needs to interact with and understand other Go code.

---

## deps-walk

The `deps-walk` example is a command-line tool that visualizes the dependency graph of packages within a Go module.

**Purpose**: To help developers understand the internal architecture of a Go project by generating a focused dependency graph. The tool outputs a graph in the DOT language, which can be rendered into an image using tools like Graphviz.

**Key Features**:
- Uses `go-scan`'s `Walk` API, which leverages an efficient "imports-only" scanning mode.
- Allows limiting the traversal depth with a `--hops` flag.
- Supports filtering packages from the graph with an `--ignore` flag.
- Can switch between traversing only in-module dependencies (default) and all dependencies (`--full` flag).

This example demonstrates how the `go-scan` library can be used to build developer utilities for dependency analysis and visualization.

---

## docgen

The `docgen` example is an experimental tool that generates OpenAPI 3.1 specifications from standard `net/http` applications.

**Purpose**: To demonstrate how the `symgo` symbolic execution engine can be used to perform deep, static analysis of Go code to understand an API's structure without running it.

**Key Features**:
- Uses the `symgo` engine to symbolically execute the target application's code.
- Intercepts calls to `net/http.HandleFunc` to discover API routes and their corresponding handler functions.
- Analyzes the body of handler functions to infer request and response schemas by looking for patterns like `json.NewDecoder(...).Decode()`.
- Aggregates the discovered information into a valid OpenAPI 3.1 JSON document.

This example showcases how `symgo` and `go-scan` can be combined to build powerful static analysis tools for Go codebases.

---

## find-orphans

The `find-orphans` example is a static analysis tool that finds unused ("orphan") functions in a Go project.

**Purpose**: To identify dead code by performing a call graph analysis starting from known entry points. This helps developers clean up their codebase by removing functions and methods that are no longer referenced.

**Key Features**:
- Uses `go-scan`'s `Walker` API to discover all packages in a module or workspace.
- Employs the `symgo` symbolic execution engine to trace all possible execution paths from entry points.
- Automatically detects entry points: it uses `main.main` for binaries or all exported functions for libraries.
- Reports any function or method that is never reached during the symbolic execution as an "orphan".
- Serves as a test pilot for `symgo`'s dead-code analysis capabilities.
