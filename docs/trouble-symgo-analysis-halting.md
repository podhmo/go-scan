# Trouble Report: Analysis Halting Failures in `symgo` Evaluator

This document details several distinct categories of bugs in the `symgo` evaluator that cause symbolic execution to halt prematurely. These issues were identified from a large log file of a real-world Go codebase analysis and are confirmed by adding specific regression tests.

The core principle of `symgo` as a symbolic tracer is to be resilient and to continue analysis whenever possible, even when encountering unresolvable or unexpected code patterns. It should favor returning a `SymbolicPlaceholder` over raising a fatal error that stops the analysis of the current function body. The bugs listed below violate this principle, as they cause the evaluation of a function body to stop, preventing the discovery of subsequent function calls and leading to an incomplete call graph.

## 1. Selector Expression Failures (`evalSelectorExpr`)

This is the most common category of failure. The evaluator crashes when a selector expression (`x.y`) is used on a type it doesn't expect.

-   **Symptom**: `expected a package, instance, or pointer on the left side of selector, but got <TYPE>`
-   **Problematic Types**: `FUNCTION`, `SLICE`, `STRING`, `MAP`.
-   **Example (Fluent API)**:
    ```go
    // app.Name(...) returns a *App, but the evaluator may mistakenly return a Function object
    // with a bound receiver, causing the chained call to fail.
    app.Name("my-app").Description("a test app")
    ```
-   **Example (Named Slice)**:
    ```go
    type MySlice []int
    func (s MySlice) Sum() int { return 0 }
    s.Sum() // Fails because the left side is a SLICE
    ```
-   **Analysis**: `evalSelectorExpr` has an incomplete `switch` statement. Instead of crashing, it should handle these unexpected types gracefully. For named types like slices, it should attempt method resolution. For unhandled types, it should return a symbolic placeholder for the result of the selection, allowing analysis to continue.

## 2. Invalid Pointer Indirection (`evalStarExpr`)

-   **Symptom**: `invalid indirect of ...` (e.g., `nil`, `instance<T>`)
-   **Example**:
    ```go
    var p *int // p is nil
    _ = *p     // Halts with "invalid indirect of nil"
    ```
-   **Analysis**: `evalStarExpr` (which handles the `*` operator) does not correctly handle cases where the operand is not a valid pointer. According to its tracer design, if it encounters a `nil` or any other non-pointer object, it should return a `SymbolicPlaceholder` representing the result of the dereference, not halt the analysis.

## 3. Unsupported Unary Operations (`evalUnaryExpr`)

-   **Symptom**: `unary operator - not supported for type UNRESOLVED_FUNCTION`
-   **Example**:
    ```go
    // os.Getpid() is treated as an unresolved function by the tracer
    x := -os.Getpid()
    ```
-   **Analysis**: `evalUnaryExpr` attempts to perform the unary operation directly. When the operand is a symbolic or unresolved type, this fails. The correct behavior for a tracer is to return a new `*object.SymbolicPlaceholder` that represents the result of the operation (e.g., "result of applying '-' to result of os.Getpid").

## 4. `len()` on Unsupported Types

-   **Symptom**: `argument to \`len\` not supported, got INSTANCE` (or `POINTER`, `VARIADIC`)
-   **Example**:
    ```go
    // getSymbolic() returns a symbolic placeholder for an instance
    x := getSymbolic()
    _ = len(x) // Fails here
    ```
-   **Analysis**: The intrinsic function that models the built-in `len()` only supports concrete slice, map, and string objects. When passed a symbolic value (which is common in `symgo`), it should return a new `*object.SymbolicPlaceholder` representing the length, not halt execution.

## 5. Invalid Binary Operation on Return Value

-   **Symptom**: `invalid left operand for complex expression: RETURN_VALUE`
-   **Example**:
    ```go
    // time.Now() returns a value that gets wrapped in *object.ReturnValue
    // The subsequent division fails.
    _ = time.Now().UnixNano() / 1e6 * 1e6
    ```
-   **Analysis**: `evalBinaryExpr` receives an `*object.ReturnValue` as an operand. It does not unwrap the underlying `Value` before attempting the binary operation, leading to a type error. The function should be updated to unwrap `*object.ReturnValue` from its operands.

## 6. Incorrect Symbol Resolution for `recover`

-   **Symptom**: `not a function: PACKAGE` when calling `recover()`
-   **Example**:
    ```go
    // In a package that also imports a library named 'recover'
    defer func() {
        _ = recover() // Incorrectly resolves to the package, not the built-in
    }()
    ```
-   **Analysis**: The evaluator's identifier resolution logic incorrectly prioritizes package names in the current scope over built-in functions like `recover()`. This leads to an attempt to "call" a package object. The resolution order should be fixed to check for built-ins first.