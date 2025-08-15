# Minigo Standard Library FFI Limitations

This document outlines the limitations discovered while attempting to generate bindings for and test several Go standard library packages with the `minigo` interpreter using the FFI binding generator (`minigo-gen-bindings`).

**Note:** A new, preferred method of integrating stdlib packages via direct source interpretation has been developed. This method bypasses many of the limitations described below. See [`plan-minigo-stdlib-limitations.md`](./plan-minigo-stdlib-limitations.md) for details on the new strategy. This document is preserved to record the specific issues with the FFI-based approach.

## Summary of Core FFI Limitations

The investigation revealed several fundamental limitations in the FFI bridge. These limitations prevent many common Go programming patterns from working correctly when called via FFI bindings.

1.  **No Support for Method Calls on Go Structs**: The most significant limitation is that `minigo` cannot call methods on Go structs that are returned by bound functions. The bindings only register package-level functions. When a function like `regexp.Compile` returns a `*regexp.Regexp` struct, any subsequent method calls on that struct (e.g., `compiled.FindStringSubmatch(...)`) will fail with an `undefined field` error. This affects a large portion of the standard library, which relies heavily on this object-oriented pattern.

2.  **Interpreter Halts on Returned Go Errors**: When a bound Go function returns a non-nil `error` as its second return value (a common Go pattern), the `minigo` interpreter appears to halt or panic. It does not return the error as a usable value to the script. This makes it impossible to write idiomatic error-handling code in `minigo` for functions that can fail.

3.  **Binding Generator Fails on Generic Functions**: The binding generator (`minigo-gen-bindings`) does not support Go generics. When it encounters a generic function, it attempts to bind it without type instantiation, resulting in generated Go code that fails to compile.

4.  **Unsupported Type Conversions**: The FFI bridge and interpreter have limited support for type conversions.
    -   The `[]byte("string")` conversion is not implemented, failing with a `not a function: ARRAY_TYPE` error.
    -   The conversion of a `minigo` array of strings to a Go `[]string` is not implemented, causing functions like `strings.Join` to fail with a `unsupported conversion from ARRAY to []string` error.

## Package-Specific Analysis

### `slices` (Now Supported via Source Interpretation)

-   **Original Limitation**: Go Generics.
-   **Analysis**: This package could not be used with the FFI binding generator. The generator produced a non-compiling `install.go` file because it cannot handle generic functions.
-   **Resolution**: The `slices` package was successfully implemented using the new **direct source interpretation** method. The `minigo` interpreter was enhanced to evaluate the `slices.go` source file directly, bypassing the FFI limitations entirely. This proves that the interpreter itself can handle complex generic code when it has access to the source.

### `time`

-   **Limitation**: Method calls and error handling.
-   **Analysis**: A test attempting to call `t.Year()` on a `time.Time` object returned by `time.Parse` failed, demonstrating the method call limitation. A second test attempting to check the error returned by `time.Parse("invalid-date")` failed because the interpreter halted instead of returning the `*time.ParseError` object.

### `bytes`

-   **Limitations**:
    -   **FFI**: Method calls (e.g., `(*bytes.Buffer).Write`) are not supported.
    -   **FFI**: The `[]byte("string")` type conversion is not supported by the interpreter.
    -   **Direct Source Interpretation**: The interpreter incorrectly parses the function signature for `bytes.Equal`, causing a "wrong number of arguments" error.
-   **Analysis**: The `bytes` package is partially usable via FFI bindings for its package-level functions (`Equal`, `Compare`, `Contains`, etc.). However, scripts must manually construct byte slices as `[]int` literals. The idiomatic Go conversion `[]byte("...")` fails because the interpreter does not have a built-in conversion handler and incorrectly treats it as a function call on the `ARRAY_TYPE`. Direct source interpretation is currently blocked by a bug in the interpreter's function signature parsing logic.

### `regexp`

-   **Limitation**: Method calls.
-   **Analysis**: A test using `regexp.Compile` succeeded in getting a `*regexp.Regexp` object, but failed when trying to call the `.FindStringSubmatch()` method on it. Tests must be limited to package-level functions like `regexp.MatchString`.

### `io`, `net/http`, and other interface-heavy packages

-   **Limitation (Inferred)**: Method calls on interfaces.
-   **Analysis**: While not tested directly after discovering the core limitation, these packages are expected to be largely unusable via FFI. Their functionality relies almost entirely on methods defined by interfaces (e.g., `io.Reader`, `io.Writer`) and methods on returned structs (e.g., `*http.Response`, `*http.Request`). Meaningful tests are not possible without method call support.

## Potential for Future Fixes

