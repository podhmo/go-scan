# Plan for `neo-convert`: A Guide to Re-implementation

This document outlines the progress, key decisions, and future tasks for rebuilding the `examples/convert2` tool. It is intended to be detailed enough for a developer to re-implement the tool without direct access to the original source code.

## 1. High-Level Goal

The `convert2` tool is a command-line application that parses Go source files for special `// convert:` annotations and generates type conversion functions. The goal is to create a robust, maintainable, and easy-to-use code generation tool that automates the tedious task of writing boilerplate conversion code.

---

## 2. Core Components

The tool is composed of three main architectural components:

*   **CLI Entrypoint (`main.go`)**: Handles command-line arguments (`-input`, `-output`), orchestrates the parsing and generation steps, and manages logging.
*   **Parser (`parser/parser.go`)**: Analyzes the source code to identify conversion rules and build a model of the types involved.
*   **Generator (`generator/generator.go`)**: Takes the model from the parser and generates the Go source code for the conversion functions.

---

## 3. Data Structures (The Intermediate Representation)

The parser and generator communicate via a set of data structures defined in the `internal/model` package. This is the heart of the tool's architecture.

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
// Corresponds to: // convert:pair <SrcType> -> <DstType>
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

## 4. Annotation Syntax

The tool is driven by annotations in Go comments.

### Top-Level Annotations

These are typically placed at the package level (e.g., in `doc.go` or `conversions.go`).

#### `// convert:pair`

This is the primary annotation that triggers the generation of a top-level conversion function.

**Syntax**: `// convert:pair <SourceType> -> <DestinationType>[, option=value, ...]`

*   **`<SourceType>`**: The source struct type.
*   **`<DestinationType>`**: The destination struct type.
*   **Options**:
    *   `max_errors=<int>`: The maximum number of errors to collect before stopping the conversion. `0` means unlimited.

**Example**:
```go
// convert:pair User -> UserDTO, max_errors=10
```
This will generate a function `ConvertUserToUserDTO(ctx context.Context, src User) (UserDTO, error)`.

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

## 5. Key Implementation Details & Rationale

*   **Parser Implementation (`go-scan`)**: The parser should be implemented using `github.com/podhmo/go-scan`. This library simplifies walking the AST and resolving type information, which is a significant challenge when using `go/parser` alone.
*   **Generator Worklist**: The generator should use a worklist pattern. It starts with the pairs from `// convert:pair` annotations. As it processes struct fields, if it encounters a nested struct-to-struct conversion that doesn't have a global `using` rule, it adds a new pair to the worklist. This ensures all necessary helper functions are generated recursively. A `map[string]bool` should be used to track already processed pairs to prevent infinite loops from circular dependencies.
*   **Error Handling (`errorCollector`)**: The generated code must include a helper struct called `errorCollector`. This struct should accumulate errors along with their field paths (e.g., `User.Address.Street`). This provides much richer debugging information than failing on the first error.
*   **Rule Priority**: The generator must respect the rule priority:
    1.  Field-level `using` tag.
    2.  Global `convert:rule`.
    3.  Automatic conversion (direct assignment, pointer handling, recursive call for nested structs).

---

## 6. Generated Code Example

Given these source models and annotations:

```go
package simple

import "time"

// convert:pair Input -> Output

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
// Code generated by convert2 tool. DO NOT EDIT.
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

## 7. Future Tasks (TODO)

*   **Implement Slice, Map, and Array Conversions**: The generator needs logic to loop through these collections and convert each element/value.
*   **Validator Rule Implementation**: Implement the logic to call validator functions after a destination struct is populated.
*   **Improve Import Management**: Handle import alias collisions robustly. Consider using `golang.org/x/tools/imports` for final output formatting.
*   **Expand Test Coverage**: Create a comprehensive test suite that verifies all features and edge cases.
*   **Complete `README.md`**: Write user-facing documentation with installation, usage, and examples.
