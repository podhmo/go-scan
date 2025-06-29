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
