# Plan: Generating and Testing Standard Library Bindings

This document outlines the plan and methodology used to generate standard library bindings for `minigo`, test them, and investigate the interpreter's limitations.

## Objective

The primary goal was to improve `minigo`'s standard library support by generating bindings for a wide range of commonly used Go packages. A key part of this task was to use this process to perform compatibility checks, identify the limitations of the `minigo` FFI, and document the findings.

## Process

### 1. Library Selection

The process began with a curated list of essential standard library packages. This list was later expanded based on user feedback to include more common packages.

**Initial List:**
- `fmt`
- `strings`
- `encoding/json`
- `strconv`
- `math/rand`
- `time`
- `bytes`
- `io`
- `os`
- `regexp`
- `text/template`

**Expanded List (User-Requested):**
- `errors`
- `net/http`
- `net/url`
- `path/filepath`
- `sort`
- `slices` (This was later removed due to limitations with Go generics).

### 2. Binding Generation

A command-line tool, `examples/minigo-gen-bindings`, was used to scan Go packages and generate `install.go` files that register package symbols with the `minigo` interpreter.

Several improvements were made to this tool and the process:

- **Correct Directory Structure**: The generator was modified to create a directory structure that mirrors the source package path (e.g., generating bindings for `encoding/json` in `minigo/stdlib/encoding/json/` rather than `minigo/stdlib/json/`).
- **Symbol Deduplication**: The generator was fixed to handle cases where a package exports a function and a constant with the same name. It now uses a map to ensure unique symbols are registered, preventing the generation of invalid Go code.
- **Automation**: A `make gen-stdlib` target was added to the root `Makefile` to automate the process of cleaning and regenerating bindings for all selected packages.

### 3. Testing Strategy

The testing approach was designed to uncover compatibility issues and limitations.

- **Test File**: A new test file, `minigo/minigo_stdlib_custom_test.go`, was created to house all new tests for the generated bindings.
- **Testing Pattern**: Tests follow a consistent pattern:
    1. A minigo script is defined as a Go string.
    2. A new `minigo.Interpreter` instance is created.
    3. The relevant standard library bindings are installed (e.g., `stdbytes.Install(interp)`).
    4. The script is loaded and evaluated by the interpreter.
    5. The state of the interpreter's global environment is checked to assert that the script produced the correct results.
- **Goal-Oriented Testing**: The initial goal was to test both common use cases and boundary conditions. However, the testing process quickly pivoted to investigating and confirming fundamental limitations of the interpreter. The tests were rewritten multiple times to isolate and prove these limitations.

### 4. Summary of Findings and Limitations

The testing phase was successful in identifying several major limitations in `minigo`'s ability to interoperate with standard Go code. The key findings include:

- Inability to call methods on Go structs returned by bound functions.
- Lack of support for Go generics.
- Interpreter halting on Go functions that return errors.
- Unrecognized `byte` type alias.

A detailed technical breakdown of these issues and their impact on each standard library package is documented in **[`trouble-minigo-stdlib-limitations.md`](./trouble-minigo-stdlib-limitations.md)**.
