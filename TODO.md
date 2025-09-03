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
    - **`scantest`: Path to Import Path Conversion**: Added a `scantest.ToImportPath` helper function (a wrapper around `locator.ResolvePkgPath`) to simplify converting file paths to Go import paths in tests.
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
    - **Centralized Package & Symbol Resolution**: Refactored the evaluator to use a central package cache (`pkgCache`). This ensures that for any given import path, a single, canonical package object is created and used, fixing a class of bugs related to inconsistent symbol resolution. This resolves failures with unexported constants in cross-package calls, improves handling of aliased imports, and correctly resolves package names that differ from their import paths (e.g., `gopkg.in/yaml.v2`). ([docs/trouble-symgo-identifier-not-found.md](./docs/trouble-symgo-identifier-not-found.md))
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
- **`symgo`: Embedded Field Access**: The engine now also correctly resolves and traces field access on embedded structs.
- **`symgo`: Type Switch Support**: The symbolic execution engine now correctly handles `ast.TypeSwitchStmt` nodes, allowing it to analyze code that uses type switches (`switch v := i.(type)`). It correctly scopes the typed variable (`v`) within each case block.
- **`symgo`: Efficient & Configurable Scanning Policy**: The symbolic execution engine no longer scans packages outside the workspace by default, significantly improving performance and scalability. It now uses a `ScanPolicyFunc` to provide fine-grained, dynamic control over which packages are scanned from source versus being treated as symbolic placeholders. This replaces the older, less flexible `WithExtraPackages` mechanism.
- **`symgo`: Shallow Scanning**: The `symgo` evaluator is now more robust and performant when dealing with types from packages outside the defined scan policy. It can now create symbolic placeholders for unresolved types, allowing analysis to continue without crashing and enabling symbolic tracing of method calls on these types. This significantly improves the accuracy of tools like `find-orphans` when analyzing code with external dependencies. ([docs/plan-symgo-shallow-scan.md](./docs/plan-symgo-shallow-scan.md))
- **`symgo`: Field Access on Symbolic Receivers**: The `symgo` evaluator can now correctly access struct fields on symbolic receivers (e.g., a receiver of a method that is the entry point of analysis). This fixes a bug where field access was incorrectly failing with an "undefined method" error, particularly on structs that use `_ struct{}` to enforce keyed literals.
- **`go-scan`: Declarations-Only Scanning**: Added a `WithDeclarationsOnlyPackages` option to the `goscan.Scanner`. For packages specified with this option, the scanner parses all top-level declarations (types, functions, variables) but explicitly discards function bodies. This allows tools like `docgen` to obtain necessary type information from packages like `net/http` without incurring the cost and complexity of symbolically executing their entire implementation. This provides a significant performance and stability improvement for analyzing code that depends on large standard library packages.


## To Be Implemented


### `symgo` Engine Improvements ([docs/plan-symgo-refine2.md](./docs/plan-symgo-refine2.md))
- [x] **Analysis**: Investigate timeout and critical errors by re-running e2e tests.
- [-] **Bugfix: Infinite Recursion**: (partially resolved)
    - [x] Add a recursion guard (e.g., using a map to track visited nodes) to `scanner.TypeInfoFromExpr` to prevent re-evaluation of the same type expression.
    - [x] Write a targeted unit test in the `scanner` package that fails before the fix and passes after, reproducing the infinite recursion scenario.
    - [ ] Verify the fix by running the `find-orphans` e2e test and confirming it runs to completion without timing out.If it doesn't heal, start investigating again.
- [x] **Bugfix: External Type Resolution**:
    - [x] Investigate why types from external packages (e.g., `log/slog.Logger`) are resolved as `object.UnresolvedFunction` instead of a symbolic type representation.
    - [x] Modify the `symgo` evaluator and/or `scanner` to ensure that unresolved types are consistently represented as symbolic type placeholders, not functions.
    - [x] Add a regression test to `symgo` that attempts to use an external type and fails if the "invalid indirect" error occurs.
- [ ] **DX: Add Timeout Flag to `find-orphans`**:
    - [ ] Add a `--timeout` flag (e.g., `--timeout 30s`) to the `find-orphans` CLI.
    - [ ] Use `context.WithTimeout` in the `run` function to cancel the analysis if it exceeds the specified duration.
    - [ ] Document the new flag in the tool's help message and README.
- [ ] **Follow-up: Full Entry Point Analysis**:
    - [ ] Once the critical recursion and type resolution bugs are fixed, execute the `find-orphans` e2e test again.
    - [ ] Perform a full analysis of the complete log output.
    - [ ] Create new items in `TODO.md` for any remaining `WARN` or `ERROR` messages that indicate bugs.


### `minigo` Refinements ([docs/plan-minigo.md](./docs/plan-minigo.md))
- [x] Write comprehensive documentation for the API, supported language features, and usage examples.
- [x] Implement `defer` and `recover` statements.

