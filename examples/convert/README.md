# Go Type Converter (`examples/convert`)

This directory contains the `convert` tool, a command-line application that automatically generates Go type conversion functions based on **annotations** and **struct tags**. It uses `go-scan` to parse Go source code, understand type structures, and then generate the necessary boilerplate code for converting one struct type to another.

> **Note: New IDE-Friendly Method Available**
>
> This annotation-based tool is the original generator. For new projects, we recommend using the new `define`-based tool, which provides a more robust, type-safe, and IDE-friendly way to configure conversions.
>
> See the [**`../convert-define`**](../convert-define) example for details.

## Overview

In many Go applications, you need to convert data between different struct types. Manually writing these conversion functions is tedious and error-prone. This tool automates the process using source code annotations.

## Key Features

*   **Annotation-Driven**: Generation is triggered by a `@derivingconvert` annotation in the source struct's doc comment.
*   **Flexible Field Mapping**: Fields are matched automatically with a clear priority:
    1.  An explicit name in a `convert:"<name>"` tag.
    2.  A name in a `json:"<name>"` tag.
    3.  The normalized field name.
*   **Custom Conversion Logic**:
    *   Use the `convert:",using=<func>"` tag for field-specific custom conversion functions.
    *   Define global type-to-type conversion rules with `// convert:rule "<Src>" -> "<Dst>", using=<func>`.
*   **Recursive Generation**: Automatically handles nested structs, slices, maps, and pointers.
*   **CLI Tool**: A proper command-line interface for easy integration into build processes.

## Annotation and Tag Reference

### `@derivingconvert`
Triggers the generation of a conversion function. Placed in the doc comment of the source struct.

**Syntax**: `@derivingconvert(<DestinationType>[, option=value, ...])`

### `// convert:rule`
Defines a global rule for type conversion or validation.

**Conversion Rule**: `// convert:rule "<SourceType>" -> "<DestinationType>", using=<FunctionName>`

### `convert` Struct Tag
Controls the conversion of a specific field.

**Syntax**: `` `convert:"[destinationFieldName],[option=value],..."` ``
*   `[destinationFieldName]`: Maps to a different field name in the destination struct. Use `-` to skip the field.
*   `using=<funcName>`: Use a custom function for this field's conversion.

## How to Use

1.  **Annotate your code**: Add `@derivingconvert` annotations to your source structs and any necessary `convert` tags or `// convert:rule` comments.

2.  **Run the tool**: Execute the tool from your terminal, specifying the target package and output file.

    ```bash
    go run github.com/podhmo/go-scan/examples/convert \
      -pkg "github.com/your/project/models" \
      -output "github.com/your/project/models/generated_converters.go"
    ```

## As a Library

The components of this tool (`parser`, `generator`, `model`) can also be used as a library to build more complex code generation tools. The `../convert-define` example is a demonstration of this, as it uses the `generator` and `model` packages from this module.
