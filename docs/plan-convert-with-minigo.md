# Plan: API Specification for `minigo`-based `convert` Configuration

## 1. Introduction

The `convert` tool is a powerful code generator that automates the creation of type-safe conversion functions based on annotations in Go source code. While this annotation-based system is flexible, it lacks IDE support, making complex mapping rules difficult to write and maintain.

This document provides a precise API specification for a new, complementary method of configuring the `convert` tool. This method leverages the `minigo` interpreter's underlying components (`go-scan`, AST walking) to parse a standard Go file as a declarative configuration script. This provides a superior, IDE-friendly developer experience.

## 2. Core Concept: The Declarative Mapping File

The configuration is defined in a standard Go file (e.g., `converter.go`), which is marked with a build tag (e.g., `//go:build codegen`) to be excluded from the final application build. Its sole purpose is to serve as a structured input for the `convert` tool.

Inside this file, the user calls functions from a new `generate` package. These functions are declarative stubs that are interpreted by the `minigo`-based parser.

## 3. The `generate` API Specification

This section defines the precise API for the `generate` package.

### 3.1. `generate.Convert()`

This is the top-level function that defines a conversion pair.

**Signature:**
```go
func Convert(src any, dst any, options ...Option)
```

-   **`src any`**: A zero-value expression of the source struct (e.g., `source.User{}`).
-   **`dst any`**: A zero-value expression of the destination struct (e.g., `destination.User{}`).
-   **`options ...Option`**: A variadic list of configuration options that define the mapping rules.

**Parser Interpretation:**
-   A call to `Convert` creates a `model.ConversionPair` in the `ParsedInfo` IR.
-   The `go-scan` engine resolves the types of `src` and `dst` to populate the `SrcTypeInfo` and `DstTypeInfo` fields of the pair.

---

### 3.2. `Option` Functions

These functions are passed as the variadic `options` to `Convert()`.

#### `generate.Map()`
Maps a source field to a destination field with a different name.

**Signature:**
```go
func Map(dstFieldName string, srcFieldName string) Option
```

**Parser Interpretation:**
-   Creates a mapping rule for a `model.FieldInfo`. The parser looks up `dstFieldName` in the destination struct's fields and `srcFieldName` in the source struct's fields. This directly corresponds to the `convert:"<name>"` tag functionality.

#### `generate.Ignore()`
Skips a field in the destination struct.

**Signature:**
```go
func Ignore(dstFieldName string) Option
```

**Parser Interpretation:**
-   Marks the specified `dstFieldName` to be skipped during code generation, equivalent to `convert:"-"`.

#### `generate.Use()`
Specifies a custom function to handle the conversion of a single field.

**Signature:**
```go
func Use(dstFieldName string, customFunc any) Option
```
-   **`customFunc any`**: A function identifier (e.g., `funcs.Translate`).

**Parser Interpretation:**
-   The parser resolves `customFunc` to its function declaration using `go-scan`.
-   It analyzes the function's signature to ensure compatibility.
-   It populates the `UsingFunc` field for the corresponding destination field, equivalent to the `using=<func>` tag. This is used for both simple type conversions (e.g., `int` -> `string`) and full struct conversions (e.g., `func(SrcContact) DstContact`).

#### `generate.Computed()`
Defines a destination field that is computed from a function call involving source fields.

**Signature:**
```go
func Computed(dstFieldName string, computerFuncCall any) Option
```
-   **`computerFuncCall any`**: A literal function call expression, where arguments reference the source struct type (e.g., `funcs.MakeFullName(source.User{}.FirstName, source.User{}.LastName)`).

**Parser Interpretation:**
-   This is a specialized parser rule. The parser analyzes the `computerFuncCall` expression (`ast.CallExpr`).
-   It identifies the function being called (`funcs.MakeFullName`).
-   It iterates through the function's arguments. For each argument that is a selector on the source type (e.g., `source.User{}.FirstName`), it rewrites the expression to use the generator's source variable (e.g., `src.FirstName`).
-   This populates a `model.ComputedField` rule in the IR, linking the destination field to the generated function call.

#### `generate.Rule()`
Defines a global type-to-type conversion rule.

**Signature:**
```go
func Rule(srcType any, dstType any, customFunc any) Option
```
-   **`srcType`, `dstType any`**: Zero-value expressions of the source and destination types (e.g., `time.Time{}`, `""`).

**Parser Interpretation:**
-   Creates a `model.TypeRule` in the `ParsedInfo` IR. It resolves the types of `srcType` and `dstType` and associates them with the resolved `customFunc`. This is equivalent to `// convert:rule "<Src>" -> "<Dst>", using=<func>`.

