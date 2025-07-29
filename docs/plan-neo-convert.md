# Plan for `convert`: A Guide to Re-implementation and Migration

This document outlines the progress, key decisions, and future tasks for rebuilding the `examples/convert` tool. It is intended to be detailed enough for a developer to re-implement the tool and migrate from the old prototype.

## 1. High-Level Goal

The `convert` tool is a command-line application that parses Go source files for special `@derivingconvert` annotations in doc comments and generates type conversion functions. The goal is to create a robust, maintainable, and easy-to-use code generation tool that automates the tedious task of writing boilerplate conversion code, replacing the existing prototype in `examples/convert`.

---

## 2. Migration from `examples/convert` Prototype

The existing `examples/convert` directory contains a prototype that serves as a proof-of-concept. This new implementation will formalize and replace it.

### Key Differences from the Prototype

| Feature | `examples/convert` (Prototype) | `convert` (Current Implementation) |
| :--- | :--- | :--- |
| **Invocation** | `go run main.go` with hardcoded paths. | Standard CLI tool (`convert -pkg <pkg> -output <file>`). |
| **Rule Definition** | Hardcoded logic in `main.go`'s generator. | Annotation-driven (`@derivingconvert`, `// convert:rule`). |
| **Field Mapping** | Basic name matching, some hardcoded logic. | Struct tags (`convert:"..."`) and global rules for full control. |
| **Error Handling** | None in generated code. | Rich error handling with `model.ErrorCollector` to report all errors. |
| **Extensibility** | Requires changing the generator code. | Pluggable via custom functions (`using=...`). |
| **Recursion** | Manually written recursive calls. | Automatic recursive generation for nested structs, slices, and maps. |
| **Type Resolution** | Basic, within the same package. | Advanced, cross-package resolution powered by `go-scan`. |
| **Code Formatting** | Manual. | Automatic formatting using `goimports`. |

### Migration Plan

The transition from the prototype to the new tool will involve the following steps:

1.  **Develop the Core Tool**: Implement the annotation-based `convert` tool as a standalone CLI application according to this plan.
2.  **Replicate `examples/convert` Logic**:
    *   Add `@derivingconvert` annotations to the doc comments of `SrcUser` and `SrcOrder` in the `models` package to define the conversions to `DstUser` and `DstOrder`.
    *   Implement the custom logic from the prototype's `converter/converter.go` (e.g., `translateDescription`, combining `FirstName` and `LastName`) as helper functions.
    *   Use `// convert:rule` and `convert:` tags to map these helper functions to the correct fields and types (e.g., for `time.Time` -> `string`).
3.  **Generate New Converters**: Run the new `convert` tool on `examples/convert/models` to generate a new `generated_converters.go`.
4.  **Update Tests**: Modify `converter/converter_test.go` to use the newly generated top-level functions (e.g., `ConvertUserToDstUser`) instead of the old manual ones.
5.  **Remove Prototype Code**: Once the tests pass with the generated code, delete the prototype's `main.go` and the manual `converter/converter.go`. The `examples/convert` directory will then serve as a clean example of the new tool's usage.

---

## 3. Core Components

The tool is composed of three main architectural components:

*   **CLI Entrypoint (`examples/convert/main.go`)**: Handles command-line arguments (`-input`, `-output`), orchestrates the parsing and generation steps, and manages logging.
*   **Parser (`parser/parser.go`)**: Analyzes the source code to identify conversion rules and build a model of the types involved.
*   **Generator (`generator/generator.go`)**: Takes the model from the parser and generates the Go source code for the conversion functions.

---

## 4. Data Structures (The Intermediate Representation)

The parser and generator communicate via a set of data structures defined in the `model` package. This is the heart of the tool's architecture.

### `ParsedInfo`

This is the top-level container that holds all the information extracted from the source code.

```go
// ParsedInfo holds all parsed conversion rules and type information.
type ParsedInfo struct {
	PackageName     string
	PackagePath     string // Import path of the package being parsed
	ConversionPairs []ConversionPair
	GlobalRules     []TypeRule
	Structs         map[string]*StructInfo       // Keyed by struct name (e.g. "MyStruct")
	NamedTypes      map[string]*TypeInfo         // Keyed by type name (e.g. "MyInt" for type MyInt int)
	FileImports     map[string]map[string]string // filePath -> {alias -> importPath}
}
```

### `TypeInfo`

Represents a resolved Go type. This is a crucial and complex struct.

