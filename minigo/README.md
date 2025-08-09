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
- **Clear Error Reporting**: Provides formatted stack traces on runtime errors, making it easier to debug configuration scripts.

## Basic Usage

Here is a conceptual example of how to use `minigo` to load an application configuration.

#### 1. Your Go Application

First, define the Go struct you want to populate and any Go functions or variables you want to expose to the script.

```go
// main.go
package main

import (
    "context"
    "fmt"
    "github.com/podhmo/go-scan/minigo"
)

// This is the Go struct we want to populate from the script.
type AppConfig struct {
    ListenAddr   string
    TimeoutSec   int
    FeatureFlags []string
}

// A Go function we want to make available in the script.
func GetDefaultPort() string {
    return ":8080"
}

func main() {
    // The minigo script, acting as a configuration file.
    script := `
package main

import "strings"

// The entry point function that returns the config.
func GetConfig() {
    // This is the struct we will return.
    // Its definition is not in this script; it will be matched by reflection.
    return {
        ListenAddr: GetDefaultPort(), // Call a Go function from the script.
        TimeoutSec: 30,
        FeatureFlags: strings.Split(env.FEATURES, ","), // Use an injected variable.
    }
}
`
    // Go variables to inject into the script's global scope.
    injectedVars := map[string]any{
        "env": map[string]string{
            "FEATURES": "new_ui,enable_metrics",
        },
        "GetDefaultPort": GetDefaultPort, // Inject the Go function.
    }

    // Run the interpreter.
    result, err := minigo.Run(context.Background(), minigo.Options{
        Source:     []byte(script),
        Filename:   "config.mgo",
        EntryPoint: "GetConfig",
        Globals:    injectedVars,
    })
    if err != nil {
        panic(err) // The error will be nicely formatted with a stack trace.
    }

    // Extract the result into our Go struct.
    var cfg AppConfig
    if err := result.As(&cfg); err != nil {
        panic(err)
    }

    fmt.Printf("Configuration loaded: %+v\n", cfg)
    // Expected Output: Configuration loaded: {ListenAddr::8080 TimeoutSec:30 FeatureFlags:[new_ui enable_metrics]}
}
```

#### 2. The Configuration Script

The script itself is written in a file (e.g., `config.mgo`) and uses Go-like syntax.

```go
// config.mgo
package main

import "strings"

// The entry point function that returns the config.
func GetConfig() {
    // This is the struct we will return.
    // Its definition is not in this script; it will be matched by reflection.
    return {
        ListenAddr: GetDefaultPort(), // Call a Go function from the script.
        TimeoutSec: 30,
        FeatureFlags: strings.Split(env.FEATURES, ","), // Use an injected variable.
    }
}
```

When the Go program is run, it will execute the `GetConfig` function from the script, using the injected `GetDefaultPort` function and `env` variable, and then unmarshal the returned struct into the `cfg` variable.
