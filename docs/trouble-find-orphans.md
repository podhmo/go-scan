# Trouble Shooting: `find-orphans` and `symgo` Evaluation

This document details the debugging process for a persistent issue encountered while developing the `find-orphans` tool. The core problem is that the symbolic execution engine, `symgo`, is not correctly tracking function and method calls, resulting in an empty usage map and a report where nearly all functions are incorrectly flagged as orphans.

## Symptoms

1.  The `find-orphans` test (`examples/find-orphans/main_test.go`) consistently fails.
2.  The non-verbose output lists almost every function, including those that are clearly used (like `main`, `greeter.New`, `greeter.SayHello`), as orphans.
3.  The verbose output is missing the "-- Used Functions --" section entirely, confirming the `usageMap` is not being populated.
4.  A corresponding test in `symgo` itself (`TestEvalCallExpr_VariousPatterns/method_chaining`) also fails, indicating the bug is in the evaluator, not just the `find-orphans` tool's usage of it.

## Investigation and Fixes Attempted

The debugging process involved several iterations, fixing progressively deeper bugs in the `symgo` evaluator.

### 1. Test Setup Issues

-   **Initial Problem**: The test setup used an in-memory file overlay with `scantest`, but the `ModuleWalker` and `Locator` components of `go-scan` expected a real file system, causing path resolution errors.
-   **Fix**: The test was refactored to write files to a temporary directory (`t.TempDir()`) and run the scanner against that real directory. This resolved all setup-related errors.

### 2. Isolated `Eval` Calls

-   **Problem**: The analysis loop in `find-orphans` was initially calling `Interpreter.Eval()` for every function declaration in the codebase. Each `Eval` call used its own environment, so state was not shared, and the call graph could not be traced.
-   **Fix**: The loop was corrected to only evaluate `main` package functions named `main` as the entry points for the analysis.

### 3. `ReturnValue` Unwrapping

-   **Problem**: Even after fixing the analysis loop, type information was being lost during variable assignment. The `symgo` evaluator was not unwrapping the `*object.ReturnValue` object that function calls return before assigning the underlying value to a variable.
-   **Fix**: Modified `evalIdentAssignment` in `symgo/evaluator/evaluator.go` to check for and unwrap `*object.ReturnValue` objects.

### 4. Missing Method Call Implementation

-   **Problem**: The evaluator had no logic to handle method calls on concrete types (e.g., `myStruct.DoSomething()`). The `evalSelectorExpr` logic for `*object.Instance` and `*object.Variable` only checked for intrinsics and did not attempt to resolve the method to a function declaration.
-   **Fix**: A new `evalMethodCall` helper function was created and integrated into `evalSelectorExpr`. This new function is responsible for finding the method declaration in the type's package and creating a callable intrinsic (`boundFn`) with the receiver correctly bound to the function's environment.

## Current Status and Root Cause Analysis

Despite all the above fixes, the tests still fail with the same symptoms: the `usageMap` is empty. The `defaultIntrinsic` is never called.

The most recent attempt was a major refactoring of `evalMethodCall` to correctly look up and bind receivers for method calls on concrete types. This also failed.

The fundamental issue remains: `symgo`'s `evalCallExpr` successfully calls `applyFunction`, but the chain of evaluation within `applyFunction` (which recursively calls `Eval`) does not seem to trigger the `defaultIntrinsic` for nested calls.

**Hypothesis:** The `Interpreter`'s environment (`i.globalEnv`) is correctly populated with `*object.Function` definitions when all files are evaluated. However, when `Apply` is called on the `main` function, the subsequent calls within its body (e.g., `greeter.New()`) are resolved to `*object.Function` objects, but the `evalCallExpr` that should be triggered for them is somehow being bypassed or the `defaultIntrinsic` is not firing.

The problem is extremely subtle and lies deep within the evaluator's logic for how it handles environments and function application. The `TestDefaultIntrinsic` passes, but it tests a very simple case. The more complex scenario in `find-orphans` with cross-package calls and method calls is failing. Further debugging will require a step-by-step trace of the `Eval` and `Apply` calls for the `find-orphans` test case.
