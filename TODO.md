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
    - **Core Interpreter**: The engine is fully implemented, supporting expressions, variables (`var`, `const`, `iota`), assignments, and all major control flow statements (`if`, `for`, `switch`, `break`, `continue`). It also supports `for...range` over integers (e.g., `for i := range 10`).
    - **Functions and Data Structures**: Supports user-defined functions, rich error reporting with stack traces, and composite types including structs, slices, and maps.
    - **Advanced Language Features**: Includes full support for pointers (`&`, `*`), method definitions on structs, interface definitions and dynamic dispatch, struct embedding, and basic generics. The interpreter's `for` loops now correctly create per-iteration variables, preventing common closure-related bugs and aligning with modern Go semantics.
    - **Go Interoperability**: Provides a robust bridge to Go, allowing scripts to call Go functions, access Go variables, and unmarshal script results back into Go structs via `Result.As()`. Lazy, on-demand loading of imported Go packages is also supported.
- **Final API for `convert` Tool**: A new IDE-native method for configuring the `convert` tool using a `define` package. This allows for type-safe, statically valid Go code for defining conversion rules, improving the developer experience over the previous annotation-based system.
- **Parallel go-scan**: Implemented concurrent parsing and made the core scanner thread-safe.
- **Automated Minigo Bindings Generation**: Created a tool to automatically generate `minigo` bindings for Go packages, including initial support for several standard library packages. ([docs/plan-minigo-gen-bindings.md](./docs/plan-minigo-gen-bindings.md)) (now integrated as `minigo gen-bindings`)
- **MiniGo REPL**: Added a REPL for interactive script evaluation. ([docs/plan-minigo-repl.md](./docs/plan-minigo-repl.md))
- **Full `encoding/json` Support in `minigo`**: Implemented `json.Marshal` and `json.Unmarshal` with support for field tags and complex nested structs. ([docs/trouble-minigo-encoding-json.md](docs/trouble-minigo-encoding-json.md))
- **Extended Standard Library Support & Compatibility**:
    - Automated the generation of bindings for a wide range of standard library packages.
    - Implemented a test suite to validate generated bindings and conducted a thorough compatibility analysis, documenting key limitations and driving numerous interpreter enhancements. ([docs/trouble-minigo-stdlib-limitations.md](./docs/trouble-minigo-stdlib-limitations.md))
    - Resolved a critical FFI bug that prevented method calls on pointers to in-script FFI struct variables, unblocking stateful packages like `text/scanner`. ([docs/trouble-minigo-go-value-method-call.md](./docs/trouble-minigo-go-value-method-call.md))
    - Implemented support for interpreting the `slices` and `errors` packages from source by adding language features like 3-index slicing, variadic functions, and support for struct literals with scoped variables.

## To Be Implemented

### `minigo` Refinements ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [ ] Write comprehensive documentation for the API, supported language features, and usage examples.

### `minigo` FFI and Language Limitations
- [-] **Fix empty slice type inference**: Type inference for empty slice literals is weak and defaults to `[]any`. This causes legitimate generic functions (like `slices.Sort`) to fail type checks when they shouldn't. The interpreter should ideally preserve the declared type (e.g., `[]int`) even if the literal is empty. (Note: This is fixed for empty slice and map literals.)
- [ ] **Fix typed nil handling**: The interpreter does not correctly handle typed `nil` values for slices and interfaces, causing incorrect behavior in type inference and equality checks.
- [x] **Fix `json.Unmarshal` error propagation**: The FFI fails to correctly propagate `*json.UnmarshalTypeError` from `json.Unmarshal`, returning a `nil` value instead. This prevents scripts from handling JSON validation errors correctly.
- [x] **Improve method call support for stateful objects**: The FFI and evaluator have trouble with packages like `container/list` where methods (`PushBack`, `Next`) modify the internal state of a Go object in a way that is not correctly reflected back into the script environment. This prevents effective use of stateful, object-oriented packages.
- [x] **Support slice operator on Go-native arrays**: The interpreter does not support the slice operator (`[:]`) on `object.GoValue` types that wrap Go arrays (e.g., `[16]byte`). This was discovered when testing `crypto/md5` and blocks the use of functions that return native Go arrays.
- [x] **Improve generic type inference for composite types**: The type inference engine fails to infer type parameters when they are part of a composite type in a function argument (e.g., inferring `E` from a parameter of type `[]E`). This was discovered when testing `slices.Sort` and currently blocks its use via source interpretation.
- [x] **Improve interpreter performance for complex algorithms**: `slices.Sort` fails to complete within the test timeout, indicating severe performance bottlenecks when interpreting complex code like sorting algorithms.

