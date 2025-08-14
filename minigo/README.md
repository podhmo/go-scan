# minigo

`minigo` is a simple, embeddable script engine for Go applications, designed primarily to serve as a powerful and type-safe **configuration language**. It interprets a subset of the Go language, allowing developers to write dynamic configurations with familiar syntax.

## Core Concept

The primary goal of `minigo` is to replace static configuration files like YAML or JSON with dynamic Go-like scripts. Its key feature is the ability to execute a script and **unmarshal the result directly into a Go struct** in a type-safe manner. This provides the flexibility of a real programming language for your configurations, without sacrificing integration with your Go application's static types.

`minigo` is powered by `go-scan`, which allows it to understand Go source code without relying on the heavier `go/types` or `go/packages` libraries. It uses an AST-walking interpreter to execute scripts.

## Key Features

- **Familiar Syntax**: Write configurations using a subset of Go's syntax, including variables, functions, `if` statements, and `for` loops.
- **Type-Safe Unmarshaling**: Directly populate your Go structs from script results.
- **Go Interoperability**: Inject Go variables and functions from your host application into the script's environment.
- **Lazy Imports**: To ensure fast startup and efficient execution, package imports are loaded lazily. Files from an imported package are only read and parsed when a symbol from that package is accessed for the first time.
- **Go Interoperability**: Inject Go variables and functions from your host application into the script's environment.
- **Lazy Imports**: To ensure fast startup and efficient execution, package imports are only read and parsed when a symbol from that package is accessed for the first time.
- **Special Forms (Macros)**: Register Go functions that receive the raw AST of their arguments, enabling the creation of custom DSLs and control structures without evaluating the arguments beforehand.
- **Generics**: Supports generic structs, functions, and type aliases.
- **Clear Error Reporting**: Provides formatted stack traces on runtime errors, making it easier to debug configuration scripts.

## Usage

Here is a conceptual example of how to use `minigo` to load an application configuration by calling a specific function within a script.

#### 1. Define Your Go Types and Functions

First, define the Go struct you want to populate and any Go functions or variables you want to expose to the script.

```go
// config.go
package main

// AppConfig is the Go struct we want to populate from the script.
type AppConfig struct {
    ListenAddr   string
    TimeoutSec   int
    FeatureFlags []string
}

// GetDefaultPort is a Go function we want to make available in the script.
func GetDefaultPort() string {
    return ":8080"
}
```

#### 2. Write the Configuration Script

The script itself is written in a file (e.g., `config.mgo`) and uses Go-like syntax. It defines an entry point function, like `GetConfig`, that returns the configuration.

```go
// config.mgo
package main

import "strings"

// GetConfig is the entry point function that returns the config.
// It can call Go functions (GetDefaultPort) and access Go variables (env)
// that are registered with the interpreter.
func GetConfig() {
    // The struct returned here will be matched with the Go AppConfig struct
    // by reflection during the `result.As()` call.
    return struct {
        ListenAddr   string
        TimeoutSec   int
        FeatureFlags []string
    }{
        ListenAddr:   GetDefaultPort(),
        TimeoutSec:   30,
        FeatureFlags: strings.Split(env.FEATURES, ","),
    }
}
```

#### 3. Run the Interpreter

