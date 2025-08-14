# minigo

`minigo` is a simple, embeddable script engine for Go applications. It interprets a subset of the Go language, allowing developers to write dynamic configurations or extend application logic with familiar syntax.

Its key feature is the ability to execute a script and **unmarshal the result directly into a Go struct** in a type-safe manner.

## Standard Library Support

`minigo` can interact with Go's standard library using two primary methods. The recommended approach is to use direct source interpretation whenever possible.

### 1. Direct Source Interpretation (Recommended)

`minigo` can directly load, parse, and interpret the Go source code of many standard library packages at runtime. This is the most powerful and modern integration method.

-   **How it Works**: The interpreter finds the stdlib source in your `GOROOT`, parses it into an AST, and makes the package available for import within your script.
-   **Key Advantage**: This method fully supports **generic functions**, such as those in the `slices` package. It executes the actual Go source, providing high fidelity to Go's semantics.
-   **Example**: Your script can `import "slices"` and use `slices.Clone()` without any pre-generated binding files.

```go
// Your Go application
// ...
// This helper function finds and loads the "slices" package source.
interp.LoadGoSourceAsPackage("slices")
// ...
```

```go
// Your script (e.g., script.mgo)
package main

import "slices"

func main() {
    original := []int{1, 2, 3}
    copied := slices.Clone(original)
    return copied
}
```

### 2. FFI Bindings (Fallback)

For packages that cannot be directly interpreted (e.g., those using CGO or complex `unsafe` operations), you can use the `minigo-gen-bindings` tool. This tool generates Go files that create a Foreign Function Interface (FFI) bridge between `minigo` and the pre-compiled Go package.

-   **How it Works**: The tool scans a compiled package and generates `install.go` code that registers the package's functions with the `minigo` interpreter.
-   **Limitation**: This method **does not support generic functions**. It is best suited for exposing specific, non-generic functions from packages like `bytes` or `net/http`.

## Usage Example

Here is a simple example of running a `minigo` script.

#### 1. Go Application

```go
// main.go
package main

import (
	"context"
	"fmt"
	"log"
	"github.com/podhmo/go-scan/minigo"
)

func main() {
    interp, err := minigo.NewInterpreter()
    if err != nil {
        log.Fatalf("Failed to create interpreter: %v", err)
    }

    // Script to be executed
    script := `
        package main
        func main() {
            return 1 + 2
        }
    `

    // Load and evaluate the script
    if err := interp.LoadFile("script.mgo", []byte(script)); err != nil {
        log.Fatalf("Failed to load script: %v", err)
    }
    if _, err := interp.Eval(context.Background()); err != nil {
        log.Fatalf("Failed to eval script: %v", err)
    }

    // Call the 'main' function and get the result
    result, err := interp.Call(context.Background(), "main")
    if err != nil {
        log.Fatalf("Failed to call main: %v", err)
    }

    // Unmarshal the result into a Go variable
    var val int
    if err := result.As(&val); err != nil {
        log.Fatalf("Failed to unmarshal result: %v", err)
    }

    fmt.Printf("Script result: %d\n", val) // Output: Script result: 3
}
```

## Limitations

-   **No Concurrency**: `minigo` does not support `goroutine`s or `channel`s.
-   **Incomplete Language Support**: While many Go features are supported, some complex aspects of the language may not be fully implemented. Direct source interpretation is limited by the set of language features the interpreter can understand.
