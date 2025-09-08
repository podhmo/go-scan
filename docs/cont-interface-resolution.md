# Continuation: Fixing Interface Resolution and Related Failures in `symgo`

## Initial Prompt

(The user's original prompt was in Japanese. This is a faithful translation.)

> Please read and implement one task from `TODO.md`. If necessary, you can break it down into sub-tasks and write them into `TODO.md`.
>
> After that, please proceed with the work. First, please fix the failing tests under `symgo`. Continue fixing the code until the tests succeed. After finishing the work, please be sure to update `TODO.md` at the end.
>
> This is believed to be within the scope of the "symgo: Implement Robust Interface Resolution" task, but your current task is to fix the failing tests.
>
> If you cannot complete it, please also add a note to `TODO.md`.

## Goal

The primary goal is to fix all failing tests under the `./symgo/...` path. The test failures point to several distinct but related issues in the `symgo` symbolic execution engine, with the most critical one being the inability to correctly resolve interface method calls.

## Initial Implementation Attempt

My initial analysis after running the tests identified that many failures, especially `TestInterfaceResolution` and `TestAnalyzeMinigoPackage`, pointed to a problem in how function call environments were created. The `docs/trouble-symgo-interface-resolution.md` document confirmed this, stating that the `extendFunctionEnv` function in `symgo/evaluator/evaluator.go` was not correctly preserving the static type of interface parameters.

My first attempts focused on patching `extendFunctionEnv` and the related `evalSelectorExpr` function. However, these attempts were clumsy and resulted in a series of build errors due to my own mistakes in using the patching tools. I repeatedly had to reset the workspace, which made it difficult to make progress.

## Roadblocks & Key Discoveries

The main roadblock was a series of self-inflicted build errors. However, through this process and by analyzing the test failures and logs, I made several key discoveries about the root causes:

1.  **Interface Resolution is a Two-Part Problem:** The logic for handling interface method calls was fundamentally flawed. In `evalSelectorExpr`, upon detecting a call like `myInterface.MyMethod()`, the code was immediately trying to create the *symbolic result* of the call. This is incorrect. The correct approach is a two-step process: `evalSelectorExpr` should return a *callable object* representing the method itself, and only when that object is invoked should `applyFunction` produce the symbolic result. My initial patches failed because they didn't respect this separation of concerns, leading to `not a function: MULTI_RETURN` errors.

2.  **Anonymous Interfaces Have No Name:** The `Finalize` method, which connects interface calls to concrete types, relies on a key created from the interface's package path and name. For anonymous interfaces (e.g., `var myVar interface { Do() }`), the name is empty. This creates a malformed key (`..Do`) and causes the resolution to fail. This was the cause of the `TestSymgo_AnonymousTypes` failure.

3.  **Recursion Detection Was Too Naive:** The `TestServeError` failure was caused by a recursion check in `applyFunction` that simply counted how many times the same function definition appeared in the call stack. This incorrectly flagged deep but legitimate recursive functions as infinite loops. A correct implementation needs to consider not just the function being called, but also its arguments, to see if the program state is actually stuck in a loop.

4.  **Test Setup Matters:** The `TestAnalyzeMinigoPackage` failure was a red herring related to the core interface problem. It was failing because the test itself was manually creating `object.Function` objects without populating the crucial `Def (*scanner.FunctionInfo)` field. The `extendFunctionEnv` function correctly relies on this field, so its absence caused a "identifier not found" error because parameter binding was skipped entirely.

## Major Refactoring Effort

Based on these discoveries, I formulated a comprehensive refactoring plan. I have reset the workspace to a clean state to apply this plan correctly.

The required changes are:

1.  **In `symgo/object/object.go`**:
    *   The `SymbolicPlaceholder` struct has a field `UnderlyingMethod *scanner.MethodInfo`. This should be changed to `UnderlyingFunc *scanner.FunctionInfo`. This makes the field more general and resolves type conflicts when dealing with interface methods, which are represented as `*scanner.FunctionInfo` in the `go-scan` package.

2.  **In `symgo/evaluator/evaluator.go`**:
    *   The `callFrame` struct must be modified to include the arguments of the call: `Args []object.Object`. This is necessary for the improved recursion check.
    *   A helper function, `areArgsEqual(a, b []object.Object) bool`, must be added. It can use a simple heuristic of comparing the `Inspect()` string of each argument to determine if two calls have the same arguments.
    *   The recursion detection logic inside `applyFunction` must be replaced. The new logic should check if a call frame exists in the stack with the same function definition (`Def`). If it does, it should then compare the receiver (for methods) or the arguments (for functions, using `areArgsEqual`) to determine if it's a true infinite recursion.
    *   The `evalSelectorExpr` function must be updated. When it detects a call on a variable of an interface type, it should:
        a. Check if the interface is named (`staticType.Name != ""`). If so, record the call in the `calledInterfaceMethods` map for the `Finalize` step.
        b. Find the corresponding `*scanner.FunctionInfo` for the method from the interface definition.
        c. Return a `*object.SymbolicPlaceholder` containing this `FunctionInfo` in its `UnderlyingFunc` field, and the receiver object in its `Receiver` field.
    *   The `applyFunction` function must be updated. In the `switch` statement for `*object.SymbolicPlaceholder`, it must check for the presence of `UnderlyingFunc`. If present, it should use the information in that `FunctionInfo` (the method signature) to generate the correct symbolic return values (a single `SymbolicPlaceholder` or a `MultiReturn` object).
    *   Any other references to the old `UnderlyingMethod` field (e.g., in the `*object.Nil` case of `evalSelectorExpr`) must be updated to `UnderlyingFunc`.

## Current Status

The workspace has been reset to its original state. No changes have been applied yet. The next step is to apply the comprehensive patch described above.

## References

*   `docs/trouble-symgo-interface-resolution.md`: Provided the initial (and ultimately correct) diagnosis of the problem in `extendFunctionEnv`.
*   `TODO.md`: Contains the original task list.

## TODO / Next Steps

1.  Apply the change to `symgo/object/object.go`: `UnderlyingMethod *scanner.MethodInfo` -> `UnderlyingFunc *scanner.FunctionInfo`.
2.  Apply the comprehensive patch to `symgo/evaluator/evaluator.go` that includes the `callFrame` update, the `areArgsEqual` helper, the new recursion logic, and the updated `evalSelectorExpr`/`applyFunction` logic for interface methods.
3.  Run all tests in `./symgo/...`. It is expected that `TestInterfaceResolution`, `TestServeError`, and the various interface-related `MULTI_RETURN` tests will now pass.
4.  Address any remaining test failures, such as `TestEvalClosures`.
5.  Remove this continuation document (`docs/cont-interface-resolution.md`) and update `TODO.md` to reflect the completed work.
