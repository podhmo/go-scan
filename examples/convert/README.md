# Go Type Converter Example (`examples/convert`)

This directory contains the `convert` tool, a command-line application that automatically generates Go type conversion functions. It uses `go-scan` to parse Go source code, understand type structures via annotations and struct tags, and then generate the necessary boilerplate code for converting one struct type to another.

This tool formalizes and replaces the original prototype that was in this directory.

## Overview

In many Go applications, you need to convert data between different struct types, such as:
*   Converting from a database model to an API response model (DTO).
*   Transforming data from an external service's format to an internal application format.
*   Mapping between different versions of a data structure.

Manually writing these conversion functions is tedious, error-prone, and repetitive. This tool automates the process, providing powerful features for customization and error handling.

## Key Features

*   **Annotation-Driven**: Generation is triggered by a `@derivingconvert` annotation in the source struct's doc comment.
*   **Flexible Field Mapping**: Fields are matched automatically with a clear priority:
    1.  An explicit name in a `convert:"<name>"` tag.
    2.  A name in a `json:"<name>"` tag.
    3.  The normalized field name.
*   **Custom Conversion Logic**:
    *   Use the `convert:",using=<func>"` tag for field-specific custom conversion functions.
    *   Define global type-to-type conversion rules with `// convert:rule "<Src>" -> "<Dst>", using=<func>`.
*   **Validation**: Add validation rules for destination types using `// convert:rule "<Dst>", validator=<func>`.
*   **Rich Error Handling**: The generated code can collect multiple errors during a single conversion and return them as a single `error`.
*   **Recursive Generation**: Automatically handles nested structs, slices, maps, and pointers.
*   **CLI Tool**: A proper command-line interface for easy integration into build processes.

## Project Structure

*   `main.go`: The CLI entrypoint for the `convert` tool.
*   `parser/parser.go`: Contains the logic for parsing Go source files, reading annotations, and building an intermediate representation of the conversion requirements.
*   `generator/generator.go`: Takes the parsed information and generates the Go conversion functions.
*   `model/`: Defines the data structures (e.g., `ConversionPair`, `TypeRule`) used to pass information between the parser and generator.
*   `sampledata/`: Contains sample source and destination structs used for demonstrating and testing the tool.
*   `testdata/`: Contains golden files for testing the output of the generator.

## Annotation and Tag Reference

### `@derivingconvert`
Triggers the generation of a conversion function. Placed in the doc comment of the source struct.

**Syntax**: `@derivingconvert(<DestinationType>[, option=value, ...])`
*   `<DestinationType>`: The destination struct type.
*   `max_errors=<int>`: The maximum number of errors to collect. `0` means unlimited.

**Example**:
```go
// @derivingconvert(UserDTO, max_errors=10)
type User struct {
    // ...
}
```

### `// convert:rule`
Defines a global rule for type conversion or validation. Typically placed at the package level.

**Conversion Rule**: `// convert:rule "<SourceType>" -> "<DestinationType>", using=<FunctionName>`
```go
// convert:rule "time.Time" -> "string", using=convertTimeToString
```

**Validator Rule**: `// convert:rule "<DestinationType>", validator=<FunctionName>`
```go
// convert:rule "string", validator=validateStringNotEmpty
```

### `// convert:import`
Defines an alias for an external package path, which can then be used in `using` and `validator` rules. This is useful for centralizing conversion or validation logic in a shared package.

**Syntax**: `// convert:import <alias> <path>`
*   `<alias>`: The alias to use for the package (e.g., `myfuncs`).
*   `<path>`: The full import path of the package (e.g., `"example.com/project/utils/myfuncs"`).

**Example**:
```go
// convert:import funcs "example.com/project/converters"
// convert:rule "time.Time" -> "string", using=funcs.TimeToString
// convert:rule "string", validator=funcs.ValidateNonEmpty
```

### `// convert:variable`
Declares a local variable within the generated converter function. This is useful for stateful operations that need to be shared across multiple `using` functions, such as using a `strings.Builder` to construct a value from several source fields.

**Syntax**: `// convert:variable <name> <type>`
*   `<name>`: The name of the variable.
*   `<type>`: The type of the variable (e.g., `strings.Builder`, `*int`).

**Example**:
The variable is declared once per function and can be accessed by any `using` function that takes it as an argument.
```go
// // convert:variable builder strings.Builder
// @derivingconvert(Dst)
type Src struct {
	FirstName string
	LastName  string
}

type Dst struct {
	FullName string `convert:",using=buildFullName(&builder, src.FirstName, src.LastName)"`
}

// buildFullName would be a helper function you write.
// func buildFullName(builder *strings.Builder, firstName, lastName string) string { ... }
```


### `convert` Struct Tag
Controls the conversion of a specific field.

**Syntax**: `` `convert:"[destinationFieldName],[option=value],..."` ``
*   `[destinationFieldName]`: Maps to a different field name in the destination struct. Use `-` to skip the field.
*   `using=<funcName>`: Use a custom function for this field's conversion.
*   `required`: Reports an error if a source pointer field is `nil`.

**Example**:
```go
type User struct {
    ID        int64
    Email     string    `convert:"UserEmail"`
    Password  string    `convert:"-"`
    CreatedAt time.Time `convert:",using=convertTimeToString"`
    Manager   *User     `convert:",required"`
}
```

## How to Use

The `convert` tool is a command-line application.

1.  **Annotate your code**: Add `@derivingconvert` annotations to your source structs and any necessary `convert` tags or `// convert:rule` comments.

2.  **Run the tool**: Execute the tool from your terminal, specifying the target package and output file.

    ```bash
    go run github.com/podhmo/go-scan/examples/convert \
      -pkg "github.com/your/project/models" \
      -output "github.com/your/project/models/generated_converters.go"
    ```
    *(Note: Adjust paths for your project structure.)*

3.  **Use the generated code**: The tool will create a file (e.g., `generated_converters.go`) containing the conversion functions. You can then call these functions directly in your application code. For a source type `User` and destination `UserDTO`, the tool will generate:
    *   `func ConvertUserToUserDTO(ctx context.Context, src *User) (*UserDTO, error)`

## Role of `go-scan`

`go-scan` is essential for this tool. It allows the parser to:
*   Read and understand the structure of Go types (structs, fields, tags) **without compiling the code**.
*   Resolve type information across different packages, which is critical for handling complex models.
*   Access documentation comments to find the driving `@derivingconvert` annotations.
*   Manage imports dynamically in the generated code via its `ImportManager`.
