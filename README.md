# go-scan

`go-scan` is a static analysis library for Go, designed around a core philosophy of **shallow, lazy scanning**. It parses Go source files to extract information about types, functions, and constants without requiring a fully resolved type-checked environment.

Its primary advantage lies in its trade-off: by not building a complete, deeply-checked type graph like `go/packages`, `go-scan` can analyze codebases much faster and with fewer dependencies. This makes it ideal for build-time tools like code generators, ORM mappers, or API documentation generators, where full type integrity is not a prerequisite, but speed and simplicity are.

The library intelligently resolves type definitions across packages on-demand, supports Go workspaces (`go.work`), and can find packages in your local module cache.

🚧 This library is currently under development.

## Features

- **Shallow and Lazy AST Parsing**: Operates directly on the AST (`go/ast`) to extract information quickly without full type-checking.
- **Cross-Package Resolution**: Lazily resolves type definitions across different packages as they are needed.
- **Go Workspace Support**: Correctly handles multi-module projects using a `go.work` file.
- **Recursive Type Handling**: Safely parses recursive types (e.g., `type Node struct { Next *Node }`) and circular package dependencies.
- **Flexible Scanning**: Scans packages using import paths, directory paths, file paths, and even wildcard patterns (`...`).
- **Detailed Information Extraction**:
    - **Structs**: Parses fields, tags, and embedded structs.
    - **Complex Types**: Understands pointers (`*`), slices (`[]`), maps (`map[K]V`), and generics.
    - **Type Aliases**: Recognizes type aliases (e.g., `type UserID int`) and their underlying types.
    - **Functions**: Extracts signatures of top-level functions and methods.
    - **Constants**: Extracts top-level `const` declarations.
- **GoDoc Parsing**: Captures documentation comments for all major declarations.
- **Symbol Location Cache**: Optionally caches the file location of scanned symbols to accelerate subsequent analyses.
- **External Type Overrides**: Allows you to provide synthetic definitions for external types (like `time.Time` or `uuid.UUID`) to prevent unwanted scanning and control how they are represented.

## Quick Start

The primary entry point is the `goscan.Scanner`, which can be configured with various options. Here's a simple example of how to scan a package and inspect its types.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/podhmo/go-scan"
)

func main() {
	ctx := context.Background()

	// Create a new scanner starting from the current directory.
	// WithGoModuleResolver() enables finding packages in GOROOT and the module cache.
	scanner, err := goscan.New(
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create scanner", slog.Any("error", err))
		os.Exit(1)
	}

	// Scan a package by its import path.
	// The Scan method can also accept directory paths, file paths, or wildcard patterns.
	// For this example, we use test data from the go-scan repository.
	pkgImportPath := "github.com/podhmo/go-scan/testdata/multipkg/api"
	pkgs, err := scanner.Scan(ctx, pkgImportPath)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to scan package", slog.String("package", pkgImportPath), slog.Any("error", err))
		os.Exit(1)
	}

	// The Scan method returns a slice of packages.
	for _, pkgInfo := range pkgs {
		fmt.Printf("Scanned Package: %s\n", pkgInfo.Name)
		for _, t := range pkgInfo.Types {
			// Find a specific struct
			if t.Name == "Handler" {
				fmt.Printf("Found struct: %s\n", t.Name)
				for _, f := range t.Struct.Fields {
					// Lazily resolve the type definition of a field.
					// This might trigger a scan of another package.
					def, err := f.Type.Resolve()
					if err != nil {
						slog.WarnContext(ctx, "Could not resolve field", slog.String("field", f.Name), slog.Any("error", err))
						continue
					}
					slog.InfoContext(ctx, "Field resolved", slog.String("field", f.Name), slog.String("resolved_type", def.Name))
				}
			}
		}
	}
}
```

## Advanced Usage

### Flexible Scanning with `Scan`

The `scanner.Scan(ctx, patterns...)` method is a powerful and flexible way to specify what you want to scan. It accepts one or more string patterns, which can be:

- **An Import Path**: `"github.com/podhmo/go-scan/testdata/multipkg/api"`
- **A Directory Path**: `"./testdata/multipkg/api"`
- **A File Path**: `"./testdata/multipkg/api/api.go"`
- **A Wildcard Path**: `"github.com/podhmo/go-scan/testdata/multipkg/..."` (scans all sub-packages)

The scanner intelligently determines the type of pattern and handles it accordingly.

### Go Workspace Support

If your project uses a `go.work` file, `go-scan` can operate in workspace mode. This allows it to correctly resolve dependencies between the different modules in your workspace.

To enable this, provide the paths to all the modules in your workspace using the `WithModuleDirs` option.

```go
scanner, err := goscan.New(
    goscan.WithModuleDirs([]string{"./app", "./lib"}),
    goscan.WithGoModuleResolver(),
)
```

The scanner will create a composite view of all modules, allowing for seamless cross-module type resolution.

### Caching Symbol Locations

For tools that repeatedly look up symbol locations, `go-scan` offers a persistent cache.

- **Enable Caching**: Set the `scanner.CachePath` field to the desired JSON file path.
- **Save the Cache**: **Crucially**, you must call `defer scanner.SaveSymbolCache(ctx)` to ensure the cache is written to disk when your program exits.

```go
import "path/filepath"

// Enable caching.
scanner.CachePath = filepath.Join(os.TempDir(), "my-app-symbol-cache.json")
defer scanner.SaveSymbolCache(ctx)

// Now, calls to FindSymbolDefinitionLocation will be much faster on subsequent runs.
filePath, err := scanner.FindSymbolDefinitionLocation(ctx, "github.com/podhmo/go-scan.Scanner")
```

### Overriding External Types

Sometimes you need to prevent `go-scan` from analyzing a type from an external package (e.g., `time.Time`, which can cause issues in certain build contexts) and instead provide a synthetic definition. This is done with `WithExternalTypeOverrides`.

```go
import "github.com/podhmo/go-scan/scanner"

// Define an override for "time.Time".
overrides := scanner.ExternalTypeOverride{
    "time.Time": &scanner.TypeInfo{
        Name:    "Time",
        PkgPath: "time",
        Kind:    scanner.StructKind,
    },
}

// Pass the overrides to the scanner during creation.
s, err := goscan.New(
    goscan.WithExternalTypeOverrides(overrides),
)
```
When the scanner encounters `time.Time`, it will use your synthetic `TypeInfo` instead of parsing the "time" package. The resulting `scanner.FieldType` will have its `IsResolvedByConfig` flag set to `true`.

## Testing

The `scantest` package provides helpers for writing tests against `go-scan`. For more details, see the [`scantest/README.md`](./scantest/README.md).

## More Examples

- **`examples/derivingjson`**: Demonstrates generating JSON marshaling/unmarshaling methods.
- **`examples/derivingbind`**: Shows how to generate methods for binding HTTP request data to struct fields.
- **`examples/minigo`**: A mini Go interpreter that uses `go-scan` for parsing.
- **`examples/convert`**: A prototype for generating type conversion functions between structs.
- **`examples/deps-walk`**: A command-line tool to visualize package dependencies.
