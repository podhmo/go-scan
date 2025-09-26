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

## Final Verification: Mutual Recursion

Based on user feedback, an explicit test case for mutual recursion (`ping-pong`) was added. The fix for Bug #2 proved successful. The tool now correctly identifies and displays the call graph for mutually recursive functions, labeling the second call in the cycle as `[recursive]`. The feature is now considered complete and robust.

## Final Fix: Resolving Higher-Order Function Recursion (2025-09-26)

After a lengthy investigation, the root cause of the stack overflow and incorrect call graphs was identified and fixed. The issue was a combination of two subtle bugs in the `symgo` evaluator, which only manifested when analyzing complex recursive patterns, particularly those involving higher-order functions.

### Root Cause Analysis

1.  **Premature Evaluation Termination in `if` Statements**: The `evalIfStmt` function was designed to explore both the `then` and `else` branches of a conditional. However, if a branch contained a `return` statement, the evaluator would propagate the `*object.ReturnValue` upwards, causing the evaluation of the parent function to terminate immediately. This prevented the analysis of any code following the `if` statement. In the `indirect` and `mutual` test cases, a guard clause like `if n > 1 { return }` was causing the subsequent recursive call (e.g., `cont(Pong, n+1)`) to be skipped entirely, leading to an incomplete call graph.

2.  **Overeager Scanning of Function Arguments**: The `evalCallExpr` function had a mechanism (`scanFunctionLiteral`) to trace calls inside anonymous functions passed as arguments. However, it was incorrectly attempting to scan *all* function-like arguments, including named functions like `Ping` and `Pong` when they were passed to the `cont` helper. This created an infinite analysis loop: `Ping` calls `cont(Pong)`, which triggered a scan of `Pong`, which calls `cont(Ping)`, triggering a scan of `Ping`, and so on, eventually causing a stack overflow.

### The Solution

A two-part fix was implemented in `symgo/evaluator/evaluator.go`:

1.  **Corrected `if` Statement Evaluation**: The `evalIfStmt` function was modified to no longer propagate `*object.ReturnValue` from its branches. It now evaluates both the `then` and `else` paths for their side effects (i.e., tracing function calls) and then always returns `nil`, allowing the analysis of the parent function to continue to subsequent statements. This ensures all possible code paths are explored, as intended by the symbolic engine's design.

2.  **Scoped Function Argument Scanning**: The `evalCallExpr` function was fixed to only trigger `scanFunctionLiteral` for actual anonymous function literals. The check `if fn, ok := arg.(*object.Function); ok` was refined to `if fn, ok := arg.(*object.Function); ok && fn.Name == nil`. This correctly restricts the deep scanning to anonymous functions while treating named functions passed as values as simple arguments, breaking the infinite recursion loop.

With these changes, the `symgo` engine now correctly handles all tested recursion patterns—simple, mutual, and indirect recursion via higher-order functions—producing a complete and accurate call graph. The `goinspect` tests now pass with the correct, expected output.