```go
// TypeInfo holds resolved information about a type.
type TypeInfo struct {
	Name        string // Simple name (e.g., "MyType", "int", "string")
	FullName    string // Fully qualified name (e.g., "example.com/pkg.MyType", "int")
	PackageName string // Package name where the type is defined or alias used (e.g., "pkg", "time")
	PackagePath string // Full package import path (e.g., "example.com/pkg", "time")
	Kind        TypeKind
	IsBasic     bool
	IsPointer   bool
	IsSlice     bool
	IsArray     bool
	IsMap       bool
	IsInterface bool
	IsFunc      bool
	Elem        *TypeInfo   // Element type for pointers, slices, arrays
	Key         *TypeInfo   // Key type for maps
	Value       *TypeInfo   // Value type for maps
	Underlying  *TypeInfo   // Underlying type for named types (e.g., int for type MyInt int)
	StructInfo  *StructInfo // If Kind is KindStruct or KindIdent resolving to a struct
	AstExpr     ast.Expr    // Original AST expression for the type
}

// TypeKind defines the kind of a type.
type TypeKind int
const (
	KindUnknown TypeKind = iota
	KindBasic
	KindIdent // Identifier, could be a struct, named type, etc.
	KindPointer
	KindSlice
	KindArray
	KindMap
	KindInterface
	KindStruct // Specifically a struct type definition
	KindNamed  // A named type (type MyInt int)
	KindFunc
)
```

### `StructInfo` and `FieldInfo`

Represent a struct and its fields.

```go
// StructInfo holds information about a parsed struct.
type StructInfo struct {
	Name            string
	Fields          []FieldInfo
	Type            *TypeInfo // TypeInfo for this struct
	IsAlias         bool      // True if this struct is a type alias to another struct
	UnderlyingAlias *TypeInfo // If IsAlias, this points to the TypeInfo of the actual struct
}

// FieldInfo holds information about a field within a struct.
type FieldInfo struct {
	Name         string
	OriginalName string
	TypeInfo     *TypeInfo
	Tag          ConvertTag
	ParentStruct *StructInfo
}
```

### `ConversionPair`, `TypeRule`, and `ConvertTag`

These structs directly map to the user-provided annotations.

```go
// ConversionPair defines a top-level conversion between two types.
// Corresponds to: @derivingconvert(<DstType>, [option=value, ...])
type ConversionPair struct {
	SrcTypeName string
	DstTypeName string
	SrcTypeInfo *TypeInfo
	DstTypeInfo *TypeInfo
	MaxErrors   int
}

// TypeRule defines a global rule for converting between types or validating a type.
// Corresponds to:
// // convert:rule "<SrcType>" -> "<DstType>", using=<funcName>
// // convert:rule "<DstType>", validator=<funcName>
type TypeRule struct {
	SrcTypeName   string
	DstTypeName   string
	SrcTypeInfo   *TypeInfo
	DstTypeInfo   *TypeInfo
	UsingFunc     string
	ValidatorFunc string
}

// ConvertTag holds parsed values from a `convert` struct tag.
// Corresponds to: `convert:"[dstFieldName],[option=value],..."`
type ConvertTag struct {
	DstFieldName string // Destination field name. "-" means skip. Empty means auto-map.
	UsingFunc    string // Custom function for this field.
	Required     bool   // If true and source pointer is nil, report error.
	RawValue     string
}
```

---

## 5. Annotation Syntax

The tool is driven by annotations in Go comments.

### Type-Level Annotations

This annotation is placed in the doc comment block of a source struct type.

#### `@derivingconvert`

This is the primary annotation that triggers the generation of a top-level conversion function from the source type to a destination type.

**Syntax**: `@derivingconvert(<DestinationType>[, option=value, ...])`

*   **`<DestinationType>`**: The destination struct type.
*   **Options**:
    *   `max_errors=<int>`: The maximum number of errors to collect before stopping the conversion. `0` means unlimited.

**Example**:
```go
// @derivingconvert(UserDTO, max_errors=10)
type User struct {
    // ... fields
}
```
This will generate a function `ConvertUserToUserDTO(ctx context.Context, src User) (UserDTO, error)`.

### Global Annotations

These are typically placed at the package level (e.g., in `doc.go` or `conversions.go`).

#### `// convert:rule`

Defines a global rule for type conversions or validations.

**Syntax 1 (Conversion Rule)**: `// convert:rule "<SourceType>" -> "<DestinationType>", using=<FunctionName>`

*   This rule applies to any field conversion between `<SourceType>` and `<DestinationType>`.
*   `<FunctionName>` must be a function with a compatible signature (e.g., `func(ec *errorCollector, src <SourceType>) <DestinationType>`).

**Example**:
```go
// convert:rule "time.Time" -> "string", using=convertTimeToString
```

