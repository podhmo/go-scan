# Deriving JSON for oneOf

`derivingjson` is an experimental tool that leverages the `github.com/podhmo/go-scan` library to automatically generate `UnmarshalJSON` and `MarshalJSON` methods for Go structs that have a structure analogous to JSON Schema's `oneOf`.

## Overview

In JSON Schema, `oneOf` signifies that a field can be one of several types. A common way to represent this concept in Go involves using an interface type, a set of concrete structs that implement this interface, and a container struct that includes a discriminator field to identify the specific type.

This tool aims to generate:
- An `UnmarshalJSON` method for container structs, enabling unmarshaling into the appropriate concrete type based on the discriminator's value.
- A `MarshalJSON` method for the concrete types, which injects the discriminator field and value into the JSON output.

## Features

-   Type information analysis using `github.com/podhmo/go-scan`.
-   **Unmarshaling**: Targets container structs annotated with `@deriving:unmarshal`.
-   **Marshaling**: Targets concrete implementer structs annotated with `@deriving:marshal`.
-   Identifies the discriminator field (e.g., `Type string `json:"type"``) and the `oneOf` target interface field to generate the appropriate logic.
-   The tool searches for concrete types implementing the interface within the same package.

## Usage (Conceptual)

1.  Add the `@deriving:unmarshal` annotation in the comment of the **container struct** (the one with the interface field) to generate `UnmarshalJSON` for it.
2.  Add the `@deriving:marshal` annotation in the comment of each **concrete struct** that implements the `oneOf` interface to generate `MarshalJSON` for it.
3.  Run `derivingjson` from the command line, specifying the target package path.

    ```bash
    go run examples/derivingjson/main.go <file_path_1.go> [file_path_2.go ...]
    # Or after building
    # ./derivingjson <file_path_1.go> [file_path_2.go ...]
    ```

    Example (single file, implies processing its package):
    ```bash
    go run examples/derivingjson/main.go ./examples/derivingjson/testdata/simple/models.go
    ```

    Example (multiple files from different packages):
    ```bash
    go run examples/derivingjson/main.go ./examples/derivingjson/testdata/separated/models/models.go ./examples/derivingjson/testdata/separated/shapes/shapes.go
    ```
3.  A file named like `packagename_deriving.go`, containing the implemented `UnmarshalJSON` method, will be generated in the package directory of each processed Go file.

## Disclaimer

This tool is experimental, serving as a trial and demonstration for the `go-scan` library.