-   **Method Calls**: This would require a significant change to the `minigo` evaluator, allowing it to look up methods on the `reflect.Type` of a wrapped `object.GoValue`. This is a major undertaking.
-   **Error Handling**: The FFI bridge (`ffibridge`) needs to be modified to check for non-nil error return values and wrap them in a `minigo` error object instead of panicking. This seems more achievable than full method support.
-   **Generics (for FFI)**: The binding generator could be improved to simply ignore generic functions, preventing it from generating non-compiling code. However, given the success of the source interpretation method, improving the FFI generator for generics is a low priority.
-   **`byte` Keyword**: ~~This would require adding `byte` as a built-in type alias for `uint8` in the `minigo` parser or evaluator. This is a feasible fix.~~ **(FIXED)** The interpreter now recognizes `byte` as a built-in type name.

### `errors`

-   **Limitation (Direct Source Interpretation)**: ~~Sequential Declaration Order~~. **(FIXED)**
-   **Analysis**: An attempt to load `errors` via direct source interpretation previously failed because the interpreter processed declarations sequentially.
-   **Resolution**: The interpreter now implements a **two-pass evaluation strategy**. The first pass registers all top-level identifiers (types, functions, vars, consts), and the second pass evaluates variable and constant initializers. This resolves the sequential declaration issue, and the `errors` package can now be interpreted from source.

### `strings`
-   **Limitations**:
    -   **FFI**: The FFI bridge cannot convert a `minigo` array of strings into a Go `[]string`, causing functions like `strings.Join` to fail.
    -   **Direct Source Interpretation**: The interpreter does not support string indexing (`s[i]`), which is required by many functions in the `strings` package.
-   **Analysis**: The `strings` package is partially usable via FFI for functions that take and return simple strings (`Contains`, `HasPrefix`, `ToUpper`, etc.). However, functions that operate on slices of strings (`Join`) fail due to the FFI's missing type conversion logic. Direct source interpretation is blocked by the lack of string indexing support in the interpreter, a fundamental language feature.

### `sort`

-   **Limitation (Direct Source Interpretation)**: ~~No Transitive Dependency Resolution~~. **(FIXED)**
-   **Analysis**: An attempt to use `sort.Ints` previously failed because the interpreter did not recursively load dependencies (i.e., `sort`'s import of `slices`).
-   **Resolution**: The `go-scan` library's file merging logic was fixed, and the `minigo` interpreter was enhanced to create a unified `FileScope` for all files in a package. This ensures that when a package like `sort` is loaded, its own imports (like `slices`) are correctly resolved.

---

## Fundamental Design Limitations

Beyond the specific features required for individual packages, the investigation revealed some fundamental design choices in `minigo` that limit its broader compatibility with standard Go code.

### Integer Type Simplification

-   **Description**: For simplicity, the `minigo` interpreter treats all Go integer types (`int`, `int8`, `uint8`, `uint64`, etc.) as a single internal type: `object.Integer`, which holds a standard `int64` value. The original type information is discarded during evaluation.
-   **Limitation**: This simplification prevents `minigo` from correctly handling:
    1.  **Unsigned Integers**: The distinction between signed and unsigned integers is lost.
    2.  **Integer Overflow**: `minigo` does not replicate Go's specific overflow rules for different integer sizes.
    3.  **Large `uint64` Values**: Values greater than `math.MaxInt64` cannot be represented, even though they are valid in Go's `uint64`.
-   **Impact**: While this simplification works for many common cases, it makes `minigo` unsuitable for scripts that rely on precise integer typing, bitwise operations on unsigned integers, or large `uint64` values (e.g., in cryptography or hashing packages).
-   **Potential Solutions**:
    -   **Approach A (High Effort, High Correctness)**: Introduce distinct object types for different integer kinds (e.g., `object.Uint8`, `object.Int32`, `object.Uint64`). This would be a major undertaking, requiring changes to the parser, evaluator, object system, and FFI bridge, but would provide the most accurate simulation of Go's type system.
    -   **Approach B (Medium Effort, Medium Correctness)**: Enhance the existing `object.Integer` to include metadata about its original Go type (e.g., `Kind: reflect.Uint8`). This would allow the FFI bridge to make more intelligent conversions and could enable the evaluator to simulate some type-specific behaviors without a full type system overhaul.

## Analysis of Untested Complex Packages

Based on the limitations discovered above, the remaining standard library packages were not individually tested, as their failure is predictable.

-   **`os`, `net/http`, `net`**: These packages are fundamentally incompatible because they rely on `CGO` and direct `syscalls` to interact with the operating system and network stack. The `minigo` interpreter is pure Go and cannot execute C code or make system calls.
-   **`time`, `net/url`, `regexp`**: These packages rely heavily on methods defined on their core struct types (`time.Time`, `url.URL`, `regexp.Regexp`). As discovered with the FFI-based tests, `minigo` does not support method calls on Go objects, so these would fail.
-   **`io`**: This package's utility comes from its core interfaces, `io.Reader` and `io.Writer`. While `minigo` has some support for interfaces, the complexity of implementing and using them for I/O operations is beyond its current capabilities.
-   **`fmt`, `text/template`**: These packages are highly complex and make extensive use of reflection (`reflect`), which is not fully supported by `minigo`. They would also fail due to the other limitations already identified (sequential declaration, method calls, etc.).
