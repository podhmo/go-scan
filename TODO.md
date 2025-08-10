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

## To Be Implemented

### `minigo` Refinements ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [ ] **Implement Remaining Built-in Functions**:
    - [ ] `copy`
    - [ ] `delete`
    - [ ] `cap`
    - [ ] `make`
    - [ ] `new`
    - [ ] `complex`
    - [ ] `real`
    - [ ] `imag`
    - [ ] `clear`
    - [ ] `close`
    - [ ] `panic`
    - [ ] `recover`
- [ ] Write comprehensive documentation for the API, supported language features, and usage examples.

### Final API Specification for IDE-Native convert Configuration ([docs/plan-convert-with-minigo.md](./docs/plan-convert-with-minigo.md))
- [x] **Implement `object.AstNode`**: Create a new type in the `minigo/object` package to wrap a `go/ast.Node`.
- [x] **Implement Special Form Mechanism**: Modify the `minigo` evaluator to recognize "special form" functions and to not evaluate their arguments.
- [x] **Enhance Special Forms with Evaluator Context**: Modify the `SpecialFormFunction` signature to receive more context (like the evaluator instance or the current file scope), providing access to the scanner and symbol resolution capabilities.
- [x] **Enhance Go Interop Layer**: Update the interoperability layer to correctly unwrap `object.AstNode` and pass a raw `ast.Node` to a Go function that expects it.
- [x] **Add Unit Tests**: Write unit tests within the `minigo` package to verify that a Go function registered as a special form can correctly receive the AST of its arguments. (Note: Existing tests like `TestSpecialForm` already cover this.)
- [x] **Create `examples/convert/define` Package**: Create the new package containing the stub API functions (`Convert`, `Rule`, `Mapping`) and the empty `Config` struct.
- [ ] **Create CLI Entrypoint**: Create a new command (`examples/convert/cmd/convert-define`) for the new tool.
- [ ] **Implement Core Parser**: In the new command, implement the main parser logic that initializes the enhanced `minigo` interpreter and registers the `define` API functions as special forms.
- [ ] **Implement `define.Rule` Parsing**: Implement the logic to handle `define.Rule(customFunc)` calls. This involves using `go-scan` to resolve the function, inferring types from its signature, and creating a `model.TypeRule`.
- [ ] **Implement `define.Mapping` Parsing**: Implement the logic to handle the `ast.FuncLit` passed to `define.Mapping`. This involves setting up a sub-walker for the function body.
- [ ] **Implement `Config` Method Parsing**: Implement the logic within the sub-walker to parse calls to `c.Assign`, `c.Convert`, and `c.Compute`. This includes analyzing their arguments (selectors and expressions) and creating the appropriate `model.FieldMap` or `model.ComputedField` rules.
- [ ] **IR Construction**: Ensure the parser correctly assembles all the parsed rules into a single, valid `model.ParsedInfo` struct.
- [ ] **Enhance Generator for Implicit Mapping**: Modify the existing `generator` to automatically map all fields with matching names *before* it processes the explicit rules from the `ParsedInfo` struct.
- [ ] **Integrate Parser and Generator**: In the `cmd/convert-define` main function, plumb the `ParsedInfo` struct from the new parser into the enhanced generator.
- [ ] **Add Integration Tests**: Create a comprehensive test suite for the `convert-define` command. This should include a `define.go` script as input and assert that the generated Go code is correct.
- [ ] **Write User Documentation**: Update the project's `README.md` and any other relevant user-facing documentation to explain the new, preferred method for defining conversions.
