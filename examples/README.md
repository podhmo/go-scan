# Examples

This directory contains example projects that demonstrate the capabilities and usage of the `go-scan` library. Each subdirectory focuses on a specific use-case or feature.

## Available Examples

- [derivingjson](#derivingjson)
- [derivingbind](#derivingbind)
- [minigo](#minigo)

---

## derivingjson

The `derivingjson` example showcases an experimental tool that leverages the `go-scan` library.

**Purpose**: To automatically generate `UnmarshalJSON` methods for Go structs that are structured to represent `oneOf` semantics (similar to JSON Schema's `oneOf`). This is useful when a field can be one of several distinct types, and a discriminator field is used to determine the actual type during unmarshalling.

**Key Features**:
- Uses `go-scan` for type information analysis.
- Targets structs specifically annotated with `@deriving:unmarshall` in their GoDoc comments.
- Identifies a discriminator field (e.g., `Type string \`json:"type"\``) and a target interface field to generate the appropriate unmarshalling logic.
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
- Processes structs annotated with `@derivng:binding` in their GoDoc comments.
- Determines data binding sources for fields based on a combination of struct-level GoDoc annotations and individual field tags (e.g., `in:"path"`, `query:"name"`, `header:"X-API-Key"`).
- Supports a wide range of Go built-in types for binding, including scalars, pointers to scalars, and slices of scalars.
- Can bind the entire request body to a specific field or map JSON body fields to struct fields if the struct itself is marked with `in:"body"`.

This example illustrates how `go-scan` can facilitate the creation of tools for request binding and similar web framework-related code generation.

---

## minigo

The `minigo` example is a miniature Go interpreter designed to showcase and test the capabilities of the `github.com/podhmo/go-scan` library.

**Overview**

`minigo` is a simplified interpreter that can parse and execute a small subset of Go-like syntax. Its primary purpose is to serve as a practical example and a test bed for the `go-scan` library, particularly demonstrating how `go-scan` can be used to analyze Go source code for more complex applications like interpreters or static analysis tools.

**Core `go-scan` Features Highlighted**

The `go-scan` library (`github.com/podhmo/go-scan`) provides robust tools for parsing Go source code and extracting type information without relying on `go/packages` or `go/types`. This example leverages several key features:

-   **AST-based Parsing**: `go-scan` directly parses the Abstract Syntax Tree (AST) of Go source files.
-   **Type Information Extraction**: It can extract detailed information about structs, type aliases, function types, constants, and function signatures.
-   **Documentation Parsing**: GoDoc comments are captured.
-   **Cross-Package Type Resolution (Lazy Loading)**: `go-scan` can resolve type definitions across package boundaries within the same module using a lazy loading mechanism. When a type from another package is encountered (e.g., `models.User`), its full definition is only parsed and loaded when `Resolve()` is explicitly called. This is efficient and resilient.
-   **Package Locator**: Finds the module root (`go.mod`) and resolves internal Go import paths.

**Package Imports in `minigo`**

`minigo` supports importing symbols (currently constants only) from other packages within the same Go module.

**Lazy Import Specification**

Import handling in `minigo` is designed to be "lazy":

-   When an `import` statement (e.g., `import "my/pkg"` or `import p "my/pkg"`) is encountered, `minigo` only records a mapping between the local package name and the import path.
-   **No files from the imported package are read or parsed at this stage.**
-   The actual scanning of the imported package and loading of its symbols occurs only when a symbol from that package is first referenced (e.g., `pkg.MyConst`).

This lazy approach ensures that `minigo` only expends resources on parsing packages that are actually used.

**Referencing Imported Symbols**

Symbols can be referenced using the package's base name or an alias:

```go
// Without Alias
import "mytestmodule/testpkg"
var MyMessage = testpkg.ExportedConst

// With Alias
import pkga "mytestmodule/testpkg"
var MyNumber = pkga.AnotherExportedConst
```

**Supported Symbols**

Currently, only **exported constants** are supported for import.

**Unsupported Import Forms**

-   Dot Imports: `import . "my/pkg"`
-   Blank Imports for Execution: `import _ "my/pkg"` (as `minigo` doesn't support `init` functions for side effects).

**Running `minigo` (Conceptual)**

```bash
cd examples/minigo
go run main.go your_script.mgo
```
*(Note: `main.go` and the exact execution mechanism are illustrative.)*

This example showcases how `go-scan` can be used to build complex tools like interpreters, with a particular emphasis on its efficient lazy loading capabilities for handling dependencies.
