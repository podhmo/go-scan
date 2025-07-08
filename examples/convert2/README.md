# Go Type Converter Generator (convert2)

`convert2` is a command-line tool that automatically generates Go code for converting data from one struct type to another. It uses Go package-level comment annotations and struct field tags to define conversion rules. This tool aims to reduce boilerplate and errors often associated with manual data transformations.

It relies solely on `go/ast` for parsing Go source files, avoiding the need for `go/types` and full type checking, which makes it potentially faster for code generation tasks where full compilation context isn't strictly necessary.

## Features

*   **Struct to Struct Conversion**: Generate functions to convert between different struct types.
*   **Annotation-Driven**: Define conversion rules declaratively using comments and tags.
    *   Package-level annotations for top-level conversion pairs and global type rules.
    *   Struct field tags for fine-grained control over field mapping.
*   **Recursive Conversion (Planned)**: Handles nested structs and slices.
*   **Pointer Handling (Planned)**: Manages conversions between pointer and non-pointer types, including `required` checks.
*   **Custom Logic (Planned)**: Allows specifying custom transformation functions (`using`) and validators (`validator`).
*   **Error Collection**: Generated code includes an error collector that reports multiple conversion errors at once, with path tracking for easy debugging.

## Installation

*(Details to be added. Typically involves `go install` or cloning the repository.)*

```bash
# Placeholder for installation command
# go install example.com/convert2@latest
```

## Usage

The generator is run from the command line, pointing it to the directory containing your Go source files with model definitions and conversion annotations.

```bash
convert2 -input ./path/to/your/models [-output ./path/to/generated/code]
```

*   `-input <directory>`: (Required) The directory containing Go source files with struct definitions and `convert:` annotations.
*   `-output <directory>`: (Optional) The directory where generated `*_gen.go` files will be written. Defaults to the input directory.

### Annotations

Conversion rules are defined using special comments in your Go source files.

#### 1. Package-Level Annotations

Place these comments directly before the `package` clause in any Go file within the target package (conventionally `doc.go` or a dedicated `conversions.go`).

*   **Define a Top-Level Conversion Pair:**
    `// convert:pair <SrcType> -> <DstType>[, option=value, ...]`
    This generates an exported function `Convert<SrcType>(ctx context.Context, src SrcType) (DstType, error)`.
    *   `max_errors=<int>`: (Optional) Max errors to collect for this pair (0 for unlimited).
    *   Example: `// convert:pair SrcUser -> DstUser, max_errors=10`

*   **Define a Global Type Conversion Rule:**
    `// convert:rule "<SrcType>" -> "<DstType>", using=<funcName>`
    Specifies a custom function to use whenever converting from `<SrcType>` to `<DstType>` globally within the generated code for this package. Type names must be quoted.
    *   Example: `// convert:rule "string" -> "time.Time", using=customParseTime`
    *   The `funcName` must adhere to specific signatures (see "Custom Functions").

*   **Define a Global Type Validator:**
    `// convert:rule "<DstType>", validator=<funcName>`
    Specifies a function to validate values of `<DstType>` after they have been converted.
    *   Example: `// convert:rule "openapi.StatusEnum", validator=ValidateStatusEnum`
    *   The `funcName` must adhere to specific signatures (see "Custom Functions").

#### 2. Struct Field Tags

Place these tags on fields within your **source** struct definitions.

*   **Field Mapping and Options:**
    `` `convert:"[dstFieldName],[option=value],..."` ``
    *   `dstFieldName`:
        *   Name of the corresponding field in the destination struct.
        *   If omitted, the generator attempts to auto-map by normalized field name.
        *   If `-`, the source field is excluded from conversion.
    *   Options:
        *   `using=<funcName>`: Use a specific custom function for this field's conversion. Overrides global rules.
        *   `required`: If the source field is a pointer and is `nil`, report an error. (Only applicable if source field is `*T` and destination is `T` or also `*T`).
    *   Examples:
        *   `FieldA string \`convert:"FieldAInDst"\`` (Simple rename)
        *   `FieldB *int \`convert:",required"\`` (Auto-map, error if nil)
        *   `FieldC string \`convert:"-"\`` (Skip this field)
        *   `FieldD customtypes.SourceUUID \`convert:"UUID,using=convertSourceUUIDToString"\``

### Generated Code

*   The tool generates files with a `*_gen.go` suffix in the specified output directory.
*   These files contain:
    *   The `errorCollector` helper struct and its methods.
    *   Exported top-level conversion functions (e.g., `ConvertSrcUser`).
    *   Unexported helper functions for nested conversions (e.g., `srcAddressToDstAddress`).

### Custom Functions

User-defined functions specified via `using` or `validator` must follow these signatures:

*   **`using` (Field or Type Conversion):**
    *   `func(ec *errorCollector, val SrcFieldType) DstFieldType`
    *   `func(ctx context.Context, ec *errorCollector, val SrcFieldType) DstFieldType` (if context is needed)
*   **`validator` (Type Validation):**
    *   `func(ec *errorCollector, val DstFieldType)`

## Development

*(Details about building from source, running tests, etc.)*

See `todo.md` for a list of planned features and ongoing development tasks.

---

*This README provides a basic outline. More details and examples will be added as development progresses.*
