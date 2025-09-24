# Plan: `examples/goinspect` - A Call Graph Explorer using `symgo`

This document outlines the plan for creating a new example tool, `goinspect`. This tool will analyze Go source code and display the call graph for specified functions, demonstrating the capabilities of the `symgo` symbolic execution engine. Its primary purpose is to provide a high-level overview of call relationships for documentation and code understanding, similar to `godoc` but with a focus on call graphs.

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
- **Scoped Analysis**: The analysis must be restricted to a "primary analysis scope". Calls to packages outside this scope will be shown as terminal nodes.
- **Advanced Call Detection**: The tool must detect not only direct function/method calls but also calls to functions passed as arguments (higher-order functions).
- **Function Filtering**: The user should be able to filter the initial set of functions to analyze (e.g., exported only vs. all).
- **Accessor/Getter/Setter Distinction**: The output should visually distinguish simple functions like accessors, getters, and setters from more complex functions.
- **Flexible Output Formatting**:
    - **Default Hierarchical Output**: A text-based, indented tree representing the call graph.
    - **Short Format (`--short`)**: An abbreviated format that shortens function signatures, for example by replacing the argument list with `(...)`.
    - **Expanded/UID Format (`--expand`)**: A verbose format that shows the entire call graph without abbreviation. To handle cycles and repeated calls, each function will be assigned a unique ID (UID) upon its first appearance. Subsequent calls to the same function will be rendered as a reference, e.g., `#<id>`.

## 3. Implementation Plan

The implementation will be broken down into four main parts.

### a. Command-Line Interface (CLI)

- Use the standard `flag` package to create the command-line interface.
- Define flags to control the tool's behavior:
    - `--pkg <pattern>`: (Required) A glob pattern for the target packages that form the primary analysis scope.
    - `--include-unexported`: (Optional) A boolean flag to include unexported functions as analysis entry points. Defaults to `false`.
    - `--short`: (Optional) A boolean flag to enable the short output format.
    - `--expand`: (Optional) A boolean flag to enable the expanded, UID-based output format.
    - `// TODO`: Consider adding a flag to specify a single function name as the entry point.

### b. Package Scanning

- Use the `goscan` library to locate and parse the target packages specified by the `--pkg` flag.
- This will produce the `*packages.Package` objects and ASTs necessary for `symgo`.

### c. Call Graph Construction

This is the core of the tool and will rely heavily on `symgo`.

1.  **Initialize `symgo`**: Create a new `symgo.Evaluator` instance. The `Resolver` will be configured to strictly enforce the primary analysis scope defined by the `--pkg` patterns.
2.  **Define Entry Points**: Iterate through the functions in the scanned packages. Based on the `--include-unexported` flag, create a list of `scanner.FunctionInfo` objects to serve as the starting points for the analysis.
3.  **Trace Execution**: For each entry point function:
    - Start symbolic execution using the `symgo` evaluator.
    - **Higher-Order Functions**: Leverage `symgo`'s `scanFunctionLiteral` heuristic, which automatically analyzes anonymous functions passed as arguments. This will be key to detecting calls within them.
    - **Accessor/Getter/Setter Detection**: After building the graph, post-process each `scanner.FunctionInfo` node. Analyze the function's body to determine if it's a simple accessor (e.g., a single return statement of a struct field). This information will be stored with the node.
    - We will build a graph data structure (e.g., a map `map[FunctionInfo][]FunctionInfo`) to store the caller-callee relationships.
4.  **Handle Recursion**: The call graph must handle recursive calls gracefully. `symgo`'s built-in bounded recursion will prevent infinite loops during analysis. The output formatting step will use the UID mechanism in `--expand` mode to represent cycles.

### d. Output Formatting

- The output logic will be chosen based on the `--short` and `--expand` flags.
- **Default/Short Mode**: A recursive function will traverse the graph, printing function signatures. The `--short` flag will cause it to print `(...)` for arguments.
- **Expand Mode**:
    - A pre-traversal step will assign a unique integer ID to every function node in the graph.
    - A recursive traversal function will keep track of which function IDs have already been printed.
    - On the first visit to a node, it will print the full function signature and its children.
    - On subsequent visits, it will print a reference like `func my.Func(...) #<id>` and stop traversing.

## 4. `symgo` Usage Details

