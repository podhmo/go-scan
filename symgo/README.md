# `symgo` - A Symbolic Execution Engine for Go

`symgo` is a library that performs symbolic execution on Go source code. It is designed to analyze code paths and understand the behavior of functions without actually running them. It builds an Abstract Syntax Tree (AST) and "evaluates" it, replacing concrete values with symbolic representations.

This is particularly useful for static analysis tools that need to understand program semantics, such as documentation generators, security scanners, or tools for finding dead code.

## Core Concepts

- **Symbolic Execution**: Instead of using actual values (like the number `10`), `symgo` uses symbols (like `x`). When it encounters an operation (e.g., `x + 5`), it records the operation on the symbol (`x+5`) rather than computing a final value.

- **Objects**: The engine represents all values, symbols, and types as `object.Object`. This includes concrete values (`object.String`, `object.Int`) and symbolic ones (`object.SymbolicPlaceholder`, `object.Variable`).

- **Scope**: `symgo` maintains a lexical scope to track variables, functions, and types. This allows it to resolve identifiers correctly as it traverses the code.

- **Intrinsics**: `symgo` allows you to register "intrinsic" functions. These are special Go functions that the symbolic engine can call when it encounters a call to a specific function in the source code (e.g., `http.HandleFunc`). The intrinsic can then inspect the symbolic arguments and record information about the call.

## Managing Analysis Scope

A critical aspect of `symgo` is managing the **analysis scope**. By default, `symgo` will attempt to analyze the full source code of any function it encounters. This can lead to two problems:

1.  **Performance**: Analyzing an entire dependency tree, including the Go standard library, can be extremely slow.
2.  **Errors**: `symgo` does not have built-in knowledge of every standard library function (especially those related to I/O, reflection, or CGo). If it encounters a function it cannot analyze and is not configured to ignore, **the symbolic execution will fail**.

To manage this, you must define a clear boundary for the analysis. `symgo` provides three mechanisms for this, which can be used together.

### 1. `WithPrimaryAnalysisScope(patterns ...string)`

This is the **primary and recommended** way to define the analysis scope. It takes a list of Go import path patterns (e.g., `"example.com/mymodule/..."`) that `symgo` should analyze deeply. Any package that does not match these patterns will not have its function bodies evaluated. `symgo` will still see the function signatures, but will treat calls to them as "black boxes," returning a symbolic placeholder instead of executing them.

### 2. `WithSymbolicDependencyScope(patterns ...string)`

This is a performance optimization that works with the underlying `go-scan` engine. It is very similar to `goscan.WithDeclarationsOnlyPackages`. It tells the scanner to parse only the *declarations* (types, function signatures, constants, etc.) for the given package patterns, while completely discarding function bodies before `symgo` even sees them.

This is highly efficient for large external dependencies (like `net/http` or `database/sql`) where you need the type information to be available for compilation and type checking, but want to prevent `symgo` from ever attempting to analyze their complex internal logic.

### 3. `WithScanPolicy(policy ScanPolicyFunc)`

This is a low-level and powerful option that provides a function to manually control the scan policy on a per-package basis. The policy function receives a package path and returns `true` if the package should be scanned deeply, and `false` otherwise.

This can be useful for complex scenarios where the pattern-based scopes are not sufficient. However, it is more verbose and can be more error-prone. It should be used with caution when a more fine-grained policy is required.

### Example Configuration

Here is a robust configuration for a tool analyzing a local module that depends on `net/http`.

```go
import (
    "github.com/podhmo/go-scan"
    "github.com/podhmo/go-scan/symgo"
)

// The module path of the code you want to analyze.
const myModulePath = "example.com/me/mymodule"

// 1. Configure the base scanner.
// We tell it to only parse declarations for `net/http`, improving performance.
goScanner, err := goscan.New(
    goscan.WithDeclarationsOnlyPackages([]string{"net/http"}),
)
if err != nil {
    // handle error
}

// 2. Configure the symgo interpreter.
interpreter, err := symgo.NewInterpreter(
    goScanner,
    // Tell symgo to only perform deep analysis on our own code.
    symgo.WithPrimaryAnalysisScope(myModulePath + "/..."),
)
if err != nil {
    // handle error
}

// The interpreter is now ready to analyze code from `myModulePath`.
// When it encounters a call to `http.HandleFunc`, it will see the function
// signature but will not try to execute its body, avoiding errors and
-// improving performance.
```

## Debuggability

The `symgo` interpreter includes a tracing mechanism to help debug the symbolic execution flow. By providing a `Tracer` implementation, you can monitor which AST nodes are being visited by the evaluator.

### Usage

1.  Define a type that implements the `symgo.Tracer` interface:

    ```go
    import "go/ast"

    type MyTracer struct {
        // ... any state you need
    }

    func (t *MyTracer) Visit(node ast.Node) {
        // Your logic here, e.g., log the node type or position
        // fmt.Printf("Visiting node: %T at %s\n", node, node.Pos())
    }
    ```

2.  When creating the interpreter, pass your tracer using the `WithTracer` option:

    ```go
    import "github.com/podhmo/go-scan/symgo"

    tracer := &MyTracer{}
    interpreter, err := symgo.NewInterpreter(scanner, symgo.WithTracer(tracer))
    // ...
    ```

    Alternatively, you can use the `symgo.TracerFunc` adapter for simple, function-based tracers:

    ```go
    var visitedNodes []ast.Node
    tracer := symgo.TracerFunc(func(node ast.Node) {
        visitedNodes = append(visitedNodes, node)
    })
    interpreter, err := symgo.NewInterpreter(scanner, symgo.WithTracer(tracer))
    ```

This allows you to gain insight into the evaluator's behavior, which is invaluable for debugging custom intrinsics or understanding why a particular code path is or isn't being taken.
