# Trouble: `op.Parameters` are lost during analysis (In Progress)

## Summary
This document details an ongoing investigation into a subtle bug where OpenAPI parameters are not being correctly added to the final specification, even though the analysis logic appears to be correct.

## Context
The goal is to extend the `docgen` custom pattern system to support path, query, and header parameters. The work so far includes:
1.  Fixing a major state propagation bug where the `*openapi.Operation` object was not being returned correctly from `analyzeHandlerBody`.
2.  Adding support for a `HeaderParameter` pattern type.
3.  Creating a new test case (`full-parameters`) to exercise the new functionality.
4.  Correcting the intrinsic keys used in the test case to ensure the custom pattern handlers are called.

## Current Problem
A very strange bug is occurring. Debug logging has confirmed the following:
- The correct intrinsic handler (`patterns.HandleCustomParameter`) is being called when the analyzer finds a helper function like `GetQueryParam()`.
- Inside this handler, a new `openapi.Parameter` object is created.
- This new parameter is appended to the `Parameters` slice of the current `openapi.Operation` object (`op.Parameters = append(op.Parameters, param)`).
- Logging confirms that the length of the `op.Parameters` slice increases after the `append` call.
- The `*openapi.Operation` object is correctly returned up the call stack.

Despite all of this, the final generated `openapi.json` file is **missing the `parameters` section entirely**. The modifications to the slice are being lost somewhere between the end of the intrinsic's execution and the final JSON serialization.

## Hypothesis
The cause is not yet known, but here are the current theories:
1.  **Slice `append` semantics**: There might be a subtle issue with how `append` is used. If `append` reallocates the underlying array of the `Parameters` slice, it returns a new slice header. If this new slice header is not correctly assigned back, the change will be lost. However, the code `op.Parameters = append(...)` seems to be correct.
2.  **Hidden object copy**: There may be a copy of the `openapi.Operation` object being made somewhere that is not obvious from the code. If the intrinsics are modifying a copy, the original object would remain unchanged. This seems unlikely given that the object is passed by pointer and managed on a stack.
3.  **JSON marshaling issue**: It's possible the `json.Marshal` step is ignoring the `Parameters` field for some reason (e.g., a `json:"-"` tag). A quick check of the `openapi.Operation` struct should confirm or deny this.

Further investigation is needed. The next step is to examine the `openapi.Operation` struct and to trace the `op` object's pointer address at various points in the execution to see if it ever changes unexpectedly.

# [RESOLVED] Trouble: Custom Intrinsics for Intra-Module Helpers Not Firing

## Summary
This section documents a resolved bug where custom patterns (intrinsics) for helper functions were not being triggered when the helper function resided in a different package within the same Go module.

## Initial State & Problem
The investigation began with the `TestDocgen_newFeatures` test failing. The test was designed to verify that `docgen` could analyze handlers in a `main` package that called helper functions (e.g., `helpers.RenderJSON`) in a `helpers` sub-package. The custom intrinsics tied to these helper functions were not being called, resulting in an incomplete OpenAPI specification.

The investigation was complicated by the fact that the test case itself was broken and missing its source files. After reconstructing the test case based on this document, the bug was successfully reproduced.

## Root Cause Analysis
After extensive debugging, the root cause was identified in the `symgo` interpreter.

The `docgen` analyzer works by evaluating isolated function bodies (`ast.BlockStmt`) for each HTTP handler. However, when `symgo.Interpreter.Eval` was called with a `BlockStmt`, it created a new, empty evaluation environment. This environment was not populated with the import declarations from the file that contained the handler.

As a result, when the evaluator encountered a call to `helpers.RenderJSON`, it could not resolve the `helpers` identifier to its full import path (`new-features/helpers`). This caused the intrinsic lookup to fail, as the key (`new-features/helpers.RenderJSON`) could not be constructed.

## Solution
The bug was fixed by modifying `symgo.Interpreter.Eval`. Before evaluation starts, the interpreter's global environment is now pre-populated with the import declarations from the package containing the code to be analyzed. This ensures that even when an isolated code block is evaluated, the evaluator has the necessary context to resolve imported packages and successfully look up the associated intrinsics.

This fix was implemented and verified, and all related tests now pass.

# Future Improvements: Debuggability

During the investigation of the intra-module intrinsic bug, it became clear that debugging the symbolic execution process is difficult. It was hard to prove whether a specific AST node was being visited by the evaluator without adding temporary logging statements.

As a future improvement, `symgo` could be enhanced with a built-in tracing or visiting mechanism. For example, a `Tracer` interface could be passed to the `Interpreter`:

```go
type Tracer interface {
    Visit(node ast.Node)
}
```

The `Evaluator` would then call `tracer.Visit(node)` for every node it evaluates. A test or a debug mode in a tool like `docgen` could provide a tracer implementation that records all visited nodes. This would allow a developer to easily compare the set of all nodes in an AST (`ast.Inspect`) with the set of nodes the evaluator actually visited, quickly identifying any missed branches or skipped nodes. This would have significantly accelerated the debugging process for the issue described above.
