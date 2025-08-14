# Minigo Standard Library Compatibility limitations

This document outlines the limitations discovered while attempting to generate bindings for and test several Go standard library packages with the `minigo` interpreter.

## Summary of Core Limitations

The investigation revealed several fundamental limitations in the current `minigo` interpreter and its FFI (Foreign Function Interface) bridge. These limitations prevent many common Go programming patterns from working correctly.

1.  **No Support for Method Calls on Go Structs**: The most significant limitation is that `minigo` cannot call methods on Go structs that are returned by bound functions. The bindings only register package-level functions. When a function like `regexp.Compile` returns a `*regexp.Regexp` struct, any subsequent method calls on that struct (e.g., `compiled.FindStringSubmatch(...)`) will fail with an `undefined field` error. This affects a large portion of the standard library, which relies heavily on this object-oriented pattern.

2.  **Interpreter Halts on Returned Go Errors**: When a bound Go function returns a non-nil `error` as its second return value (a common Go pattern), the `minigo` interpreter appears to halt or panic. It does not return the error as a usable value to the script. This makes it impossible to write idiomatic error-handling code in `minigo` for functions that can fail.

3.  **No Support for Generic Functions**: The binding generator (`minigo-gen-bindings`) does not support Go generics. When it encounters a generic function, such as those in the `slices` package, it attempts to bind it without type instantiation. This results in generated Go code that fails to compile.

4.  **`byte` Type Alias is Unrecognized**: The `minigo` parser does not recognize the built-in `byte` type alias. Scripts attempting to use a type conversion like `[]byte("hello")` will fail with an `identifier not found: byte` error. The workaround is to use integer slices (e.g., `[]int{...}`) to represent byte slices.

## Package-Specific Analysis

### `time`

-   **Limitation**: Method calls and error handling.
-   **Analysis**: A test attempting to call `t.Year()` on a `time.Time` object returned by `time.Parse` failed, demonstrating the method call limitation. A second test attempting to check the error returned by `time.Parse("invalid-date")` failed because the interpreter halted instead of returning the `*time.ParseError` object.

### `bytes`

-   **Limitation**: Method calls and `byte` keyword.
-   **Analysis**: Tests failed when calling methods like `.Write()` on a `*bytes.Buffer` instance. Additionally, creating a byte slice with `[]byte("...")` failed. The working test had to be rewritten to use `bytes.Equal` and `bytes.Compare` (package-level functions) and construct byte slices using `[]int` literals.

### `regexp`

-   **Limitation**: Method calls.
-   **Analysis**: A test using `regexp.Compile` succeeded in getting a `*regexp.Regexp` object, but failed when trying to call the `.FindStringSubmatch()` method on it. Tests must be limited to package-level functions like `regexp.MatchString`.

### `slices`

-   **Limitation**: Go Generics.
-   **Analysis**: This package could not be used at all. The binding generator produced a non-compiling `install.go` file because it cannot handle generic functions. The package had to be removed from the generation list.

### `io`, `net/http`, and other interface-heavy packages

-   **Limitation (Inferred)**: Method calls on interfaces.
-   **Analysis**: While not tested directly after discovering the core limitation, these packages are expected to be largely unusable. Their functionality relies almost entirely on methods defined by interfaces (e.g., `io.Reader`, `io.Writer`) and methods on returned structs (e.g., `*http.Response`, `*http.Request`). Meaningful tests are not possible without method call support.

## Potential for Future Fixes

-   **Method Calls**: This would require a significant change to the `minigo` evaluator. The interpreter would need to be able to look up methods on the `reflect.Type` of a wrapped `object.GoValue`. This is a major undertaking.
-   **Error Handling**: The FFI bridge (`ffibridge`) needs to be modified to check for non-nil error return values and wrap them in a `minigo` error object instead of panicking. This seems more achievable than full method support.
-   **Generics**: The binding generator would need to be made aware of type parameters and skip generating bindings for generic functions. This is a feasible fix for the generator tool.
-   **`byte` Keyword**: This would require adding `byte` as a built-in type alias for `uint8` in the `minigo` parser or evaluator. This is also a feasible fix.
