# Minigo Standard Library FFI Limitations

This document outlines the limitations discovered while attempting to generate bindings for and test several Go standard library packages with the `minigo` interpreter using the FFI binding generator (`minigo-gen-bindings`).

**Note:** A new, preferred method of integrating stdlib packages via direct source interpretation has been developed. This method bypasses many of the limitations described below. See [`plan-minigo-stdlib-limitations.md`](./plan-minigo-stdlib-limitations.md) for details on the new strategy. This document is preserved to record the specific issues with the FFI-based approach.

## Summary of Core FFI Limitations

The investigation revealed several fundamental limitations in the FFI bridge. These limitations prevent many common Go programming patterns from working correctly when called via FFI bindings.

1.  **Support for Method Calls on Go Structs**: `minigo`'s evaluator can execute methods on Go objects that are returned by FFI function calls (e.g., a `*regexp.Regexp` object returned by `regexp.Compile`). This is achieved at runtime by using reflection to look up and invoke the method on the wrapped Go value (`object.GoValue`). This is a powerful feature that enables significant compatibility with object-oriented patterns in the standard library.

2.  **Graceful Error Handling**: The FFI bridge now correctly handles Go functions that return an `error` value. Instead of halting, the interpreter wraps the non-nil `error` in an `object.GoValue`, allowing the `minigo` script to receive it and perform idiomatic error checking. This was verified with `time.Parse`.

3.  **Binding Generator Fails on Generic Functions**: The binding generator (`minigo-gen-bindings`) does not support Go generics. When it encounters a generic function, it attempts to bind it without type instantiation, resulting in generated Go code that fails to compile.

4.  **Unsupported Type Conversions**: **(FIXED)** The FFI bridge and interpreter previously had limited support for type conversions. These issues have been resolved.
    -   ~~The `[]byte("string")` conversion is not implemented, failing with a `not a function: ARRAY_TYPE` error.~~ **(FIXED)**
    -   ~~The conversion of a `minigo` array of strings to a Go `[]string` is not implemented, causing functions like `strings.Join` to fail with a `unsupported conversion from ARRAY to []string` error.~~ **(FIXED)**
    -   **Resolution**: The interpreter's type conversion logic and FFI bridge have been enhanced. Conversions for `[]byte(string)`, `string([]byte)`, `minigo` array to `[]string`, and Go `[]string` to `minigo` array are now all supported.

## Package-Specific Analysis

### `slices` (Now Supported via Source Interpretation)

-   **Original Limitation**: Go Generics.
-   **Analysis**: This package could not be used with the FFI binding generator. The generator produced a non-compiling `install.go` file because it cannot handle generic functions.
-   **Resolution**: The `slices` package was successfully implemented using the new **direct source interpretation** method. The `minigo` interpreter was enhanced to evaluate the `slices.go` source file directly, bypassing the FFI limitations entirely. This proves that the interpreter itself can handle complex generic code when it has access to the source.

### `strconv`

-   **Limitation**: None observed.
-   **Analysis**: A comprehensive FFI-based test for `strconv` now passes. This required fixing the interpreter to handle `rune` literals (e.g., `'f'`) and adding support for converting `minigo` float objects to Go `float64` types in the FFI bridge. The test covers `Atoi`, `Itoa`, `ParseFloat`, `FormatFloat`, `ParseBool`, and `FormatBool`, including error handling cases. This package is now considered highly compatible with the FFI bridge.

### `time`

-   **Limitation**: Method calls (`t.Year()`) have not been successfully tested.
-   **Analysis**: The FFI bridge now correctly handles errors from `time.Parse`, returning a non-nil error object to the script instead of halting. This is a significant improvement. However, tests for method calls on the returned `time.Time` object are still needed to confirm full compatibility, as the evaluator's general method support might not cover all cases (e.g., non-struct receivers or other `time.Time` specifics).

### `bytes`

-   **Limitation**: Method calls on returned structs (e.g., `(*bytes.Buffer).Write`) have not been tested. Direct source interpretation has known parsing bugs.
-   **Status**: **Highly Compatible (via FFI)**
-   **Analysis**: The previously blocking limitation, the lack of a `[]byte(string)` type conversion, has been **fixed**. The comprehensive FFI-based test for `bytes` now passes, confirming that all major package-level functions are compatible with the interpreter.

