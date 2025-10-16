# `symgo` - A Symbolic Execution Engine for Go

`symgo` is a library that performs symbolic execution on Go source code. It builds upon the **shallow, lazy scanning** capabilities of `go-scan` to analyze all possible code paths and understand the behavior of functions without actually running them. It parses Go source into an Abstract Syntax Tree (AST) and then "evaluates" it, replacing concrete values with symbolic representations.

This makes it a powerful tool for static analysis, such as generating API documentation, finding dead code, or tracing call graphs through complex interfaces.

## Core Concepts: A Tracer, Not an Interpreter

It is crucial to understand that `symgo` is **not a standard interpreter**. It does not execute code linearly. Instead, it is a **symbolic tracer** designed to discover what *could* happen in any possible code path.

- **Symbolic Execution**: Instead of using concrete values (like the number `10`), `symgo` uses symbols (like `x`). When it encounters an operation (e.g., `x + 5`), it records the operation on the symbol (`x+5`) rather than computing a final value.

- **Path Exploration**: `symgo` intentionally explores all branches of control flow.
    - An `if` statement evaluates **both** the `then` and `else` blocks to trace calls in each.
    - A `for` loop body is evaluated **exactly once** to find calls within it, avoiding infinite loops.
    - A `type switch` on an interface explores **every** case by creating a hypothetical symbolic instance of that type.
    - For more details, see `docs/analysis-symgo-implementation.md`.

- **Objects**: The engine represents all values—concrete and symbolic—as `object.Object` (e.g., `object.String`, `object.Variable`, `object.SymbolicPlaceholder`).

- **Intrinsics**: `symgo` allows you to register "intrinsic" functions. These are custom Go functions that the engine calls when it encounters a specific function in the source code (e.g., `http.HandleFunc`). The intrinsic can then inspect the symbolic arguments to record information about the call, effectively teaching the engine the semantics of library functions.

## Managing Analysis Scope

A critical aspect of using `symgo` is managing the **analysis scope**. Analyzing an entire dependency tree, including the Go standard library, is slow and error-prone. You must define a clear boundary for the analysis.

`symgo` provides two primary options for this:

1.  **`WithPrimaryAnalysisScope(patterns ...string)`**: **(Recommended)** This defines the packages that `symgo` should analyze deeply. It accepts Go import path patterns (e.g., `"example.com/mymodule/..."`). Any package *not* matching these patterns will be treated as a "black box": its function signatures will be available, but their bodies will not be executed.

2.  **`WithSymbolicDependencyScope(patterns ...string)`**: This is a performance optimization that tells the underlying `go-scan` engine to parse only the *declarations* (types, function signatures, etc.) for the given package patterns, completely discarding function bodies. This is highly effective for large external dependencies (like `net/http`) where you need type information but must prevent `symgo` from analyzing their complex internal logic.

### Example: Robust Configuration

Here is a robust configuration for a tool analyzing a local module that depends on `net/http`.

```go
import (
    "github.com/podhmo/go-scan"
    "github.com/podhmo/go-scan/symgo"
)

// The module path of the code you want to analyze.
const myModulePath = "example.com/me/mymodule"

// 1. Create the base scanner.
// The scanner itself doesn't need extra configuration in this case,
// as symgo will manage the dependency scope.
goScanner, err := goscan.New()
if err != nil {
    // handle error
}

// 2. Configure the symgo interpreter.
interpreter, err := symgo.NewInterpreter(
    goScanner,
    // Tell symgo to only perform deep analysis on our own code.
    symgo.WithPrimaryAnalysisScope(myModulePath + "/..."),
    // Tell symgo to treat `net/http` as a symbolic dependency,
    // loading its types but not its code.
    symgo.WithSymbolicDependencyScope("net/http"),
)
if err != nil {
    // handle error
}

// The interpreter is now ready. When it encounters a call to `http.HandleFunc`,
// it will see the function signature but will not try to execute its body,
// avoiding errors and improving performance.
```

## Advanced Features

### Memoization for Performance

For complex analyses where the same functions may be evaluated multiple times, you can enable memoization to cache the results of function analysis.

```go
interpreter, err := symgo.NewInterpreter(
    scanner,
    symgo.WithMemoization(true),
    // other options...
)
```
This is disabled by default to ensure predictable behavior for all tools but can provide a significant performance boost.

### Finalizing Analysis with `Finalize()`

After the main evaluation is complete, `symgo` may have a list of unresolved method calls on interfaces. The `Finalize()` method performs a post-analysis step to connect these interface calls to their concrete implementations based on the types that were observed during the evaluation.

**It is crucial to call `Finalize()` if you need to resolve call graphs involving interfaces.**

```go
// ... after all Eval() and Apply() calls ...
interpreter.Finalize(ctx)

// Now, the analysis results will include connections between
// interface method calls and their concrete implementations.
```

### Debugging with Tracers

`symgo` includes a tracing mechanism to help debug the symbolic execution flow. By providing a `Tracer` implementation, you can monitor which AST nodes are being visited.

```go
import "github.com/podhmo/go-scan/symgo"

// Use TracerFunc for a simple, function-based tracer.
tracer := symgo.TracerFunc(func(event symgo.TraceEvent) {
    fmt.Printf("Visiting node: %T at %s\n", event.Node, event.Node.Pos())
})

interpreter, err := symgo.NewInterpreter(scanner, symgo.WithTracer(tracer))
```

## Testing with `symgotest`

The `symgotest` package offers helpers to streamline testing of `symgo`-based analyses. It provides convenient functions to run the interpreter on in-memory source code and make assertions on the results.

```go
import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestMyAnalysis(t *testing.T) {
	source := `
package mypkg
func Add(x, y int) int {
    return x + y
}
`
	ctx := context.Background()
	r := symgotest.Run(t, ctx, source, "mypkg") // Run the analysis

	// Assert that the 'Add' function was found
	addFn, ok := r.Lookup("Add")
	if !ok {
		t.Fatal("Add function not found")
	}
	// ... more assertions on the function object
}
```

## Basic Usage

Here is a simplified example of using the `symgo` interpreter.

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
	// 1. Set up go-scan with an in-memory file overlay.
	s, err := goscan.New(
		goscan.WithOverlay(map[string][]byte{"myapp/main.go": []byte(source)}),
	)
	if err != nil {
		panic(err)
	}

	// 2. Create a new symgo interpreter.
	interp, err := symgo.NewInterpreter(s,
		symgo.WithPrimaryAnalysisScope("myapp"),
	)
	if err != nil {
		panic(err)
	}

	// 3. Scan the package to get its AST and type info.
	pkg, err := s.ScanPackageFromImportPath(context.Background(), "myapp")
	if err != nil {
		panic(err)
	}

	// 4. Evaluate the AST file. This populates the interpreter's scope
	// with top-level declarations like the `double` function.
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

	// 6. Inspect the results.
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