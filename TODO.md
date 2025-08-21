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
- **`docgen` and Symbolic-Execution-like Engine (`symgo`)**: A symbolic execution engine (`symgo`) and a demonstration tool (`docgen`) that uses it to generate OpenAPI 3.1 specifications from `net/http` server code. The engine can trace function calls, resolve types, and evaluate handler logic to infer routes, methods, parameters, and request/response schemas. The `docgen` tool includes a golden-file test suite and can output in both JSON and YAML. ([docs/plan-symbolic-execution-like.md](./docs/plan-symbolic-execution-like.md))
    - Removed manual stubs for standard library types (`net/http`, `io`, etc.) from the `docgen` example, as `go-scan`'s improved module-aware resolution now handles them automatically.

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

### `symgo` Engine Refinements
- [x] **Fix slice literal type inference regression**: The evaluator's `evalCompositeLit` incorrectly resolves the type of a slice literal (e.g., `[]User{}`) to its element type (`User`), losing the "slice-ness" of the value. This prevents `docgen` from generating correct response schemas for endpoints that return slices. See [docs/trouble-docgen.md](./docs/trouble-docgen.md) for details.
- [x] **Fix non-slice response schema generation in `docgen`**: While fixing the slice response issue, a regression was introduced where handlers returning single struct instances (e.g., `func() User`) no longer have their response schemas generated correctly. This was tracked in [docs/trouble-docgen.md](./docs/trouble-docgen.md) and was resolved by enhancing the `symgo` evaluator to handle field assignments, multi-value return assignments, and to lazy-load package constants.
- [x] **Create `symgo.Interpreter` Facade**: Create a new `symgo` package with an `Interpreter` type, modeled after `minigo.Interpreter`, to provide a clean public API for the engine. ([docs/trouble-docgen-use-symgo.md](./docs/trouble-docgen-use-symgo.md))
    - [x] Implement a `NewInterpreter()` constructor that properly initializes the internal scanner and evaluator.
    - [x] Implement a public `RegisterIntrinsic()` method on the interpreter.
    - [x] Implement a public `Eval()` method to run the analysis.
    - [x] Re-export necessary types like `symgo.Object` and `symgo.Function` from the new package.
- [x] **Refactor `docgen` to use `symgo.Interpreter`**: The `docgen` tool has been refactored to use the `symgo.Interpreter`, replacing the initial manual AST traversal. The `symgo` engine itself was enhanced to support this.
- [x] **Resolve variable arguments in intrinsics**: Ensure that when a variable is passed to an intrinsic function, the function receives the variable's underlying value (`*object.String`, etc.) rather than the `*object.Variable` wrapper. This simplifies intrinsic implementation and aligns argument evaluation with return value evaluation.
- [x] **Support string concatenation**: Implemented support for the `+` operator for string types in binary expressions.

### `symgo` and `docgen` Improvements

> Note: `docgen` is intended to be a test pilot for `symgo`. When discovering missing features or bugs in `docgen`, the preferred workflow is to return to the `symgo` engine, break the problem down into a minimal test case, add that test, and then modify the `symgo` implementation until the test passes.

- [x] **Fix `docgen` integration test**: The `examples/docgen/main_test.go` was previously skipped because it failed to generate a response schema for handlers that make calls on a bound interface (`http.ResponseWriter`). This was caused by two issues: 1) a state propagation problem where side effects on the `openapi.Operation` object were lost, and 2) an evaluator bug where `[]byte` type conversions were not handled, preventing intrinsics from being called. Both issues have been resolved.
    - [x] Re-fixed the state propagation issue by ensuring the modified `openapi.Operation` object is correctly returned from the handler body analysis.
- [x] **Extend Custom Patterns**: Extend the `minigo`-based pattern system to support configuring path, query, and header parameter extraction, similar to how `requestBody` and `responseBody` are handled now.
    - [x] **Investigate and fix parameter loss**: The custom parameter handlers are being called, but the extracted parameters are not appearing in the final OpenAPI spec. The modifications to the `openapi.Operation` object are being lost.
