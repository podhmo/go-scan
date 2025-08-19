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

## 2. Method Calls on Composite Literals

-   **Test Case**: `TestEvalCallExpr_VariousPatterns/method_call_on_a_struct_literal`
-   **Code**: `S{}.Do()`
-   **Status**: **Fixed**

### Problem Description

The evaluator could not handle a method call where the receiver was a composite literal (a struct being instantiated directly, like `S{}`). The test failed because the `Do` method's intrinsic was never found or called.

### Root Cause Analysis

The issue was two-fold:

1.  **Missing `CompositeLit` Evaluation**: The main `Eval` function did not have a case for `*ast.CompositeLit` nodes. When the evaluator encountered `S{}`, it didn't know what to do with it and couldn't produce a symbolic object that `evalSelectorExpr` could use.
2.  **Incomplete Type Name Resolution**: The initial implementation for handling composite literals did not correctly form a fully-qualified type name (e.g., `example.com/me.S`). It resolved the type name to just `S`, which caused a mismatch with the key used to register the intrinsic in the test.

### Solution

The fix involved several changes in `symgo/evaluator/evaluator.go`:

1.  A new case for `*ast.CompositeLit` was added to the `Eval` function's switch statement.
2.  A new function, `evalCompositeLit`, was implemented. This function uses the `scanner` to resolve the type of the literal and creates a symbolic `*object.Instance` to represent it.
3.  The logic within `evalCompositeLit` was refined to correctly construct a fully-qualified type name by combining the package's import path with the type's local name, ensuring it matches the intrinsic keys.
4.  The `evalSelectorExpr` function was also improved to check for methods on both value (`(T).Method`) and pointer (`(*T).Method`) receivers, making it more robust.

## 3. Method Chaining

-   **Test Case**: `TestEvalCallExpr_VariousPatterns/method_chaining`
-   **Code**: `NewGreeter("world").WithName("gopher").Greet()`
-   **Status**: **Fixed**

### Problem Description

The test for method chaining was failing. The final method in the chain, `Greet`, was never being called, and the test assertions failed as a result.

### Root Cause Analysis

The problem was a simple but crucial error in the test setup. The keys used to register the intrinsics for the `Greeter` methods were not using fully-qualified type names. For example, the test was registering `(*main.Greeter).Greet` instead of the correct `(*example.com/me.Greeter).Greet`.

The evaluator was correctly resolving the types and looking for the fully-qualified keys, but since the test had registered the wrong keys, no match was found.

### Solution

The fix was to update the "method chaining" test case in `symgo/evaluator/evaluator_call_test.go` to use programmatically-generated, fully-qualified names for the intrinsic keys. This was done by using `fmt.Sprintf` with the package's import path (`pkg.ImportPath`) to build the correct keys at test runtime. This ensures the keys registered in the test match the keys the evaluator is looking for, allowing the method chain to be resolved correctly.
