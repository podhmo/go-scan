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
- **Orphan-Style Top-Level Output**: The tool should only display functions that are not called by any other function within the analysis scope as top-level entries in the output. Functions that are called by others should only appear nested within their callers. This ensures the output focuses on the true entry points of the program or library.
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
    - `--target <name>`: (Optional) The fully qualified name of a function or method to use as an entry point (e.g., `mypkg.MyFunc`, `(*mypkg.MyType).MyMethod`). Can be specified multiple times. If omitted, all exported functions in the scanned packages are used as entry points.
    - `--include-unexported`: (Optional) A boolean flag to include unexported functions as analysis entry points. Defaults to `false`. This is ignored if `--target` is used.
    - `--short`: (Optional) A boolean flag to enable the short output format.
    - `--expand`: (Optional) A boolean flag to enable the expanded, UID-based output format.

### b. Package Scanning

- Use the `goscan` library to locate and parse the target packages specified by the `--pkg` flag.
- This will produce the `*packages.Package` objects and ASTs necessary for `symgo`.

### c. Call Graph Construction

This is the core of the tool and will rely heavily on `symgo`.

1.  **Initialize `symgo`**: Create a new `symgo.Evaluator` instance. The `Resolver` will be configured to strictly enforce the primary analysis scope defined by the `--pkg` patterns.
2.  **Define Entry Points**:
    - If the `--target` flag is provided, the tool will search through all scanned packages to find the specific functions or methods that match the provided names. These matches will be the sole entry points for the analysis.
    - If `--target` is omitted, the tool will iterate through all functions in the scanned packages and use the exported ones (or all functions if `--include-unexported` is set) as the entry points.
3.  **Trace Execution**: For each entry point function, start symbolic execution using the `symgo` evaluator to build a complete call graph (e.g., a map `map[FunctionInfo][]FunctionInfo`).
4.  **Filter for Top-Level Functions**: After the full call graph is constructed:
    - Create a set of all functions that have been *called* at least once by iterating through the values of the call graph map.
    - Filter the initial list of entry points from step 2, keeping only those that are *not* present in the "callee" set. These are the true top-level functions.
5.  **Post-Processing**:
    - **Accessor/Getter/Setter Detection**: Analyze each `scanner.FunctionInfo` node in the graph. Analyze the function's body to determine if it's a simple accessor (e.g., a single return statement of a struct field). This information will be stored with the node.
6.  **Handle Recursion**: The call graph must handle recursive calls gracefully. `symgo`'s built-in bounded recursion will prevent infinite loops during analysis. The output formatting step will use the UID mechanism in `--expand` mode to represent cycles.

### d. Output Formatting

- The output logic will be chosen based on the `--short` and `--expand` flags.
- The recursive printer will be initialized with the filtered list of true top-level functions.
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

- [x] **1. Foundational CLI & Scanning**:
    - [x] Create the `main` package and basic CLI structure using the `flag` package.
    - [x] Implement the logic to parse the `--pkg` pattern and use `goscan` to load the packages.
    - [x] Implement the `--include-unexported` flag to filter the initial list of functions.

- [x] **2. Core Call-Graph Analysis**:
    - [x] Initialize the `symgo.Evaluator` with the scanned packages and a strict primary analysis scope.
    - [x] Implement the core tracing loop that iterates through entry-point functions.
    - [x] Implement a `Visitor` or `Trace` hook to capture function call relationships and populate a graph data structure.

- [x] **3. Basic Hierarchical Output**:
    - [x] Implement a recursive printer that traverses the call graph.
    - [x] Produce the default indented, hierarchical text output with full function signatures.

- [x] **4. Advanced Features & Formatting**:
    - [x] Implement the `--short` output format.
    - [x] Implement the `--expand` output format, including UID assignment and reference (`#<id>`) rendering.
    - [x] Implement a heuristic to detect simple accessor/getter/setter functions.
    - [x] Add a visual marker (e.g., `[accessor]`) to the output for identified accessors.

- [x] **5. Testing**:
    - [x] Create a `testdata` directory with various Go files covering the scenarios in the "Testing Strategy" section.
    - [x] Set up a golden-file testing framework.
    - [x] Add golden-file tests for the default, `--short`, and `--expand` output formats.
    - [x] Add golden-file tests for accessor detection and cross-package calls.

## 8. Known Limitations and Future Work

The development and testing of `goinspect` have highlighted several limitations in the underlying `symgo` engine. These issues prevent `goinspect` from generating a complete call graph in certain scenarios. Addressing them will require enhancements to `symgo` itself.

### a. `symgo` Engine Limitations

-   **Cross-Package Call Representation**: The symbolic execution engine does not currently represent calls to functions outside the primary analysis scope as terminal nodes in the call graph. For example, a call to `another.Helper()` from a `features` package is not captured if `another` is not in the primary scope. The expected behavior is for `symgo` to yield a symbolic placeholder for such calls, which `goinspect` could then display.

-   **Higher-Order and Anonymous Functions**: The engine has limited support for analyzing anonymous functions (function literals) passed as arguments to other functions. This leads to two problems in `goinspect`:
    1.  The function signature is not resolved correctly and may appear as `unhandled_type_*ast.FuncType`.
    2.  The symbolic execution does not trace calls made *inside* the anonymous function body.

### b. `goinspect` Future Enhancements

-   Once the `symgo` limitations are addressed, `goinspect` should be updated to correctly visualize the new information. This includes:
    -   Displaying calls to functions outside the primary scope as terminal nodes.
    -   Showing the correct signature for higher-order functions.
    -   Including calls made from within anonymous functions in the call graph.