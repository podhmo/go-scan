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

# Trouble: Custom Intrinsics for Intra-Module Helpers Not Firing

## Summary
This document details the investigation into a bug where custom patterns (intrinsics) for helper functions are not being triggered when the helper function resides in a different package within the same Go module being analyzed. This results in an incomplete OpenAPI specification, missing responses, parameters, etc.

## Context
The goal is to implement two new features in `docgen`:
1.  Support for `map[string]any` in response schemas.
2.  A new `defaultResponse` custom pattern to define responses with specific status codes (e.g., for standard error formats).

To test this, a new test case (`new-features`) was created. This test defines API handlers in a `main` package and helper functions (e.g., `RenderJSON`, `RenderError`) in a `helpers` sub-package. A `patterns.go` file defines intrinsics to be applied to these helper functions.

## Current Problem
The `TestDocgen_newFeatures` test fails. The generated OpenAPI output correctly identifies the handlers and their descriptions, but it is completely missing the `responses` and `parameters` sections. This indicates that the custom intrinsics for `new-features/helpers.RenderJSON` and `new-features/helpers.RenderError` are never being called during the symbolic execution of the handlers.

## Hypothesis & Investigation
The initial hypothesis was that the key used for the intrinsic was incorrect.
- **Attempt 1:** Used `main.RenderJSON`. This was incorrect as `symgo` needs a fully-qualified path.
- **Attempt 2:** Moved helpers to a sub-package `helpers` and used the key `new-features/helpers.RenderJSON`. This matches the structure of other working tests and appears to be the correct key format that `symgo`'s `evalSelectorExpr` should resolve.

Despite this, the test still fails in the same way. The `symgo` evaluation logic appears to be correct on paper: `evalSelectorExpr` should identify the package path and function name, create the key, and look it up in the currently scoped intrinsics registry.

The current leading theory is that there is a subtle issue in how `symgo` resolves packages or looks up intrinsics when dealing with intra-module dependencies that are not the standard library. The analyzer seems to be successfully loading the custom patterns, but the evaluator is failing to match a call site (`helpers.RenderJSON`) to the registered intrinsic.

## Next Steps
To isolate the problem from the complexity of the `docgen` analyzer, the next step is to create a minimal test directly within the `symgo` package. This test will:
1.  Programmatically create a `symgo.Interpreter`.
2.  Use `scantest` to define a small, in-memory Go module with `main` and `helpers` packages.
3.  Register a custom intrinsic for a function in the `helpers` package.
4.  Symbolically execute a function in `main` that calls the helper.
5.  Assert whether the intrinsic was triggered.

This will confirm if the bug is in `symgo`'s core evaluation logic or in the `docgen` setup.

**Update:** The `symgo`-level test described above was created and **passed**. This confirms the core evaluation logic is sound. The issue is specific to the `docgen` analyzer's usage of `symgo`.

Further experiments were conducted:
-   Isolating the failing handler (`/settings`) did not make it pass. This disproves the theory of state corruption between handler analyses.
-   Simplifying a complex, nested map literal in a failing handler to a simple `map[string]string` also did not make it pass. This disproves the theory of unsupported syntax causing the evaluator to silently fail.

The root cause remains unknown, but it is highly specific to the `docgen` analyzer's state management when evaluating handlers. The final state of the code is being submitted with the failing test case to allow for external review.
