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
