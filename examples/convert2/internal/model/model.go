package model

import (
	"fmt"
	"go/ast"
)

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

// ConversionPair defines a top-level conversion between two types.
// Corresponds to: // convert:pair <SrcType> -> <DstType>[, option=value, ...]
type ConversionPair struct {
	SrcTypeName string    // Raw source type string from annotation
	DstTypeName string    // Raw dest type string from annotation
	SrcTypeInfo *TypeInfo // Resolved source type
	DstTypeInfo *TypeInfo // Resolved dest type
	MaxErrors   int       // Default: 0 (unlimited)
}

// TypeRule defines a global rule for converting between types or validating a type.
// Corresponds to:
// // convert:rule "<SrcType>" -> "<DstType>", using=<funcName>
// // convert:rule "<DstType>", validator=<funcName>
type TypeRule struct {
	SrcTypeName   string    // Raw source type string from annotation
	DstTypeName   string    // Raw dest type string from annotation
	SrcTypeInfo   *TypeInfo // Resolved source type (optional)
	DstTypeInfo   *TypeInfo // Resolved dest type
	UsingFunc     string    // Name of the custom conversion function
	ValidatorFunc string    // Name of the custom validation function
}

// StructInfo holds information about a parsed struct.
type StructInfo struct {
	Name            string
	Fields          []FieldInfo
	Node            *ast.StructType // AST node for the struct
	Type            *TypeInfo       // TypeInfo for this struct
	IsAlias         bool            // True if this struct is a type alias to another struct (e.g. type MyStruct OtherStruct)
	UnderlyingAlias *TypeInfo       // If IsAlias, this points to the TypeInfo of the actual struct
}

// FieldInfo holds information about a field within a struct.
// Corresponds to: `convert:"[dstFieldName],[option=value],..."`
type FieldInfo struct {
	Name         string
	OriginalName string      // Original field name in the source struct
	TypeInfo     *TypeInfo   // Resolved type information for the field
	Tag          ConvertTag  // Parsed `convert` tag
	ParentStruct *StructInfo // Reference to the parent struct
	AstField     *ast.Field  // Original AST field node
}

// ConvertTag holds parsed values from a `convert` struct tag.
type ConvertTag struct {
	DstFieldName string // Destination field name. "-" means skip. Empty means auto-map.
	UsingFunc    string // Custom function for this field.
	Required     bool   // If true and source pointer is nil, report error.
	RawValue     string // The raw string value of the tag
}

func NewParsedInfo(packageName, packagePath string) *ParsedInfo {
	return &ParsedInfo{
		PackageName:     packageName,
		PackagePath:     packagePath,
		ConversionPairs: []ConversionPair{},
		GlobalRules:     []TypeRule{},
		Structs:         make(map[string]*StructInfo),
		NamedTypes:      make(map[string]*TypeInfo),
		FileImports:     make(map[string]map[string]string),
	}
}

// Helper to convert ast.Expr to a string representation (for debugging or simple cases)
// This is a simplified version. For full accuracy, go/printer might be needed.
func AstExprToString(expr ast.Expr, currentPkgName string) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return AstExprToString(e.X, currentPkgName) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + AstExprToString(e.X, currentPkgName)
	case *ast.ArrayType:
		lenStr := ""
		if e.Len != nil {
			lenStr = AstExprToString(e.Len, currentPkgName)
		}
		return "[" + lenStr + "]" + AstExprToString(e.Elt, currentPkgName)
	case *ast.MapType:
		return "map[" + AstExprToString(e.Key, currentPkgName) + "]" + AstExprToString(e.Value, currentPkgName)
	case *ast.InterfaceType:
		if e.Methods == nil || len(e.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}" // Simplified
	case *ast.FuncType:
		return "func(...)" // Simplified
	case *ast.BasicLit:
		return e.Value
	default:
		// Fallback for other types, might not be perfect.
		// Using a fixed string or more detailed logging might be better.
		return fmt.Sprintf("%T", e) // Returns the type of the AST node itself
	}
}
