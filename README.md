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


## More Examples

*   **`examples/derivingjson`**: Demonstrates generating JSON marshaling/unmarshaling methods, including support for `oneOf` (sum types).
*   **`examples/derivingbind`**: Shows how to generate methods for binding data from HTTP requests (query, path, header, body) to struct fields.
*   **`examples/minigo`**: A mini Go interpreter that uses `go-scan` for parsing and understanding Go code structure.
*   **`examples/convert`**: A prototype for generating type conversion functions between different Go structs. See `examples/convert/README.md` for details.
*   **`examples/deps-walk`**: A command-line tool to visualize package dependencies within a module. See `examples/deps-walk/README.md` for details.
