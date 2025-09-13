# Continuation: Fix for Error on Pointer Operations with Unresolved Types

## Initial Prompt

The user initiated the task to fix an error in the `symgo` symbolic execution engine. The error occurred when the engine tried to perform a pointer operation (dereference) on a variable whose type was from an out-of-policy (unscanned) package. The error log showed `invalid indirect of <Unresolved Function: strings.Builder> (type *object.UnresolvedFunction)`, indicating that an unresolved type (`strings.Builder`) was being incorrectly categorized as a function, leading to a crash during the pointer operation.

## Goal

The primary goal is to fix a fundamental issue where any out-of-policy type identifier (e.g., `strings.Builder`) is miscategorized as an `*object.UnresolvedFunction`. This leads to an error when any pointer operation (like dereferencing with `*`) is performed on a variable of such a type. The fix should correctly handle these unresolved types across the engine, not just in the context of the `new()` function.

## Initial Implementation Attempt

My first approach was to modify the `new` intrinsic to return a pointer to a generic `*object.SymbolicPlaceholder`. This approach was rejected as it was too specific to the `new` function and did not provide a structure that could be "simply unwrapped" by the rest of the evaluator, as requested by user feedback.

## Roadblocks & Key Discoveries

My initial focus on the `new()` function was too narrow and missed the core issue. The user correctly pointed out that the problem is more general. The key discovery is that the root cause is in `evalSelectorExpr`. When it encounters a symbol from an unscanned package, it defaults to creating an `*object.UnresolvedFunction`, regardless of whether the symbol is a type, function, or variable. This is incorrect.

When a pointer operation (like `*p`) is evaluated via `evalStarExpr`, it receives this `UnresolvedFunction` object where it expects a pointer. This causes the `invalid indirect` error. The problem is not limited to `new()`; it would happen for any pointer to an unresolved type. The engine needs a way to represent an unresolved *type* distinctly from an unresolved *function*.

## Major Refactoring Effort

Based on these discoveries, the planned refactoring should target the core representation of unresolved symbols:

1.  **Introduce `*object.UnresolvedType`**: A new object type should be created to specifically represent a type from an out-of-policy package. This object would carry the package path and type name.

2.  **Modify `symgo/evaluator/evaluator.go`**:
    *   `evalSelectorExpr` should be updated to return this new `*object.UnresolvedType` when it encounters an identifier that appears to be a type within an unscanned package.
    *   `evalStarExpr` (which handles the `*` operator) must be taught to handle a pointer to an `*object.UnresolvedType`. When it does, it should return a symbolic instance of that unresolved type.

3.  **Modify `symgo/intrinsics/builtins.go`**: The `BuiltinNew` function should be updated to accept `*object.UnresolvedType` as a valid argument, creating an instance and returning a pointer to it.

4.  **`symgo/regression_unresolved_new_test.go`**: The regression test should be updated to not only test `new()` but also a direct pointer dereference on a variable holding an unresolved type, ensuring the fix is general.

## Current Status

The implementation is nearly complete. The core logic for the fix is in place, and the regression test has been written. However, I have encountered persistent tooling issues with `replace_with_git_merge_diff` that have prevented me from successfully applying the final changes to `symgo/intrinsics/builtins.go`. The last attempt resulted in a build failure due to a malformed file. The logic itself is believed to be correct, but its application has been problematic.

## References

- `docs/dev-guide.md`: For general guidance on the symbolic execution engine.

## TODO / Next Steps

1.  **Introduce a new object type**: `*object.UnresolvedType`, to specifically represent types from out-of-policy packages.
2.  **Modify `evalSelectorExpr`**: In `symgo/evaluator/evaluator.go`, change it to return an `*object.UnresolvedType` for unresolved type symbols.
3.  **Modify `evalStarExpr`**: In `symgo/evaluator/evaluator.go`, teach it to handle pointers to `*object.UnresolvedType`.
4.  **Modify `BuiltinNew`**: In `symgo/intrinsics/builtins.go`, teach it to handle `*object.UnresolvedType` as an argument.
5.  **Verify with Tests**: Run the full test suite (`go test -v ./...`) to ensure the fix is correct and no regressions were introduced.
6.  **Finalize and Submit**: Once all tests pass, the task is complete and the changes can be submitted.
