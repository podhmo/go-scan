# Detecting Recursive Calls in `goinspect`

This document tracks the process of implementing recursion detection in the `goinspect` tool.

## Task Overview

The goal is to enhance the `goinspect` tool to identify and label recursive function calls in its output. The desired output format is to prepend `[recursive]` to the function call line, similar to how `[accessor]` is used for getter/setter methods.

- **Target Tool**: `examples/goinspect`
- **Detection Scope**: Simple recursion (a function calling itself) and mutual recursion (a function calling another function that eventually calls back to the original).
- **Function Identification**: Use the AST declaration position (`Pos`) to uniquely identify functions, as `scanner.FunctionInfo` objects are not stable across different analysis passes.
- **Performance**: Enable memoization in the `symgo` engine to improve analysis performance.
- **Verification**: The `Recursive` function in `examples/goinspect/testdata/src/myapp/main.go` should be correctly identified and labeled in the updated `default.golden` file.

## Initial Analysis

1.  **Code Location**: The core logic for printing the call graph is in `examples/goinspect/main.go`, specifically within the `Printer.printRecursive` method.
2.  **Existing Cycle Detection**: The `printRecursive` method already contains a `visited` map (`map[string]bool`) which is used to detect cycles during the printing traversal. This is the perfect mechanism to leverage for identifying recursion. When `p.visited[id]` is true, it means we are trying to print a function that is already in our current print stack, which is the definition of recursion in this context.
3.  **Function ID**: The `getFuncID` function correctly generates a stable ID based on the package path and the function's declaration position, as required.
4.  **Plan**:
    - Enable memoization in `symgo.NewInterpreter`.
    - Modify the `if p.visited[id]` block in `printRecursive` to add the `[recursive]` prefix to the output.
    - Run tests with `-update` to regenerate golden files.
    - Verify the change in `default.golden`.
    - Update `TODO.md`.

## Implementation Steps

- **Enabled Memoization**: In `examples/goinspect/main.go`, the call to `symgo.NewInterpreter` was updated to include the `symgo.WithMemoization(true)` option. This should improve performance for larger analyses.

- **Added Recursion Prefix**: The `printRecursive` function was modified. The block that previously printed a generic `(cycle detected)` message now prepends `[recursive] ` to the output line, making the nature of the cycle explicit.

  ```go
  // Before
  if p.visited[id] {
      // ...
      fmt.Fprintf(p.Out, "%s%s%s ... (cycle detected%s)\n", strings.Repeat("  ", indent), accessorPrefix, formatted, cycleRef)
      return
  }

  // After
  if p.visited[id] {
      // This is a recursive call (direct or mutual).
      recursivePrefix := "[recursive] "
      // ...
      fmt.Fprintf(p.Out, "%s%s%s%s ... (cycle detected%s)\n", strings.Repeat("  ", indent), recursivePrefix, accessorPrefix, formatted, cycleRef)
      return
  }
  ```