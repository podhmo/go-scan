# TODO

> **Note on updating this file:**
> -   Do not move individual tasks to the "Implemented" section.
> -   A whole feature section (e.g., "convert Tool Implementation") should only be moved to "Implemented" when all of its sub-tasks are complete.
> -   For partially completed features, use checkboxes (`[x]` for complete, `[-]` for partially complete). A feature is considered partially complete if it has been implemented but has associated tests that are currently disabled.
> -   For partially completed features, use checkboxes (`[x]`) to mark completed sub-tasks.

This file tracks implemented features and immediate, concrete tasks.

For more ambitious, long-term features, see [docs/near-future.md](./docs/near-future.md).

## Implemented

- **Core Scanning Engine**: A robust, AST-based engine for parsing Go code. It supports lazy, on-demand, cross-package type resolution, and correctly handles complex scenarios like recursive types and generic type definitions. It can extract detailed information about all major Go constructs, including structs, functions, interfaces, and constants.
- **Dependency Analysis**: Includes the `deps-walk` command-line tool for visualizing dependency graphs (in DOT or Mermaid format) and a powerful underlying library for programmatic graph traversal, including forward and reverse dependency analysis.
- **Code Generation Framework**:
    - **`convert` Tool**: A feature-rich tool for generating type-safe conversion functions, driven by annotations (`@derivingconvert`), struct tags (`convert:"..."`), and global rules (`// convert:rule`). Supports nested types, custom functions, and comprehensive error collection.
    - **`derivingjson` & `derivingbind`**: Tools for generating JSON marshaling/unmarshaling and request binding logic.
    - **Unified Generator (`deriving-all`)**: An efficient, single-pass generator that combines the functionality of `derivingjson` and `derivingbind`.
- **Developer Experience & Testing**:
    - **`scantest` Library**: A testing harness for creating isolated, in-memory tests for tools built with `go-scan`.
    - **In-Memory File Overlay**: Allows providing file content in memory, essential for testing and tools that modify code before scanning.
    - **Debuggability**: Provides `--inspect` and `--dry-run` modes for easier debugging and testing of code generators.
- **`minigo` Script Engine**: A nearly complete, embeddable script engine that interprets a large subset of Go.
    - **Core Interpreter**: The engine is fully implemented, a`supp`orting expressions, variables (`var`, `const`, `iota`), assignments, and all major control flow statements (`if`, `for`, `switch`, `break`, `continue`).
    - **Functions and Data Structures**: Supports user-defined functions, rich error reporting with stack traces, and composite types including structs, slices, and maps.
    - **Advanced Language Features**: Includes full support for pointers (`&`, `*`), method definitions on structs, interface definitions and dynamic dispatch, struct embedding, and basic generics.
    - **Go Interoperability**: Provides a robust bridge to Go, allowing scripts to call Go functions, access Go variables, and unmarshal script results back into Go structs via `Result.As()`. Lazy, on-demand loading of imported Go packages is also supported.
- **Final API for `convert` Tool**: A new IDE-native method for configuring the `convert` tool using a `define` package. This allows for type-safe, statically valid Go code for defining conversion rules, improving the developer experience over the previous annotation-based system.
    - [x] **`minigo` Enhancements**: The underlying `minigo` interpreter was enhanced with special form support, allowing it to analyze the AST of function arguments.
    - [x] **`define` API**: A new `define` package with functions like `Convert`, `Rule`, and `NewMapping` was created to provide a clean, user-facing API.
    - [x] **`convert-define` Command**: A new command was created to parse these definition files and generate the conversion code.
    - [x] **Comprehensive Documentation**: The `README.md` for the `convert` example was updated to reflect the new recommended workflow.
- **Parallel go-scan**:
    - [x] **Task 1: Make `goscan.Scanner` Thread-Safe**
    - [x] **Task 2: Refactor `scanner.scanGoFiles` for Concurrent Parsing**
- **Automated Minigo Bindings Generation** ([docs/plan-minigo-gen-bindings.md](./docs/plan-minigo-gen-bindings.md)):
    - [x] **Core Function: List Exported Symbols**
    - [x] **Build the Generator Tool**
    - [x] **Generate and Test Standard Library Bindings**:
        - [x] `fmt`
        - [x] `strings`
        - [x] `strconv`
        - [-] `encoding/json` (bindings generated, but runtime support is incomplete; see `docs/trouble-minigo-encoding-json.md`)
    - [x] **Generate and Test Standard Library Bindings**
