# Plan: Standard Library Support Strategies

This document outlines the strategies and findings related to integrating Go's standard library with `minigo`.

## Objective

The primary goal is to expand `minigo`'s standard library support, making it a more powerful and useful scripting engine. This involves finding robust methods for integrating Go packages and understanding the interpreter's capabilities and limitations.

## Strategies and Findings

Two primary methods for integrating standard library packages have been explored, each with its own trade-offs.

### Strategy 1: Direct Source Interpretation (Preferred Method)

This is a new and powerful approach that involves loading the Go source code of a standard library package directly into the `minigo` interpreter at runtime.

- **Process**:
    1. The `goscan.Locator` (with `WithGoModuleResolver`) is used to find the source directory of a stdlib package from the host `GOROOT`.
    2. A new `interpreter.LoadGoSourceAsPackage()` method reads the `.go` files, parses them into an AST, and evaluates the declarations within the interpreter's environment.
    3. This creates a native `minigo` package object that can be imported and used by scripts.

- **Success Case (`slices` package)**: This method was successfully used to implement the `slices` package. This was a significant achievement because `slices` consists entirely of generic functions, which the FFI binding generator (Strategy 2) cannot handle. This approach required enhancing the `minigo` evaluator with several new language features, including variadic arguments (`...`) and full 3-index slice expressions.

- **Advantages**:
    - **Supports Generics**: Bypasses the limitations of the FFI generator, allowing `minigo` to use modern, generic Go code.
    - **Full Compatibility**: Executes the actual Go source, providing high fidelity to Go's semantics (assuming the interpreter supports all required language features).
    - **No Pre-compilation Step**: Integration is done at runtime, simplifying the build process.

- **Limitations**:
    - The `minigo` interpreter must support all Go syntax used in the source file.
    - Does not work for packages that rely on CGO, `unsafe`, or other features that cannot be interpreted.

### Strategy 2: FFI Binding Generation (Fallback Method)

This was the original approach, which uses the `minigo-gen-bindings` tool to create a bridge between `minigo` and pre-compiled Go functions.

- **Process**: The tool scans a compiled Go package and generates an `install.go` file that registers package-level functions and constants with the `minigo` interpreter's FFI registry.

- **Advantages**:
    - Works for compiled code and can bridge to packages that use `unsafe` or CGO, as long as the functions being bound do not expose those details in their signatures.

- **Limitations**:
    - **No Generics Support**: The generator fails to create compiling code for generic functions.
    - **No Method Call Support**: The bridge only works for package-level functions. It cannot handle method calls on structs returned from functions (e.g., `(*bytes.Buffer).Write`).
    - **Brittle Error Handling**: The FFI bridge tends to halt the interpreter on Go errors rather than propagating them as `minigo` error objects.
    - **Incomplete Type Support**: Does not recognize `byte` as a built-in alias.

## Recommended Workflow for Stdlib Support

Based on these findings, the following workflow is recommended when adding a new standard library package to `minigo`:

1.  **Attempt Direct Source Interpretation First**: This is the most robust and future-proof method. Create a test case that uses `LoadGoSourceAsPackage` to load the target package and execute a simple function.
2.  **Implement Missing Language Features**: If the test fails, analyze the error to see if it's caused by a missing feature in the `minigo` evaluator (e.g., a specific AST node type is not handled). Prioritize implementing these features.
3.  **Use FFI Binding as a Fallback**: If direct interpretation is not feasible (e.g., the package has CGO dependencies), use the `minigo-gen-bindings` tool to create bindings for a curated list of essential, non-generic, package-level functions.

A detailed technical breakdown of the specific limitations encountered with the FFI binding approach is documented in **[`trouble-minigo-stdlib-limitations.md`](./trouble-minigo-stdlib-limitations.md)**.

### 3. Testing Strategy

The testing approach was designed to uncover compatibility issues and limitations.

- **Test File**: A new test file, `minigo/minigo_stdlib_custom_test.go`, was created to house all new tests for the generated bindings.
- **Testing Pattern**: Tests follow a consistent pattern:
    1. A minigo script is defined as a Go string.
    2. A new `minigo.Interpreter` instance is created.
    3. The relevant standard library bindings are installed (e.g., `stdbytes.Install(interp)`).
    4. The script is loaded and evaluated by the interpreter.
    5. The state of the interpreter's global environment is checked to assert that the script produced the correct results.
- **Goal-Oriented Testing**: The goal is to test both common use cases and boundary conditions to ensure broad compatibility. The initial testing phase successfully identified several fundamental limitations in the interpreter, which currently block more extensive testing. The tests have been adapted to validate the working functionality while clearly documenting these limitations.
