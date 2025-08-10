# Final API Specification for IDE-Native `convert` Configuration

## 1. Introduction

This document provides the final API specification for a new, IDE-native method of configuring the `convert` tool. This approach ensures that the entire configuration is written as **statically valid Go code that passes the type checker**, providing a seamless IDE experience.

The core principle is to define conversion rules by calling methods on a **configurator object** within a function literal. This allows a `minigo`-based parser to analyze the Abstract Syntax Tree (AST) of these method calls to deduce the user's intent, without requiring the user to write any invalid Go code (such as assigning between incompatible types).

## 2. The `generate` API Specification

The public API is designed for clarity, type safety, and IDE-friendliness.

### 2.1. Top-Level Functions

#### `generate.Convert()`
Defines a top-level conversion between two struct types.

**Signature:**
```go
func Convert(src any, dst any, mapping Mapping)
```
-   `src`, `dst any`: Zero-value expressions of the source and destination structs (e.g., `source.User{}`).
-   `mapping Mapping`: A `Mapping` object that defines the conversion rules.

#### `generate.Rule()`
Defines a global, reusable conversion rule.

**Signature:**
```go
func Rule(customFunc any)
```
-   `customFunc any`: A function identifier (e.g., `convutil.TimeToString`).

**Parser Interpretation:**
-   The parser uses `go-scan` to resolve `customFunc` to its declaration.
-   It **infers** the source type, destination type, and package import path directly from the function's signature (e.g., `func(time.Time) string`).
-   This creates a global `model.TypeRule` in the `ParsedInfo` IR.

### 2.2. The Configurator Pattern

#### `generate.Mapping()`
Creates the mapping configuration for a `Convert` call.

**Signature:**
```go
func Mapping(mapFunc any) Mapping
```
-   `mapFunc any`: A function literal with the signature `func(c *Config, dst *DestType, src *SrcType)`.

#### `generate.Config`
The `Config` object, `c`, is the configurator passed to the `mapFunc`. It provides methods to define field mappings. These methods are stubs and do nothing at runtime; they are markers for the AST parser.

**`c.Assign(dstField, srcField any)`**
Defines a direct mapping between two fields with compatible types.

-   **Parser Interpretation:** The parser extracts the selectors for `dstField` and `srcField` and creates a direct mapping rule. This is for fields that can be assigned directly (e.g., `string` to `string`).

**`c.Convert(dstField, srcField, converterFunc any)`**
Defines a mapping where the source field must be converted using a custom function.

-   **Parser Interpretation:** The parser extracts the selectors and resolves the `converterFunc`. It validates that the function's signature is compatible with the types of the source and destination fields. This corresponds to the `using=` annotation.

**`c.Compute(dstField, expression any)`**
Defines a mapping for a destination field that is computed from an arbitrary expression, typically a function call involving multiple source fields.

-   **Parser Interpretation:** The parser captures the `expression`'s AST. It rewrites any selectors that reference the `src` parameter to use the generator's internal source variable. This corresponds to the `computed=` annotation.

## 3. Final Example: The Definitive Mapping File

This example demonstrates the final API. It is 100% valid Go code that passes type checking, is fully supported by IDEs, and covers all scenarios from the sample data.

```go
//go:build codegen
// +build codegen

package main

import (
	"github.com/podhmo/go-scan/examples/convert/convutil"
	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/funcs"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"

	"github.com/podhmo/go-scan/tools/generate"
)

func main() {
	// Define global rules. Types and imports are inferred from signatures.
	generate.Rule(convutil.TimeToString)
	generate.Rule(convutil.PtrTimeToString)

	// Define the main User conversion.
	generate.Convert(source.SrcUser{}, destination.DstUser{},
		generate.Mapping(func(c *generate.Config, dst *destination.DstUser, src *source.SrcUser) {
			// Use c.Convert for a field requiring a 'using='-style function.
			c.Convert(dst.UserID, src.ID, funcs.UserIDToString)

			// Use c.Compute for a computed field.
			c.Compute(dst.FullName, funcs.MakeFullName(src.FirstName, src.LastName))

			// Use c.Assign for fields with different types that have a registered Rule.
			// The generator will see the SrcAddress -> DstAddress rule and apply it.
			c.Assign(dst.Address, src.Address)

			// Use c.Convert for a struct field with a specific converter function.
			c.Convert(dst.Contact, src.ContactInfo, funcs.ConvertSrcContactToDstContact)

			// Fields with matching types and names are assigned automatically by the generator
			// if not specified here. For clarity, we can list them with c.Assign.
			c.Assign(dst.Details, src.Details)

			// The global Rule for time.Time -> string will be applied automatically.
			c.Assign(dst.CreatedAt, src.CreatedAt)
			c.Assign(dst.UpdatedAt, src.UpdatedAt)
		}),
	)

	// Define conversion for the nested address struct.
	generate.Convert(source.SrcAddress{}, destination.DstAddress{},
		generate.Mapping(func(c *generate.Config, dst *destination.DstAddress, src *source.SrcAddress) {
			c.Assign(dst.FullStreet, src.Street)
			c.Assign(dst.CityName, src.City)
		}),
	)

	// And all other required conversions...
	generate.Convert(source.SrcInternalDetail{}, destination.DstInternalDetail{},
		generate.Mapping(func(c *generate.Config, dst *destination.DstInternalDetail, src *source.SrcInternalDetail) {
			c.Assign(dst.ItemCode, src.Code)
			c.Convert(dst.LocalizedDesc, src.Description, funcs.Translate)
		}),
	)
}
```

## 4. Parser and Generator Interaction

-   **Parser's Role**: The `minigo`-based parser's only job is to translate the API calls in the mapping file into the `model.ParsedInfo` IR. It understands the `generate.Config` methods (`Assign`, `Convert`, `Compute`) and translates their AST arguments into the appropriate `FieldMap`, `UsingFunc`, and `ComputedField` rules in the IR.
-   **Generator's Role**: The generator remains **unchanged**. It receives the `ParsedInfo` IR and is unaware of how it was created. It recursively generates conversion functions, applies global `TypeRule`s, and handles nested structs, slices, and maps as it always has. This separation of concerns is critical for stability and correctness.