- **MiniGo REPL** ([docs/plan-minigo-repl.md](./docs/plan-minigo-repl.md)):
    - [x] **Task 1: Modify `main.go` to conditionally start the REPL.**
    - [x] **Task 2: Implement the basic REPL loop in `main.go`.**
    - [x] **Task 3: Add the `replFileScope` field to the `Interpreter` struct.**
    - [x] **Task 4: Implement the `EvalLine` method in `minigo.go`.**
    - [x] **Task 5: Integrate `EvalLine` into the REPL loop.**
    - [x] **Task 6: Implement the `.help` and `.reset` meta-commands.**
    - [x] **Task 7: Verify `import` statement functionality.**
    - [x] **Task 8: Add tests for the new REPL functionality.**
    - [x] **Task 9: Run `make format` and `make test` to ensure all checks pass.**
- **Full `encoding/json` Support in `minigo`** ([docs/trouble-minigo-encoding-json.md](docs/trouble-minigo-encoding-json.md)):
    - [x] **Implement `json.Marshal` for structs**: Enhance the FFI to convert `minigo` structs to `map[string]any` when calling Go functions that accept `interface{}`, as detailed in `docs/trouble-minigo-encoding-json.md`.
    - [x] **Support `json.Unmarshal`**: Implemented a recursive `json.Unmarshal` solution with an FFI pointer bridge. It now supports nested, recursive, and cross-package structs.
    - [x] **Support Struct Field Tags**: Requires parser and object model changes to recognize and utilize `json:"..."` tags.
- **Extended Standard Library Support & Compatibility Analysis**:
    - [x] **Fix Binding Generator**: Patched the `minigo-gen-bindings` tool to support sub-package directory structures (e.g., `encoding/json`) and to de-duplicate symbols, preventing compilation errors.
    - [x] **Automate Generation**: Added a `make gen-stdlib` target to automate the generation of a wide range of stdlib packages.
    - [x] **Implement Test Suite**: Created a test suite (`minigo/minigo_stdlib_custom_test.go`) to validate the generated bindings.
    - [x] **Investigate & Document Limitations**: Through testing, identified and documented several core limitations of the `minigo` interpreter. See `docs/trouble-minigo-stdlib-limitations.md` for a full analysis.
    - [x] **Generated Bindings For**:
        - `fmt`, `strings`, `encoding/json`, `strconv`, `math/rand`, `time`, `bytes`, `io`, `os`, `regexp`, `text/template`, `errors`, `net/http`, `net/url`, `path/filepath`, `sort`.


## To Be Implemented


### `minigo` Standard Library Support (`slices`)
- [x] **Implement source loading**: Add a mechanism (`LoadGoSourceAsPackage`) to load a Go source file and evaluate it as a self-contained package.
- [x] **Add required language features**: To support `slices.Clone`, implement the following in the evaluator:
    - [x] Evaluation of `*ast.File` nodes.
    - [x] Assignment to index expressions (`slice[i] = value`).
    - [x] Full 3-index slice expressions (`slice[low:high:max]`).
    - [x] Variadic arguments in function calls (`...`).

### `minigo` FFI and Language Limitations ([docs/trouble-minigo-stdlib-limitations.md](./docs/trouble-minigo-stdlib-limitations.md))
- [x] **Implement Method Calls on Go Objects**: Enhance the interpreter to support calling methods on Go structs returned from bound functions (e.g., `(*bytes.Buffer).Write`). This is the highest-impact improvement for stdlib compatibility. (See `docs/trouble-minigo-stdlib-limitations.md`).
- [x] **Graceful Error Handling for Go Functions**: Modify the FFI to return `error` values from Go functions as `minigo` error objects, rather than halting execution.
- [x] **Fix FFI method call return handling**: The FFI wrapper for method calls currently discards `nil` error values in `(value, error)` returns, preventing correct multi-value assignment. This was discovered while testing `text/template`.
- [x] **Improve FFI Support for Go Generics**: Update the binding generator to correctly handle (or at least ignore) generic Go functions to prevent it from generating non-compiling code. This is a limitation of the binding tool, not the core interpreter.
- [x] **Add `byte` as a Built-in Type**: Add the `byte` keyword as a built-in alias for `uint8` in the interpreter to support `[]byte` literals.

### `minigo` Standard Library Compatibility Analysis (`bytes`, `strings`, `math/rand`)
- [x] **Write tests for `bytes` package functions.**
- [x] **Write tests for `strings` package functions.**
- [x] **Write tests for `math/rand` package functions.**
- [x] **Analyze test results and document limitations.**
- [x] **Update `docs/trouble-minigo-stdlib-limitations.md` with findings.**

### Future Interpreter Enhancements (for Stdlib Support)
- [x] **Implement two-pass evaluation for top-level declarations**: To fix the "Sequential Declaration Processing" limitation, modify the interpreter to first scan all top-level declarations (types, funcs, vars, consts) in a package before evaluating any code.
- [x] **Add support for string indexing**: Enhance the evaluator to handle the index operator (`s[i]`) on string objects. 
- [x] **Implement transitive dependency loading**: Add a mechanism to the interpreter to automatically load and parse imported packages that are not already in memory.
- [x] **Audit and fix function signature parsing**: Investigate and fix bugs in the function signature parsing logic, using the `bytes.Equal` case as a starting point.
- [x] **Improve FFI type conversions**:
    - [x] Implement conversion from `minigo` array of strings to Go `[]string`.
