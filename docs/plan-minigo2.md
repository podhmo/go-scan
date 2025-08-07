# Plan for minigo2 Implementation

## 1. Introduction and Concept

**minigo2** will be a reimplementation of the `minigo` concept, evolving from a simple example into a robust, embeddable script engine for Go applications. The primary use case for `minigo2` is to serve as a **configuration language**. It will allow developers to write configuration files using a subset of Go syntax, which can then be interpreted at runtime by a Go application.

The core goals of `minigo2` are:
- **Readability and Familiarity**: Leverage Go-like syntax for configuration and scripting, making it intuitive for Go developers.
- **Go Interoperability**: Seamlessly pass data and functions between the host Go application and the `minigo2` script. The results of script evaluation should be easily accessible from Go, likely via the `reflect` package.
- **Rich Type System**: Utilize `go-scan` to understand and work with actual Go types defined within the host application or its dependencies.
- **Developer Experience**: Provide clear, human-readable stack traces on errors to simplify debugging of scripts.
- **Simplicity**: Prioritize clarity of implementation and ease of use over raw execution speed. The engine does not need to be a JIT or a highly optimized AOT compiler.

`minigo2` will **not** be a full Go implementation. It will interpret a curated subset of the language focused on its role as a configuration and scripting engine. It will strictly avoid `go/packages` and `go/types` in favor of the lazy, AST-based approach provided by `go-scan`.

## 2. Core Architecture

The `minigo2` engine will be composed of several key components:

