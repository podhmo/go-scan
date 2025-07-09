package main

import (
	"fmt"
	"go/ast"
	"go/token" // Import the token package
	"sort"
	"strings"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define all possible object types our interpreter will handle.
const (
	STRING_OBJ           ObjectType = "STRING"
	INTEGER_OBJ          ObjectType = "INTEGER"
	BOOLEAN_OBJ          ObjectType = "BOOLEAN"
	NULL_OBJ             ObjectType = "NULL"
	RETURN_VALUE_OBJ     ObjectType = "RETURN_VALUE"
	ERROR_OBJ            ObjectType = "ERROR"
	FUNCTION_OBJ         ObjectType = "FUNCTION"
	BUILTIN_FUNCTION_OBJ ObjectType = "BUILTIN_FUNCTION"
	BREAK_OBJ            ObjectType = "BREAK"
	CONTINUE_OBJ         ObjectType = "CONTINUE"
	STRUCT_DEF_OBJ       ObjectType = "STRUCT_DEF"
	STRUCT_INSTANCE_OBJ  ObjectType = "STRUCT_INSTANCE"
	ARRAY_OBJ            ObjectType = "ARRAY"
	SLICE_OBJ            ObjectType = "SLICE"
	MAP_OBJ              ObjectType = "MAP"
	DEFINED_TYPE_OBJ     ObjectType = "DEFINED_TYPE" // For type definitions like type MyInt int
)

// Object is the interface that all value types in our interpreter will implement.
type Object interface {
	Type() ObjectType // Returns the type of the object.
	Inspect() string  // Returns a string representation of the object's value.
}

// --- String Object ---

// String represents a string value.
type String struct {
	Value string
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }

// --- Integer Object ---
type Integer struct {
	Value int64
}

func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return fmt.Sprintf("%d", i.Value) }

// --- Boolean Object ---
type Boolean struct {
	Value bool
}

func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return fmt.Sprintf("%t", b.Value) }

var (
	TRUE     = &Boolean{Value: true}
	FALSE    = &Boolean{Value: false}
	NULL     = &Null{}
	BREAK    = &BreakStatement{}
	CONTINUE = &ContinueStatement{}
)

func nativeBoolToBooleanObject(input bool) *Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

// --- Null Object ---
type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

// --- Error Object ---
type Error struct {
	Message string
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string  { return "ERROR: " + e.Message }
func (e *Error) Error() string    { return e.Message }

// --- Comparability ---
type Comparable interface {
	Compare(other Object) (int, error)
}

func (s *String) Compare(other Object) (int, error) {
	otherStr, ok := other.(*String)
	if !ok {
		return 0, fmt.Errorf("type mismatch: cannot compare %s with %s", s.Type(), other.Type())
	}
	if s.Value < otherStr.Value {
		return -1, nil
	}
	if s.Value > otherStr.Value {
		return 1, nil
	}
	return 0, nil
}

// --- UserDefinedFunction Object ---
type UserDefinedFunction struct {
	Name           string
	Parameters     []*ast.Ident
	Body           *ast.BlockStmt
	Env            *Environment
	FileSet        *token.FileSet
	ParamTypeExprs []ast.Expr
	IsExternal     bool
	PackagePath    string
	PackageAlias   string
}

func (udf *UserDefinedFunction) Type() ObjectType { return FUNCTION_OBJ }
func (udf *UserDefinedFunction) Inspect() string {
	params := []string{}
	for _, p := range udf.Parameters {
		params = append(params, p.Name)
	}
	name := udf.Name
	if name == "" {
		name = "[anonymous]"
	}
	if udf.IsExternal {
		return fmt.Sprintf("func %s [external](%s) { ... }", name, strings.Join(params, ", "))
	}
	return fmt.Sprintf("func %s(%s) { ... }", name, strings.Join(params, ", "))
}

// --- ReturnValue Object ---
type ReturnValue struct {
	Value Object
}

func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }
func (rv *ReturnValue) Inspect() string {
	if rv.Value == nil {
		return "return <nil>"
	}
	return rv.Value.Inspect()
}

// --- BuiltinFunction Object ---
type BuiltinFunctionType func(env *Environment, args ...Object) (Object, error)

type BuiltinFunction struct {
	Fn   BuiltinFunctionType
	Name string
}

