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
- **Automated Minigo Bindings Generation**: Created a tool to automatically generate `minigo` bindings for Go packages, including initial support for several standard library packages. ([docs/plan-minigo-gen-bindings.md](./docs/plan-minigo-gen-bindings.md))
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
- [ ] **Fix empty slice type inference**: Type inference for empty slice literals is weak and defaults to `[]any`. This causes legitimate generic functions (like `slices.Sort`) to fail type checks when they shouldn't. The interpreter should ideally preserve the declared type (e.g., `[]int`) even if the literal is empty.
- [x] **Fix `json.Unmarshal` error propagation**: The FFI fails to correctly propagate `*json.UnmarshalTypeError` from `json.Unmarshal`, returning a `nil` value instead. This prevents scripts from handling JSON validation errors correctly.
- [x] **Improve method call support for stateful objects**: The FFI and evaluator have trouble with packages like `container/list` where methods (`PushBack`, `Next`) modify the internal state of a Go object in a way that is not correctly reflected back into the script environment. This prevents effective use of stateful, object-oriented packages.
- [x] **Support slice operator on Go-native arrays**: The interpreter does not support the slice operator (`[:]`) on `object.GoValue` types that wrap Go arrays (e.g., `[16]byte`). This was discovered when testing `crypto/md5` and blocks the use of functions that return native Go arrays.
- [x] **Improve generic type inference for composite types**: The type inference engine fails to infer type parameters when they are part of a composite type in a function argument (e.g., inferring `E` from a parameter of type `[]E`). This was discovered when testing `slices.Sort` and currently blocks its use via source interpretation.
- [x] **Improve interpreter performance for complex algorithms**: `slices.Sort` fails to complete within the test timeout, indicating severe performance bottlenecks when interpreting complex code like sorting algorithms.
