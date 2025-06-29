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

## Adopting Go 1.23 Experimental Iterator Functions (Range-Over-Function)

### Context

For the `astwalk` package, specifically the `ToplevelStructs` function, a decision was made to return an iterator function compatible with Go 1.23's experimental "range-over-function" feature. The function signature is `func(fset *token.FileSet, file *ast.File) func(yield func(*ast.TypeSpec) bool)`.

### Rationale

The primary motivation for adopting this experimental feature was to explore modern Go idioms and provide a potentially more ergonomic and efficient way to iterate over AST nodes, especially when the number of nodes could be large.

- **Ergonomics**: The `for ... range` syntax over a function call can be more readable and idiomatic Go compared to manually managing a callback or channel-based iteration.
- **Efficiency**: Iterators can be more memory-efficient as they process items one by one, avoiding the need to allocate a slice for all items upfront. This is particularly beneficial when dealing with large source files or when the consumer might only need a subset of the items.
- **Lazy Evaluation**: The work to find the next item is only done when the consumer requests it.

### Decision Process & Alternatives Considered

1.  **Returning a Slice (`[]*ast.TypeSpec`)**: This was the initial, more conventional approach.
    - *Pros*: Simple to implement and understand. Widely compatible with all Go versions.
    - *Cons*: Less memory-efficient for large datasets as it requires allocating memory for all struct type specifications at once. The caller receives all data even if only a few items are needed.

2.  **Callback-based Iterator (`func(fset *token.FileSet, file *ast.File, yield func(*ast.TypeSpec) bool)`)**: A function that takes a callback `yield` which is called for each item.
    - *Pros*: More memory-efficient than returning a slice. Allows early exit if the `yield` function returns `false`. Compatible with older Go versions.
    - *Cons*: Can be slightly less ergonomic to use compared to `for...range`.

3.  **Channel-based Iterator (`func(fset *token.FileSet, file *ast.File) <-chan *ast.TypeSpec`)**: A function that returns a channel from which items can be received.
    - *Pros*: Enables concurrent processing if desired. Familiar pattern in Go.
    - *Cons*: Can be slower due to channel overhead. More complex to implement correctly (e.g., ensuring the goroutine producing items on the channel always exits).

4.  **Go 1.23 Range-Over-Function (Chosen)**:
    - *Pros*: Combines the ergonomic `for...range` syntax with the efficiency of lazy evaluation and potential for early exit. Represents a forward-looking approach.
    - *Cons*:
        - **Experimental**: As of Go 1.22/1.23, this feature is experimental. This means its API or behavior could change in future Go versions, or it might even be removed (though less likely for popular features).
        - **Go Version Dependency**: Requires Go 1.23+ (or a Go version that supports the specific `GOEXPERIMENT` flag, e.g., `GOEXPERIMENT=rangefunc`). This limits the usability of the `astwalk` package for projects using older Go versions.
        - **Build/Test Complexity**: May require setting `GOEXPERIMENT=rangefunc` during builds or tests, adding a slight complexity to the development workflow if not using Go 1.23 toolchain by default.

### Conclusion

The decision to use the Go 1.23 iterator pattern for `ToplevelStructs` was made with the understanding of its experimental nature and the Go version constraint. It serves as an exploration of new language features within this project. If broader compatibility with older Go versions becomes a critical requirement, this function might need to be refactored or an alternative provided. For now, it aligns with a forward-looking development approach. The `go.mod` file for the module has been updated to `go 1.23`.
