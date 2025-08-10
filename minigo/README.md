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

The main Go application creates an `Interpreter`, registers the Go functions and variables, loads and evaluates the script files, and then calls the entry point function.

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