### Symbolic-Execution-like Engine (`symgo`) ([docs/plan-symbolic-execution-like.md](./docs/plan-symbolic-execution-like.md))
- [x] **M1: `symgo` Core Engine**:
    - [x] **Object System**: Define the `symgo/object` package with the `Object` interface and initial concrete types (`String`, `Function`, `Error`, `SymbolicPlaceholder`).
    - [x] **Scope Management**: Implement the `symgo/scope` package for lexical scoping, supporting nested environments.
    - [x] **Core Evaluator**: Implement the `symgo/evaluator` with the main `Eval` dispatch loop.
        - [x] Support basic AST nodes: `ast.BasicLit`, `ast.Ident`.
        - [x] Support basic AST nodes: `ast.AssignStmt`, `ast.ReturnStmt`.
        - [x] Support basic control flow: `if`, `for`, `switch` (heuristic-based).
    - [x] **Import & Symbol Resolution**:
        - [x] Handle `import` statements by creating placeholder package objects.
        - [x] Implement lazy, on-demand package loading using `go-scan` when a symbol from an unloaded package is accessed (e.g., `pkg.Symbol`).
        - [x] Integrate the resolved symbol information into the `symgo` scope.
        - [x] (Note: The lazy-loading mechanism from the `minigo` implementation can be used as a reference.)
    - [x] **Function Evaluation Strategy**:
        - [x] Implement recursive evaluation for intra-module function calls.
        - [x] Implement an intrinsic function registry (`symgo/intrinsics`).
        - [x] Return `SymbolicPlaceholder` objects for calls to extra-module functions that are not intrinsics.
- [x] **M2: `docgen` Tool & Basic `net/http` Analysis**:
    - [x] **Project Setup**:
        - [x] Create the `examples/docgen` CLI application skeleton.
        - [x] Define local structs for a minimal OpenAPI 3.1 model (`examples/docgen/openapi`).
        - [x] Create a sample `net/http` API to use as the analysis target (`examples/docgen/sampleapi`).
    - [x] **Core Analyzer**:
        - [x] Implement the main analysis orchestrator that uses `go-scan` and the `symgo` interpreter.
        - [x] Implement logic to find calls to `(*http.ServeMux).HandleFunc` to extract route paths and handler functions, targeting modern Go 1.22+ patterns.
        - [x] Use `go-scan`'s `WithExternalTypeOverrides` to provide stubs for complex stdlib types like `http.Request`.
    - [x] **Handler Analysis**:
        - [x] Analyze handler function patterns to find HTTP methods (now done via parsing the `HandleFunc` pattern string, e.g., "GET /path").
        - [x] Extract `operationId` from the function name and `description` from godoc comments.
    - [x] **Testing**: Write an integration test to verify basic route, method, and description extraction from the sample API.
- [-] **M3: Schema and Parameter Analysis**:
    - [x] **Request/Response Body Analysis**:
        - [x] Implement pattern matching to detect calls like `json.NewDecoder(...).Decode(&req)`.
        - [x] Use the `symgo` scope to resolve the type of the `req` variable and analyze its struct definition to build a request schema.
        - [x] Implement similar pattern matching for response-writing functions (e.g., `json.NewEncoder(...).Encode(resp)`).
    - [x] **Query Parameter Analysis**:
        - [x] Implement intrinsics to detect `r.URL.Query().Get("...")`.
        - [x] Implement the extensible `CallPattern` registry (`examples/docgen/patterns`).
    - [ ] **Interface/Higher-Order Function Handling**:
        - [ ] Implement context-based type binding in `symgo` to handle interfaces like `io.Writer`.
        - [ ] Add intrinsics for common `net/http` higher-order functions like `http.TimeoutHandler` to trace into the actual handler.
    - [x] **Testing**: Enhance the integration test to verify that request/response schemas and query parameters are correctly extracted.
