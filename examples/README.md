# Examples

This directory contains example projects that demonstrate the capabilities and usage of the `go-scan` library. Each subdirectory focuses on a specific use-case or feature.

## Available Examples

- [derivingjson](#derivingjson)
- [derivngbind](#derivngbind)

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

## derivngbind

The `derivngbind` example demonstrates a code generator built with `go-scan`.

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
