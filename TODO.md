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
- **Dependency Analysis**: Includes the `deps-walk` command-line tool for visualizing dependency graphs (in DOT or Mermaid format) and a powerful underlying library for programmatic graph traversal, including forward and reverse dependency analysis. Wildcard (`...`) patterns are supported in both `Scan` and `Walk` functions, correctly resolving both filesystem and import path patterns.
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
    - **Advanced Language Features**: Includes full support for pointers (`&`, `*`), method definitions on structs, interface definitions and dynamic dispatch, struct embedding, and basic generics. The interpreter's `for` loops now correctly create per-iteration variables, preventing common closure-related bugs, and aligning with modern Go semantics.
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
- **`docgen` and Symbolic-Execution-like Engine (`symgo`)**: A symbolic execution engine (`symgo`) and a demonstration tool (`docgen`) that uses it to generate OpenAPI 3.1 specifications from `net/http` server code. The engine can trace function calls, resolve types, and evaluate handler logic to infer routes, methods, parameters, and request/response schemas. The `docgen` tool includes a golden-file test suite and can output in both JSON and YAML. ([docs/plan-symbolic-execution-like.md](./docs/plan-symbolic-execution-like.md))
    - Removed manual stubs for standard library types (`net/http`, `io`, etc.) from the `docgen` example, as `go-scan`'s improved module-aware resolution now handles them automatically.
- **Type-Safe `docgen` Patterns**: Enhanced the `docgen` tool to allow defining analysis patterns using type-safe function and method references (`Fn: mypkg.MyFunc`, `Fn: (*mypkg.MyType)(nil).MyMethod`, `Fn: myInstance.MyMethod`) instead of string keys. This involved creating `GoSourceFunction` and `GoMethodValue` objects in the `minigo` interpreter, and handling `BoundMethod` objects, to preserve the necessary definition context and ensure correct symbol and method resolution from various expression forms. ([docs/plan-docgen-minigo-fn-ref.md](./docs/plan-docgen-minigo-fn-ref.md))
- **Scanner Refactoring**: Refactored the main `goscan.Scanner` to separate responsibilities. Lightweight dependency analysis methods (e.g., `Walk`, `FindImporters`) have been moved to a new `goscan.ModuleWalker` struct, while heavyweight parsing methods remain on `goscan.Scanner`. This clarifies the API and improves separation of concerns.
- **`symgo` Evaluator Enhancements**: The symbolic execution engine's evaluator has been significantly improved, resolving critical bugs that previously blocked call-graph analysis tools.
    - **Reliable Method Dispatch**: Implemented robust logic to handle method calls on concrete struct types, including pointer vs. non-pointer receivers.
    - **Correct Type Propagation**: Ensured type information is correctly propagated through variable assignments and function returns.
    - **Robust Environment Management**: Fixed environment and scope handling during function application to ensure nested calls correctly trigger intrinsics.
- **Find Orphan Functions and Methods**: A new tool `examples/find-orphans` that uses the improved `symgo` engine to perform whole-program analysis and identify unused functions and methods. It supports multi-module workspaces and `//go:scan:ignore` annotations. It intelligently detects whether to run in "application mode" (starting from `main.main`) or "library mode" (starting from all exported functions).

## To Be Implemented

### `minigo` Refinements ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [ ] Write comprehensive documentation for the API, supported language features, and usage examples.

### `minigo` FFI and Language Limitations
- [x] **Fix empty slice type inference**: Type inference for empty slice literals is weak and defaults to `[]any`. This causes legitimate generic functions (like `slices.Sort`) to fail type checks when they shouldn't. The interpreter should ideally preserve the declared type (e.g., `[]int`) even if the literal is empty. (Note: This is fixed for empty slice and map literals.)
- [x] **Fix typed nil handling**: The interpreter does not correctly handle typed `nil` values for slices and interfaces, causing incorrect behavior in type inference and equality checks.

### `symgo` Interpreter Limitations
- [x] **Interface Method Call Tracing**: The interpreter did not previously trigger the default intrinsic for method calls on interface types. This prevented tools like `find-orphans` from correctly analyzing code that relies on interfaces. See [docs/trouble-find-orphans.md](./docs/trouble-find-orphans.md) for details. (Note: This is now fixed. The interpreter correctly creates a placeholder for interface method calls, which can be inspected by a default intrinsic.)

### Future Enhancements
- [ ] **`symgo`: Tracing and Debuggability**: Enhance the tracing mechanism to provide a more detailed view of the symbolic execution flow.
- [x] **`find-orphans`: Automatic `go.work` Detection**: Enhance the `--workspace-root` flag to automatically detect and use a `go.work` file if it exists in the root directory. This would involve parsing the `go.work` file to determine the list of modules to analyze, providing a more idiomatic way to define a workspace. If `go.work` is not found, the tool should fall back to the current behavior of scanning all subdirectories for `go.mod` files.
- [x] **`find-orphans`: Advanced Usage Analysis (Interfaces)**: The `symgo` engine and `find-orphans` tool have been enhanced to allow for more precise analysis of interface method calls. The engine now correctly tracks the set of possible concrete types for an interface variable, even across control-flow branches.
- [x] **`find-orphans`: Reporting and Final Touches**
    - [x] Implement formatted output for both default (orphans only) and verbose modes. (Note: Added JSON output via `-json` flag.)
- [x] **`find-orphans`: Multi-Module Workspace Support**: Allow `find-orphans` to treat multiple Go modules within a single repository as a unified "workspace".
    - [x] **Discovery**: Implement logic in `find-orphans` to discover all `go.mod` files under the directory specified by the `--workspace-root` flag.
    - [x] **Multi-Scanner Architecture**: Refactor the `analyzer` to manage a list of `*goscan.Scanner` instances, one for each discovered module.
    - [x] **Unified `symgo` View**: Create a facade or "meta-scanner" to provide `symgo.Interpreter` with a unified view of packages across all modules in the workspace. This may involve changes to `symgo` itself.
    - [x] **Update Analysis Logic**: Ensure the main analysis loop correctly collects declarations, builds maps, and identifies entry points from the aggregated set of all packages.
    - [x] **Add Tests**: Create a comprehensive test case with a multi-module project to validate that cross-module function calls are correctly tracked and orphans are identified accurately across the entire workspace.
- [x] **`scantest`: Path to Import Path Conversion**: Enhance `scantest.Run` with an option or helper to automatically convert filesystem path patterns (like `.`) into their corresponding Go import path patterns, simplifying test setup for tools that consume import paths.
