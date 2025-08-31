# `symgo` - A Symbolic Execution Engine for Go

`symgo` is a library that performs symbolic execution on Go source code. It is designed to analyze code paths and understand the behavior of functions without actually running them. It builds an Abstract Syntax Tree (AST) and "evaluates" it, replacing concrete values with symbolic representations.

This is particularly useful for static analysis tools that need to understand program semantics, such as documentation generators, security scanners, or tools for finding dead code.

## Core Concepts

- **Symbolic Execution**: Instead of using actual values (like the number `10`), `symgo` uses symbols (like `x`). When it encounters an operation (e.g., `x + 5`), it records the operation on the symbol (`x+5`) rather than computing a final value.

- **Objects**: The engine represents all values, symbols, and types as `object.Object`. This includes concrete values (`object.String`, `object.Int`) and symbolic ones (`object.SymbolicPlaceholder`, `object.Variable`).

- **Scope**: `symgo` maintains a lexical scope to track variables, functions, and types. This allows it to resolve identifiers correctly as it traverses the code.

- **Intrinsics**: `symgo` allows you to register "intrinsic" functions. These are special Go functions that the symbolic engine can call when it encounters a call to a specific function in the source code (e.g., `http.HandleFunc`). The intrinsic can then inspect the symbolic arguments and record information about the call.

- **Analysis Scopes**: By default, `symgo` performs deep, symbolic execution only on packages within the current Go workspace. To provide fine-grained control over this behavior, `symgo` uses two main options:

  - **`WithPrimaryAnalysisScope(patterns ...string)`**: This is the **recommended** way to define the analysis scope. It takes a list of Go import path patterns (e.g., `"example.com/mymodule/..."`) that `symgo` should analyze deeply. Any package outside this scope will not have its function bodies evaluated.

  - **`WithSymbolicDependencyScope(patterns ...string)`**: This option tells the underlying `go-scan` engine to parse only the *declarations* (types, function signatures, etc.) for the given package patterns, while completely discarding function bodies. This is highly efficient for large external dependencies (like `net/http`) where you need type information but want to prevent `symgo` from getting lost in complex implementation details.

  - **`WithScanPolicy(policy ScanPolicyFunc)`**: (DEPRECATED) This option provides a function to manually control the scan policy. It is less expressive and more error-prone than the scope-based options and may be removed in the future.

  **Example:**

  ```go
  import "github.com/podhmo/go-scan/symgo"

  // Configure the interpreter to deeply analyze your module's code
  // and treat `net/http` as a symbolic dependency.
  interpreter, err := symgo.NewInterpreter(
      goScanner, // A pre-configured go-scan.Scanner
      symgo.WithPrimaryAnalysisScope("example.com/me/mymodule/..."),
      symgo.WithSymbolicDependencyScope("net/http"),
  )
  ```

## Interaction with `go-scan` Options

`symgo`'s behavior is heavily influenced by the underlying `go-scan.Scanner` it is given. One particularly important option is `goscan.WithDeclarationsOnlyPackages`.

### Declarations-Only Scanning

For performance and stability, especially when analyzing code that depends on large packages like `net/http`, you may not want `symgo` to evaluate the entire implementation of that package. However, you still need its type definitions and function signatures to be available.

You can achieve this by configuring the `goscan.Scanner` with `WithDeclarationsOnlyPackages`. For any package specified with this option, `go-scan` will parse all top-level declarations but will then discard the function bodies before `symgo` sees the AST.

When `symgo` encounters a call to a function from such a package, it will see that the function has no body (`Body: nil`). The evaluator handles this gracefully, treating the function as a no-op and returning a symbolic placeholder for its result. This allows analysis to continue without getting lost in the complexity of external code, while still having access to all necessary type information.

**Example:**

```go
// In your tool's setup code:
import "github.com/podhmo/go-scan"

// Configure the main scanner
goScanner, err := goscan.New(
    // ... other options
    goscan.WithDeclarationsOnlyPackages([]string{"net/http", "database/sql"}),
)

// Now, create the symgo interpreter with this scanner.
// symgo will automatically respect the declarations-only setting.
interpreter, err := symgo.NewInterpreter(goScanner)
```

This approach is the recommended way to handle large, well-known dependencies that you don't need to analyze deeply.

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

Here is a simplified example of how to use the `symgo` interpreter to analyze a small piece of Go code.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
)

func main() {
	source := `
package main

func double(n int) int {
	return n * 2
}

func main() {
	x := 10
	y := double(x)
}
`
	// 1. Set up go-scan, which symgo uses for parsing and type resolution.
	s, err := goscan.New(goscan.WithGoModuleResolver())
	if err != nil {
		panic(err)
	}

	// 2. Create a new symgo interpreter.
	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		panic(err)
	}

	// 3. Parse the source code into an in-memory file.
	f, fset, err := s.ParseFile(context.Background(), "main.go", source)
	if err != nil {
		panic(err)
	}

	// 4. Evaluate the file. This populates the interpreter's scope with
	// top-level declarations like the `double` function.
	_, err = interp.Eval(context.Background(), f, &goscan.Package{Fset: fset, AstFiles: map[string]*ast.File{"main.go": f}})
	if err != nil {
		panic(err)
	}

	// 5. Find the `main` function and "apply" it to run the analysis.
	mainFn, ok := interp.FindObject("main")
	if !ok {
		panic("main function not found")
	}
	_, err = interp.Apply(context.Background(), mainFn.(*symgo.Function), nil, nil)
	if err != nil {
		panic(err)
	}

	// 6. Inspect the results by looking up variables in the scope.
	y, ok := interp.FindObject("y")
	if !ok {
		panic("variable y not found")
	}

	fmt.Println("Symbolic value of y:", y.Inspect())
}
```
