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
    - **Core Interpreter**: The engine is fully implemented, supporting expressions, variables (`var`, `const`, `iota`), assignments, and all major control flow statements (`if`, `for`, `switch`, `break`, `continue`).
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
    - [x] **Generate and Test Standard Library Bindings**

## To Be Implemented

### `minigo` Refinements ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [ ] **Implement Remaining Built-in Functions**:
    - [x] `copy`
    - [x] `delete`
    - [x] `cap`
    - [x] `make`
    - [x] `new`
    - [ ] `complex`
    - [ ] `real`
    - [ ] `imag`
    - [x] `clear`
    - [ ] `close`
    - [x] `panic`
    - [x] `recover`
- [x] **Range Over Function**: Support `for...range` loops over functions.
- [x] **Support Increment and Decrement Operators**: Implement `++` and `--` as statements.
- [ ] Write comprehensive documentation for the API, supported language features, and usage examples.
