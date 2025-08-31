# minigo

`minigo` is a simple, embeddable script engine for Go applications, designed primarily to serve as a powerful and type-safe **configuration language**. It interprets a curated subset of the Go language, allowing developers to write dynamic configurations with familiar syntax.

`minigo` is powered by `go-scan`, which allows it to understand Go source code without relying on the heavier `go/types` or `go/packages` libraries. It uses an AST-walking interpreter to execute scripts.

## Simple Usage

The easiest way to use `minigo` is with the `Run` function. It allows you to execute a script and unmarshal the result into a Go struct in a single call.

#### 1. Define Your Go Struct

First, define the Go struct you want to populate from the script.

```go
// main.go
package main

type AppConfig struct {
    ListenAddr   string
    TimeoutSec   int
    FeatureFlags []string
}

func GetDefaultPort() int {
    return 8080
}
```

#### 2. Write the Configuration Script

The script uses Go-like syntax. It can call Go functions (like `GetDefaultPort`) and access variables (like `env`) that you provide.

```go
// config.mgo
package main

// GetConfig returns the application configuration.
func GetConfig() {
    // The struct returned here will be matched with the Go AppConfig struct
    // by reflection during the `result.As()` call.
    return struct{
        ListenAddr   string
        TimeoutSec   int
        FeatureFlags []string
    }{
        ListenAddr:   "0.0.0.0",
        TimeoutSec:   GetDefaultPort(),
        FeatureFlags: []string{"new_ui", "enable_metrics"},
    }
}
```

#### 3. Run the Interpreter

Use `minigo.Run` to execute the script and `result.As()` to populate your struct.

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
    script := `
package main

func GetConfig() {
    return struct{
        ListenAddr   string
        TimeoutSec   int
        FeatureFlags []string
    }{
        ListenAddr:   "0.0.0.0",
        TimeoutSec:   GetDefaultPort(),
        FeatureFlags: []string{"new_ui", "enable_metrics"},
    }
}
`
	// Run the interpreter.
	result, err := minigo.Run(context.Background(), minigo.Options{
		Source:     []byte(script),
		EntryPoint: "GetConfig",
		Globals: map[string]any{
			"GetDefaultPort": GetDefaultPort,
		},
	})
	if err != nil {
		log.Fatal(err) // The error will be nicely formatted with a stack trace.
	}

	// Extract the result into our Go struct.
	var cfg AppConfig
	if err := result.As(&cfg); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Configuration loaded: %+v\n", cfg)
	// Expected Output: Configuration loaded: {ListenAddr:0.0.0.0 TimeoutSec:8080 FeatureFlags:[new_ui enable_metrics]}
}

```

## Key Features

- **Familiar Syntax**: Write configurations using a subset of Go's syntax.
- **Type-Safe Unmarshaling**: Directly populate your Go structs from script results using reflection.
- **Go Interoperability**: Inject Go variables and functions from your host application into the script's global scope.
- **Lazy Imports**: To ensure fast startup, package imports are only parsed when a symbol from that package is first accessed.
- **Clear Error Reporting**: Provides formatted stack traces on runtime errors, making it easier to debug scripts.

## Supported Language Features

`minigo` supports a significant subset of the Go language, focusing on features useful for scripting and configuration.

#### Supported
- **Variables**: `var`, short assignment `:=`, `const`, and `iota`.
- **Basic Types**: `int`, `float`, `string`, `bool`. (Note: All integer types are treated as `int64`, all floats as `float64`).
- **Composite Types**: Structs (`type T struct`), slices (`[]T`), and maps (`map[K]V`).
- **Control Flow**: `if/else`, `for` loops (all forms), `switch` statements, `break`, and `continue`.
- **`for...range`**: Works on slices, maps, and integers (e.g., `for i := range 10`).
- **Functions**: User-defined functions, `return` statements, and closures.
- **Pointers**: Full support for pointers (`&`, `*`) and the `new()` built-in.
- **Methods**: Defining methods on structs.
- **Structs**: Field access, assignment, and struct literals (keyed and unkeyed).
- **Interfaces**: Interface definitions and dynamic dispatch are supported.
- **Generics**: Basic support for generic functions and types.
- **Built-ins**: `len`, `cap`, `append`, `make`, `new`, `panic`, and `recover`.
- **Imports**: `import` statements for standard library packages (via FFI or source) and other in-memory scripts.
- **Error Handling**: `defer`, `panic`, and `recover` for structured error handling and resource management.

#### Not Supported
- **Concurrency**: `go` statements, `chan` types, and `select` statements. `minigo` is a single-threaded interpreter.
- **Unsafe Operations**: The `unsafe` package is not supported.

## Go Interoperability

You can easily bridge your Go application and `minigo` scripts.

### Injecting Go Code with `Globals`
The `minigo.Options.Globals` map allows you to inject Go variables and functions directly into the script's global scope.

- **Variables**: Any Go variable (struct, map, slice, primitive) can be passed. The script will receive it as a `minigo` object.
- **Functions**: Any Go function can be passed. `minigo` automatically wraps it in a callable builtin, handling type conversions for arguments and return values.

### Extracting Results with `As()`
The `result.As(&myStruct)` method uses reflection to populate a Go struct from a `minigo` struct, map, or other object. It matches fields by name (case-insensitively) and performs type conversions.

## Advanced Usage: The Interpreter API

For more complex scenarios, such as multi-file scripts, a persistent environment, or custom package loading, you can use the `Interpreter` API directly.

```go
// Create a new interpreter.
interp, err := minigo.NewInterpreter(nil) // Pass a goscan.Scanner if needed
if err != nil {
    log.Fatalf("Failed to create interpreter: %v", err)
}

// Register Go functions to be accessible from the script via an import path.
interp.Register("strings", map[string]any{
    "Split": strings.Split,
})

// Load multiple script files into the interpreter's memory.
interp.LoadFile("config.mgo", script1)
interp.LoadFile("utils.mgo", script2)

// First, evaluate all loaded files to process top-level declarations.
if err := interp.EvalDeclarations(context.Background()); err != nil {
    log.Fatalf("Failed to eval script: %v", err)
}

// Now, call a specific entry point function.
result, err := interp.Call(context.Background(), "GetConfig")
if err != nil {
    log.Fatalf("Failed to call GetConfig: %v", err)
}

// ... then use result.As(&cfg) as before.
```

## Command-Line Tools

### `minigo gen-bindings`
`minigo` can interact with Go's standard library. The most reliable way is to generate FFI (Foreign Function Interface) bindings.

The `go run ./examples/minigo gen-bindings` command scans a compiled Go package and generates a Go file that registers that package's functions with the `minigo` interpreter, making them available to scripts. This is the preferred way to use packages like `strings`, `bytes`, `fmt`, etc.