func (bf *BuiltinFunction) Type() ObjectType { return BUILTIN_FUNCTION_OBJ }
func (bf *BuiltinFunction) Inspect() string {
	if bf.Name != "" {
		return fmt.Sprintf("<builtin function %s>", bf.Name)
	}
	return "<builtin function>"
}

var _ Object = (*BuiltinFunction)(nil)

// --- Break Statement Object ---
type BreakStatement struct{}

func (bs *BreakStatement) Type() ObjectType { return BREAK_OBJ }
func (bs *BreakStatement) Inspect() string  { return "break" }

// --- Continue Statement Object ---
type ContinueStatement struct{}

func (cs *ContinueStatement) Type() ObjectType { return CONTINUE_OBJ }
func (cs *ContinueStatement) Inspect() string  { return "continue" }

// --- StructDefinition Object ---
type StructDefinition struct {
	Name         string
	Fields       map[string]string
	EmbeddedDefs []*StructDefinition
	FieldOrder   []string
	FileSet      *token.FileSet
	IsExternal   bool
	PackagePath  string
}

func (sd *StructDefinition) Type() ObjectType { return STRUCT_DEF_OBJ }
func (sd *StructDefinition) Inspect() string {
	var parts []string
	processedFields := make(map[string]bool)
	for _, fieldName := range sd.FieldOrder {
		if typeName, ok := sd.Fields[fieldName]; ok {
			parts = append(parts, fmt.Sprintf("%s %s", fieldName, typeName))
			processedFields[fieldName] = true
		} else {
			for _, embDef := range sd.EmbeddedDefs {
				if embDef.Name == fieldName {
					parts = append(parts, embDef.Name)
					break
				}
			}
		}
	}
	for name, typeName := range sd.Fields {
		if !processedFields[name] {
			parts = append(parts, fmt.Sprintf("%s %s", name, typeName))
		}
	}
	return fmt.Sprintf("struct %s { %s }", sd.Name, strings.Join(parts, "; "))
}

// --- Array Object ---
type Array struct {
	Elements []Object
}

func (a *Array) Type() ObjectType { return ARRAY_OBJ }
func (a *Array) Inspect() string {
	var out strings.Builder
	elements := []string{}
	for _, e := range a.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// --- Slice Object ---
type Slice struct {
	Elements []Object
}

func (s *Slice) Type() ObjectType { return SLICE_OBJ }
func (s *Slice) Inspect() string {
	var out strings.Builder
	elements := []string{}
	for _, e := range s.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[]")
	out.WriteString(strings.Join(elements, ", "))
	return out.String()
}

// --- Hashable Interface & HashKey ---
type Hashable interface {
	HashKey() (HashKey, error)
}

type HashKey struct {
	Type     ObjectType
	Value    uint64
	StrValue string
}

func (i *Integer) HashKey() (HashKey, error) {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}, nil
}

func (s *String) HashKey() (HashKey, error) {
	return HashKey{Type: s.Type(), StrValue: s.Value}, nil
}

func (b *Boolean) HashKey() (HashKey, error) {
	var value uint64
	if b.Value {
		value = 1
	}
	return HashKey{Type: b.Type(), Value: value}, nil
}

// --- Map Object ---
type MapPair struct {
	Key   Object
	Value Object
}

type Map struct {
	Pairs map[HashKey]MapPair
}