![minigo2 Architecture](https://i.imgur.com/example.png)  <!-- Conceptual image placeholder -->

1.  **Entry Point (`minigo2.Run`)**: A simple, library-style API for executing scripts. It will accept the script source (as a string or file path) and an optional context for passing in Go variables and functions.

2.  **Parser (Powered by `go-scan`)**: `minigo2` will not have its own parser. Instead, it will use `go/parser` to get the AST of the script, and a dedicated `goscan.Scanner` instance to resolve types and parse imported Go packages. This allows `minigo2` to directly reuse Go's AST structures (`go/ast`).

3.  **Interpreter Engine (`eval` loop)**: The heart of `minigo2`. This component will be an AST-walking interpreter. It will traverse the AST provided by the parser and evaluate each node recursively.
    - It will manage the execution state, including scopes and environments.
    - It will handle control flow statements like `if`, `for`, and function calls.

4.  **Object System (`object.Object`)**: A set of internal types used to represent data within the interpreter at runtime. This will be similar to `minigo` but expanded for better Go interoperability.
    - `object.Integer`, `object.String`, `object.Boolean`
    - `object.StructInstance`, `object.Function`
    - A special `object.GoValue` that wraps a `reflect.Value`, allowing native Go values to be passed into the script.

5.  **Environment Model (`object.Environment`)**: A structure for managing variable and symbol lookups. It will support nested scopes to correctly handle global, function, and block-level variables.

6.  **Go Interop Layer (The `reflect` Bridge)**: This is a critical component for the configuration use case.
    - **Go -> minigo2**: It will allow the host application to "inject" Go variables and functions into the `minigo2` environment. These will be wrapped in `object.GoValue` or a `BuiltinFunction` equivalent.
    - **minigo2 -> Go**: It will provide a mechanism to extract the result of a script's evaluation and convert it from a `minigo2` internal object back into a Go variable using `reflect.Value.Set`.

7.  **Error Handling & Call Stack**: A mechanism to track function calls within the script. If an error occurs, this call stack will be used to generate a formatted, easy-to-read error message, including file names and line numbers.

## 3. Detailed Component Design

### 3.1. Entry Point API

The primary API will be a function within the `minigo2` package.

```go
package minigo2

// Options configures the interpreter environment.
type Options struct {
    // Globals allows injecting Go variables into the script's global scope.
    // The map key is the variable name in the script.
    // The value can be any Go variable, which will be made available via reflection.
    Globals map[string]any

    // Source is the script content.
    Source []byte

    // Filename is the name of the script file, used for error messages.
    Filename string

    // EntryPoint is the name of the function to execute.
    EntryPoint string
}

// Result holds the outcome of a script execution.
type Result struct {
    // Value is the raw minigo2 object returned by the script.
    Value object.Object

    // Error contains any runtime error, pre-formatted with a stack trace.
    Error error
}

// Run executes a minigo2 script.
func Run(ctx context.Context, opts Options) (*Result, error) {
    // ... implementation ...
}
```

### 3.2. Parsing with `go-scan`

- A `minigo2.Interpreter` struct will hold a `goscan.Scanner` instance.
- When `Run` is called, it will first use `go/parser.ParseFile` to get the AST for the user's script (`.mgo` file).
- The interpreter will walk the declarations of the script AST. When it encounters an `import` statement, it will use its internal `goscan.Scanner` to lazily `ScanPackageByImport` when a symbol from that package is first accessed (e.g., `fmt.Println`).
- `go-scan` will handle finding the package, parsing it, and returning its exported symbols (`PackageInfo`), which the interpreter will then load into its environment.

### 3.3. Object System and Go Interop

The `object.Object` interface will be the foundation. The key to interoperability is how we bridge `minigo2` objects and Go's `reflect.Value`.

- **From Go to minigo2**:
  - When `Options.Globals` is provided, the interpreter will iterate through the map.
  - For each entry, it will use `reflect.ValueOf(value)` to get a `reflect.Value`.
  - This `reflect.Value` will be wrapped in a new `object.GoValue{Value: reflect.Value}`.
  - If the value is a Go function, it will be wrapped in a `BuiltinFunction` that uses `reflect.Value.Call` to execute it. Arguments from the script will be converted from `object.Object` to `reflect.Value` before the call.

- **From minigo2 to Go**:
  - The `Result` object can have a helper method: `As(target any) error`.
  - `result.As(&myConfigStruct)` would work as follows:
    1. It takes a pointer `&myConfigStruct` as `target`.
    2. It uses `reflect.ValueOf(target).Elem()` to get the settable `reflect.Value` of `myConfigStruct`.
    3. It inspects the `result.Value` (which would be an `object.StructInstance`).
    4. It iterates through the fields of the `object.StructInstance` and uses `reflect` to find the corresponding field in `myConfigStruct` and set its value. This involves converting `minigo2` objects (`object.Integer`, `object.String`) back to standard Go types.

### 3.4. Error Handling and Stack Traces

This will be adapted directly from `minigo`.
- The `Interpreter` will have a `callStack []*CallFrame`.
- A `CallFrame` will store the function name and the position (`token.Position`) of the call site.
- When a function is entered, a frame is pushed to the stack. When it exits, the frame is popped.
- If `eval` encounters an error, it will be wrapped in a custom error type.
- This custom error will have a `Format()` method that iterates through the `callStack` to build a readable trace, including file, line, and column information from the `token.Position`.

## 4. Implementation Phases

The development of `minigo2` will be broken down into the following phases:

1.  **Phase 1: Core Interpreter and Basic Types**
    - [ ] Set up the project structure (`minigo2/`, `minigo2/object/`, etc.).
    - [ ] Define the `object.Object` interface and basic types: `Integer`, `String`, `Boolean`, `Null`.
    - [ ] Implement the basic `eval` loop that can evaluate simple expressions (literals, binary/unary expressions).
    - [ ] Write unit tests for all expression evaluations.

2.  **Phase 2: Variables, Scopes, and Control Flow**
    - [ ] Implement the `object.Environment` for managing scopes.
    - [ ] Add support for `var` declarations and assignments (`=`, `:=`).
    - [ ] Implement `if/else` statements.
    - [ ] Implement `for` loops (with conditions, no range-based yet).
    - [ ] Add support for `break` and `continue`.

3.  **Phase 3: Functions and Call Stack**
    - [ ] Implement user-defined functions (`func` declarations).
    - [ ] Implement the call stack mechanism for function calls.
    - [ ] Implement `return` statements.
    - [ ] Implement basic error formatting with the stack trace.

4.  **Phase 4: Integration with `go-scan` for Imports**
    - [ ] Create the main `Interpreter` struct that holds a `goscan.Scanner`.
    - [ ] Implement the logic to handle `import` statements.
    - [ ] Implement `evalSelectorExpr` (e.g., `pkg.Symbol`) to trigger `go-scan`'s `ScanPackageByImport`.
    - [ ] Load constants and functions from scanned packages into the environment.
    - [ ] Test importing and using functions/constants from standard library packages (e.g., `strings.Join`).

5.  **Phase 5: Structs and Go Interop Layer**
    - [ ] Add support for `type ... struct` declarations and struct literal instantiations.
    - [ ] Implement the `object.GoValue` to wrap `reflect.Value`.
    - [ ] Implement the logic to inject Go variables from `Options.Globals`.
    - [ ] Implement the logic to wrap Go functions as `BuiltinFunction`s.
    - [ ] Implement the `Result.As(target any)` method for extracting data back into Go structs.

6.  **Phase 6: Refinement and Documentation**
    - [ ] Thoroughly test all features, especially the `reflect` bridge.
    - [ ] Improve error messages and stack traces.
    - [ ] Write comprehensive documentation for the API and usage examples.
    - [ ] Ensure `make format` and `make test` pass cleanly.

## 5. Conceptual Usage Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/user/project/minigo2"
)

// This is the Go struct we want to populate from the script.
type AppConfig struct {
    ListenAddr string
    TimeoutSec int
    FeatureFlags []string
}

// A Go function we want to make available in the script.
func GetDefaultPort() string {
    return ":8080"
}

func main() {
    // The minigo2 script, acting as a configuration file.
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
    // The Go variable to inject into the script.
    injectedVars := map[string]any{
        "env": map[string]string{
            "FEATURES": "new_ui,enable_metrics",
        },
        "GetDefaultPort": GetDefaultPort, // Inject the Go function.
    }

    // Run the interpreter.
    result, err := minigo2.Run(context.Background(), minigo2.Options{
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
    // Output: Configuration loaded: {ListenAddr::8080 TimeoutSec:30 FeatureFlags:[new_ui enable_metrics]}
}
```
