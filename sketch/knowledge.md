# Knowledge Base

## Testing Go Modules with Nested `go.mod` Files

When a subdirectory within a Go module contains its own `go.mod` file, it effectively becomes a nested or sub-module. This is sometimes done intentionally, for example, to conduct acceptance tests where the sub-module mimics an independent consumer of the parent module's packages, or to manage a distinct set of dependencies for a specific part of the project (like examples or tools).

### Running Tests in a Nested Module

If you try to run tests for packages within such a nested module from the parent module's root directory using standard package path patterns (e.g., `go test ./path/to/nested/module/...`), you might encounter errors like "no required module provides package" or other resolution issues. This happens because the Go toolchain gets confused about which `go.mod` file to use as the context.

To correctly run tests for a nested module:

1.  **Change Directory**: Navigate into the root directory of the nested module (i.e., the directory containing its specific `go.mod` file).
    ```bash
    cd path/to/nested/module
    ```

2.  **Run Tests**: Execute `go test` commands from within this directory. You can specify packages relative to this nested module's root.
    ```bash
    # To test all packages within the nested module
    go test ./...

    # To test a specific sub-package within the nested module
    go test ./subpackage
    ```

**Example from this Repository (`go-scan`):**

The `examples/derivingjson` directory in this repository contains its own `go.mod` file. This is intentional, designed to simulate an acceptance test environment where `derivingjson` (and its generated code) is treated as if it were a separate module consuming functionalities from the main `go-scan` module.

To test the models within `examples/derivingjson/models`:

```bash
cd examples/derivingjson
go test ./models
```

**Important Note on `examples` Directory `go.mod`:**

Please do not delete the `go.mod` file located in example directories (like `examples/derivingjson/go.mod`). These are specifically set up to ensure that the examples can be built and tested as if they were separate modules, which is crucial for acceptance testing of the main library's features from an external perspective.

This approach ensures that the tests accurately reflect how an external consumer would use the library and helps catch integration issues that might not be apparent when testing everything within a single module context.

## Using Go 1.24+ Iterator Functions (Range-Over-Function)

### Context

For the `astwalk` package, specifically the `ToplevelStructs` function, a decision was made to return an iterator function compatible with Go's "range-over-function" feature (stabilized in Go 1.24). The function signature is `func(fset *token.FileSet, file *ast.File) func(yield func(*ast.TypeSpec) bool)`.

### Rationale

This approach was chosen to leverage modern Go idioms for iterating over AST nodes, offering potential benefits in ergonomics and efficiency, especially for large datasets.

- **Ergonomics**: The `for ... range` syntax over a function call is idiomatic and readable.
- **Efficiency**: Iterators process items one by one, which can be more memory-efficient than allocating a slice for all items upfront.
- **Lazy Evaluation**: Work to find the next item is done only when requested.

### Implementation Notes

The `ToplevelStructs` function in the `astwalk` package provides an iterator for top-level struct type specifications within a Go source file.

- **Usage**: It can be used with a `for...range` loop in Go 1.24 and later.
- **Go Version Dependency**: This feature requires Go 1.24 or newer. The main module's `go.mod` file is set to `go 1.24`.

### Conclusion

The use of the range-over-function pattern for `ToplevelStructs` aligns with modern Go practices, offering a clean and efficient way to process AST nodes. Users of the `astwalk` package should ensure their environment uses Go 1.24 or a later version.

---

## Centralized Import Management in Code Generators: `ImportManager`

**Context:**

When developing multiple code generators (e.g., `examples/derivingjson`, `examples/derivingbind`), a common challenge arises in managing `import` statements for the generated Go code. Each generator needs to:
1.  Identify all necessary packages to import based on the types and functions used in the generated code.
2.  Assign appropriate package aliases, especially to avoid conflicts if multiple imported packages have the same base name (e.g., `pkg1/errors` and `pkg2/errors`).
3.  Ensure that generated aliases do not clash with Go keywords (e.g., `type`, `range`).
4.  Handle sanitization of package names that might not be valid Go identifiers if used directly as aliases (e.g., paths containing dots or hyphens like `example.com/ext.pkg`).
5.  Produce a final list of import paths and their aliases for inclusion in the generated file.

