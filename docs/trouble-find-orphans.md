# Trouble Shooting: `find-orphans` and `symgo` Evaluation

This document details the debugging process for a persistent issue encountered while developing the `find-orphans` tool. The core problem is that the symbolic execution engine, `symgo`, is not correctly tracking function and method calls, resulting in an empty usage map and a report where nearly all functions are incorrectly flagged as orphans.

## Symptoms

1.  The `find-orphans` test (`examples/find-orphans/main_test.go`) consistently fails.
2.  The non-verbose output lists almost every function, including those that are clearly used (like `main`, `greeter.New`, `greeter.SayHello`), as orphans.
3.  The verbose output is missing the "-- Used Functions --" section entirely, confirming the `usageMap` is not being populated.
4.  A corresponding test in `symgo` itself (`TestEvalCallExpr_VariousPatterns/method_chaining`) also fails, indicating the bug is in the evaluator, not just the `find-orphans` tool's usage of it.

## Investigation and Fixes Attempted

The debugging process involved several iterations, fixing progressively deeper bugs in the `symgo` evaluator.

1.  **Test Setup Issues**: The initial test setup used an in-memory file overlay, but the `go-scan` `Locator` component expects a real file system. This was fixed by refactoring the tests to write to a temporary directory.

2.  **Isolated `Eval` Calls**: The analysis loop was initially calling `Interpreter.Eval()` for every function declaration. This was incorrect as it doesn't trace the call graph. The fix was to only `Apply()` the `main` function as the entry points.

3.  **`ReturnValue` Unwrapping**: It was discovered that the evaluator was not unwrapping `*object.ReturnValue` objects after a function call, causing type information to be lost on assignment. A fix was added to `evalIdentAssignment`.

4.  **Missing Method Call Implementation**: The evaluator had no logic to handle method calls on concrete types (e.g., `myStruct.DoSomething()`). The `evalSelectorExpr` logic for `*object.Instance` and `*object.Variable` only checked for intrinsics. A new `evalMethodCall` helper function was created to handle this, but it is still not working correctly.

## Current Status and Root Cause Analysis

Despite all the above fixes, the tests still fail with the same symptoms: the `usageMap` is empty. The `defaultIntrinsic` is never called.

The fundamental issue remains: `symgo`'s `evalCallExpr` successfully calls `applyFunction`, but the chain of evaluation within `applyFunction` (which recursively calls `Eval`) does not seem to trigger the `defaultIntrinsic` for nested calls.

**Hypothesis:** The `Interpreter`'s environment (`i.globalEnv`) is correctly populated with `*object.Function` definitions when all files are evaluated. However, when `Apply` is called on the `main` function, the subsequent calls within its body (e.g., `greeter.New()`) are resolved to `*object.Function` objects, but the `evalCallExpr` that should be triggered for them is somehow being bypassed or the `defaultIntrinsic` is not firing.

The problem is extremely subtle and lies deep within the evaluator's logic for how it handles environments and function application. The `TestDefaultIntrinsic` passes, but it tests a very simple case. The more complex scenario in `find-orphans` with cross-package calls and method calls is failing. Further debugging will require a step-by-step trace of the `Eval` and `Apply` calls for the `find-orphans` test case.

## Required `symgo` Enhancements

To make `find-orphans` and other complex tools viable, the `symgo` evaluator needs the following features/fixes:

-   **Reliable Method Dispatch**: The logic in `evalMethodCall` must be able to correctly resolve and execute a method call on a variable or instance of a concrete struct type, including handling pointer vs. non-pointer receivers correctly.
-   **Correct Type Propagation**: Type information must be correctly propagated through variable assignments (`:=`, `=`) and function returns. The `ReturnValue` unwrapping was one part of this, but other leaks may exist.
-   **Robust Environment Management**: The call stack and lexical scoping must be handled correctly so that when a function `A` calls function `B`, the execution of `B` occurs in the correct environment and the `defaultIntrinsic` is triggered for the call.
-   **Tracing and Debuggability**: The `--inspect` flag or a similar mechanism should be extended to provide a detailed trace of the symbolic execution flow, including which functions are called, what their arguments are, and what values are returned. This would have made debugging this issue significantly easier.

Until these issues are addressed, building tools that rely on deep call graph analysis (like `find-orphans`) with `symgo` will be unreliable.
