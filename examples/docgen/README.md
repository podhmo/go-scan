# `docgen`: OpenAPI 3.1 Documentation Generator

`docgen` is a command-line tool that generates an OpenAPI 3.1 specification from a standard `net/http` Go application. It serves as a practical example of the `symgo` symbolic execution engine.

## What It Does

The tool analyzes the Go source code of a sample API (`./sampleapi`) to extract API metadata. It works by:

1.  **Finding the Entrypoint**: It starts by locating a specific function (e.g., `NewServeMux`) in the target package.
2.  **Symbolic Execution with `symgo`**: It uses the `symgo` interpreter to "execute" the code symbolically. Instead of running an HTTP server, it traces function calls.
3.  **Pattern Matching**: It uses built-in handlers (intrinsics) to recognize standard library functions like `net/http.HandleFunc`. For non-standard frameworks or helper functions, it can load custom analysis patterns from a user-provided file.
4.  **Deep Handler Analysis**: It then symbolically executes the handler function itself to find calls to `json.NewDecoder`, `r.URL.Query().Get()`, or custom helper functions defined in the patterns file. It uses these to infer request schemas, query parameters, and response schemas.
5.  **Generating the Spec**: Finally, it aggregates all the collected metadata and prints a valid OpenAPI 3.1 specification to standard output in either JSON or YAML format.

## How to Run

You can run `docgen` from the root of the `go-scan` repository.

### Prerequisites

The tool has its own dependencies defined in `examples/docgen/go.mod`. Ensure they are installed:
```sh
cd examples/docgen
go mod tidy
cd ../..
```

### Usage

```sh
go run ./examples/docgen [flags] [package_path]
```

**Arguments:**
- `package_path`: The import path of the package to analyze (e.g., `github.com/podhmo/go-scan/examples/docgen/sampleapi`).

**Flags:**
- `-format <string>`: The output format. Can be `json` (default) or `yaml`.
- `-patterns <string>`: The path to a Go file containing custom analysis patterns.
- `-entrypoint <string>`: The name of the function or variable to start analysis from (default: `NewServeMux`).
- `-include-pkg <string>`: An external package path to be included in the **primary analysis scope**. By default, `docgen` only performs deep source code analysis on the target module. Use this flag to instruct it to also perform a deep analysis on a specific dependency. This flag can be specified multiple times.
- `-debug`: Enable debug logging for the analysis.

### Examples

**Generate JSON output (default entrypoint):**
```sh
go run ./examples/docgen github.com/podhmo/go-scan/examples/docgen/sampleapi > openapi.json
```

**Generate YAML output with a specific entrypoint:**
```sh
go run ./examples/docgen -format=yaml -entrypoint=NewServeMux github.com/podhmo/go-scan/examples/docgen/sampleapi > openapi.yaml
```

## Customizing Analysis with Patterns

For real-world applications that use custom helper functions for rendering responses or parsing requests, you can provide `docgen` with a patterns file. This file is a Go script interpreted by `minigo`.

A key feature is the ability to define patterns in a **type-safe** way using the `Fn` field, which avoids brittle, string-based keys.

**Example `patterns.go`:**
```go
//go:build minigo
package main

import (
    "myapp/helpers"
    "myapp/models"
    "github.com/podhmo/go-scan/examples/docgen/patterns"
)

var Patterns = []patterns.PatternConfig{
    // A pattern for a function reference
    {
        Fn: helpers.RenderJSON,
        Type: patterns.ResponseBody,
        ArgIndex: 2,
    },
    // A pattern for a method reference
    {
        Fn: (*models.User)(nil).Update,
        Type: patterns.RequestBody,
        ArgIndex: 0,
    },
}
```

You would then run `docgen` with the `--patterns` flag:
```sh
go run ./examples/docgen --patterns=./patterns.go myapp/api main
```

For more details on creating pattern files and all available pattern types, see [../../sketch/summary-docgen-customize-by-minigo.md](../../docs/summary-docgen-customize-by-minigo.md).
