# go-scan

A lightweight Go type scanner that parses Go source files to extract information about types, constants, and functions without relying on `go/packages` or `go/types`. It works by directly parsing the AST (`go/ast`), making it fast and dependency-free for build-time tool generation.

This tool is designed for applications like OpenAPI document generation, ORM code generation, or any other task that requires static analysis of Go type definitions.

🚧 This library is currently under development.

## Features

- **AST-based Parsing**: Directly uses `go/parser` and `go/ast` for high performance.
- **Cross-Package Type Resolution**: Lazily resolves type definitions across different packages within the same module.
- **Handles Recursive Types**: Correctly resolves recursive type definitions (e.g., `type Node struct { Next *Node }`) and circular dependencies between packages without getting stuck in infinite loops.
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
- **External Type Overrides**: Allows specifying how types from external (or even internal) packages should be interpreted by the scanner, e.g., treating `uuid.UUID` as a `string`.

## Quick Start

### Example Usage

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/podhmo/go-scan"
	// If you are using the main package directly, import path might be different
	// For library usage, it's typically:
	// "github.com/podhmo/go-scan"
)

func main() {
	ctx := context.Background() // Or your application's context

	// Create a new scanner, starting search for go.mod from the current directory
	scanner, err := goscan.New(".")
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create scanner", slog.Any("error", err))
		os.Exit(1)
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
			slog.WarnContext(ctx, "Failed to save symbol cache", slog.Any("error", err))
		}
	}()
	// --- End Optional: Enable Symbol Cache ---

	// Scan a package by its import path
	// Replace with an actual import path from your project or testdata.
	// The example below uses testdata included in this repository.
	pkgImportPath := "github.com/podhmo/go-scan/testdata/multipkg/api"
	pkgInfo, err := scanner.ScanPackageByImport(pkgImportPath)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to scan package", slog.String("package", pkgImportPath), slog.Any("error", err))
		os.Exit(1)
	}

	for _, t := range pkgInfo.Types {
		if t.Name == "Handler" {
			for _, f := range t.Struct.Fields {
				// Lazily resolve the type definition from another package
				def, err := f.Type.Resolve()
				if err != nil {
					slog.WarnContext(ctx, "Could not resolve field", slog.String("field", f.Name), slog.Any("error", err))
					continue
				}

				slog.InfoContext(ctx, "Field resolved", slog.String("field", f.Name), slog.String("resolved_type", def.Name))
				if def.Kind == goscan.StructKind { // goscan.StructKind is correct here as it's a re-exported constant
					slog.InfoContext(ctx, "Struct details", slog.String("type", def.Name), slog.Int("field_count", len(def.Struct.Fields)))
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
			slog.WarnContext(ctx, "Could not find definition location for symbol", slog.String("symbol", symbolFullName), slog.Any("error", err))
		} else {
			slog.InfoContext(ctx, "Definition of symbol found", slog.String("symbol", symbolFullName), slog.String("path", filePath))
		}
	}
}
```

## Overriding External Type Resolution

In some scenarios, you might want to treat specific types from external (or even internal) packages as different Go types. For example, you might want all instances of `github.com/google/uuid.UUID` to be recognized as a simple `string` by the scanner, or a custom `pkg.MyTime` to be treated as `time.Time`.

The `go-scan.Scanner` provides a method `SetExternalTypeOverrides()` to achieve this. You pass a map where the key is the fully qualified type name (e.g., `"github.com/google/uuid.UUID"`) and the value is the target Go type string (e.g., `"string"`).

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner" // For scanner.ExternalTypeOverride type
)

func main() {
	ctx := context.Background() // Or your application's context

	s, err := goscan.New("./testdata/externaltypes") // Assuming testdata/externaltypes has its own go.mod
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create scanner", slog.Any("error", err))
		os.Exit(1)
	}

	// Define overrides
	overrides := scanner.ExternalTypeOverride{
		"github.com/google/uuid.UUID": "string",    // Treat uuid.UUID as string
		"example.com/somepkg.Time":    "time.Time", // Treat somepkg.Time as time.Time
	}
	s.SetExternalTypeOverrides(overrides)

	// Scan a package that uses these types
	pkgInfo, err := s.ScanPackageByImport("example.com/externaltypes") // Module from testdata
	if err != nil {
		slog.ErrorContext(ctx, "Failed to scan package", slog.String("package", "example.com/externaltypes"), slog.Any("error", err))
		os.Exit(1)
	}

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Name == "ObjectWithUUID" && typeInfo.Struct != nil {
			for _, field := range typeInfo.Struct.Fields {
				if field.Name == "ID" {
					// field.Type.Name will be "string"
					// field.Type.IsResolvedByConfig will be true
					slog.InfoContext(ctx, "Field overridden",
						slog.String("field_name", field.Name),
						slog.String("original_pkg", field.Type.PkgName),
						slog.String("original_type", field.Type.Name), // This will show the overridden type name
						slog.String("overridden_to_type", field.Type.Name), // Correctly shows the target type
						slog.Bool("is_resolved_by_config", field.Type.IsResolvedByConfig),
					)
				}
			}
		}
		if typeInfo.Name == "ObjectWithCustomTime" && typeInfo.Struct != nil {
			for _, field := range typeInfo.Struct.Fields {
				if field.Name == "Timestamp" {
					// field.Type.Name will be "time.Time"
					// field.Type.IsResolvedByConfig will be true
					slog.InfoContext(ctx, "Field overridden",
						slog.String("field_name", field.Name),
						slog.String("overridden_to_type", field.Type.Name),
						slog.Bool("is_resolved_by_config", field.Type.IsResolvedByConfig),
					)
				}
			}
		}
	}
}
```

When a type is resolved using an override:
- The `FieldType.Name` will reflect the target type string from the override map.
- The `FieldType.IsResolvedByConfig` flag will be set to `true`.
- Calling `FieldType.Resolve()` on such a type will return `nil, nil` (no error, no further `TypeInfo`), as it's considered "resolved" by the configuration.

This feature is useful for simplifying complex external types down to their common representations (like `string` or `int`) or for mapping types from non-existent/stub packages during analysis.

## Caching Symbol Locations

The scanner can cache the file paths where symbols (types, functions, constants) are defined. This is useful for tools that repeatedly need to look up symbol locations.

- Caching is enabled by setting the `scanner.CachePath` field to a non-empty string representing the desired path for the cache file.
- If `scanner.CachePath` is an empty string (the default for a new `Scanner` instance), caching is disabled.
- There is no default cache path if `CachePath` is left empty; it must be explicitly provided to enable caching.
- **Crucially**, if caching is enabled (i.e., `CachePath` is set), you should call `defer scanner.SaveSymbolCache()` after creating your scanner instance. This ensures the cache is written to disk when your program finishes. `SaveSymbolCache` will do nothing if `CachePath` is empty.
- The `scanner.FindSymbolDefinitionLocation("package/import/path.SymbolName")` method leverages this cache. If caching is enabled and a symbol is not found in the cache (or if the cached file path is no longer valid), it will attempt to scan the relevant package and update the cache. If caching is disabled, it will always perform a fresh scan.

This library is currently under development. See `docs/todo.md` for planned features.

## More Examples

*   **`examples/derivingjson`**: Demonstrates generating JSON marshaling/unmarshaling methods, including support for `oneOf` (sum types).
*   **`examples/derivingbind`**: Shows how to generate methods for binding data from HTTP requests (query, path, header, body) to struct fields.
*   **`examples/minigo`**: A mini Go interpreter that uses `go-scan` for parsing and understanding Go code structure.
*   **`examples/convert`**: A prototype for generating type conversion functions between different Go structs. See `examples/convert/README.md` for details.