### `minigo` FFI and Language Limitations
- [x] **Fix empty slice type inference**: Type inference for empty slice literals is weak and defaults to `[]any`. This causes legitimate generic functions (like `slices.Sort`) to fail type checks when they shouldn't. The interpreter should ideally preserve the declared type (e.g., `[]int`) even if the literal is empty. (Note: This is fixed for empty slice and map literals.)
- [x] **Fix typed nil handling**: The interpreter does not correctly handle typed `nil` values for slices and interfaces, causing incorrect behavior in type inference and equality checks.


### `symgo` Interpreter Limitations
- [x] **Mismatching Import Path and Package Name**: The interpreter's import resolution now correctly handles packages with mismatched import paths and package names (e.g., `gopkg.in/yaml.v2`). A follow-on bug related to the resolution of built-in types (`string`, `int`, etc.) during type conversions has also been fixed by centralizing built-in type definitions in the `universe` scope. ([docs/trouble-symgo-identifier-not-found.md](./docs/trouble-symgo-identifier-not-found.md))
- [x] **Resilience to Undefined Identifiers**: The interpreter now correctly creates symbolic placeholders for undefined identifiers found in packages that are outside the defined scan policy, allowing analysis to continue without erroring. This has been fully validated. ([docs/trouble-symgo-identifier-not-found.md](./docs/trouble-symgo-identifier-not-found.md))
- [x] **Infinite Recursion**: The interpreter now prevents infinite recursion by tracking the call stack. The detection mechanism correctly distinguishes between recursive calls to the same function and method calls on different receivers, preventing both false negatives (missing true recursion) and false positives (e.g., in linked-list traversals).
- [x] **`symgo`: `defer` and `go` statement Tracing**: The `symgo` interpreter now traces `CallExpr` nodes inside `*ast.DeferStmt` and `*ast.GoStmt`, preventing false positives in tools like `find-orphans`.
- [x] **Branch statements (`break`, `continue`)**: The interpreter now handles `*ast.BranchStmt`, allowing it to correctly model control flow in loops.
- [x] **`for...range` statements**: The interpreter now handles `*ast.RangeStmt`. A function call in the range expression (e.g., `for _ := range getItems()`) will be traced.
- [x] **Pointer Dereferencing**: The interpreter now handles `*ast.StarExpr` for dereferencing. This was previously incorrectly identified as a missing feature for `*ast.UnaryExpr` with `Op: token.MUL`. This change enables tracing method calls on pointer types (e.g., `(*p).MyMethod()`).
- [x] **Slice Expressions**: The interpreter does not handle `*ast.SliceExpr`. Function calls used as index expressions are not traced.
- [x] **`select` statements**: The interpreter does not handle `*ast.SelectStmt`. Function calls within channel communications are not traced.
- [x] **Interface Method Call Tracing**: The interpreter did not previously trigger the default intrinsic for method calls on interface types. This prevented tools like `find-orphans` from correctly analyzing code that relies on interfaces. See [docs/trouble-find-orphans.md](./docs/trouble-find-orphans.md) for details. (Note: This is now fixed. The interpreter correctly creates a placeholder for interface method calls, which can be inspected by a default intrinsic.)
- [x] **Numeric Types**: The interpreter now handles `integer`, `float`, and `complex` literals and arithmetic.
- [x] **Character Literals**: The interpreter now handles `char` and `rune` literals (e.g., `'a'`, `'\n'`).
- [x] **Unary Operators**: The interpreter now handles the primary unary operators: logical not (`!`), negation (`-`), unary plus (`+`), and bitwise complement (`^`).
- [x] **Map Literals**: The interpreter does not have concrete support for map literals; they are treated as symbolic placeholders. (Note: Now symbolically evaluated, tracing calls in keys and values.)
- [x] **Function Literals as Arguments**: The interpreter now scans the bodies of function literals passed as arguments to other functions, allowing it to trace calls within them (e.g., `t.Run(..., func() { ... })`).
- [x] **Function Literals as Return Values**: The interpreter now correctly traces calls inside closures that are returned from other functions by ensuring that package-level environments are correctly populated and captured.
- [x] **Generics**:
  - [x] Support for evaluating calls to generic functions with explicit type arguments (e.g., `myFunc[int](...)`).
  - [x] Support for evaluating generic type instantiations in composite literals (e.g., `MyType[int]{...}`).
  - [x] The evaluator is now robust to calls to generic functions where type arguments are omitted (e.g., `myFunc(...)`). It does not crash and treats the call as symbolic. Full type inference is not implemented.
  - [x] The evaluator is now robust to generic functions with interface constraints (e.g., `[T fmt.Stringer]`). It does not crash and does not perform constraint checking.
