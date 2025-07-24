package parser

import "github.com/podhmo/go-scan/scanner"

// ParsedInfo holds all parsed conversion rules and type information.
type ParsedInfo struct {
	PackageName     string
	PackagePath     string // Import path of the package being parsed
	ConversionPairs []ConversionPair
	Structs         map[string]*StructInfo // Keyed by struct name
}

// ConversionPair defines a top-level conversion between two types.
// Corresponds to: @derivingconvert(<DstType>)
type ConversionPair struct {
	SrcTypeName      string
	DstTypeName      string
	SrcInfo          *scanner.TypeInfo
	DstInfo          *scanner.TypeInfo
	SrcPkgImportPath string
	DstPkgImportPath string
	MaxErrors        int
}

// StructInfo holds information about a parsed struct.
type StructInfo struct {
	Name   string
	Fields []FieldInfo
	Node   *scanner.TypeInfo
}

// FieldInfo holds information about a field within a struct.
type FieldInfo struct {
	Name string
	Tag  ConvertTag
	Node *scanner.FieldInfo
}

// ConvertTag holds parsed values from a `convert` struct tag.
type ConvertTag struct {
	DstFieldName string // Destination field name. "-" means skip. Empty means auto-map.
	UsingFunc    string // Custom function for this field.
	Required     bool   // If true and source pointer is nil, report error.
	RawValue     string
}
