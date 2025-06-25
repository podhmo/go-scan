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
- **Symbol Definition Caching**: (Experimental) Optionally caches the file location of scanned symbols (`types`, `functions`, `constants`) to speed up subsequent analyses by tools that need this information. The cache is stored as a JSON file.

## Quick Start

### Example Usage

```go
package main

import (
	"fmt"
	"log"

	"github.com/podhmo/go-scan/typescanner"
	// If you are using the main package directly, import path might be different
	// For library usage, it's typically:
	// "github.com/podhmo/go-scan/typescanner"
)

func main() {
	// Create a new scanner, starting search for go.mod from the current directory
	scanner, err := typescanner.New(".")
	if err != nil {
		log.Fatalf("Failed to create scanner: %v", err)
	}

	// --- Optional: Enable Symbol Cache ---
	// To enable symbol definition caching, set the CachePath.
	// If CachePath is an empty string (which is the default for a new Scanner), caching is disabled.
	// scanner.CachePath = filepath.Join(os.TempDir(), "my-app-symbol-cache.json") // Example
	// Or, for a user-level cache (ensure your app handles dir creation if needed by CachePath):
	// homeDir, _ := os.UserHomeDir()
	// if homeDir != "" {
	//    _ = os.MkdirAll(filepath.Join(homeDir, ".your-app-name"), 0750) // Ensure dir exists
	//    scanner.CachePath = filepath.Join(homeDir, ".your-app-name", "go-scan-symbols.json")
	// }

	// Important: Ensure to save the cache when your program exits if caching is enabled.
	// SaveSymbolCache will do nothing if scanner.CachePath is empty.
	defer func() {
		if err := scanner.SaveSymbolCache(); err != nil {
			log.Printf("Warning: Failed to save symbol cache: %v", err)
		}
	}()
	// --- End Optional: Enable Symbol Cache ---

	// Scan a package by its import path
	// Replace with an actual import path from your project or testdata.
	// The example below uses testdata included in this repository.
	pkgImportPath := "github.com/podhmo/go-scan/testdata/multipkg/api"
	pkgInfo, err := scanner.ScanPackageByImport(pkgImportPath)
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

	// --- Optional: Using FindSymbolDefinitionLocation (often with cache) ---
	// This method attempts to find the file where a symbol is defined.
	// It uses the cache if enabled and will fallback to scanning if the symbol isn't found in cache
	// or if the cached entry is stale (e.g., file deleted).
	if pkgInfo.Types != nil && len(pkgInfo.Types) > 0 {
		firstType := pkgInfo.Types[0]
		symbolFullName := pkgImportPath + "." + firstType.Name

		filePath, err := scanner.FindSymbolDefinitionLocation(symbolFullName)
		if err != nil {
			log.Printf("Could not find definition location for %s: %v", symbolFullName, err)
		} else {
			fmt.Printf("\nDefinition of symbol %s found at: %s\n", symbolFullName, filePath)
		}
	}
}
```

## Caching Symbol Locations

The scanner can cache the file paths where symbols (types, functions, constants) are defined. This is useful for tools that repeatedly need to look up symbol locations.

- Caching is enabled by setting the `scanner.CachePath` field to a non-empty string representing the desired path for the cache file.
- If `scanner.CachePath` is an empty string (the default for a new `Scanner` instance), caching is disabled.
- There is no default cache path if `CachePath` is left empty; it must be explicitly provided to enable caching.
- **Crucially**, if caching is enabled (i.e., `CachePath` is set), you should call `defer scanner.SaveSymbolCache()` after creating your scanner instance. This ensures the cache is written to disk when your program finishes. `SaveSymbolCache` will do nothing if `CachePath` is empty.
- The `scanner.FindSymbolDefinitionLocation("package/import/path.SymbolName")` method leverages this cache. If caching is enabled and a symbol is not found in the cache (or if the cached file path is no longer valid), it will attempt to scan the relevant package and update the cache. If caching is disabled, it will always perform a fresh scan.

This library is currently under development. See `docs/todo.md` for planned features.