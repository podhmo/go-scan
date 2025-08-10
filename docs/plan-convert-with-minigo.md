# Final API Specification for IDE-Native `convert` Configuration

## 1. Introduction

This document provides the final API specification for a new, IDE-native method of configuring the `convert` tool. This approach ensures that the entire configuration is written as **statically valid Go code that passes the type checker**, providing a seamless IDE experience.

The core principle is to define **exceptions and custom logic** for the conversion process. By default, the generator will automatically map all fields with matching names. The API described here is used to override this default behavior.

The configuration is defined by calling methods on a **configurator object** within a function literal. This allows a `minigo`-based parser to analyze the Abstract Syntax Tree (AST) of these method calls to deduce the user's intent.

## 2. The `define` API Specification

The public API is housed in a new `define` package. Its purpose is to define conversion rules.

### 2.1. Default Behavior: Implicit Mapping

By default, the generator will automatically map any field in the source struct to a field in the destination struct if their names are identical (or become identical after normalization, e.g., `last_name` matches `LastName`). Users **do not** need to specify these mappings. The `define` API is for handling cases that fall outside this default behavior.

### 2.2. Top-Level Functions

#### `define.Convert()`
Defines a conversion between two struct types, specifying any custom mapping logic.

**Signature:**
```go
func Convert(src any, dst any, mapping Mapping)
```
-   `src`, `dst any`: Zero-value expressions of the source and destination structs.
-   `mapping Mapping`: A `Mapping` object that defines the exceptional mapping rules for this pair.

#### `define.Rule()`
Defines a global, reusable conversion rule for a specific type-to-type conversion.

**Signature:**
```go
func Rule(customFunc any)
```
-   `customFunc any`: A function identifier (e.g., `convutil.TimeToString`). The parser infers the source type, destination type, and import path from the function's signature.

### 2.3. The Configurator Pattern

#### `define.Mapping()`
Creates the mapping configuration for a `Convert` call.

**Signature:**
```go
func Mapping(mapFunc any) Mapping
```
-   `mapFunc any`: A function literal with the signature `func(c *Config, dst *DestType, src *SrcType)`.

#### `define.Config`
The `Config` object, `c`, provides methods to define field-level exceptions to the default mapping behavior.

**`c.Map(dstField, srcField any)`**
Defines a mapping between two fields with **different names**.

-   **Parser Interpretation:** The parser creates a mapping rule between the two fields. This is only necessary when the source and destination field names do not match.

**`c.Convert(dstField, srcField, converterFunc any)`**
Defines a mapping that requires a custom conversion function.

-   **Parser Interpretation:** The parser creates a rule to use the `converterFunc` for the specified field conversion. This is used when the default assignment or a global `Rule` is not sufficient.

**`c.Compute(dstField, expression any)`**
Defines a mapping for a destination field that is computed from an expression.

-   **Parser Interpretation:** The parser captures the `expression`'s AST and creates a `computed=` rule.

## 3. Final Example: Definitive Mapping File

This example demonstrates the final, refined API. Note how it only specifies the exceptions. `dst.Details`, `dst.CreatedAt`, etc., are not mentioned because they will be mapped automatically.

```go
//go:build codegen
// +build codegen

package main

import (
	"github.com/podhmo/go-scan/examples/convert/convutil"
	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/funcs"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"

	"github.com/podhmo/go-scan/tools/define" // Using the new 'define' package
)

func main() {
	// Define global rules for types that cannot be mapped automatically.
	define.Rule(convutil.TimeToString)
	define.Rule(convutil.PtrTimeToString)

	// Define the conversion from SrcUser to DstUser, only specifying the exceptions.
	define.Convert(source.SrcUser{}, destination.DstUser{},
		define.Mapping(func(c *define.Config, dst *destination.DstUser, src *source.SrcUser) {
			// Exception 1: Different names AND a custom function.
			c.Convert(dst.UserID, src.ID, funcs.UserIDToString)

			// Exception 2: A computed field.
			c.Compute(dst.FullName, funcs.MakeFullName(src.FirstName, src.LastName))

			// Exception 3: Different names.
			c.Map(dst.Contact, src.ContactInfo)
		}),
	)

	// Define conversion for a nested struct with name differences.
	define.Convert(source.SrcAddress{}, destination.DstAddress{},
		define.Mapping(func(c *define.Config, dst *destination.DstAddress, src *source.SrcAddress) {
			c.Map(dst.FullStreet, src.Street)
			c.Map(dst.CityName, src.City)
		}),
	)

	// Define conversion for another struct with name differences and a custom function.
	define.Convert(source.SrcInternalDetail{}, destination.DstInternalDetail{},
		define.Mapping(func(c *define.Config, dst *destination.DstInternalDetail, src *source.SrcInternalDetail) {
			c.Map(dst.ItemCode, src.Code)
			c.Convert(dst.LocalizedDesc, src.Description, funcs.Translate)
		}),
	)
}
```

## 4. Parser and Generator Interaction

-   **Parser's Role**: The parser translates the `define` API calls into the `model.ParsedInfo` IR. It understands that it is only receiving exceptions to the default behavior.
-   **Generator's Role**: The generator's logic is enhanced. Before generating the conversion for a struct pair, it first performs a pass to identify and automatically map all fields with matching names that have not been explicitly configured by the parser. It then processes the explicit rules from the IR, which take precedence. This two-phase approach (default mapping + explicit overrides) ensures correctness and implements the desired user experience.
