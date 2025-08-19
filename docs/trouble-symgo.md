# Symgo Engine Troubleshooting and Fixes

This document tracks issues found and fixed in the `symgo` symbolic execution engine, serving as a log for future development and debugging.

## 1. Nested Function Calls

-   **Test Case**: `TestEvalCallExpr_VariousPatterns/nested_function_calls`
-   **Code**: `add(add(1, 2), 3)`
-   **Status**: **Fixed**

### Problem Description

The test for nested function calls was failing because the intrinsic (mock) for the `add` function was never being called. The evaluator was instead trying to evaluate the *actual* body of the `add` function.

### Root Cause Analysis

The root cause was the lookup order within the `evalIdent` function, which is responsible for resolving identifiers like `add`. The function was implemented to check the local environment for a function declaration *before* checking the intrinsic registry.

1.  The evaluator would first parse the file and place an `*object.Function` for `add` into the environment.
2.  The test would then register an `*object.Intrinsic` for `add`.
3.  When `evalIdent` was called for `add`, it would find the `*object.Function` in the environment first and return it, never reaching the code that checks for the intrinsic.

### Solution

The fix was to reverse the lookup order in `evalIdent`. The function now checks for a registered intrinsic **first**, and only if one is not found does it proceed to check the environment. This allows tests to correctly override and mock functions that exist in the package being analyzed. This change was made in `symgo/evaluator/evaluator.go`.