**Syntax 2 (Validator Rule)**: `// convert:rule "<DestinationType>", validator=<FunctionName>`

*   This rule applies after a value of `<DestinationType>` is populated.
*   `<FunctionName>` must be a function with a compatible signature (e.g., `func(ec *errorCollector, val <DestinationType>)`).

**Example**:
```go
// convert:rule "string", validator=validateStringNotEmpty
```

### Field-Level Annotation (`convert` tag)

This annotation is placed in a struct field tag to control the conversion of that specific field.

**Syntax**: `` `convert:"[destinationFieldName],[option=value],..."` ``

*   **`[destinationFieldName]`**: The name of the field in the destination struct.
    *   If omitted, the source field name is used.
    *   If `-`, the field is skipped entirely.
*   **Options**:
    *   `using=<funcName>`: Use a custom function for this field's conversion. This has the highest priority.
    *   `required`: If the source field is a pointer and is `nil`, an error will be reported.

**Example**:
```go
type User struct {
    ID        int64
    Email     string    `convert:"UserEmail"`
    Password  string    `convert:"-"` // Skip this field
    CreatedAt time.Time `convert:",using=convertTimeToString"`
    Manager   *User     `convert:",required"`
}
```

---

## 6. Key Implementation Details & Rationale

*   **Parser Implementation (`go-scan`)**: The parser is heavily reliant on `github.com/podhmo/go-scan`. The tool uses `go-scan` not just for walking the AST, but critically for resolving type information across packages. The `scanner.FieldType` and `goscan.ImportManager` are core components that the generator depends on to understand type structures and manage imports in the generated code.
*   **Implicit Recursive Generation**: Instead of an explicit worklist, the generator leverages `go-scan`'s type resolution to achieve recursion. When generating the conversion for a field, it checks if the field's type is another struct that has a known conversion pair. If so, it generates a direct call to that pair's conversion function (e.g., `convertSrcSubStructToDstSubStruct(...)`). This approach simplifies the generator logic by relying on the completeness of the parsed model provided by the parser and `go-scan`.
*   **Error Handling (`model.ErrorCollector`)**: The generated code uses the `model.ErrorCollector` struct, which is included in the `model` package. This struct accumulates errors along with their field paths (e.g., `User.Address.Street`), providing rich debugging information instead of failing on the first error. The collector's path tracking (`Enter`/`Leave`) is generated for nested structs, slices, and maps.
*   **Rule Priority**: The generator must respect the rule priority:
    1.  Field-level `using` tag.
    2.  Global `convert:rule`.
    3.  Automatic conversion (direct assignment, pointer handling, recursive call for nested structs).

---

## 7. Generated Code Example

Given these source models and annotations:

```go
package simple

import "time"

// @derivingconvert(Output)
type Input struct {
    ID   int
    Name string
    Time time.Time `convert:",using=timeToString"`
}

type Output struct {
    ID   int
    Name string
    Time string
}

func timeToString(ec *errorCollector, t time.Time) string {
    return t.Format(time.RFC3339)
}
```

The generator should produce a file like `simple_gen.go` containing approximately this code:

```go
// Code generated by convert tool. DO NOT EDIT.
package simple

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ... (errorCollector struct definition) ...

// inputToOutput converts Input to Output.
func inputToOutput(ec *errorCollector, src Input) Output {
	dst := Output{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field ID
	ec.Enter("ID")
	dst.ID = src.ID
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	// Mapping field Name
	ec.Enter("Name")
	dst.Name = src.Name
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	// Mapping field Time
	ec.Enter("Time")
	// Applying field tag: using timeToString
	dst.Time = timeToString(ec, src.Time)
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}

func ConvertInputToOutput(ctx context.Context, src Input) (Output, error) {
	ec := newErrorCollector(0)
	dst := inputToOutput(ec, src)
	if ec.HasErrors() {
		return dst, errors.Join(ec.Errors()...)
	}
	return dst, nil
}
```

---

## 8. Future Tasks (TODO)

*   **Validator Rule Implementation**: Implement the logic to call validator functions after a destination struct is populated.
*   **Improve Import Management**: Handle import alias collisions robustly. The current implementation uses `goimports` which is a good first step, but more complex alias collision scenarios might require more advanced logic.
*   **Expand Test Coverage**: Create a comprehensive test suite that verifies all features and edge cases.
*   **Complete `README.md`**: Write user-facing documentation with installation, usage, and examples.
*   **Parse `max_errors` from Annotation**: Implement parsing for the `max_errors` option in the `@derivingconvert` annotation and pass it to the `ErrorCollector`.
*   **Handle Map Key Conversion**: Implement logic to convert map keys when the source and destination map key types are different.