Implementing this logic independently in each generator leads to code duplication, potential inconsistencies, and an increased likelihood of bugs.

**Solution: `goscan.ImportManager`**

To address this, a utility struct `goscan.ImportManager` was introduced within the `go-scan` library itself. This provides a centralized and reusable solution for import management.

**Key Features and Design Decisions:**

*   **Centralized Logic**: `ImportManager` encapsulates all the common logic for adding imports, resolving alias conflicts, and formatting the final import list.
*   **`Add(path string, requestedAlias string) string`**:
    *   Registers an import path.
    *   If `requestedAlias` is empty, it derives a base alias from the last element of the import path.
    *   **Sanitization**: It sanitizes the base alias by replacing common non-identifier characters like `.` and `-` with `_` (e.g., `ext.pkg` becomes `ext_pkg`).
    *   **Keyword Handling**: If the sanitized alias is a Go keyword (e.g., `range`), it appends `_pkg` (e.g., `range_pkg`). This check is performed *before* the identifier validity check to ensure keywords are handled correctly even if they are valid identifiers.
    *   **Identifier Validity**: If the (potentially keyword-adjusted) alias is still not a valid Go identifier (e.g., starts with a number, or was empty after sanitization), it prepends `pkg_`. A fallback to a hash-based name is included for extreme edge cases where the alias becomes problematic (e.g. just "pkg_").
    *   **Conflict Resolution**: If the final candidate alias is already in use by a different import path, it appends a numeric suffix (e.g., `myalias1`, `myalias2`) until a unique alias is found.
    *   Returns the actual alias to be used in qualified type names.
    *   If the `path` is the same as the `currentPackagePath` (the package for which code is being generated), it returns an empty string, as types from the current package do not need qualification.