func (m *Map) Type() ObjectType { return MAP_OBJ }
func (m *Map) Inspect() string {
	var out strings.Builder
	pairs := []string{}
	for _, pair := range m.Pairs {
		pairs = append(pairs, fmt.Sprintf("%s: %s", pair.Key.Inspect(), pair.Value.Inspect()))
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

// --- StructInstance Object ---
type StructInstance struct {
	Definition     *StructDefinition
	FieldValues    map[string]Object
	EmbeddedValues map[string]*StructInstance
}

func (si *StructInstance) Type() ObjectType { return STRUCT_INSTANCE_OBJ }
func (si *StructInstance) Inspect() string {
	var directFieldParts []string
	for name, value := range si.FieldValues {
		directFieldParts = append(directFieldParts, fmt.Sprintf("%s: %s", name, value.Inspect()))
	}
	sort.Strings(directFieldParts)

	var embeddedParts []string
	if len(si.EmbeddedValues) > 0 {
		var embTypeNames []string
		for typeName := range si.EmbeddedValues {
			embTypeNames = append(embTypeNames, typeName)
		}
		sort.Strings(embTypeNames)
		for _, typeName := range embTypeNames {
			embInstance := si.EmbeddedValues[typeName]
			embeddedParts = append(embeddedParts, fmt.Sprintf("%s: %s", typeName, embInstance.Inspect()))
		}
	}
	finalParts := append(directFieldParts, embeddedParts...)
	return fmt.Sprintf("%s { %s }", si.Definition.Name, strings.Join(finalParts, ", "))
}

// --- DefinedType Object ---
// DefinedType represents a type definition like `type MyInt int`.
// It stores the new type's name and information about its underlying type.
type DefinedType struct {
	Name string // Name of the defined type (e.g., "MyInt")
	// UnderlyingType represents the actual type category (e.g., INTEGER_OBJ, STRING_OBJ, or STRUCT_DEF_OBJ if it's based on a struct)
	UnderlyingTypeObj Object // This could be an Integer, String, Boolean, or even a StructDefinition or another DefinedType
	FileSet           *token.FileSet // FileSet for context, if needed for error reporting related to this type
	IsExternal        bool           // True if this definition came from an imported package
	PackagePath       string         // Import path of the package if IsExternal is true
}

func (dt *DefinedType) Type() ObjectType { return DEFINED_TYPE_OBJ }
func (dt *DefinedType) Inspect() string {
	// The inspection should clearly indicate it's a type definition.
	// For example: "type MyInt (underlying: INTEGER)"
	// Or if underlying is complex: "type MyPoint (underlying: struct { X int; Y int })"
	var underlyingInspect string
	if dt.UnderlyingTypeObj != nil {
		underlyingInspect = dt.UnderlyingTypeObj.Inspect()
		// If the underlying object is a type definition itself (e.g. StructDefinition), its Inspect() might be suitable.
		// If it's a primitive wrapper (Integer, String), we might want its type name.
		switch dt.UnderlyingTypeObj.Type() {
		case INTEGER_OBJ:
			underlyingInspect = "int" // Simplified, assuming base types are known
		case STRING_OBJ:
			underlyingInspect = "string"
		case BOOLEAN_OBJ:
			underlyingInspect = "bool"
		case STRUCT_DEF_OBJ:
			// sd.Inspect() already includes "struct Name { ... }"
			// For a type definition, we might just want the name of the underlying struct type or its full definition.
			// Using its existing Inspect() might be too verbose here.
			// Let's use the underlying type's name if it's a struct def.
			if sd, ok := dt.UnderlyingTypeObj.(*StructDefinition); ok {
				underlyingInspect = sd.Name // e.g. "Point"
			}
		// Add cases for other underlying types as needed
		default:
			underlyingInspect = string(dt.UnderlyingTypeObj.Type())
		}
	} else {
		underlyingInspect = "<unknown>"
	}

	return fmt.Sprintf("type %s (underlying: %s)", dt.Name, underlyingInspect)
}

// Helper to get the actual Minigo ObjectType of the underlying type.
// e.g. for `type MyInt int`, this would return INTEGER_OBJ.
// For `type MyPoint PointStruct`, this would return STRUCT_DEF_OBJ (or STRUCT_INSTANCE_OBJ when instantiated).
// This needs careful thought. For a *definition* `type MyInt int`, the underlying *kind* is INTEGER.
// The `UnderlyingTypeObj` field will store a prototypical object of that kind, or a definition object.
func (dt *DefinedType) GetUnderlyingObjectType() ObjectType {
	if dt.UnderlyingTypeObj != nil {
		// If UnderlyingTypeObj is a StructDefinition, its Type() is STRUCT_DEF_OBJ.
		// If UnderlyingTypeObj is an Integer, its Type() is INTEGER_OBJ.
		return dt.UnderlyingTypeObj.Type()
	}
	// This case should ideally not be reached if the DefinedType is correctly constructed.
	// However, if it represents a forward declaration or an unresolved type,
	// it might be different. For now, assume UnderlyingTypeObj is always populated.
	return NULL_OBJ // Should be an error or a specific "unresolved type"
}
