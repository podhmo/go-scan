# Minigo Standard Library FFI Limitations

This document outlines the limitations discovered while attempting to generate bindings for and test several Go standard library packages with the `minigo` interpreter using the FFI binding generator (`minigo-gen-bindings`).

**Note:** A new, preferred method of integrating stdlib packages via direct source interpretation has been developed. This method bypasses many of the limitations described below. See [`plan-minigo-stdlib-limitations.md`](./plan-minigo-stdlib-limitations.md) for details on the new strategy. This document is preserved to record the specific issues with the FFI-based approach.

## Summary of Core FFI Limitations

The investigation revealed several fundamental limitations in the FFI bridge. These limitations prevent many common Go programming patterns from working correctly when called via FFI bindings.

1.  **No Support for Method Calls on Go Structs**: The most significant limitation is that `minigo` cannot call methods on Go structs that are returned by bound functions. The bindings only register package-level functions. When a function like `regexp.Compile` returns a `*regexp.Regexp` struct, any subsequent method calls on that struct (e.g., `compiled.FindStringSubmatch(...)`) will fail with an `undefined field` error. This affects a large portion of the standard library, which relies heavily on this object-oriented pattern.

2.  **Interpreter Halts on Returned Go Errors**: When a bound Go function returns a non-nil `error` as its second return value (a common Go pattern), the `minigo` interpreter appears to halt or panic. It does not return the error as a usable value to the script. This makes it impossible to write idiomatic error-handling code in `minigo` for functions that can fail.

3.  **Binding Generator Fails on Generic Functions**: The binding generator (`minigo-gen-bindings`) does not support Go generics. When it encounters a generic function, it attempts to bind it without type instantiation, resulting in generated Go code that fails to compile.

4.  **`byte` Type Alias is Unrecognized**: The `minigo` parser does not recognize the built-in `byte` type alias. Scripts attempting to use a type conversion like `[]byte("hello")` will fail with an `identifier not found: byte` error. The workaround is to use integer slices (e.g., `[]int{...}`) to represent byte slices.

## Package-Specific Analysis

### `slices` (Now Supported via Source Interpretation)

-   **Original Limitation**: Go Generics.
-   **Analysis**: This package could not be used with the FFI binding generator. The generator produced a non-compiling `install.go` file because it cannot handle generic functions.
-   **Resolution**: The `slices` package was successfully implemented using the new **direct source interpretation** method. The `minigo` interpreter was enhanced to evaluate the `slices.go` source file directly, bypassing the FFI limitations entirely. This proves that the interpreter itself can handle complex generic code when it has access to the source.

### `time`

-   **Limitation**: Method calls and error handling.
-   **Analysis**: A test attempting to call `t.Year()` on a `time.Time` object returned by `time.Parse` failed, demonstrating the method call limitation. A second test attempting to check the error returned by `time.Parse("invalid-date")` failed because the interpreter halted instead of returning the `*time.ParseError` object.

### `bytes`

-   **Limitation**: Method calls and `byte` keyword.
-   **Analysis**: Tests failed when calling methods like `.Write()` on a `*bytes.Buffer` instance. Additionally, creating a byte slice with `[]byte("...")` failed. The working test had to be rewritten to use `bytes.Equal` and `bytes.Compare` (package-level functions) and construct byte slices using `[]int` literals.

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
-   **`byte` Keyword**: ~~This would require adding `byte` as a built-in type alias for `uint8` in the `minigo` parser or evaluator. This is a feasible fix.~~ **(FIXED)** The interpreter now recognizes `byte`, `uint`, `uint64`, and `float64` as built-in type names.

### `errors`

-   **Limitation (Direct Source Interpretation)**: ~~Sequential Declaration Order~~. **(FIXED)**
-   **Analysis**: An attempt to load `errors` via direct source interpretation previously failed because the interpreter processed declarations sequentially.
-   **Resolution**: The interpreter now implements a **two-pass evaluation strategy**. The first pass registers all top-level identifiers (types, functions, vars, consts), and the second pass evaluates variable and constant initializers. This resolves the sequential declaration issue, and the `errors` package can now be interpreted from source.

### `strings`

-   **Limitation (Direct Source Interpretation)**: No String Indexing.
-   **Analysis**: An attempt to use `strings.ToUpper` fails with the error `index operator not supported for STRING`. The `minigo` interpreter does not currently support accessing individual characters or bytes of a string via an index (e.g., `s[i]`). This is a fundamental language feature required by many functions in the `strings` package.

### `sort`

-   **Limitation (Direct Source Interpretation)**: ~~No Transitive Dependency Resolution~~. **(FIXED)**
-   **Analysis**: An attempt to use `sort.Ints` previously failed because the interpreter did not recursively load dependencies (i.e., `sort`'s import of `slices`).
-   **Resolution**: The `go-scan` library's file merging logic was fixed, and the `minigo` interpreter was enhanced to create a unified `FileScope` for all files in a package. This ensures that when a package like `sort` is loaded, its own imports (like `slices`) are correctly resolved.

### `bytes`

-   **Limitation (Direct Source Interpretation)**: Incorrect Function Signature Parsing.
-   **Analysis**: An attempt to use `bytes.Equal` fails with the error `wrong number of arguments. got=2, want=1`. The `minigo` interpreter incorrectly determines that `bytes.Equal` takes only one argument, despite its Go signature being `func Equal(a, b []byte) bool`. This suggests a flaw in how the interpreter parses or registers function signatures from source files.

### `path/filepath`

-   **Limitation (Direct Source Interpretation)**: Sequential Declaration Order.
-   **Analysis**: An attempt to use `filepath.Join` fails with the error `identifier not found: join`. This is the same limitation discovered with the `errors` package, where an exported function relies on an unexported helper function that is defined later in the source file.

### `strconv`

-   **Limitation (Direct Source Interpretation)**: Sequential Declaration Order.
-   **Analysis**: An attempt to use `strconv.Atoi` fails with the error `identifier not found: intSize`. This is another instance of the sequential declaration limitation, where an unexported constant (`intSize`) is used by a function before it has been declared in the file.

### `encoding/json`

-   **Limitation (Direct Source Interpretation)**: Sequential Declaration Order.
-   **Analysis**: An attempt to use `json.Marshal` fails with the error `identifier not found: newEncodeState`. The test did not even reach code that uses `reflect`, as it was blocked by the same sequential declaration limitation seen in other packages. An unexported helper function (`newEncodeState`) is called before it is defined in the source file.

---

## Analysis of Untested Complex Packages

Based on the limitations discovered above, the remaining standard library packages were not individually tested, as their failure is predictable.

-   **`os`, `net/http`, `net`**: These packages are fundamentally incompatible because they rely on `CGO` and direct `syscalls` to interact with the operating system and network stack. The `minigo` interpreter is pure Go and cannot execute C code or make system calls.
-   **`time`, `net/url`, `regexp`**: These packages rely heavily on methods defined on their core struct types (`time.Time`, `url.URL`, `regexp.Regexp`). As discovered with the FFI-based tests, `minigo` does not support method calls on Go objects, so these would fail.
-   **`io`**: This package's utility comes from its core interfaces, `io.Reader` and `io.Writer`. While `minigo` has some support for interfaces, the complexity of implementing and using them for I/O operations is beyond its current capabilities.
-   **`fmt`, `text/template`**: These packages are highly complex and make extensive use of reflection (`reflect`), which is not fully supported by `minigo`. They would also fail due to the other limitations already identified (sequential declaration, method calls, etc.).
