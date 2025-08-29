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
    - **Test Package Handling**: The scanner now correctly handles directories containing both a standard package (`pkg`) and its external test package (`pkg_test`) when the `include-tests` option is enabled, preventing "mismatched package names" errors.
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
- **`symgo` Evaluator Enhancements**: The symbolic execution engine's evaluator has been significantly improved, resolving critical bugs that previously blocked call-graph analysis tools. This includes better handling of method entry points, reliable dispatch for both pointer and non-pointer receivers, correct type propagation, robust environment management for nested calls, and fixes for panics related to naked returns and incorrect path resolution for external packages.
- **`symgo`: Cross-Module Source Scanning**: Added an option (`--include-pkg` in `docgen`) to allow the `symgo` engine to treat specified external packages as if they were internal, enabling deep, source-level analysis of their functions.
- **Find Orphan Functions and Methods**: A new tool `examples/find-orphans` that uses the improved `symgo` engine to perform whole-program analysis and identify unused functions and methods. It supports multi-module workspaces and `//go:scan:ignore` annotations. It intelligently detects whether to run in "application mode" (starting from `main.main`) or "library mode" (starting from all exported functions).
    - **[x]** **Specification and Logic Refinement**: The specification for the tool's behavior, particularly for library mode, has been significantly clarified and documented in `examples/find-orphans/spec.md`. The core logic has been updated to match. A bug related to intra-package method analysis has been fixed in `symgo`. The previously suspected upstream bug in `go-scan`'s file filtering was found to be a misunderstanding in the test setup itself; the test was using a filename (`not_a_test.go`) that was correctly being filtered as a test file because its name ends with the `_test.go` suffix. The test has been corrected.
    - **Correct Intra-Package Analysis**: Fixed a bug where unexported methods were incorrectly reported as orphans if they were only called by exported methods within the same package. The analysis now correctly treats exported methods as entry points in library mode.
    - **Result Filtering**: The tool now automatically excludes `main.main` and `init` functions from the list of orphans, as they are fundamental entry points.
    - **Scoped Analysis**: The tool's analysis can now be strictly scoped to a user-defined set of target packages. Orphans from upstream dependencies are no longer reported unless those dependencies are explicitly included in the input patterns. This is controlled by resolving file path patterns (`./...`) and import path patterns (`example.com/...`) into a canonical set of target packages. Directory traversal can be fine-tuned with the `--exclude-dirs` flag (e.g., to ignore `testdata`).
    - **[x]** **Enhanced Library Mode Analysis**: In library mode, the analysis now correctly includes all `init` functions and the `main.main` function (if present) as starting points for the call-graph traversal, ensuring that functions used only by these entry points are not incorrectly reported as orphans.
    - **[x]** **Global Variable Initializer Analysis**: Confirmed that functions called in global `var` initializers are correctly treated as "used" in all analysis modes. Added a regression test and updated `spec.md` to reflect this behavior.
- **`find-orphans`: Robust Path Handling**: The tool now correctly handles relative paths for the `--workspace-root` flag and resolves target path patterns (`./...`) relative to the specified workspace root, not the current working directory. This allows for more intuitive and predictable behavior when running the tool from subdirectories.
- **`symgo`: Embedded Method Resolution**: The symbolic execution engine can now correctly resolve and trace method calls on embedded structs, enabling more accurate call-graph analysis for tools like `find-orphans`.
- **`symgo`: Type Switch Support**: The symbolic execution engine now correctly handles `ast.TypeSwitchStmt` nodes, allowing it to analyze code that uses type switches (`switch v := i.(type)`). It correctly scopes the typed variable (`v`) within each case block.
- **`symgo`: Efficient & Configurable Scanning Policy**: The symbolic execution engine no longer scans packages outside the workspace by default, significantly improving performance and scalability. It now uses a `ScanPolicyFunc` to provide fine-grained, dynamic control over which packages are scanned from source versus being treated as symbolic placeholders. This replaces the older, less flexible `WithExtraPackages` mechanism.
- **`symgo`: Shallow Scanning**: The `symgo` evaluator is now more robust and performant when dealing with types from packages outside the defined scan policy. It can now create symbolic placeholders for unresolved types, allowing analysis to continue without crashing and enabling symbolic tracing of method calls on these types. This significantly improves the accuracy of tools like `find-orphans` when analyzing code with external dependencies. ([docs/plan-symgo-shallow-scan.md](./docs/plan-symgo-shallow-scan.md))
- **`go-scan`: Declarations-Only Scanning**: Added a `WithDeclarationsOnlyPackages` option to the `goscan.Scanner`. For packages specified with this option, the scanner parses all top-level declarations (types, functions, variables) but explicitly discards function bodies. This allows tools like `docgen` to obtain necessary type information from packages like `net/http` without incurring the cost and complexity of symbolically executing their entire implementation. This provides a significant performance and stability improvement for analyzing code that depends on large standard library packages.

## To Be Implemented

### `minigo` Refinements ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [ ] Write comprehensive documentation for the API, supported language features, and usage examples.

### `minigo` FFI and Language Limitations
- [x] **Fix empty slice type inference**: Type inference for empty slice literals is weak and defaults to `[]any`. This causes legitimate generic functions (like `slices.Sort`) to fail type checks when they shouldn't. The interpreter should ideally preserve the declared type (e.g., `[]int`) even if the literal is empty. (Note: This is fixed for empty slice and map literals.)
- [x] **Fix typed nil handling**: The interpreter does not correctly handle typed `nil` values for slices and interfaces, causing incorrect behavior in type inference and equality checks.