- [x] **Add built-in type conversions**:
    - [x] Implement mutual conversion between `string` and `[]byte` (e.g., `[]byte("foo")`).

### `minigo` Refinements ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [x] **Implement Remaining Built-in Functions**:
    - [x] `copy`
    - [x] `delete`
    - [x] `cap`
    - [x] `make`
    - [x] `new`
    - [x] `complex`
    - [x] `real`
    - [x] `imag`
    - [x] `clear`
    - [x] `close`
    - [x] `panic`
    - [x] `recover`
- [x] **Range Over Function**: Support `for...range` loops over functions.
- [x] **Support Increment and Decrement Operators**: Implement `++` and `--` as statements.
- [ ] Write comprehensive documentation for the API, supported language features, and usage examples.

### `minigo` Interpreter Enhancements
- [x] **Support arbitrary byte sequences in string literals**: The interpreter fails when it encounters string constants containing null bytes (`\x00`), such as `len8tab` from the `math/bits` package. This blocks the interpretation of packages like `slices` that depend on it. The string handling logic in `minigo` needs to be improved to support these literals correctly.
- [ ] **Fix generic type parameter scope**: The interpreter fails to resolve type parameters (e.g., `E` in `slices.Sort[S ~[]E, E cmp.Ordered]`) within the body of a generic function when the function is loaded from source. This points to a bug in how the evaluation environment is managed for generic functions. This currently blocks the use of the `slices` package.

### Toolchain Improvements
- [x] **Improve `go-scan` constant evaluation**: The static analyzer currently fails to resolve the values of some computed constants (e.g., `len8tab` in `math/bits`), causing `minigo` to fail when interpreting packages that depend on them (like `slices.Sort`). The scanner should be enhanced to evaluate these constant expressions.

### `minigo` Language and FFI Enhancements from Stdlib Investigation ([docs/trouble-minigo-stdlib-limitations.md](./docs/trouble-minigo-stdlib-limitations.md))
- [x] **Support Struct Literals with Local Variables**: Enhance the evaluator to handle struct literals that are initialized with variables from the current function's scope (e.g., `&errorString{text}`). This was found to be a blocker for interpreting the `errors` package from source.
- [x] **Support Methods on In-Script Pointers**: Enable the FFI bridge to resolve method calls on pointers to structs that are created and manipulated entirely within a `minigo` script (e.g., `var s scanner.Scanner; var p = &s; p.Init(...)`). This was a blocker for using the `text/scanner` package.
- [x] **Fix Generic Function Argument Counting**: The interpreter incorrectly counts arguments for generic functions when multiple arguments share the same generic type parameter (e.g., `func Equal[S ~[]E, E comparable](s1, s2 S) bool`). This causes `wrong number of arguments` errors.
- [x] **Support Interfaces with Type Lists**: The interpreter can now parse interfaces defined with a type list (e.g., `type Ordered interface { ~int | ~string }`). This unblocks interpretation of packages like `cmp`.
- [x] **Improve Stack Trace for Non-Existent Files**: Ensure a full stack trace is displayed even when a source file mentioned in the trace does not exist on disk (e.g., `[Error opening source file: open main.go: no such file or directory]`).

### `minigo` FFI Struct Instantiation ([docs/trouble-minigo-go-value-method-call.md](./docs/trouble-minigo-go-value-method-call.md))
- [x] **Task 1: Differentiate FFI types from in-script types.**
  - In `evalGenDecl`, when evaluating `var s T`, determine if `T` refers to a Go type from the FFI or a struct defined in the script. This may require adding a flag or method to `object.Type` or `object.StructDefinition` to mark its origin.
- [x] **Task 2: Instantiate FFI types as `*object.GoValue`.**
  - Modify `evalGenDecl` so that if `T` is an FFI type, it creates a `*object.GoValue` that wraps a zero-valued instance of the corresponding Go type (`reflect.Zero(goType)`). This will involve looking up the `reflect.Type` from a registry.
- [x] **Task 3: Update pointer evaluation logic.**
  - The `evalSelectorExpr` and `assignValue` functions need to be updated to correctly handle `*object.Pointer`s that point to `*object.GoValue`s, allowing method calls and field assignments to work via reflection.
- [x] **Task 4: Re-enable and verify the `text/scanner` test.**
  - Remove the skip from `TestStdlib_TextScanner_FFI` in `minigo/minigo_stdlib_custom_test.go` and ensure it passes.
- [x] **Task 5: Update documentation.**
  - Update `docs/trouble-minigo-go-value-method-call.md` and `docs/trouble-minigo-stdlib-limitations.md` to reflect that the issue is resolved.