*   **`Qualify(packagePath string, typeName string) string`**:
    *   A convenience method that calls `Add(packagePath, "")` internally to ensure the package is registered and to get its managed alias.
    *   Returns the correctly qualified type name (e.g., `alias.TypeName` or just `TypeName` if it's a local type or built-in).
*   **`Imports() map[string]string`**:
    *   Returns a map of all registered import paths to their final, resolved aliases, suitable for passing to `goscan.GoFile`.

**Benefits:**

*   **Reduced Boilerplate**: Generators no longer need to implement their own complex import tracking and alias resolution logic.
*   **Consistency**: Ensures a consistent approach to alias generation and conflict handling across different generators using `go-scan`.
*   **Improved Robustness**: Centralized logic is easier to test thoroughly, leading to more reliable import management. The `ImportManager` includes specific handling for keywords, invalid identifiers, and common path-based naming issues.
*   **Simpler Generator Code**: The client code in generators (like `examples/derivingjson/main.go`) becomes cleaner as they can delegate import path and type qualification Nasıls to the `ImportManager`.

**Application:**

The `ImportManager` was successfully integrated into `examples/derivingjson/main.go` and `examples/derivingbind/main.go`, significantly simplifying their import management sections. The development process involved iterative refinement of the alias generation rules within `ImportManager.Add` based on test cases covering various scenarios (keyword clashes, dot/hyphen in package names, alias conflicts).

---

## Stabilizing CWD and Command Execution in the Jules Sandbox Environment

**Note: The following insights are specific to the Jules AI agent's sandbox environment and do not represent general development environment issues or solutions.**

### Problem Overview

During certain development tasks (e.g., implementing `for range` in `examples/minigo`), the behavior of the sandbox environment when using the `run_in_bash_session` tool became unstable. Key symptoms included:

*   The `cd` command failing with a `No such file or directory` error, even when `ls` confirmed the target directory's existence.
*   Makefile targets such as `make format` or `make test` failing due to not being executed in the expected Current Working Directory (CWD).
*   The `reset_all()` tool not always resolving these CWD-related issues.

These problems made it difficult for the agent to accurately ascertain the file system's state and execute commands as intended.

### Solution/Stabilization Practice (Jules Environment Specific)

Based on extensive trial-and-error and user guidance, the following procedure was found to improve the stability of command execution:

1.  **Attempt Environment Reset**: First, execute `reset_all()` to try and return the sandbox environment to its initial state. This is done with the expectation of resetting not just file contents but also some internal states.

2.  **Explicit `cd` to Root Directory**: When executing commands in `run_in_bash_session`, especially after `reset_all()` or when the CWD is uncertain, first change the directory to `/app`, which is assumed to be the repository root within the sandbox.
    ```bash
    cd /app
    ```

3.  **Execute Target Command**: Following the `cd /app` command, execute the desired command using `&&`.
    *   To run a `make` target in a specific directory:
        ```bash
        cd /app && make -C path/to/makefile/dir target
        ```
        Or, if the `make` target handles `cd` internally:
        ```bash
        cd /app && make target
        ```
    *   To run Go commands in a specific directory:
        ```bash
        cd /app && cd path/to/go_module_dir && go test -v ./...
        ```

Prepending `cd /app` significantly increased the likelihood that subsequent commands would execute in the expected CWD, allowing previously failing `cd`, `make`, and Go test commands to succeed.

### Why This Approach Might Be Effective (Hypothesis)

In the Jules sandbox environment, it's possible that the CWD state is not consistently maintained across `run_in_bash_session` calls, or that `reset_all()` does not reliably set the CWD back to `/app`. By explicitly changing to `/app` first, subsequent relative path interpretations and the CWD for subprocesses (like `make`) may become more stable.

This insight is recorded as a potentially effective troubleshooting step for CWD-related anomalies encountered within the Jules environment.

---

## Dynamic Resolution of Standard Library Functions

**Context:**

A requirement emerged to support functions from standard library packages (like `slices.Clone`) without pre-generating or manually registering bindings. The `minigo` interpreter, via `go-scan`'s `WithGoModuleResolver` option, is capable of locating the source code for these packages.

**Problem & Discovery:**

Initial attempts to call `slices.Clone` led to an `internal inconsistency: symbol ... found by scanner but not in final package info` error. This was traced to the `findSymbolInPackageInfo` function in `minigo/evaluator/evaluator.go`, which was only equipped to look for constants and struct types within the scanned package information, but not functions.

**Solution & Surprising Behavior:**

1.  **The Fix**: The immediate fix was to enhance `findSymbolInPackageInfo` to also search the `Functions` slice of the `goscan.Package` info. When a function is found, a stub `*object.Function` is created using the AST (`*ast.FuncDecl`) provided by the scanner.

2.  **Unexpected Success**: It was hypothesized that this would lead to a new runtime error, as the `minigo` evaluator should not be able to execute the raw Go AST body of a standard library function like `slices.Clone`. Surprisingly, the test *passed*.

**Analysis of Success:**

Further investigation revealed that the `minigo` evaluator, when encountering the `*object.Function` stub for `slices.Clone`, proceeded to evaluate its body: `return append(S(nil), s...)`. The key factors for its success are:
*   The body consists of a call to another function, `append`, which is a `minigo` **builtin**.
*   The evaluator's generic type inference (`inferGenericTypes`) was capable of resolving the generic type parameter `S` to the concrete type of the slice passed to `Clone`.
*   The type conversion `S(nil)` was successfully handled, likely being interpreted as creating a `nil` value of the inferred slice type.

**Conclusion:**

The `minigo` interpreter possesses a powerful, albeit perhaps unintentional, capability: it can dynamically resolve and execute the AST of Go standard library functions, provided that the function's body is composed entirely of constructs that the `minigo` evaluator itself can understand (like calls to its own builtins).

This hybrid approach—finding a function's AST via `go-scan` and then evaluating that AST within `minigo`—allows for the dynamic use of some Go code without explicit bindings. This is a significant piece of knowledge about the system's architecture. The crucial fix was ensuring the symbol resolution path (`findSymbolInPackageInfo`) correctly identified function ASTs from the scanner's output.

---

## The Journey to Supporting `sort.Ints`

**Context:**

The implementation of two-pass evaluation was intended to fix the "Sequential Declaration Processing" limitation in the `minigo` interpreter. The primary success metric for this task was the ability to correctly interpret the standard library's `sort.Ints` function, which transitively depends on the `slices` package—a package that uses forward-referencing internally.

This task evolved into a multi-stage debugging process that revealed several other core limitations of the interpreter and its interaction with the FFI (Foreign Function Interface).

**The Debugging Cascade:**

1.  **Initial Goal: Direct Source Interpretation**: The initial plan was to test `sort.Ints` by having the interpreter load its source code directly. The new two-pass evaluator was expected to handle the forward references in the `slices` dependency.

2.  **Failure 1: Missing Built-in Types**: The test immediately failed with `identifier not found: uint`. This was because the interpreter's `evalIdent` function did not recognize `uint` as a valid built-in type name. This was patched, only to lead to a subsequent failure for `uint64`. This revealed that the interpreter's list of known primitive types was incomplete.

3.  **Failure 2: Type Conversion**: After adding the types, a new error, `not a function: TYPE`, occurred. This was because the interpreter was trying to handle a type conversion, `uint(n)`, as a regular function call. The `Eval` logic for `ast.CallExpr` was enhanced to detect when the "function" being called is actually a type and to dispatch to a new `evalTypeConversion` handler.

4.  **Failure 3: Complex Constant Evaluation**: The next failure, `could not convert constant "len8tab"`, revealed a more fundamental limitation. The `go-scan` tool, when encountering a complex computed constant (like the array literal for `len8tab` in `math/bits`), was unable to resolve its value and returned an empty string. The interpreter's `constantInfoToObject` function could not handle this. This was deemed a significant blocker for the direct source interpretation approach for this package.

5.  **Strategic Pivot to FFI**: As per the project's own documentation (`plan-minigo-stdlib-limitations.md`), when direct interpretation is not feasible, the fallback is to use FFI bindings. The test case for `sort.Ints` was rewritten to use the pre-generated bindings (`stdsort.Install`).

6.  **Failure 4: Pass-by-Value Slice Semantics**: The FFI-based test then failed with an assertion error: the slice was not being sorted. This was because the FFI bridge (`objectToReflectValue`) was creating a *copy* of the minigo slice's data into a new Go slice. The Go `sort.Ints` function sorted this copy, leaving the original interpreter-side array untouched.

7.  **Failure 5: FFI Data Copy-Back Bug**: A fix was implemented to copy the data from the (now sorted) Go slice back into the original `minigo` array after the FFI call completed. This fix, however, introduced new regressions. The `nativeToValue` function, responsible for converting the Go slice elements back to `minigo` objects, was not comprehensive. It incorrectly wrapped primitive types like `float64` and `uint8` in a generic `*object.GoValue` instead of their specific `*object.Float` or `*object.Integer` types, causing type mismatch errors in other tests.

**Final Resolution:**

The final fix involved two key changes:
1.  **Enhancing `nativeToValue`**: The function was updated to correctly handle a wider range of Go primitive numeric types (`float64`, `uint8`, etc.), ensuring that data copied back from the FFI bridge retained the correct `minigo` object type.
2.  **Pragmatic Simplification**: During this fix, the decision was made to treat all Go integer types (signed and unsigned) as a single `minigo` `*object.Integer` (which wraps an `int64`). This was a pragmatic choice to solve the immediate problem and get the `sort` test passing, with the understanding that a more nuanced internal type system is a possible future enhancement but not strictly necessary for the current scope.

This entire process highlights the interconnectedness of the interpreter's components and provides valuable knowledge for future standard library integration efforts. The `sort.Ints` test case served as an excellent catalyst for hardening the type system, FFI bridge, and evaluation strategy.
