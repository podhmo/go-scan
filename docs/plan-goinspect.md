# Plan: `examples/goinspect` - A Call Graph Explorer using `symgo`

This document outlines the plan for creating a new example tool, `goinspect`. This tool will analyze Go source code and display the call graph for specified functions, demonstrating the capabilities of the `symgo` symbolic execution engine.

## 1. Overview

The goal of `goinspect` is to provide a simple command-line utility that inspects Go source code and generates a human-readable call graph. Given a starting function, it traces all subsequent function calls within the defined scope, presenting them in a hierarchical, indented format.

This tool will serve as a practical example of how to leverage the `symgo` engine. Instead of manually traversing the AST, which can be inaccurate for complex cases (like methods on embedded types or interface method calls), `goinspect` will use `symgo`'s powerful symbolic execution capabilities to trace execution paths and build a precise call graph.

The target output, inspired by `podhmo/goinspect`, looks like this:

```
package github.com/podhmo/goinspect/internal/x

  func x.F(s x.S)
    func x.log() func()
    func x.F0()
      func x.log() func()
      func x.F1()
        func x.H()
    func x.H()
```

## 2. Functional Requirements

- **Package Targeting**: The user must be able to specify one or more Go packages to analyze (e.g., `./...`).
- **Function Filtering**: The user should be able to filter the initial set of functions to analyze. For example:
    - Analyze only exported functions.
    - Analyze all functions, including unexported ones.
- **Hierarchical Output**: The output must be a text-based, indented tree representing the call graph.
- **Accurate Signatures**: The output should display the correct function signatures, including package names.

## 3. Implementation Plan

The implementation will be broken down into four main parts.

### a. Command-Line Interface (CLI)

- Use the standard `flag` package to create the command-line interface.
- Define flags to control the tool's behavior:
    - `--pkg <pattern>`: (Required) A glob pattern for the target packages (e.g., `github.com/user/repo/pkg/...`).
    - `--include-unexported`: (Optional) A boolean flag to include unexported functions as analysis entry points. Defaults to `false`.
    - `// TODO`: Consider adding a flag to specify a single function name as the entry point.

### b. Package Scanning

- Use the `goscan` library to locate and parse the target packages specified by the `--pkg` flag.
- This will produce the `*packages.Package` objects and ASTs necessary for `symgo`.

### c. Call Graph Construction

This is the core of the tool and will rely heavily on `symgo`.

1.  **Initialize `symgo`**: Create a new `symgo.Evaluator` instance. The `Resolver` will be configured with the packages scanned in the previous step.
2.  **Define Entry Points**: Iterate through the functions in the scanned packages. Based on the `--include-unexported` flag, create a list of `scanner.FunctionInfo` objects to serve as the starting points for the analysis.
3.  **Trace Execution**: For each entry point function:
    - Start symbolic execution using the `symgo` evaluator.
    - The key challenge is to capture the call graph. We will need a mechanism to hook into the `symgo` engine's evaluation process to be notified whenever a function is called. This can be achieved by providing a `Trace` function in the `symgo.Config` or by creating a custom `symgo.Visitor` that observes `object.Call` events.
    - We will build a graph data structure (e.g., a map `map[FunctionInfo][]FunctionInfo`) to store the caller-callee relationships.
4.  **Handle Recursion**: The call graph must handle recursive calls gracefully. `symgo`'s built-in bounded recursion will prevent infinite loops during analysis. The output formatting step will need to detect cycles in the graph to avoid infinite printing.

### d. Output Formatting

- Once the call graph data structure is populated, write a recursive function to traverse it.
- The function will take a `FunctionInfo` and an indentation level as arguments.
- It will print the current function's signature with the appropriate indentation, then recursively call itself for all functions that the current function calls.

## 4. `symgo` Usage Details

The power of this tool comes from `symgo`. Here’s how we plan to use it:

- **Evaluator Setup**: We will initialize the evaluator using `symgo.New(config)`. The `config` will be critical.
    - `symgo.Config.Resolver`: This will be initialized with a `goscan.Resolver` that knows about our target packages.
    - `symgo.Config.Trace`: We will likely provide a custom `io.Writer` or a function to this field. The `symgo` engine writes detailed trace information about its execution steps. We can parse this trace to find function call events.
- **Call Detection**: The most direct way to detect calls is to analyze the evaluation results. The `applyFunction` internal method is the heart of call execution in `symgo`. While we cannot call it directly, we can observe its effects. When `Eval` is called on an `ast.CallExpr`, it eventually triggers `applyFunction`. By implementing a `Visitor` or using a `Trace` hook, we can intercept the arguments to and results from this process, giving us the caller and callee information.
- **State Management**: `symgo`'s `Context` object manages the call stack. This can be inspected to understand the current call depth, which is useful for debugging and potentially for the tracing mechanism itself.

## 5. Open Issues and Considerations

- **Interface Method Calls**: `symgo` traces calls to interface methods symbolically. The output should ideally represent this ambiguity, perhaps by listing the known implementations or simply noting the call is on an interface. The initial version may just show the call to the interface method itself.
- **`go` and `defer`**: `symgo` evaluates the function calls within `go` and `defer` statements immediately. Our tracer should capture these as standard calls, but the output could optionally annotate them (e.g., `(go) myFunc()`, `(defer) cleanup()`).
- **External Libraries**: Calls to functions in packages outside the analysis scope (e.g., the standard library) will be treated as terminal nodes by `symgo`. Our tool will simply display these calls as the final leaves in a branch of the call graph, which is the correct and desired behavior.
- **Performance**: For very large codebases, analyzing every function could be slow. Future versions might include options to set the analysis depth or to focus the analysis on specific functions by name.