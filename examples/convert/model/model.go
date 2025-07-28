package model

import (
	"github.com/podhmo/go-scan/scanner"
)

// ParsedInfo holds all parsed conversion rules and type information.
type ParsedInfo struct {
	PackageName     string
	PackagePath     string // Import path of the package being parsed
	ConversionPairs []ConversionPair
	GlobalRules     []TypeRule
	Structs         map[string]*StructInfo       // Keyed by struct name (e.g. "MyStruct")
	NamedTypes      map[string]*scanner.TypeInfo // Keyed by type name (e.g. "MyInt" for type MyInt int)
}

// ConversionPair defines a top-level conversion between two types.
type ConversionPair struct {
	SrcTypeName string
	DstTypeName string
	SrcTypeInfo *scanner.TypeInfo
	DstTypeInfo *scanner.TypeInfo
	MaxErrors   int
}

// TypeRule defines a global rule for converting between types or validating a type.
type TypeRule struct {
	SrcTypeName   string
	DstTypeName   string
	SrcTypeInfo   *scanner.TypeInfo
	DstTypeInfo   *scanner.TypeInfo
	UsingFunc     string
	ValidatorFunc string
}

// StructInfo holds information about a parsed struct.
type StructInfo struct {
	Name            string
	Fields          []FieldInfo
	Type            *scanner.TypeInfo
	IsAlias         bool
	UnderlyingAlias *scanner.TypeInfo
}

// FieldInfo holds information about a field within a struct.
type FieldInfo struct {
	Name         string
	OriginalName string
	TypeInfo     *scanner.TypeInfo  // The resolved TypeInfo for the field's type
	FieldType    *scanner.FieldType // The detailed FieldType
	Tag          ConvertTag
	ParentStruct *StructInfo
}

// ConvertTag holds parsed values from a `convert` struct tag.
type ConvertTag struct {
	DstFieldName string
	UsingFunc    string
	Required     bool
	RawValue     string
}
