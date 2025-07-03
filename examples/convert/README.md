# Go Type Converter Example (`examples/convert`)

This example demonstrates a prototype for generating Go type conversion functions using the `go-scan` library. It explores how `go-scan` can be leveraged to read Go source code, understand type structures, and then automatically generate boilerplate code for converting one struct type to another.

## Overview

In many applications, you often need to convert data between different Go struct types. For example:
*   Converting from a database model to an API response model.
*   Transforming data from an external service's format to an internal application format.
*   Mapping between different versions of a data structure.

Manually writing these conversion functions can be tedious, error-prone, and repetitive, especially when dealing with complex nested structures or many fields. This example aims to automate this process.

## Project Structure

*   `models/models.go`: Defines the source (`Src*`) and destination (`Dst*`) struct types that we want to convert between.
*   `converter/converter.go`: Contains **manually written** conversion functions. This serves as a reference for what the generator should ideally produce.
*   `converter/converter_test.go`: Unit tests for the manually written conversion functions in `converter.go`.
*   `converter/generated_converters.go`: This file will be **generated** by the prototype generator logic in `main.go`.
*   `mapping/mapping.minigo`: A conceptual DSL (Domain Specific Language) file outlining how users might specify custom conversion rules in a more advanced version of the generator. This is currently for discussion and not fully implemented.
*   `main.go`:
    1.  Runs examples using the manually written converters (`runConversionExamples()`).
    2.  Contains the prototype generator logic (`generateConverterPrototype()`) which uses `go-scan` to parse `models/models.go` and generate conversion functions into `converter/generated_converters.go`.

## How it Works (Conceptual & Prototype)

1.  **Model Definition**: You define your source and destination Go structs in `models/models.go`.
2.  **Scanning with `go-scan`**: The `generateConverterPrototype()` function in `main.go` uses `go-scan` to:
    *   Parse the `models` package.
    *   Extract detailed information about `struct` types, including fields, field types, tags, and embedded structs.
3.  **Generator Logic (Prototype in `main.go`)**:
    *   It identifies pairs of source and destination types (e.g., `SrcUser` and `DstUser`).
    *   It attempts to map fields between the source and destination structs based on:
        *   (Future) Name normalization (e.g., converting `UserID` to `user_id` for matching).
        *   (Future) Struct tags (e.g., `convert:"targetFieldName"`).
        *   (Current) Some hardcoded rules based on the manual `converter.go` for demonstration.
    *   It generates Go functions that perform the assignments, including:
        *   Direct assignment for type-compatible fields.
        *   Calls to other generated or helper functions for nested structs or complex type conversions (e.g., `time.Time` to `string`).
        *   Handling of slices by iterating and converting each element.
4.  **Code Output**: The generated Go conversion functions are written to `converter/generated_converters.go`.

## Running the Example

You can run this example from the root directory of the `go-scan` project.

1.  **Run the main program**:
    This will first execute the examples using the manual converters and then run the generator prototype, which will create/overwrite `examples/convert/converter/generated_converters.go`.

    ```bash
    go run ./examples/convert/main.go
    ```

2.  **Inspect Generated Code**:
    After running, inspect `examples/convert/converter/generated_converters.go` to see the output of the prototype.

3.  **Run Tests**:
    To test the manually written converters (and potentially the generated ones if you integrate them into the test path):
    ```bash
    go test ./examples/convert/converter/...
    ```
    (You might need to be in the `examples/convert` directory or adjust paths if your Go workspace setup differs).

## Role of `go-scan`

`go-scan` is crucial for this example because:

*   It allows the generator to understand the structure of Go types **without relying on `go/types` or compiling the code**. This makes the generator faster and more lightweight.
*   It provides detailed information about fields (name, type, tags, embedded status), which is essential for mapping logic.
*   Its `PackageResolver` and type resolution capabilities (e.g., `FieldType.Resolve()`) are key to handling types from different packages (though this example primarily focuses on types within the same `models` package for simplicity in its current prototype stage).
*   It can access documentation comments, which could be used in the future for annotation-driven conversion rules.

## Future Development (for this example)

The current generator in `main.go` is a very basic prototype. A more complete generator would:

*   Implement robust field name normalization and matching.
*   Fully support struct tags for explicit field mapping.
*   Allow users to specify custom transformation functions for incompatible types (potentially via a DSL like in `mapping.minigo` or Go code registration).
*   Handle pointer-to-value, value-to-pointer, and other complex conversions more generically.
*   Manage imports in the generated file dynamically (e.g., using `go-scan`'s `ImportManager`).
*   Provide better error handling and reporting during generation.
*   Be structured as a proper command-line tool.

This example serves as a starting point and a testbed for exploring these more advanced code generation capabilities using `go-scan`.
