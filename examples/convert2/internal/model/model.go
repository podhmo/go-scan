package model

import "go/ast"

// ParsedInfo holds all parsed conversion rules and type information.
type ParsedInfo struct {
	PackageName     string
	ConversionPairs []ConversionPair
	GlobalRules     []TypeRule
	Structs         map[string]*StructInfo // Keyed by struct name
}

// ConversionPair defines a top-level conversion between two types.
// Corresponds to: // convert:pair <SrcType> -> <DstType>[, option=value, ...]
type ConversionPair struct {
	SrcType    string
	DstType    string
	MaxErrors  int    // Default: 0 (unlimited)
	SrcTypeExpr ast.Expr // AST expression for SrcType
	DstTypeExpr ast.Expr // AST expression for DstType
}

// TypeRule defines a global rule for converting between types or validating a type.
// Corresponds to:
// // convert:rule "<SrcType>" -> "<DstType>", using=<funcName>
// // convert:rule "<DstType>", validator=<funcName>
type TypeRule struct {
	SrcType       string   // Optional, empty if it's a validator rule for DstType
	DstType       string   // Target type for conversion or validation
	UsingFunc     string   // Name of the custom conversion function
	ValidatorFunc string   // Name of the custom validation function
	SrcTypeExpr   ast.Expr // AST expression for SrcType (if applicable)
	DstTypeExpr   ast.Expr // AST expression for DstType
}

// StructInfo holds information about a parsed struct.
type StructInfo struct {
	Name   string
	Fields []FieldInfo
	Node   *ast.StructType // AST node for the struct
}

// FieldInfo holds information about a field within a struct.
// Corresponds to: `convert:"[dstFieldName],[option=value],..."`
type FieldInfo struct {
	Name           string
	OriginalName   string        // Original field name in the source struct
	Type           ast.Expr      // AST expression for the field type
	Tag            ConvertTag    // Parsed `convert` tag
	ParentStruct   *StructInfo   // Reference to the parent struct
}

// ConvertTag holds parsed values from a `convert` struct tag.
type ConvertTag struct {
	DstFieldName string // Destination field name. "-" means skip. Empty means auto-map.
	UsingFunc    string // Custom function for this field.
	Required     bool   // If true and source pointer is nil, report error.
	RawValue     string // The raw string value of the tag
}

func NewParsedInfo(packageName string) *ParsedInfo {
	return &ParsedInfo{
		PackageName:     packageName,
		ConversionPairs: []ConversionPair{},
		GlobalRules:     []TypeRule{},
		Structs:         make(map[string]*StructInfo),
	}
}
