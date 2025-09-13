# Continuation: Fix for Error on Pointer Operations with Unresolved Types

## The Problem

The `symgo` symbolic execution engine incorrectly categorizes any unresolved symbol from an out-of-policy (unscanned) package as an `*object.UnresolvedFunction`. This is fundamentally incorrect when the symbol is a type (e.g., `strings.Builder`).

This miscategorization leads to a runtime error when any pointer operation (e.g., dereferencing with `*` or calling a method) is performed on a variable of that unresolved type. The evaluator sees an `UnresolvedFunction` where it expects a pointer-compatible object, and fails with an `invalid indirect` error.

The fix needs to address this core issue of type miscategorization for all pointer operations, not just for the `new()` built-in function.

## TODO / Next Steps

The task should be approached from a clean state to implement a robust solution.

1.  **Introduce `*object.UnresolvedType`**: Create a new object type in `symgo/object/object.go` to specifically represent a type from an out-of-policy package. This object must store the package path and type name.

2.  **Update `evalSelectorExpr`**: Modify the `evalSelectorExpr` function in `symgo/evaluator/evaluator.go`. When it encounters a symbol from an unscanned package, it should return an `*object.UnresolvedType` instead of the incorrect `*object.UnresolvedFunction`.

3.  **Update Pointer Operation Handlers**:
    *   Modify `evalStarExpr` (which handles `*p`) in `symgo/evaluator/evaluator.go` to correctly handle pointers to `*object.UnresolvedType`. It should return a symbolic instance of that unresolved type.
    *   Modify the `BuiltinNew` function in `symgo/intrinsics/builtins.go` to accept an `*object.UnresolvedType` as a valid argument.

4.  **Create a Comprehensive Regression Test**: Add a new test file, `symgo/regression_unresolved_type_test.go`, that reproduces the error using both a `new()` call and a direct pointer dereference on a variable whose type is from an out-of-policy package. This will ensure the fix is general and robust.

5.  **Verify and Submit**: Run the full test suite to confirm the fix and ensure no regressions have been introduced, then submit the changes.
