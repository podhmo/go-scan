# `symgo` - A Symbolic Execution Engine for Go

`symgo` is a library that performs symbolic execution on Go source code. It is designed to analyze code paths and understand the behavior of functions without actually running them. It builds an Abstract Syntax Tree (AST) and "evaluates" it, replacing concrete values with symbolic representations.

This is particularly useful for static analysis tools that need to understand program semantics, such as documentation generators, or tools for finding dead code.

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

## Basic Usage

Here is a simplified example of how to use the `symgo` interpreter to analyze a small piece of Go code. This demonstrates the setup process and how to inspect symbolic results.

```go
package main

import (
	"context"
	"fmt"
	"go/ast"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func main() {
	source := `
package myapp
func double(n int) int {
	return n * 2
}
func main() {
	y := double(10)
}`
	// 1. Set up go-scan, which symgo uses for parsing and type resolution.
	// We must use an overlay to provide the source code in-memory.
	s, err := goscan.New(
		goscan.WithOverlay(map[string][]byte{"myapp/main.go": []byte(source)}),
	)
	if err != nil {
		panic(err)
	}

	// 2. Create a new symgo interpreter.
	// We tell it to only analyze our "myapp" package.
	interp, err := symgo.NewInterpreter(s,
		symgo.WithPrimaryAnalysisScope("myapp"),
	)
	if err != nil {
		panic(err)
	}

	// 3. Scan the package. This parses the files and populates type information.
	pkg, err := s.ScanPackageByImport(context.Background(), "myapp")
	if err != nil {
		panic(err)
	}

	// 4. Evaluate the AST file. This populates the interpreter's scope with
	// top-level declarations like the `double` function.
	_, err = interp.Eval(context.Background(), pkg.AstFiles["myapp/main.go"], pkg)
	if err != nil {
		panic(err)
	}

	// 5. Find the `main` function and "apply" it to run the analysis.
	mainFnObj, ok := interp.FindObject("main")
	if !ok {
		panic("main function not found")
	}
	mainFn := mainFnObj.(*object.Function)

	_, err = interp.Apply(context.Background(), mainFn, nil, pkg)
	if err != nil {
		panic(err)
	}

	// 6. Inspect the results by looking up variables in the scope.
	yVar, ok := interp.FindObject("y")
	if !ok {
		panic("variable y not found")
	}

	// The value of 'y' is a symbolic placeholder because the multiplication
	// result is not a compile-time constant.
	fmt.Println("Symbolic value of y:", yVar.Inspect())
	// Output: Symbolic value of y: <symbolic: myapp.double(...)>
}
```

## A Practical Example: Understanding `net/http` with Intrinsics

Intrinsics are the key to building powerful analysis tools. They allow you to teach `symgo` about the semantics of functions, especially those in external libraries.

Consider the `docgen` tool, which generates OpenAPI specs from `net/http` code. On its own, `symgo` knows nothing about what `http.HandleFunc` *means*. It just sees a function call. `docgen` solves this by registering an intrinsic for `http.HandleFunc`.

**The Goal:** When `symgo` sees `http.HandleFunc("/users", myHandler)`, `docgen` needs to know that a new API route (`/users`) has been registered with a specific handler function (`myHandler`).

**The Solution:**
1.  **`docgen` registers an intrinsic:**
    ```go
    // In docgen's setup code:
    analyzer := &MyDocgenAnalyzer{}
    interpreter.RegisterIntrinsic("net/http.HandleFunc", analyzer.handleHandleFunc)
    ```

2.  **The intrinsic function is defined:**
    ```go
    // analyzer.handleHandleFunc is the intrinsic.
    // It receives the symbolic arguments passed to the original call.
    func (a *MyDocgenAnalyzer) handleHandleFunc(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
        // args[0] is the route pattern string, e.g., "/users"
        // args[1] is the handler function, e.g., myHandler

        routePattern := args[0].(*symgo.String).Value
        handlerFunc := args[1].(*symgo.Function)

        // The intrinsic can now use this information to build its own model.
        a.apiModel.AddRoute(routePattern, handlerFunc)

        // Intrinsics can return a value, or nil if the original
        // function returns void.
        return nil
    }
    ```
3.  **`symgo` evaluates the code:** When the interpreter encounters `http.HandleFunc("/users", myHandler)`, it finds the registered intrinsic. Instead of analyzing `http.HandleFunc`, it calls `analyzer.handleHandleFunc`, passing the symbolic representations of `"/users"` and `myHandler` as arguments.

This mechanism allows a tool to extract high-level semantic information from source code, which is the primary purpose of the `symgo` engine.