- **Evaluator Setup**: We will initialize the evaluator using `symgo.New(config)`.
    - **Scope Control**: The `WithPrimaryAnalysisScope` option for the `symgo.Resolver` will be used to strictly limit source-code analysis to the packages specified by the user. This is crucial for performance and for achieving the "scoped analysis" requirement.
- **Call Detection**:
    - **Direct Calls**: We will use a `Trace` hook or a `Visitor` to intercept call events, as originally planned.
    - **Higher-Order Function Calls**: `symgo` is designed to trace calls inside function literals passed as arguments. We need to ensure our tracing mechanism correctly captures the context, linking the call inside the literal back to the site where the higher-order function was called.

## 5. Open Issues and Considerations

- **Interface Method Calls**: `symgo` traces calls to interface methods symbolically. The output should ideally represent this ambiguity.
- **`go` and `defer`**: These will be captured as standard calls. The output could optionally annotate them.
- **External Libraries**: Calls to functions outside the primary analysis scope will be treated as terminal nodes, which is the desired behavior.
- **Accessor Heuristics**: Defining a robust heuristic for what constitutes an "accessor/getter/setter" will be important. It should probably be limited to functions with 1 or 2 statements that just access fields.

## 6. Testing Strategy

To ensure the tool is robust and correct, a comprehensive test suite is required. We will use a golden-file testing approach, where the output of the tool for a given set of inputs is compared against a pre-recorded `.golden` file.

The test cases should cover a wide range of Go language features and tool configurations:

### Analysis Scenarios
- **Basic Calls**: Simple function-to-function and method-to-method calls.
- **Interface Calls**: Calls on an interface method, testing that the symbolic call is recorded.
- **Embedded Types**: Calls to methods defined on an embedded struct.
- **Higher-Order Functions**: A function that accepts another function as an argument and calls it.
- **Anonymous Functions**: Calls inside an anonymous function literal.
- **`go` and `defer`**: Functions called within `go` and `defer` statements.
- **Recursive Calls**:
    - Direct recursion (`A -> A`).
    - Mutual recursion (`A -> B -> A`).
- **Scope Boundaries**: Calls to functions inside and outside the primary analysis scope to ensure external calls are treated as terminal nodes.
- **Complex Expressions**: Calls that are part of complex expressions (e.g., `foo(bar())`).

### Configuration Scenarios
- **CLI Flags**: Test various combinations of CLI flags to ensure they interact correctly.
    - `--include-unexported`
    - `--short`
    - `--expand`
    - `--short` and `--expand` used together (the tool should define a clear precedence, likely favoring `--expand`).
- **Output Formats**:
    - Verify the default hierarchical output is correct.
    - Verify the `--short` format correctly abbreviates signatures.
    - Verify the `--expand` format correctly assigns UIDs and uses them for subsequent calls and cycles.

## 7. Implementation Task List

This plan can be broken down into the following concrete implementation tasks.

- [ ] **1. Foundational CLI & Scanning**:
    - [ ] Create the `main` package and basic CLI structure using the `flag` package.
    - [ ] Implement the logic to parse the `--pkg` pattern and use `goscan` to load the packages.
    - [ ] Implement the `--include-unexported` flag to filter the initial list of functions.

- [ ] **2. Core Call-Graph Analysis**:
    - [ ] Initialize the `symgo.Evaluator` with the scanned packages and a strict primary analysis scope.
    - [ ] Implement the core tracing loop that iterates through entry-point functions.
    - [ ] Implement a `Visitor` or `Trace` hook to capture function call relationships and populate a graph data structure.

- [ ] **3. Basic Hierarchical Output**:
    - [ ] Implement a recursive printer that traverses the call graph.
    - [ ] Produce the default indented, hierarchical text output with full function signatures.

- [ ] **4. Advanced Features & Formatting**:
    - [ ] Implement the `--short` output format.
    - [ ] Implement the `--expand` output format, including UID assignment and reference (`#<id>`) rendering.
    - [ ] Implement a heuristic to detect simple accessor/getter/setter functions.
    - [ ] Add a visual marker (e.g., `[accessor]`) to the output for identified accessors.

- [ ] **5. Testing**:
    - [ ] Create a `testdata` directory with various Go files covering the scenarios in the "Testing Strategy" section.
    - [ ] Set up a golden-file testing framework.
    - [ ] Add golden-file tests for the default, `--short`, and `--expand` output formats.