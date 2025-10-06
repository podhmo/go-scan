# Trouble Report: `symgo` Evaluator Robustness Issues

This document details several distinct bugs in the `symgo` evaluator that cause symbolic execution to halt prematurely. These issues were discovered during the analysis of a large, real-world Go codebase and represent gaps in the evaluator's ability to robustly handle common Go patterns and unexpected states. According to the principles in `docs/analysis-symgo-implementation.md`, the evaluator should favor returning symbolic placeholders over crashing to ensure analysis can complete.

## 1. Fluent API / Method Chaining Failure

-   **Symptom**: `expected a package, instance, or pointer on the left side of selector, but got FUNCTION`
-   **Example Code**:
    ```go
    // Simplified from kingpin library usage
    app.Flag("debug", "Enable debug mode.").Bool()
    ```
-   **Root Cause**: The evaluator processes `app.Flag(...)` and the result is an `*object.Function` with the receiver `app` bound to it. When it then tries to evaluate `.Bool()`, the left-hand side of the selector is a function object, which `evalSelectorExpr` does not handle.
-   **Proposed Solution**: `evalSelectorExpr` should check if the left-hand object is a function with a bound receiver. If so, it should unwrap the receiver and continue the evaluation on that object.

## 2. Method on Named Slice Failure

-   **Symptom**: `expected a package, instance, or pointer on the left side of selector, but got SLICE`
-   **Example Code**:
    ```go
    type MySlice []int
    func (s MySlice) Sum() int { /* ... */ }

    s := MySlice{1, 2, 3}
    s.Sum() // Fails here
    ```
-   **Root Cause**: `evalSelectorExpr` lacks a `case` for `*object.Slice`. Additionally, `evalCompositeLit` was not correctly associating the alias type info (`MySlice`) with the created slice object, preventing method resolution.
-   **Proposed Solution**:
    1.  Fix `evalCompositeLit` to attach the alias's `TypeInfo` to the `*object.Slice`.
    2.  Add a `case *object.Slice:` to `evalSelectorExpr` that uses the `accessor` to find and resolve methods on the named slice type.

## 3. Invalid Indirect Dereference

-   **Symptom**: `invalid indirect of ...` (for `nil`, `instance`, `package`, etc.)
-   **Example Code**:
    ```go
    var p *int // p is nil
    _ = *p     // Causes "invalid indirect of nil"
    ```
-   **Root Cause**: `evalStarExpr` (which handles the `*` operator) receives an object that cannot be dereferenced (e.g., `*object.Nil`). Instead of returning a symbolic placeholder for the result, it panics.
-   **Proposed Solution**: `evalStarExpr` should be modified to check the type of the operand. If the operand is not a pointer or is a nil pointer, it should return a `*object.SymbolicPlaceholder` instead of erroring out, allowing analysis to continue.

## 4. Unsupported Unary Operation

-   **Symptom**: `unary operator - not supported for type UNRESOLVED_FUNCTION`
-   **Example Code**:
    ```go
    // Assume some.UnresolvedFunc() returns an unresolved function placeholder
    x := -some.UnresolvedFunc()
    ```
-   **Root Cause**: `evalUnaryExpr` does not handle cases where the operand is a symbolic or unresolved type. It attempts to perform the operation directly, which fails.
-   **Proposed Solution**: `evalUnaryExpr` should check if the operand is a concrete numeric type. If not, it should return a new `*object.SymbolicPlaceholder` representing the result of the unary operation.

## 5. `len()` on Unsupported Types

-   **Symptom**: `argument to \`len\` not supported, got INSTANCE` (or `POINTER`, `VARIADIC`, etc.)
-   **Example Code**:
    ```go
    // Assume getSymbolic() returns an *object.Instance
    x := getSymbolic()
    _ = len(x)
    ```
-   **Root Cause**: The built-in intrinsic for `len()` only handles concrete slice, map, and string objects. When it receives a symbolic placeholder, it fails.
-   **Proposed Solution**: The `len()` intrinsic should be updated to handle symbolic placeholders by returning a new placeholder representing the length (e.g., `<Symbolic: len of ...>`).

## 6. Incorrect Symbol Resolution for `recover`

-   **Symptom**: `not a function: PACKAGE` when calling `recover()`
-   **Root Cause**: The evaluator incorrectly resolves the identifier `recover` to an imported package that happens to be named `recover` instead of prioritizing the Go built-in `recover` function.
-   **Proposed Solution**: The evaluator's identifier resolution logic must be updated to check for built-in functions **before** checking for package names in the current scope.

## 7. `identifier not found` during test analysis

-   **Symptom**: `identifier not found: opts`
-   **Root Cause**: This is likely a scoping issue within the code being analyzed, where a variable is used before it's declared in a way the evaluator doesn't anticipate.
-   **Proposed Solution**: While this might indicate an issue in the target code, the evaluator could be made more resilient. `evalIdent` could be modified to return a symbolic placeholder for an unfound identifier instead of a fatal error, allowing analysis to proceed further. This is a lower priority fix.