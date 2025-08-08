# Go Comment Extractor - `commentof`

`commentof` is a Go library designed to parse Go source code and extract comments associated with various declarations. It provides a structured way to access documentation for functions, types (structs, aliases), constants, variables, and their respective fields, parameters, and return values.

This library uses the standard `go/parser` and `go/ast` packages to build an Abstract Syntax Tree (AST) and traverses it to link comments to their corresponding code elements.

## Features

-   **Function Documentation**: Extracts comments for functions, parameters, and return values.
-   **Type Documentation**: Extracts comments for `type` specifications, including `struct` definitions, type aliases, and their fields.
-   **Value Documentation**: Extracts comments for `const` and `var` declarations.
-   **Handles Various Comment Styles**: Correctly processes both line comments (`//`) and general comments (`/* ... */`).
-   **AST Node API**: Provides exported functions (`FromFuncDecl`, `FromGenDecl`) that operate directly on `ast` nodes, allowing for integration with other Go code analysis tools.
-   **File-based API**: Simple `FromFile` and `FromReader` functions to parse entire files.

## Data Structures

The library returns extracted information in a set of clear, structured models:

-   `commentof.Function`: Holds docs for a function, its params, and results.
-   `commentof.TypeSpec`: Holds docs for a `type` declaration.
-alueSpec`: Holds docs for `const` or `var` declarations.
-   `commentof.Struct`: Holds docs for fields within a `struct`.
-   `commentof.Field`: A generic container for any named and documented element (like a parameter or struct field).

## Basic Usage

To use the library, you can parse a Go file and then iterate through the returned data structures.

```go
package main

import (
	"fmt"
	"log"

	"github.com/example/commentof"
)

func main() {
	// The path to your Go source file
	path := "path/to/your/source.go"

	docs, err := commentof.FromFile(path)
	if err != nil {
		log.Fatalf("Failed to parse file: %v", err)
	}

	for _, item := range docs {
		switch d := item.(type) {
		case *commentof.Function:
			fmt.Printf("Function: %s\nDoc: %s\n", d.Name, d.Doc)
			for _, param := range d.Params {
				fmt.Printf("  - Param: %v (%s) -> Doc: %s\n", param.Names, param.Type, param.Doc)
			}
		case *commentof.TypeSpec:
			fmt.Printf("Type: %s\nDoc: %s\n", d.Name, d.Doc)
			if s, ok := d.Definition.(*commentof.Struct); ok {
				for _, field := range s.Fields {
					fmt.Printf("  - Field: %v (%s) -> Doc: %s\n", field.Names, field.Type, field.Doc)
				}
			}
		}
	}
}
```

## How to Run Tests

To run the tests for this library, navigate to the `commentof` directory and execute:

```bash
go test -v
```