- [x] **Channels**: The interpreter now has a concrete `object.Channel` type. The `make` intrinsic correctly creates channel objects, and receive operations (`<-ch`) are symbolically evaluated to produce a value of the correct element type. This provides the foundation for more advanced channel analysis.
- [x] **Stateful Type Tracking for Variables**: The `symgo` evaluator now correctly propagates type information (including pointer-ness) for variables during assignments and declarations. This allows for accurate method resolution on variables holding concrete types, pointer types, and interfaces, fixing several state-tracking-related bugs.
- [x] **LHS of Assignments**: The interpreter now evaluates expressions on the left-hand side of field assignments (e.g., in `foo.bar = baz`), ensuring that function calls or type assertions within `foo` are correctly traced.
- [x] **Anonymous Types**: The scanner now correctly parses anonymous `interface` and `struct` types in expressions (such as function parameters), preserving their structural definitions for the `symgo` interpreter.
- [x] **Other AST Nodes**: The interpreter now correctly handles all `ast.Node` types. This includes adding a placeholder for `*ast.StructType` to prevent "not implemented" errors during evaluation of type conversions. The completion of this item finalizes the work on the `symgo` interpreter's core evaluation loop.
    - [x] **`symgo`: Evaluator `Resolver` Refactoring**: Refactored the `symgo.evaluator.Resolver` to clarify the API around scan policy enforcement. Exported methods now consistently perform policy checks, while unexported methods can bypass them for internal use. This improves safety and aligns with the intended design.
    - [x] **`symgo`: Evaluator `accessor` Refactoring**: Refactored the `symgo.evaluator` to move `find...` methods to a new `accessor` struct.
    - [x] **`symgo`: Evaluator `Context` Refactoring**: Refactored the `symgo.evaluator` to pass `context.Context` as an argument instead of using `context.Background()` directly. This improves traceability and cancellation propagation.
    - [x] **Intra-package unexported constant/variable resolution**: The interpreter now correctly resolves unexported package-level constants and variables that are referenced from within a function in the same package by pre-loading them into the environment.
    - [x] `*ast.IfStmt` (Note: The interpreter now correctly evaluates the `Cond` expression, in addition to the `Init` statement and `Body`/`Else` blocks. A follow-up fix ensures that evaluation correctly continues after the `if` statement, resolving a regression in `docgen`.)
    - [x] `*ast.ChanType`
    - [x] `*ast.Ellipsis` (Note: Implemented for variadic arguments in function calls and definitions.)
    - [x] `*ast.FuncType`
    - [x] `*ast.InterfaceType`
    - [x] `*ast.MapType`
    - [x] `*ast.StructType`
    - [x] `*ast.EmptyStmt`
    - [x] `*ast.IncDecStmt`
    - [x] `*ast.LabeledStmt`
    - [x] `*ast.SendStmt`
- [x] **`panic` and other builtins**: The interpreter now recognizes `panic`, `nil`, `true`, and `false`. It also has placeholder implementations for most other standard built-ins (`make`, `len`, `append`, `new`, `cap`, etc.).
- [x] **Multi-value returns and assignments**: The interpreter now supports functions that return multiple values and assignments of the form `x, y := f()` and `x, y = f()`.
- [x] **`symgo`: Correctly scope function parameters**: Fixed a bug where function parameters and receivers were incorrectly set in the package scope instead of the function's local scope, causing "identifier not found" errors in nested blocks. The engine now also correctly handles analysis entry points by creating symbolic placeholders for function parameters that are not explicitly provided.
- [x] **Block Statements**: The interpreter now correctly handles nested block statements `{...}` within function bodies, creating a new lexical scope and correctly tracing function calls inside them.

### `symgo` Refinements
- [x] **Explicit Analysis Scopes**: Refactored the `symgo.Interpreter` configuration to use explicit scopes. `WithPrimaryAnalysisScope` defines packages for deep execution, and `WithSymbolicDependencyScope` defines packages for declarations-only parsing. This makes analysis more hermetic and predictable, removing the need for implicit on-demand loading of out-of-policy packages.
- [x] **Expressive Unresolved Types**: Enhanced the `scanner.TypeInfo` struct for unresolved types to include the type's `Kind` (e.g., interface, struct) if it can be determined from context. The evaluator now infers the kind from type assertions and composite literals, allowing for more precise analysis and removing unsafe assumptions in the `assignIdentifier` logic.
- [x] **Proper Error Handling in Resolver**: The `resolver.ResolveType` and `resolveTypeWithoutPolicyCheck` methods now correctly handle errors from `fieldType.Resolve(ctx)` by returning a symbolic placeholder, making the evaluator more robust against resolution failures.

### Future Enhancements
- [x] **`symgo`: Tracing and Debuggability**: Enhance the tracing mechanism to provide a more detailed view of the symbolic execution flow.
- [x] **Contextual Logging**: Warning and error logs emitted during evaluation now include the function call site (name and position) where the warning occurred. This is achieved by capturing the call stack within the evaluator and adding it to the log record.
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