- [x] **Implement full intra-module recursive evaluation**: Enhanced the `symgo` evaluator to distinguish between intra-module and extra-module function calls, recursively evaluating the former as specified in the design plan.
- [x] **Add `defaultResponse` and `map` type support**:
    - [x] Add `additionalProperties` to the `openapi.Schema` model to allow for `map` type schemas.
    - [x] Add a `defaultResponse` pattern to `docgen` to allow defining responses with arbitrary status codes from a `minigo` config.
    - **Note**: The bug preventing custom patterns from being applied in intra-module setups has been resolved by ensuring the `symgo` interpreter's environment is correctly populated with package imports before evaluation.

A set of tasks to improve the `symgo` engine and the `docgen` tool based on the analysis in `docgen/ja/from-docgen.md`.

- [x] **Step 1: Error Reporting and Engine Stabilization**
    - [x] **Extend `object.Error`**: Add a `token.Pos` field to the `Error` struct in `symgo/object/object.go` to hold source code position.
    - [x] **Improve Error Messages**: Update the `symgo.Interpreter` to use the `token.Pos` to include file, line, and column information in error messages.
    - [x] **Implement `fmt.Sprintf` Intrinsic**: Add a `symgo` intrinsic to mimic the basic behavior of `fmt.Sprintf` for dynamic string construction.
    - [x] **Support `if-else`**: Modify `evalIfStmt` in `symgo/evaluator/evaluator.go` to correctly evaluate `else` blocks.
    - [x] **Support `for` loop**: Modify `evalForStmt` in `symgo/evaluator/evaluator.go` to symbolically evaluate for loops (unroll once).
    - [x] **Support `switch-case`**: Modify `evalSwitchStmt` in `symgo/evaluator/evaluator.go` to symbolically evaluate all branches of a switch statement.
- [x] **Step 2: Debugging Features**
    - [x] **Implement Structured Logger**: Introduce an optional structured logger in the `symgo.Evaluator` to trace evaluation steps, including node info, position, and results.
    - [x] **Add Debug Flag to `docgen`**: Add a `--debug-analysis <functionName>` flag to `docgen` to enable the structured logger for a specific function.
- [x] **Step 3: User Extensibility**
    - [x] **Implement `minigo`-based Pattern Loader**: Create a loader in `docgen` that reads a Go script (`.go` file with a build-ignore tag) and parses a list of pattern definitions. This allows users to define custom analysis patterns without recompiling `docgen`. The loader uses `minigo` to evaluate the script, which returns a slice of maps defining the patterns.
    - [x] **Integrate Pattern Loader with Analyzer**: Modify the `docgen.Analyzer` to accept custom patterns via a `--patterns` command-line flag and dynamically register them as `symgo` intrinsics at startup.

### External Package Resolution Improvements
- [x] **Fix `replace` directive handling for parent directories**: Modified `go-scan` and `locator` to correctly resolve packages when a `go.mod` `replace` directive points to a directory outside of the current module's root (e.g., `../`). This unblocks testing scenarios and `minigo`-based configurations that rely on such setups.
- [x] **Fix Module-Aware Resolution in Test Environments**: The `symgo` engine relies on a module-aware `go-scan` instance to resolve types from external packages (including the standard library like `net/http`). This resolution was failing in test environments, causing method calls on external interfaces to be missed. This has been resolved by refactoring the `symgo` evaluator to use the top-level `goscan.Scanner`, ensuring it has the correct module context.
- [x] **Improve External Package Support in `scantest`**: The `scantest` helper library currently does not use a module-aware scanner by default, making it difficult to test code that relies on types from external packages (including the standard library). The `scantest.Run` function should be updated to use `goscan.WithGoModuleResolver()` automatically, or the existing tests that need it should be updated to provide a properly configured scanner.
