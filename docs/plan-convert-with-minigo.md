# Final API Specification for IDE-Native `convert` Configuration

## 1. Introduction

This document provides the final API specification for a new, IDE-native method of configuring the `convert` tool. This approach supersedes previous designs by eliminating all string-based and non-type-safe elements, resulting in a configuration experience that is fully supported by IDEs and the Go compiler.

The core principle is to define conversion rules within a function literal, allowing a `minigo`-based parser to analyze the Abstract Syntax Tree (AST) of that function to deduce the user's intent.

## 2. The `generate` API Specification

The public API is reduced to two primary functions for defining conversions, `generate.Convert` and `generate.Rule`, and one for defining the mappings themselves, `generate.Mapping`.

### 2.1. `generate.Convert()`
Defines a top-level conversion between two struct types.

**Signature:**
```go
func Convert(src any, dst any, mapping Mapping)
```
-   **`src`, `dst any`**: Zero-value expressions of the source and destination structs (e.g., `source.User{}`, `destination.User{}`).
-   **`mapping Mapping`**: A single `Mapping` object that defines the conversion rules for this pair.

**Parser Interpretation:**
-   A call to `Convert` creates a `model.ConversionPair` in the `ParsedInfo` IR. The parser processes the `mapping` argument to populate the conversion rules for this pair.

### 2.2. `generate.Mapping()`
Defines the field-level mappings for a `Convert` call.

**Signature:**
```go
func Mapping(mapFunc any) Mapping
```
-   **`mapFunc any`**: A function literal with the signature `func(dst *DestType, src *SrcType)`.

**Parser Interpretation:**
This is the core of the new parser's logic.
1.  The parser validates that `mapFunc` is an `ast.FuncLit`.
2.  It inspects the function body (`BlockStmt`) for a series of `ast.AssignStmt` nodes.
3.  For each assignment `Lhs = Rhs`:
    -   `Lhs` must be a `SelectorExpr` (e.g., `dst.FieldA`). The parser extracts "FieldA" as the destination field name. The `dst` identifier must match the first parameter name of the function literal.
    -   `Rhs` is analyzed to determine the source:
        -   If `Rhs` is a `SelectorExpr` on the source parameter (e.g., `src.FieldB`), a direct mapping from `FieldB` to `FieldA` is created.
        -   If `Rhs` is a `CallExpr` (e.g., `funcs.MakeFullName(src.FirstName, src.LastName)`), the parser creates a `ComputedField` rule. It captures the entire call expression, rewriting any references to the `src` parameter to use the generator's internal source variable.
        -   If a destination field is not present as a `Lhs` in any assignment, it is considered ignored. This replaces `generate.Ignore`.

### 2.3. `generate.Rule()`
Defines a global, reusable conversion rule between two types.

**Signature:**
```go
func Rule(customFunc any) Option
```
-   **`customFunc any`**: A function identifier (e.g., `convutil.TimeToString`).

**Parser Interpretation:**
1.  The parser uses `go-scan` to resolve the identifier to its `FuncDecl`.
2.  It analyzes the function's signature (e.g., `func(time.Time) string`) to automatically determine the source and destination types. This completely replaces the need for separate `srcType` and `dstType` arguments.
3.  The parser automatically discovers the import path of the function, replacing the need for `generate.Import`.
4.  A `model.TypeRule` is created in the `ParsedInfo` IR.

## 3. Final Example: A Fully Type-Safe Mapping File

This example demonstrates the final API's elegance and power, covering all scenarios from the sample data in a type-safe, IDE-native way.

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
	// Define global, reusable conversion rules.
	// The parser infers types and import paths from the function signatures.
	generate.Rule(convutil.TimeToString)
	generate.Rule(convutil.PtrTimeToString)
	generate.Rule(funcs.ConvertSrcContactToDstContact)
	generate.Rule(funcs.Translate)

	// Define the conversion from SrcUser to DstUser.
	generate.Convert(source.SrcUser{}, destination.DstUser{},
		generate.Mapping(func(dst *destination.DstUser, src *source.SrcUser) {
			// Direct mapping with a different name.
			dst.UserID = funcs.UserIDToString(src.ID) // using is implicit in the assignment

			// Computed field from multiple source fields.
			dst.FullName = funcs.MakeFullName(src.FirstName, src.LastName)

			// Recursive conversion for nested structs is inferred by the generator
			// because a Convert call exists for SrcAddress.
			dst.Address = src.Address

			// A custom function is used for the Contact field.
			// The `Rule` for ConvertSrcContactToDstContact will be applied.
			dst.Contact = src.ContactInfo

			// Slices and pointers are also handled automatically by the generator.
			dst.Details = src.Details
			dst.CreatedAt = src.CreatedAt
			dst.UpdatedAt = src.UpdatedAt

			// `Password` field is not assigned, so it is implicitly ignored.
		}),
	)

	// Define conversion for the nested address struct.
	generate.Convert(source.SrcAddress{}, destination.DstAddress{},
		generate.Mapping(func(dst *destination.DstAddress, src *source.SrcAddress) {
			dst.FullStreet = src.Street
			dst.CityName = src.City
		}),
	)

	// Define conversion for the struct used in slices.
	generate.Convert(source.SrcInternalDetail{}, destination.DstInternalDetail{},
		generate.Mapping(func(dst *destination.DstInternalDetail, src *source.SrcInternalDetail) {
			dst.ItemCode = src.Code
			// The `Rule` for funcs.Translate will be applied.
			dst.LocalizedDesc = src.Description
		}),
	)

	// Other conversions from the source file are defined similarly.
	generate.Convert(source.SrcOrder{}, destination.DstOrder{},
		generate.Mapping(func(dst *destination.DstOrder, src *source.SrcOrder) {
			dst.ID = src.OrderID
			dst.TotalAmount = src.Amount
			dst.LineItems = src.Items
		}),
	)
	generate.Convert(source.SrcItem{}, destination.DstItem{},
		generate.Mapping(func(dst *destination.DstItem, src *source.SrcItem) {
			dst.ProductCode = src.SKU
			dst.Count = src.Quantity
		}),
	)

	// Conversions for complex types require no special mapping logic,
	// as the generator will recursively handle them once the element types
	// (like SubSource -> SubTarget) are defined.
	generate.Convert(source.ComplexSource{}, destination.ComplexTarget{})
	generate.Convert(source.SubSource{}, destination.SubTarget{})
	generate.Convert(source.SourceWithMap{}, destination.TargetWithMap{})
}
```

## 4. Final Parser Architecture

The `minigo`-based parser's role becomes even more specialized and powerful with this API.

-   **AST-Centric Logic**: The parser's core task is to analyze the AST of the arguments passed to the `generate` functions. It no longer deals with simple literals like strings, but with complex `ast.FuncLit` and `ast.Ident` nodes.
-   **Type and Path Inference**: The parser relies heavily on `go-scan`'s resolution capabilities. When it encounters an identifier like `convutil.TimeToString`, it resolves it to its declaration to find its signature and package path, automatically populating the `ParsedInfo` IR with all necessary import and type information.
-   **Function Body Analysis**: The most complex part is the analysis of the `Mapping` function literal's body. The parser implements a specific sub-walker for `ast.AssignStmt` nodes within that body, transforming Go assignment syntax into the `model.FieldMap` and `model.ComputedField` structures required by the generator.
-   **Seamless Generator Handoff**: Despite the increased complexity of the parser, its output remains the same: a valid `model.ParsedInfo` object. This maintains the clean separation of concerns and guarantees that the existing, battle-tested code generator can be reused without modification.
