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

	// Create a new scanner.
	// By default, it only scans packages within the current Go module.
	// To enable resolution of standard library and external packages from the module cache,
	// use the `WithGoModuleResolver` option.
	scanner, err := goscan.New(".", goscan.WithGoModuleResolver())
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
	pkgInfo, err := scanner.ScanPackageFromImportPath(pkgImportPath)
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

In some scenarios, you may need to prevent the scanner from parsing a specific type from its source file. This is particularly useful for standard library types like `time.Time`, which can cause a `mismatched package names` error when a scan is triggered from a test binary (`package main`).

To handle this, you can provide a "synthetic" type definition to the scanner. The scanner will use this synthetic definition instead of attempting to parse the type's source code.

This is configured via the `WithExternalTypeOverrides` option, which accepts a `map[string]*scanner.TypeInfo`.

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner" // For scanner.TypeInfo and scanner.ExternalTypeOverride
)

func main() {
	ctx := context.Background()

	// Define an override for "time.Time".
	// The key is the fully qualified type name.
	// The value is a pointer to a scanner.TypeInfo struct that describes the type.
	overrides := scanner.ExternalTypeOverride{
		"time.Time": &scanner.TypeInfo{
			Name:    "Time",
			PkgPath: "time",
			Kind:    scanner.StructKind, // It's a struct
		},
	}

	// Pass the overrides to the scanner during creation.
	s, err := goscan.New(
		".", // Start scanning from the current directory
		goscan.WithGoModuleResolver(),      // Still useful for other packages
		goscan.WithExternalTypeOverrides(overrides),
	)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create scanner", slog.Any("error", err))
		os.Exit(1)
	}

	// Now, when the scanner encounters a field of type `time.Time`, it will use the
	// synthetic TypeInfo provided above instead of trying to scan the "time" package.
	// The resulting scanner.FieldType will have its `IsResolvedByConfig` flag set to true,
	// and its `Definition` field will point to the synthetic `TypeInfo`.
}
```

When a type is resolved using an override:
- A `scanner.FieldType` is created based on the provided `scanner.TypeInfo`.
- The `FieldType.IsResolvedByConfig` flag will be set to `true`.
- The `FieldType.Definition` field will point to the synthetic `TypeInfo` you provided.
- Calling `FieldType.Resolve()` on such a type will immediately return the linked `TypeInfo` without performing a scan.

This feature gives you fine-grained control over how specific types are interpreted, which is essential for working around complex build contexts or for simplifying external types.

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
*   **`examples/deps-walk`**: A command-line tool to visualize package dependencies within a module. See `examples/deps-walk/README.md` for details.
