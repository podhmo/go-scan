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
*   **Simpler Generator Code**: The client code in generators (like `examples/derivingjson/main.go`) becomes cleaner as they can delegate import path and type qualification to the `ImportManager`.

**Application:**

The `ImportManager` was successfully integrated into `examples/derivingjson/main.go` and `examples/derivingbind/main.go`, significantly simplifying their import management sections. The development process involved iterative refinement of the alias generation rules within `ImportManager.Add` based on test cases covering various scenarios (keyword clashes, dot/hyphen in package names, alias conflicts).