#### `generate.Import()`
Defines a package import with an alias, making its functions available to other rules.

**Signature:**
```go
func Import(alias string, path string) Option
```

**Parser Interpretation:**
-   Populates the `FileImports` map in the `ParsedInfo` IR. This allows the parser to correctly resolve functions referenced by their alias (e.g., `funcs.Translate`). This is equivalent to `// convert:import <alias> <path>`.

## 4. Revised Example Mapping File

This example demonstrates how the refined API can handle all conversion scenarios from `examples/convert/sampledata/source/source.go`.

```go
//go:build codegen
// +build codegen

package main

import (
	"time"

	"github.com/podhmo/go-scan/examples/convert/convutil"
	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/funcs"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"

	"github.com/podhmo/go-scan/tools/generate"
)

func main() {
	generate.Convert(source.SrcUser{}, destination.DstUser{},
		// Define necessary imports for custom functions
		generate.Import("convutil", "github.com/podhmo/go-scan/examples/convert/convutil"),
		generate.Import("funcs", "github.com/podhmo/go-scan/examples/convert/sampledata/funcs"),

		// Global conversion rules for time.Time and *time.Time
		generate.Rule(time.Time{}, "", convutil.TimeToString),
		generate.Rule(&time.Time{}, "", convutil.PtrTimeToString),

		// Field-specific mapping and custom function usage
		generate.Map("UserID", "ID"),
		generate.Use("UserID", funcs.UserIDToString), // Note: Chained with Map

		// Computed field from multiple source fields
		generate.Computed("FullName", funcs.MakeFullName(source.SrcUser{}.FirstName, source.SrcUser{}.LastName)),

		// Custom function for a nested struct field
		generate.Map("Contact", "ContactInfo"),
		generate.Use("Contact", funcs.ConvertSrcContactToDstContact),
	)

	// Conversion for a nested struct (will be called recursively)
	generate.Convert(source.SrcAddress{}, destination.DstAddress{},
		generate.Map("FullStreet", "Street"),
		generate.Map("CityName", "City"),
	)

	// Conversion for a struct in a slice (will be called recursively)
	generate.Convert(source.SrcInternalDetail{}, destination.DstInternalDetail{},
		generate.Map("ItemCode", "Code"),
		generate.Map("LocalizedDesc", "Description"),
		generate.Use("LocalizedDesc", funcs.Translate),
	)

	// Other conversions from the source file
	generate.Convert(source.SrcOrder{}, destination.DstOrder{},
		generate.Map("ID", "OrderID"),
		generate.Map("TotalAmount", "Amount"),
		generate.Map("LineItems", "Items"),
	)
	generate.Convert(source.SrcItem{}, destination.DstItem{},
		generate.Map("ProductCode", "SKU"),
		generate.Map("Count", "Quantity"),
	)

	// Conversions for complex types (pointers, slices, maps)
	// The generator handles recursion automatically once the element type conversions are defined.
	generate.Convert(source.ComplexSource{}, destination.ComplexTarget{})
	generate.Convert(source.SubSource{}, destination.SubTarget{})
	generate.Convert(source.SourceWithMap{}, destination.TargetWithMap{})
}
```

## 5. Revised Parser Architecture

The `minigo`-based parser is a purpose-built tool that translates the declarative Go script into the `model.ParsedInfo` IR. Its logic is tightly coupled to the `generate` API.

-   **Entry Point**: The tool is invoked with a path to the mapping file (e.g., `go run ./cmd/convert --script=converter.go`).
-   **AST-Walking**: The tool walks the AST of the mapping file, specifically identifying calls to `generate.Convert`.
-   **Stateful Parsing**: The parser maintains state within the context of a `generate.Convert` call. It processes the `Option` functions first (e.g., `Import`, `Rule`) to populate global rules, then processes field-level options (`Map`, `Use`, `Computed`) to configure the `ConversionPair`.
-   **IR Construction**: For each `generate` function call, a specific rule is executed to create or modify the corresponding object in the `ParsedInfo` struct. For example, a `generate.Map` call creates a `model.FieldMap` entry, while a `generate.Computed` call triggers the AST-rewrite logic described in section 3.2.
-   **Generator Handoff**: Once the entire mapping file is parsed, the complete `ParsedInfo` object is passed to the existing, unmodified `generator.Generate()` function. This ensures maximum reuse of the battle-tested code generation logic.
