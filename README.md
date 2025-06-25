# Go Type Scanner

A lightweight Go type scanner that parses Go source files to extract information about types, constants, and functions without relying on `go/packages` or `go/types`. It works by directly parsing the AST (`go/ast`), making it fast and dependency-free for build-time tool generation.

This tool is designed for applications like OpenAPI document generation, ORM code generation, or any other task that requires static analysis of Go type definitions.

## Features

- **AST-based Parsing**: Directly uses `go/parser` and `go/ast` for high performance.
- **Cross-Package Type Resolution**: Lazily resolves type definitions across different packages within the same module.
- **Type Definition Extraction**:
    - Parses `struct` definitions, including fields, tags, and embedded structs.
    - Handles complex types like pointers (`*`), slices (`[]`), and maps (`map[K]V`).
    - Recognizes type aliases (e.g., `type UserID int`) and their underlying types.
    - Parses function type declarations (e.g., `type HandlerFunc func()`).
- **Constant Extraction**: Extracts top-level `const` declarations.
- **Function Signature Extraction**: Extracts top-level function and method signatures.
- **Documentation Parsing**: Captures GoDoc comments for types, fields, functions, and constants.
- **Package Locator**: Finds the module root by locating `go.mod` and resolves internal package paths.

## Quick Start

### Example Usage

```go
package main

import (
	"fmt"
	"log"

	"github.com/podhmo/go-scan"
)

func main() {
	scanner, err := typescanner.New(".")
	if err != nil {
		log.Fatalf("Failed to create scanner: %v", err)
	}

	pkgInfo, err := scanner.ScanPackageByImport("github.com/podhmo/go-scan/testdata/multipkg/api")
	if err != nil {
		log.Fatalf("Failed to scan package: %v", err)
	}

	for _, t := range pkgInfo.Types {
		if t.Name == "Handler" {
			for _, f := range t.Struct.Fields {
				// Lazily resolve the type definition from another package
				def, err := f.Type.Resolve()
				if err != nil {
					log.Printf("Could not resolve field %s: %v", f.Name, err)
					continue
				}
				
				fmt.Printf("Field '%s' resolved to type '%s'\n", f.Name, def.Name)
				if def.Kind == typescanner.StructKind {
					fmt.Printf("  It is a struct with %d fields.\n", len(def.Struct.Fields))
				}
			}
		}
	}
}
```

This library is currently under development. See `docs/todo.md` for planned features.