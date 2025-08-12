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

### Final API for `convert` Tool ([docs/plan-convert-with-minigo.md](./docs/plan-convert-with-minigo.md))
- [ ] **Phase 1: `minigo` Core Enhancements**
  - [ ] Implement `object.AstNode`
  - [ ] Implement Special Form Mechanism
  - [ ] Enhance Go Interop Layer
  - [ ] Add Unit Tests
- [ ] **Phase 2: `define` Tool and Parser Implementation**
  - [ ] Create `examples/convert/define` Package
  - [ ] Create CLI Entrypoint
  - [ ] Implement Core Parser
  - [ ] Implement `define.Rule` Parsing
  - [ ] Implement `define.Mapping` Parsing
  - [ ] Implement `Config` Method Parsing
  - [ ] IR Construction
- [ ] **Phase 3: Generator Integration and Finalization**
  - [ ] Enhance Generator for Implicit Mapping
  - [ ] Integrate Parser and Generator
  - [ ] Add Integration Tests
  - [ ] Write User Documentation

### Automated Minigo Bindings Generation ([docs/plan-minigo-gen-bindings.md](./docs/plan-minigo-gen-bindings.md))
- [ ] **Core Function: List Exported Symbols**
- [ ] **Build the Generator Tool**
- [ ] **Generate and Test Standard Library Bindings**

### `minigo` Implementation ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [ ] **Phase 1: Core Interpreter and Expression Evaluation**
  - [ ] Setup project structure
  - [ ] Define `object.Object` interface and basic types (`Integer`, `String`, etc.)
  - [ ] Implement `eval` loop for basic expressions (`+`, `-`, `*`, `/`, `==`, `!`, etc.)
  - [ ] Add unit tests for expressions
- [ ] **Phase 2: Variables, Constants, and Scope**
  - [ ] Implement `object.Environment` for lexical scopes
  - [ ] Support `var`, assignment, and short variable declaration (`:=`)
  - [ ] Implement `const` declarations and `iota`
- [ ] **Phase 3: Control Flow**
  - [ ] Implement `if/else` statements
  - [ ] Implement `for` loops (standard C-style)
  - [ ] Implement `break` and `continue`
  - [ ] Implement `switch` statements
- [ ] **Phase 4: Functions and Call Stack**
  - [ ] Implement user-defined functions (`func`)
  - [ ] Implement call stack for error reporting
  - [ ] Implement `return` statements
  - [ ] Implement rich error formatting with stack traces
- [ ] **Phase 5: Data Structures and Pointers**
  - [ ] Support `type ... struct` declarations and literals
  - [ ] Support field access and assignment
  - [ ] Support slice, array, and map literals
  - [ ] Support indexing for slices, arrays, and maps
  - [ ] Implement `for...range` loops
  - [ ] Implement pointers (`&`, `*`) and `new()`
- [ ] **Phase 6: Go Interoperability and Imports**
  - [ ] Implement lazy, on-demand `import` handling
  - [ ] Implement `object.GoValue` to wrap `reflect.Value` for Go -> minigo interop
  - [ ] Wrap Go functions as callable `BuiltinFunction` objects
  - [ ] Implement `Result.As(target any)` for minigo -> Go struct unmarshaling
- [ ] **Phase 7: Refinement and Documentation**
  - [ ] Write comprehensive tests for all features
  - [ ] Write user documentation for API and language features
  - [ ] Ensure all CI checks (`make format`, `make test`) pass

### Parallel go-scan ([docs/plan-parallel-go-scan.md](./docs/plan-parallel-go-scan.md))
- [ ] **Task 1: Make `goscan.Scanner` Thread-Safe**
- [ ] **Task 2: Refactor `scanner.scanGoFiles` for Concurrent Parsing**
  - [ ] **Sub-Task 2.1: Define a Result Struct**
  - [ ] **Sub-Task 2.2: Implement the Parallel Parsing Loop**
  - [ ] **Sub-Task 2.3: Implement the Result Collection Logic**
  - [ ] **Sub-Task 2.4: Adapt the Sequential Processing Logic**
