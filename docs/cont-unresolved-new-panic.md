# Continuation: Fix for Panic on `new()` with Unresolved Types

## Initial Prompt

The user initiated the task to fix a panic in the `symgo` symbolic execution engine. The panic occurred when the engine tried to dereference a pointer to a type from an out-of-policy (unscanned) package. The error log showed `invalid indirect of <Unresolved Function: strings.Builder> (type *object.UnresolvedFunction)`, indicating that an unresolved type was being incorrectly categorized as a function, leading to a crash.

## Goal

The primary goal is to prevent this panic by correctly handling calls to the built-in `new()` function when the argument is a type from a package that has not been scanned. The fix should be robust and align with the symbolic execution engine's architecture.

## Initial Implementation Attempt

My first approach was to modify the `new` intrinsic to return a pointer to a generic `*object.SymbolicPlaceholder`. This placeholder was intended to represent the result of `new()` on an unresolved type. This approach was rejected as it was too specific to the `new` function and did not provide a structure that could be "simply unwrapped" by the rest of the evaluator, as requested by user feedback.

## Roadblocks & Key Discoveries

The initial approach was flawed because it treated the result of `new(UnresolvedType)` as a special case that the rest of the evaluator would have to handle. The key discovery, prompted by user feedback, was that `new()` should *always* return a standard `*object.Pointer` to a standard `*object.Instance`. The distinction between a resolved and unresolved type should be encapsulated within the `*object.Instance` itself, specifically in its `ResolvedTypeInfo` field. This makes the pointer itself a normal object that the evaluator can handle without special logic, simplifying the overall design.

This led to the insight that when `evalSelectorExpr` encounters a symbol from an out-of-policy package (e.g., `ext.SomeType`), it should return an object that clearly represents an unresolved symbol but carries enough information (package path and name) for other functions like `new()` to use.

## Major Refactoring Effort

Based on these discoveries, the implementation was refactored:

1.  **`symgo/evaluator/evaluator.go`**: The `evalSelectorExpr` function was modified. When it encounters a symbol from an unscanned package, instead of returning a generic placeholder, it now returns an `*object.UnresolvedFunction`. This object retains the package path and the symbol name.

2.  **`symgo/intrinsics/builtins.go`**: The `BuiltinNew` function was significantly refactored. It now includes a `case` to explicitly handle `*object.UnresolvedFunction`. When it receives one, it uses the package path and name to create a minimal, unresolved `*scanner.TypeInfo` and attaches it to a new `*object.Instance`. It then returns a standard `*object.Pointer` to this new instance.

3.  **`symgo/regression_unresolved_new_test.go`**: A new regression test was added to specifically reproduce the original bug by setting a scan policy that excludes a package and then calling `new()` on a type from that package.

## Current Status

The implementation is nearly complete. The core logic for the fix is in place, and the regression test has been written. However, I have encountered persistent tooling issues with `replace_with_git_merge_diff` that have prevented me from successfully applying the final changes to `symgo/intrinsics/builtins.go`. The last attempt resulted in a build failure due to a malformed file. The logic itself is believed to be correct, but its application has been problematic.

## References

- `docs/dev-guide.md`: For general guidance on the symbolic execution engine.

## TODO / Next Steps

1.  **Apply the fix to `BuiltinNew`**: The `BuiltinNew` function in `symgo/intrinsics/builtins.go` needs to be modified to correctly handle `*object.UnresolvedFunction`. The `switch` statement should be updated to include a case for this type, which then creates an instance with an unresolved `TypeInfo` and returns a pointer to it.
2.  **Verify with Tests**: Run the full test suite (`go test -v ./...`) to ensure that the new regression test passes and that no existing tests have been broken.
3.  **Finalize and Submit**: Once all tests pass, the task is complete and the changes can be submitted.