### `symgo` Interpreter Limitations
- [x] **Infinite Recursion**: The interpreter now prevents infinite recursion by tracking the call stack and detecting duplicate function calls with the same arguments.
- [x] **`symgo`: `defer` and `go` statement Tracing**: The `symgo` interpreter now traces `CallExpr` nodes inside `*ast.DeferStmt` and `*ast.GoStmt`, preventing false positives in tools like `find-orphans`.
- [x] **Branch statements (`break`, `continue`)**: The interpreter now handles `*ast.BranchStmt`, allowing it to correctly model control flow in loops.
- [x] **`for...range` statements**: The interpreter now handles `*ast.RangeStmt`. A function call in the range expression (e.g., `for _ := range getItems()`) will be traced.
- [x] **Pointer Dereferencing**: The interpreter now handles `*ast.StarExpr` for dereferencing. This was previously incorrectly identified as a missing feature for `*ast.UnaryExpr` with `Op: token.MUL`. This change enables tracing method calls on pointer types (e.g., `(*p).MyMethod()`).
- [x] **Slice Expressions**: The interpreter does not handle `*ast.SliceExpr`. Function calls used as index expressions are not traced.
- [x] **`select` statements**: The interpreter does not handle `*ast.SelectStmt`. Function calls within channel communications are not traced.
- [x] **Interface Method Call Tracing**: The interpreter did not previously trigger the default intrinsic for method calls on interface types. This prevented tools like `find-orphans` from correctly analyzing code that relies on interfaces. See [docs/trouble-find-orphans.md](./docs/trouble-find-orphans.md) for details. (Note: This is now fixed. The interpreter correctly creates a placeholder for interface method calls, which can be inspected by a default intrinsic.)
- [x] **Numeric Types**: The interpreter now handles `integer`, `float`, and `complex` literals and arithmetic.
- [x] **Map Literals**: The interpreter does not have concrete support for map literals; they are treated as symbolic placeholders. (Note: Now symbolically evaluated, tracing calls in keys and values.)
- [x] **Function Literals as Arguments**: The interpreter now scans the bodies of function literals passed as arguments to other functions, allowing it to trace calls within them (e.g., `t.Run(..., func() { ... })`).
- [x] **Function Literals as Return Values**: The interpreter now correctly traces calls inside closures that are returned from other functions by ensuring that package-level environments are correctly populated and captured.
- [ ] **Generics**: The interpreter does not support generic functions or types.
- [ ] **Channels**: The interpreter has limited support for channel operations (e.g., in `select` statements) but does not have a concrete channel object type, limiting analysis of channel-based logic.
- [-] **Stateful Type Tracking for Variables**: Partially implemented. A fix for simple variable declarations was added, but it causes regressions in pointer-dereferencing and intra-package method calls. See [docs/trouble-symgo-state-tracking.md](./docs/trouble-symgo-state-tracking.md) for a detailed analysis of the regressions.
- [ ] **Other AST Nodes**: The following `ast.Node` types are not yet handled by the main evaluation loop:
    - [ ] `*ast.ChanType`
    - [x] `*ast.Ellipsis` (Note: Implemented for variadic arguments in function calls and definitions.)
    - [ ] `*ast.FuncType`
    - [ ] `*ast.InterfaceType`
    - [ ] `*ast.MapType`
    - [ ] `*ast.StructType`
    - [x] `*ast.EmptyStmt`
    - [x] `*ast.IncDecStmt`
    - [ ] `*ast.LabeledStmt`
    - [ ] `*ast.SendStmt`
    - [ ] `*ast.TypeAssertExpr` (partially handled in type switches)
- [x] **`panic` and other builtins**: The interpreter now recognizes `panic`, `nil`, `true`, and `false`.
- [x] **Multi-value returns and assignments**: The interpreter now supports functions that return multiple values and assignments of the form `x, y := f()` and `x, y = f()`.
- [x] **`symgo`: Correctly scope function parameters**: Fixed a bug where function parameters and receivers were incorrectly set in the package scope instead of the function's local scope, causing "identifier not found" errors in nested blocks.
- [x] **Block Statements**: The interpreter now correctly handles nested block statements `{...}` within function bodies, creating a new lexical scope and correctly tracing function calls inside them.

### Future Enhancements
- [x] **`symgo`: Tracing and Debuggability**: Enhance the tracing mechanism to provide a more detailed view of the symbolic execution flow.
    - [x] **Contextual Logging**: Warning logs emitted during evaluation now include the function call site (name and position) where the warning occurred. This is achieved by capturing the call stack within the returned error object.
    - [x] **Source in Stack Traces**: Errors returned by the symbolic evaluator now include a full stack trace, complete with the source code line that caused the error, similar to `minigo`.
    - [x] **Structured Call Stack Logging**: Debug logs for function calls now include a structured representation of the call stack, with function names, file paths, and line numbers, significantly improving readability.
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
- [x] **`ModuleWalker`: Wildcard Support**: Added support for the `...` wildcard in both file path and import path patterns to tools like `find-orphans`, making package discovery more intuitive.
- [x] **`scantest`: Path to Import Path Conversion**: Enhance `scantest.Run` with an option or helper to automatically convert filesystem path patterns (like `.`) into their corresponding Go import path patterns,
- [ ] simplifying test setup for tools that consume import paths.