- [ ] **M4: Finalization**:
    - [ ] **OpenAPI Generation**:
        - [ ] Implement the generator component to convert the collected API metadata into the OpenAPI 3.1 model.
        - [ ] Implement YAML/JSON marshaling to print the final specification to standard output.
    - [ ] **Engine Enhancements**:
        - [ ] Add a built-in intrinsic for `fmt.Sprintf` to handle dynamic path segment construction.
    - [ ] **Documentation & Testing**:
        - [ ] Write `README.md` files for both the `symgo` library and the `docgen` tool.
        - [ ] Write a final end-to-end test that compares the generated OpenAPI spec against a "golden" file.
        - [ ] Ensure `make format` and `make test` pass for the entire repository before submission.

### `symgo` Engine Refinements
- [x] **Fix slice literal type inference regression**: The evaluator's `evalCompositeLit` incorrectly resolves the type of a slice literal (e.g., `[]User{}`) to its element type (`User`), losing the "slice-ness" of the value. This prevents `docgen` from generating correct response schemas for endpoints that return slices. See [docs/trouble-docgen.md](./docs/trouble-docgen.md) for details.
- [ ] **Fix non-slice response schema generation in `docgen`**: While fixing the slice response issue, a regression was introduced where handlers returning single struct instances (e.g., `func() User`) no longer have their response schemas generated correctly. See [docs/trouble-docgen.md](./docs/trouble-docgen.md) for details. (Note: The test for this is currently disabled in `main_test.go`.)
- [x] **Create `symgo.Interpreter` Facade**: Create a new `symgo` package with an `Interpreter` type, modeled after `minigo.Interpreter`, to provide a clean public API for the engine. ([docs/trouble-docgen-use-symgo.md](./docs/trouble-docgen-use-symgo.md))
    - [x] Implement a `NewInterpreter()` constructor that properly initializes the internal scanner and evaluator.
    - [x] Implement a public `RegisterIntrinsic()` method on the interpreter.
    - [x] Implement a public `Eval()` method to run the analysis.
    - [x] Re-export necessary types like `symgo.Object` and `symgo.Function` from the new package.
- [x] **Refactor `docgen` to use `symgo.Interpreter`**: The `docgen` tool has been refactored to use the `symgo.Interpreter`, replacing the initial manual AST traversal. The `symgo` engine itself was enhanced to support this.
- [x] **Resolve variable arguments in intrinsics**: Ensure that when a variable is passed to an intrinsic function, the function receives the variable's underlying value (`*object.String`, etc.) rather than the `*object.Variable` wrapper. This simplifies intrinsic implementation and aligns argument evaluation with return value evaluation.
- [x] **Support string concatenation**: Implemented support for the `+` operator for string types in binary expressions.

### `symgo` and `docgen` Improvements

A set of tasks to improve the `symgo` engine and the `docgen` tool based on the analysis in `docgen/ja/from-docgen.md`.

- [ ] **Step 1: Error Reporting and Engine Stabilization**
    - [ ] **Extend `object.Error`**: Add a `token.Pos` field to the `Error` struct in `symgo/object/object.go` to hold source code position.
    - [ ] **Improve Error Messages**: Update the `symgo.Interpreter` to use the `token.Pos` to include file, line, and column information in error messages.
    - [ ] **Implement `fmt.Sprintf` Intrinsic**: Add a `symgo` intrinsic to mimic the basic behavior of `fmt.Sprintf` for dynamic string construction.
    - [ ] **Support `if-else`**: Modify `evalIfStmt` in `symgo/evaluator/evaluator.go` to correctly evaluate `else` blocks.
- [ ] **Step 2: Debugging Features**
    - [ ] **Implement Structured Logger**: Introduce an optional structured logger in the `symgo.Evaluator` to trace evaluation steps, including node info, position, and results.
    - [ ] **Add Debug Flag to `docgen`**: Add a `--debug-analysis <functionName>` flag to `docgen` to enable the structured logger for a specific function.
- [ ] **Step 3: User Extensibility**
    - [ ] **Implement `minigo`-based Pattern Loader**: Create a loader in `docgen` that reads a `.minigo` script and parses a list of pattern definitions using `minigo.EvalString` and `minigo.Result.As`.
    - [ ] **Integrate Pattern Loader with Analyzer**: Modify the `docgen.Analyzer` to dynamically register intrinsics based on the patterns loaded from the `.minigo` script at startup.