### `regexp`

-   **Limitation**: None observed for basic FFI usage.
-   **Analysis**: A test using `regexp.Compile` successfully returns a `*regexp.Regexp` object. Crucially, method calls on this returned object (e.g., `re.MatchString(...)`) are supported by the evaluator's reflection-based method invocation and work correctly. This demonstrates that the FFI bridge can handle object-oriented patterns.

### `text/template`

-   **Limitation**: None observed for basic FFI usage. **(FIXED)**
-   **Analysis**: A test using the common `template.New("...").Parse("...").Execute(...)` pattern now passes. Previously, this test failed because the FFI wrapper for method calls would incorrectly discard a `nil` error value in `(value, error)` return pairs. This caused a multi-value assignment error in the `minigo` script. The FFI logic has been corrected to always return all values, ensuring that a `nil` error is correctly passed to the script as `nil`. This demonstrates that the FFI can now handle methods with multi-value returns correctly.

### `io`, `net/http`, and other interface-heavy packages

-   **Limitation (Inferred)**: Method calls on interfaces.
-   **Analysis**: While not tested directly after discovering the core limitation, these packages are expected to be largely unusable via FFI. Their functionality relies almost entirely on methods defined by interfaces (e.g., `io.Reader`, `io.Writer`) and methods on returned structs (e.g., `*http.Response`, `*http.Request`). Meaningful tests are not possible without method call support.

## Potential for Future Fixes

-   **Generics (for FFI)**: The binding generator could be improved to simply ignore generic functions, preventing it from generating non-compiling code. However, given the success of the source interpretation method, improving the FFI generator for generics is a low priority.
-   **`byte` Keyword**: ~~This would require adding `byte` as a built-in type alias for `uint8` in the `minigo` parser or evaluator. This is a feasible fix.~~ **(FIXED)** The interpreter now recognizes `byte` as a built-in type name.

### `errors`

-   **Limitation (Direct Source Interpretation)**: ~~Sequential Declaration Order~~. **(FIXED)**
-   **Analysis**: An attempt to load `errors` via direct source interpretation previously failed because the interpreter processed declarations sequentially.
-   **Resolution**: The interpreter now implements a **two-pass evaluation strategy**. The first pass registers all top-level identifiers (types, functions, vars, consts), and the second pass evaluates variable and constant initializers. This resolves the sequential declaration issue, and the `errors` package can now be interpreted from source.

### `strings`
-   **Limitation**: Direct source interpretation is not possible due to the lack of string indexing support (`s[i]`) in the interpreter.
-   **Status**: **Highly Compatible (via FFI)**
-   **Analysis**: The previously blocking FFI limitations for this package have been **fixed**. The interpreter now supports converting `minigo` arrays to Go `[]string` (for `strings.Join`) and converting Go `[]string` back to `minigo` arrays (for the result of `strings.Split`). The comprehensive FFI-based test now passes.

### `sort` (FFI)

-   **Limitation**: None observed for tested functions.
-   **Analysis**: FFI-based tests for `sort.IntsAreSorted`, `sort.Ints`, and `sort.Float64s` all passed. This includes functions that take slice arguments and, in the case of `sort.Ints`, modify the slice in-place.

### `sort` (Direct Source Interpretation)

-   **Limitation**: ~~No Transitive Dependency Resolution~~. **(FIXED)**
-   **Analysis**: An attempt to use `sort.Ints` previously failed because the interpreter did not recursively load dependencies (i.e., `sort`'s import of `slices`).
-   **Resolution**: The `go-scan` library's file merging logic was fixed, and the `minigo` interpreter was enhanced to create a unified `FileScope` for all files in a package. This ensures that when a package like `sort` is loaded, its own imports (like `slices`) are correctly resolved.

### `math/rand`

-   **Limitation**: None observed for FFI usage.
-   **Status**: **Highly Compatible (via FFI)**
-   **Analysis**: The `math/rand` package is fully compatible with the FFI bridge. Tests confirm that scripts can create new `rand.Rand` instances (`rand.New(rand.NewSource(seed))`) and call methods on them (e.g., `r.Intn(100)`) to get deterministic random numbers. This is the recommended approach for testing, as using the global functions (`rand.Seed`, `rand.Intn`) can lead to non-deterministic results due to test runner state pollution.

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
