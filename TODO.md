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

## To Be Implemented

### Parallel go-scan ([docs/plan-parallel-go-scan.md](./docs/plan-parallel-go-scan.md))
- [ ] **Task 1: Make `goscan.Scanner` Thread-Safe**
    - [ ] Locate every read and write operation on `s.visitedFiles`.
    - [ ] Wrap read operations with `s.mu.RLock()` and `s.mu.RUnlock()`.
    - [ ] Wrap write operations with `s.mu.Lock()` and `s.mu.Unlock()`.
- [ ] **Task 2: Refactor `scanner.scanGoFiles` for Concurrent Parsing**
    - [ ] **Sub-Task 2.1: Define a Result Struct**: Create a private struct to hold the result of a single file parse.
    - [ ] **Sub-Task 2.2: Implement the Parallel Parsing Loop**: Rewrite the beginning of `scanGoFiles` to manage goroutines.
    - [ ] **Sub-Task 2.3: Implement the Result Collection Logic**: After the `g.Wait()` call, collect all the results from the channel.
    - [ ] **Sub-Task 2.4: Adapt the Sequential Processing Logic**: The second half of the original `scanGoFiles` can now be adapted to work with the `parsedFileResults` slice.
