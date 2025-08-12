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

### Final API for convert Configuration ([docs/plan-convert-with-minigo.md](./docs/plan-convert-with-minigo.md))
- [ ] **Implement `object.AstNode`**: Create a new type in the `minigo/object` package to wrap a `go/ast.Node`.
- [ ] **Implement Special Form Mechanism**: Modify the `minigo` evaluator to recognize "special form" functions and to not evaluate their arguments.
- [ ] **Enhance Go Interop Layer**: Update the interoperability layer to correctly unwrap `object.AstNode` and pass a raw `ast.Node` to a Go function that expects it.
- [ ] **Add Unit Tests**: Write unit tests within the `minigo` package to verify that a Go function registered as a special form can correctly receive the AST of its arguments.
- [ ] **Create `examples/convert/define` Package**: Create the new package containing the stub API functions (`Convert`, `Rule`, `Mapping`) and the empty `Config` struct.
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

### minigo Implementation ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [ ] Set up the project structure (`minigo/`, `minigo/object/`, `minigo/evaluator/`, etc.).
- [ ] Define the `object.Object` interface and basic types: `Integer`, `String`, `Boolean`, `Nil`.
- [ ] Implement the core `eval` loop for expression evaluation.
- [ ] Support basic literals (`123`, `"hello"`).
- [ ] Support binary expressions (`+`, `-`, `*`, `/`, `==`, `!=`, `<`, `>`).
- [ ] Support unary expressions (`-`, `!`).
- [ ] Write unit tests for all expression evaluations.
- [ ] Implement the `object.Environment` for managing lexical scopes.
- [ ] Add support for `var` declarations (e.g., `var x = 10`) and assignments (`x = 20`).
- [ ] Add support for short variable declarations (`x := 10`).
- [ ] **Implement `const` declarations**, including typed (`const C int = 1`), untyped (`const C = 1`), and `iota`.
- [ ] Implement `if/else` statements.
- [ ] Implement standard `for` loops (`for i := 0; i < 10; i++`).
- [ ] Implement `break` and `continue` statements.
- [ ] **Implement `switch` statements**:
    - [ ] Support `switch` with an expression (`switch x { ... }`).
    - [ ] Support expressionless `switch` (`switch { ... }`).
    - [ ] Support `case` clauses with single or multiple expressions.
    - [ ] Support the `default` clause.
- [ ] Implement user-defined functions (`func` declarations).
- [ ] Implement the call stack mechanism for tracking function calls.
- [ ] Implement `return` statements (including returning the `nil` object).
- [ ] Implement rich error formatting with a formatted call stack.
- [ ] Add support for `type ... struct` declarations.
- [ ] Support struct literal instantiation (e.g., `MyStruct{...}`), including both keyed and unkeyed fields.
- [ ] Support field access (`myStruct.Field`) and assignment (`myStruct.Field = ...`).
- [ ] Support slice and array literals (`[]int{1, 2}`, `[2]int{1, 2}`).
- [ ] Support map literals (`map[string]int{"a": 1}`).
- [ ] Support indexing for slices, arrays, and maps (`arr[0]`, `m["key"]`).
- [ ] **Implement `for...range` loops** for iterating over slices, arrays, and maps.
- [ ] **Implement pointer support**:
    - [ ] Define a `Pointer` object type in the object system.
    - [ ] Implement the address-of operator (`&`) to create pointers to variables.
    - [ ] Implement the dereference operator (`*`) to get the value a pointer points to.
    - [ ] Support pointer-to-struct field access (e.g., `ptr.Field`).
    - [ ] Support `new()` built-in function.
- [ ] Create the main `Interpreter` struct that holds a `goscan.Scanner`.
- [ ] Implement the logic to handle `import` statements and load symbols from external Go packages.
- [ ] Implement the `object.GoValue` to wrap `reflect.Value`, allowing Go values to be injected into the script.
- [ ] Implement the logic to wrap Go functions as `BuiltinFunction` objects.
- [ ] Implement the `Result.As(target any)` method for unmarshaling script results back into Go structs.
- [ ] Thoroughly test all features, especially pointer handling and the Go interop layer.
- [ ] Write comprehensive documentation for the API, supported language features, and usage examples.
- [ ] Ensure `make format` and `make test` pass cleanly.

### Unified derivingjson and derivingbind Generator ([docs/plan-walk-once.md](./docs/plan-walk-once.md))
- [ ] **Step 1: Create the `GeneratedCode` struct.**
    -   Define the shared `GeneratedCode` struct (or a similar structure) to standardize the output of all generators. This could be in a new package like `examples/internal/generator`.
- [ ] **Step 2: Refactor the `derivingjson` generator.**
    -   Create a new `gen/generate.go` file in the `derivingjson` example.
    -   Move the `Generate` function into this new file.
    -   Modify its signature to `func Generate(...) (*generator.GeneratedCode, error)`.
    -   Update the function to return the generated code and imports instead of writing to a file.
    -   Update the original `derivingjson/main.go` to call this new function and handle the file writing, ensuring the standalone tool still works.
- [ ] **Step 3: Refactor the `derivingbind` generator.**
    -   Repeat the process from Step 2 for the `derivingbind` example.
- [ ] **Step 4: Implement the initial version of the unified `deriving-all` tool.**
    -   Create the `examples/deriving-all/main.go` file.
    -   Implement the core logic: initialize the scanner, scan the package, and call the refactored `json` and `bind` generators.
    -   Implement the logic to merge the `GeneratedCode` results (both code and imports) from all generators.
    -   Write the combined result to a single output file.
- [ ] **Step 5: Implement tests for the unified generator using `scantest`.**
    -   Create a new test file, `deriving_all_test.go`.
    -   Write a test case for a struct with only the `deriving:unmarshal` annotation.
    -   Write a test case for a struct with only the `deriving:binding` annotation.
    -   Write a test case for a struct with both annotations, and verify that the output file contains the code from both generators.
    -   Write a test case for a struct with no relevant annotations to ensure an empty output is handled gracefully.
- [ ] **Step 6: Refine and Finalize.**
    -   Clean up the code, add comments, and ensure the CLI interface for the new tool is user-friendly.
    -   Update the main `README.md` to document the new unified tool.
