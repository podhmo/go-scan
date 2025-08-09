# Examples

This directory contains example projects that demonstrate the capabilities and usage of the `go-scan` library. Each subdirectory focuses on a specific use-case or feature.

## Available Examples

- [derivingjson](#derivingjson)
- [derivingbind](#derivingbind)
- [convert](#convert)
- [minigo](#minigo)
- [deps-walk](#deps-walk)

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

The `minigo` example is a command-line interface for `minigo2`, a miniature Go interpreter built to demonstrate and test the `go-scan` library.

**Purpose**: To provide a runnable example of a language interpreter that uses `go-scan` for its core analysis features, such as resolving imported Go packages and their symbols on the fly.

**Key Features**:
-   **Powered by `minigo2`**: The actual interpreter logic resides in the `minigo2` package in the parent directory, not in the example itself.
-   **Dynamic Go Symbol Resolution**: It leverages `go-scan` to dynamically load information about functions and constants from other Go packages that are `import`ed by a script.
-   **Lazy Loading**: The underlying `minigo2` engine uses `go-scan`'s lazy-loading mechanism, meaning imported Go packages are only fully scanned when their symbols are actually referenced in the script.
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
