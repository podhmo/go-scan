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

## Bug Fix: Recursive Functions Filtered from Top-Level Output

After the initial implementation, user feedback highlighted a critical bug: the `Recursive` function, which is exported, was not appearing in the `default.golden` output as expected.

### Root Cause

The issue was in the logic that filters for "true" top-level functions. The code builds a map of all `callees` and then iterates through the potential `entryPoints`, excluding any function that appears in the `callees` map.

A recursive function calls itself, so it is present in its own list of callees in the `callGraph`. This caused the recursive function to be added to the `callees` map, which in turn caused it to be filtered out from the final list of `topLevelFunctions` to be printed.

### Solution

The fix is to adjust the filtering logic. When populating the `callees` map, we must ignore cases where a function calls itself. A function should only be considered a "callee" (and therefore not a top-level entry point) if it is called by a *different* function.

The logic will be changed to compare the caller's ID with the callee's ID and skip the entry if they match. This ensures self-recursive functions can still be considered top-level entry points.