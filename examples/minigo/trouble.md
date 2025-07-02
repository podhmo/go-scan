# `minigo` Troubleshooting and Known Issues

## Package Imports and `go.mod`

- **`go.mod` Requirement for Module Imports**: `minigo`'s ability to import constants from other packages within the same Go module (e.g., `import "mytestmodule/testpkg"`) relies on `go-scan`'s module resolution capabilities. This, in turn, requires a `go.mod` file to be present either in the directory of the main `minigo` script being executed, or in one of its parent directories, or in the current working directory (or its parents) from which `minigo` is run.
    - If `go-scan` (and by extension, `minigo`) cannot find a relevant `go.mod` file to determine the module root and module path, it will not be able to resolve import paths for packages that are part of that conceptual module.
    - In such scenarios, attempting to import a package using a module path will likely result in a "package not found or failed to scan" error during the lazy import phase (when a symbol from that package is first accessed).

- **Standard Library Imports**: Importing standard library packages (e.g., `import "fmt"`) is not currently supported in `minigo` in the same way as user-defined module packages. `minigo` has its own built-in versions of some `fmt` and `strings` functions, which are available directly without needing an `import` statement. True importing of arbitrary standard library packages for their constants or other symbols is not implemented.

- **Imports Outside a Module Context**: If `minigo` is run on a script that is not part of a Go module (i.e., no `go.mod` is discoverable), then only built-in functions will be available. Any `import` statements attempting to reference other packages (even if they are in adjacent directories) will likely fail to resolve correctly, as there is no module context to interpret the import paths.

In summary, for `minigo`'s import mechanism (as of this writing, for constants) to work for packages other than its own built-ins, the main `minigo` script and the packages it imports should reside within a Go module defined by a `go.mod` file, and this `go.mod` must be discoverable by `go-scan` based on the script's location or the current working directory.
