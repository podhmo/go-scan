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

## Bug Fix 2: Mutual Recursion Causes Empty Output

After adding a test case for mutual recursion (`Ping` calls `Pong`, `Pong` calls `Ping`), the resulting `mutual.golden` file was empty.

### Root Cause

The "orphan-style" filtering, which is intended to only show true entry points, fails for a codebase that consists entirely of a cycle. The logic marks any function that is called by another as a "callee". In the mutual recursion case, `Ping` marks `Pong` as a callee, and `Pong` marks `Ping` as a callee. Consequently, both are removed from the list of top-level functions to print, resulting in an empty list and no output.

### Solution

A fallback mechanism will be added. After the filtering logic runs, if the resulting list of top-level functions is empty (and the initial list of entry points was not), we will assume we've encountered a cyclical library and revert to using the original, unfiltered list of entry points. This correctly handles the mutual recursion case while preserving the orphan-style filtering for other cases.

## Final Verification: Mutual and Indirect Recursion

Based on user feedback, explicit test cases for both mutual recursion (`ping-pong`) and indirect recursion via a higher-order function (`indirect`) were added. After implementing the fixes described below, the tool now correctly identifies and displays the call graph for all forms of recursion, labeling the second call in the cycle as `[recursive]`. The feature is now considered complete and robust.

## Bug Fix 3: Indirect Recursion via Higher-Order Functions

A test case for indirect mutual recursion (`Ping` -> `cont` -> `Pong`) initially caused a `fatal error: stack overflow`.

### Root Cause

The `symgo` engine's recursion detection failed because it did not maintain the full logical call stack context when a function was passed as an argument. When `Ping` called `cont(Pong, ...)`, the engine would analyze `cont`, but when `cont` invoked its function argument `f` (which is `Pong`), the analysis of `Pong` would start with a fresh call stack, losing the fact that `Ping` was the original caller in the chain. This led to an infinite analysis loop: `Ping` -> `cont` -> `Pong` -> `cont` -> `Ping`...

A secondary, related stack overflow occurred because the `evalCallExpr` method would pre-scan function arguments that were themselves function literals (`scanFunctionLiteral`). This pre-scan could also trigger an infinite loop if the literal's body contained a call that led back to the original scan.

### Solution

A two-part solution was implemented in the `symgo/evaluator`:

1.  **Propagating Call Context**: The `object.Function` was enhanced with a `BoundCallStack` field. When a function is passed as an argument, the `extendFunctionEnv` method now clones the function object and "tags" it by attaching a copy of the current call stack to `BoundCallStack`. The core recursion detector in `applyFunctionImpl` was then modified to use this `BoundCallStack` for its check, if present. This allows the detector to see the full logical call path across higher-order function boundaries.

2.  **Recursion Guard for Literal Scanning**: A recursion guard was added to the `scanFunctionLiteral` method. It now uses a map (`scanLiteralInProgress`) to track which function literal bodies are currently being scanned. If it encounters a request to scan a literal that is already in progress, it halts immediately, breaking the pre-scan analysis loop.

With these changes, the `symgo` engine can now correctly handle indirect recursion, and the `indirect` test case passes.