The main Go application creates an `Interpreter`, registers the Go functions and variables, loads and evaluates the script files, and then calls the desired entry point function. This entry point can be any function defined in the script, not just `main`. This allows for flexible script designs, such as having different configuration functions for different environments (e.g., `GetDevConfig`, `GetProdConfig`).

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
    // Create a new interpreter.
    interp, err := minigo.NewInterpreter()
    if err != nil {
        log.Fatalf("Failed to create interpreter: %v", err)
    }

    // Register Go functions and variables to be accessible from the script.
    // Here, we expose `GetDefaultPort` and a map `env`.
    interp.Register("main", map[string]any{
        "GetDefaultPort": GetDefaultPort,
        "env": map[string]string{
            "FEATURES": "new_ui,enable_metrics",
        },
    })
    // We also register the `strings` package functions.
    interp.Register("strings", map[string]any{
        "Split": strings.Split,
    })

    // Load the script file into the interpreter's memory.
    // For multi-file scripts, call LoadFile for each file.
    script, err := os.ReadFile("config.mgo")
    if err != nil {
        log.Fatalf("Failed to read script: %v", err)
    }
    if err := interp.LoadFile("config.mgo", script); err != nil {
        log.Fatalf("Failed to load script: %v", err)
    }

    // First, evaluate all loaded files to process top-level declarations
    // (like function definitions).
    if _, err := interp.Eval(context.Background()); err != nil {
        log.Fatalf("Failed to eval script: %v", err)
    }

    // Now, call the specific entry point function.
    result, err := interp.Call(context.Background(), "GetConfig")
    if err != nil {
        log.Fatalf("Failed to call GetConfig: %v", err)
    }

    // Extract the result into our Go struct.
    var cfg AppConfig
    if err := result.As(&cfg); err != nil {
        log.Fatalf("Failed to unmarshal result: %v", err)
    }

    fmt.Printf("Configuration loaded: %+v\n", cfg)
    // Expected Output: Configuration loaded: {ListenAddr::8080 TimeoutSec:30 FeatureFlags:[new_ui enable_metrics]}
}
```

## Advanced: Special Forms

A "special form" is a function that receives the abstract syntax tree (AST) of its arguments directly, instead of their evaluated results. This is a powerful, low-level feature that allows you to create custom Domain-Specific Languages (DSLs) or new control flow structures within the `minigo` language.

You can register a special form using `interp.RegisterSpecial()`.

### Example: An Assertion Special Form

Imagine you want a function `assert(expression)` that only evaluates the expression if assertions are enabled. A regular function would always evaluate the expression before it is called. A special form can inspect the expression's AST and decide whether to evaluate it.

#### 1. Define the Special Form in Go

The special form function receives the raw `[]ast.Expr` slice for its arguments.

```go
// main.go
import (
    "go/ast"
    "go/token"
    "github.com/podhmo/go-scan/minigo/object"
)

var assertionsEnabled = true // Your application's toggle

// ... in your main function ...

// Register a special form named 'assert'.
interp.RegisterSpecial("assert", func(ctx *object.BuiltinContext, pos token.Pos, args []ast.Expr) object.Object {
    if !assertionsEnabled {
        return object.NIL // Do nothing if assertions are off.
    }
    if len(args) != 1 {
        return ctx.NewError(pos, "assert() requires exactly one argument")
    }

    // Since we have the AST, we can now choose to evaluate it.
    // This requires access to the interpreter's internal eval function.
    // (Note: Exposing the evaluator for this is an advanced use case.)
    // For simplicity, this example just returns a boolean based on the AST type.
    if _, ok := args[0].(*ast.BinaryExpr); ok {
        // In a real implementation, you would evaluate this expression.
        // For this example, we'll just confirm we received the AST.
        fmt.Println("Assertion expression is a binary expression!")
    }

    return object.NIL
})
```

#### 2. Use it in a Script

The script can now call `assert` with any expression. The expression `1 + 1 == 2` is not evaluated by `minigo` before being passed to the Go implementation of `assert`.

```go
// my_script.mgo
package main

func main() {
    assert(1 + 1 == 2)
}
```

## Limitations

While `minigo` is a powerful tool for configuration and scripting, it is not a full-featured Go environment and has some important limitations:

-   **No Concurrency**: `minigo` does not support `goroutine`s or `channel`s. The interpreter is single-threaded. For a detailed analysis, see [`docs/analysis-minigo-goroutine.md`](../docs/analysis-minigo-goroutine.md).
-   **`encoding/json` Support**: Full support for `json.Marshal` and `json.Unmarshal` is provided. This includes marshaling MiniGo structs to JSON and unmarshaling JSON into MiniGo structs, with support for nested, recursive, and cross-package struct definitions.
