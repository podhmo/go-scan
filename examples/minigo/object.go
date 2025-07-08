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
	INTEGER_OBJ          ObjectType = "INTEGER"          // Example for future use
	BOOLEAN_OBJ          ObjectType = "BOOLEAN"          // Example for future use
	NULL_OBJ             ObjectType = "NULL"             // Example for future use
	RETURN_VALUE_OBJ     ObjectType = "RETURN_VALUE"     // Special type to wrap return values
	ERROR_OBJ            ObjectType = "ERROR"            // To wrap errors as objects
	FUNCTION_OBJ         ObjectType = "FUNCTION"         // For user-defined functions
	BUILTIN_FUNCTION_OBJ ObjectType = "BUILTIN_FUNCTION" // For built-in functions
	BREAK_OBJ            ObjectType = "BREAK"            // For break statements
	CONTINUE_OBJ         ObjectType = "CONTINUE"         // For continue statements
	STRUCT_DEF_OBJ       ObjectType = "STRUCT_DEF"       // For struct definitions
	STRUCT_INSTANCE_OBJ  ObjectType = "STRUCT_INSTANCE"  // For struct instances
	ARRAY_OBJ            ObjectType = "ARRAY"
	SLICE_OBJ            ObjectType = "SLICE"
	MAP_OBJ              ObjectType = "MAP"
	ALIAS_DEF_OBJ        ObjectType = "ALIAS_DEF" // For type aliases like type MyInt int
	CONSTRAINED_STRING_DEF_OBJ ObjectType = "CONSTRAINED_STRING_DEF" // For "enum" type definitions
	CONSTRAINED_STRING_INSTANCE_OBJ ObjectType = "CONSTRAINED_STRING_INSTANCE" // For instances of "enum" types
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
func (s *String) Inspect() string  { return s.Value } // For simple strings, Inspect is just the value.

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

// Global instances for TRUE and FALSE to avoid re-creation and allow direct comparison.
var (
	TRUE     = &Boolean{Value: true}
	FALSE    = &Boolean{Value: false}
	NULL     = &Null{}              // Global instance for Null
	BREAK    = &BreakStatement{}    // Singleton instance for Break
	CONTINUE = &ContinueStatement{} // Singleton instance for Continue
)

// Helper function to convert native bool to interpreter's Boolean object
func nativeBoolToBooleanObject(input bool) *Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

// --- Null Object (Example for future) ---
/*
type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

var NULL = &Null{} // Global instance for Null
*/

// --- Null Object ---
type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

// --- Error Object ---
// Error wraps an error message.
type Error struct {
	Message string
	// TODO: Add position/stack trace if needed
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string  { return "ERROR: " + e.Message }
func (e *Error) Error() string    { return e.Message } // Implement error interface

// --- Comparability ---
// Some objects can be compared. This interface can be implemented by types
// that support comparison operations (e.g., ==, !=, <, >).

// Comparable is an interface for objects that can be compared with each other.
type Comparable interface {
	// Compare returns:
	// - A negative integer if the receiver is less than the argument.
	// - Zero if the receiver is equal to the argument.
	// - A positive integer if the receiver is greater than the argument.
	// It returns an error if the types are not comparable.
	Compare(other Object) (int, error)
}

// String comparison implementation.
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

// TODO:
// - Implement other object types: Integer, Boolean, Null, Array, Hash, Function, etc.
// - Implement operations for these types (e.g., arithmetic for Integers, logical for Booleans).
// - Consider how to handle type errors for operations (e.g., "hello" + 5).
// - Implement `ReturnValue` and `Error` wrapper objects for flow control and error handling.
// - Implement `Hashable` interface for objects that can be keys in a hash map.
// - Implement `Callable` interface for function objects.

// --- UserDefinedFunction Object ---
type UserDefinedFunction struct {
	Name           string // Simple name of the function
	Parameters     []*ast.Ident
	Body           *ast.BlockStmt
	Env            *Environment   // Closure: the environment where the function was defined
	FileSet        *token.FileSet // FileSet for error reporting context
	ParamTypeExprs []ast.Expr     // AST expressions for parameter types

	// Fields for external functions
	IsExternal   bool   // True if this definition came from an imported package
	PackagePath  string // Import path of the package if IsExternal is true
	PackageAlias string // The alias used for this function's package at the import site (e.g. "tp" in `import tp "some/path"`)
	// QualifiedName string // Optional: e.g., "pkg.FuncName" for inspection
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
	// For now, keep inspect simple. A more sophisticated inspect might need access to alias map for PackagePath.
	if udf.IsExternal {
		return fmt.Sprintf("func %s [external](%s) { ... }", name, strings.Join(params, ", "))
	}
	return fmt.Sprintf("func %s(%s) { ... }", name, strings.Join(params, ", "))
}

// --- ReturnValue Object ---
// ReturnValue is used to wrap return values to allow the interpreter to distinguish
// between a normal evaluation result and a value being explicitly returned.
type ReturnValue struct {
	Value Object
}

func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }
func (rv *ReturnValue) Inspect() string {
	if rv.Value == nil {
		return "return <nil>" // Should not happen if NULL_OBJ is used properly
	}
	return rv.Value.Inspect() // Or fmt.Sprintf("return %s", rv.Value.Inspect())
}

// --- BuiltinFunction Object ---

// BuiltinFunctionType defines the signature for Go functions that can be used as built-ins.
// It takes the current evaluation environment (in case the built-in needs to interact with it,
// e.g., to resolve other variables or call other MiniGo functions, though most won't need it)
// and a variable number of Object arguments, returning an Object or an error.
type BuiltinFunctionType func(env *Environment, args ...Object) (Object, error)

// BuiltinFunction represents a function that is implemented in Go and exposed to MiniGo.
type BuiltinFunction struct {
	Fn   BuiltinFunctionType
	Name string // Name of the built-in function, primarily for inspection/debugging.
}

func (bf *BuiltinFunction) Type() ObjectType { return BUILTIN_FUNCTION_OBJ }
func (bf *BuiltinFunction) Inspect() string {
	if bf.Name != "" {
		return fmt.Sprintf("<builtin function %s>", bf.Name)
	}
	return "<builtin function>"
}

// Ensure BuiltinFunction implements the Object interface.
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
// StructDefinition stores the definition of a struct.
type StructDefinition struct {
	Name         string
	Fields       map[string]string // Field name to type name (e.g., "int", "string", "pkg.OtherType")
	EmbeddedDefs []*StructDefinition
	FieldOrder   []string
	FileSet      *token.FileSet // FileSet for context, especially for external structs
	IsExternal   bool           // True if this definition came from an imported package
	PackagePath  string         // Import path of the package if IsExternal is true
}

func (sd *StructDefinition) Type() ObjectType { return STRUCT_DEF_OBJ }
func (sd *StructDefinition) Inspect() string {
	var parts []string
	// Using FieldOrder to maintain a semblance of original declaration order for inspection.
	// This is a simplified inspection; actual Go struct inspection is more complex.
	processedFields := make(map[string]bool)

	for _, fieldName := range sd.FieldOrder {
		if typeName, ok := sd.Fields[fieldName]; ok {
			parts = append(parts, fmt.Sprintf("%s %s", fieldName, typeName))
			processedFields[fieldName] = true
		} else {
			// This could be an embedded struct type name.
			// We need to find which embedded struct it corresponds to.
			// For inspection, just listing the type name of the embedded struct is enough.
			isEmbedded := false
			for _, embDef := range sd.EmbeddedDefs {
				if embDef.Name == fieldName { // fieldName here is actually the embedded type name from FieldOrder
					parts = append(parts, embDef.Name) // Just list the embedded type name
					isEmbedded = true
					break
				}
			}
			if !isEmbedded {
				// Fallback if not found in EmbeddedDefs by name (should not happen if FieldOrder is built correctly)
				// Or handle if FieldOrder stores something else for embedded fields.
				// For now, assume FieldOrder contains names of direct fields or names of embedded struct types.
			}
		}
	}

	// Add any fields that were in sd.Fields but somehow not in FieldOrder (should not happen with correct logic)
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
	// TODO: Potentially add capacity and a pointer to an underlying array for more Go-like slice semantics
}

func (s *Slice) Type() ObjectType { return SLICE_OBJ }
func (s *Slice) Inspect() string {
	var out strings.Builder
	elements := []string{}
	for _, e := range s.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[]") // Slices often visually distinct from arrays in inspect
	out.WriteString(strings.Join(elements, ", "))
	return out.String() // Note: Go's typical slice inspect shows pointer and len/cap, this is simplified
}

// --- Hashable Interface & HashKey ---

// Hashable is an interface for objects that can be used as map keys.
type Hashable interface {
	// HashKey returns a HashKey representation of the object.
	// This key is used for equality checks and as a map key.
	HashKey() (HashKey, error)
}

// HashKey uniquely identifies an object that can be a map key.
// It includes the type and a hash value (or a canonical representation for simple types).
type HashKey struct {
	Type     ObjectType
	Value    uint64 // For numeric types or actual hash values
	StrValue string // For string types, to avoid converting back and forth if not hashing
}

// Implement Hashable for existing suitable types (Integer, String, Boolean)

func (i *Integer) HashKey() (HashKey, error) {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}, nil
}

func (s *String) HashKey() (HashKey, error) {
	// For strings, we could use a proper hash function if performance becomes an issue
	// or if map keys could be very long. For simplicity, we can use the string itself
	// if map keys are directly `map[string]MapPair`. If `map[HashKey]MapPair`,
	// then we need a way to ensure HashKey struct itself is comparable for map keys.
	// Using StrValue directly in HashKey for strings.
	return HashKey{Type: s.Type(), StrValue: s.Value}, nil
}

func (b *Boolean) HashKey() (HashKey, error) {
	var value uint64
	if b.Value {
		value = 1
	} else {
		value = 0
	}
	return HashKey{Type: b.Type(), Value: value}, nil
}

// --- Map Object ---

// MapPair represents a key-value pair within a Map.
type MapPair struct {
	Key   Object
	Value Object
}

// Map represents a hash map object.
// The keys in Go's map must be comparable. For our interpreter, they must be Hashable.
// The `Pairs` map uses the `HashKey` struct as its key type. This means `HashKey` itself
// must be usable as a map key in Go (i.e., it's comparable). Structs are comparable if all their fields are.
// ObjectType (string) and uint64 are comparable.
type Map struct {
	Pairs map[HashKey]MapPair
}

func (m *Map) Type() ObjectType { return MAP_OBJ }
func (m *Map) Inspect() string {
	var out strings.Builder
	pairs := []string{}
	// Iterate over map pairs. Order is not guaranteed.
	// For consistent inspection, sort keys if possible, though HashKey sorting is non-trivial.
	// For now, accept unordered inspection.
	for _, pair := range m.Pairs {
		pairs = append(pairs, fmt.Sprintf("%s: %s", pair.Key.Inspect(), pair.Value.Inspect()))
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

// --- StructInstance Object ---
// StructInstance represents an instance of a struct.
type StructInstance struct {
	Definition     *StructDefinition
	FieldValues    map[string]Object          // Field name to its Object value (for direct fields)
	EmbeddedValues map[string]*StructInstance // Key: Embedded struct type name. Value: Instance of the embedded struct.
}

func (si *StructInstance) Type() ObjectType { return STRUCT_INSTANCE_OBJ }
func (si *StructInstance) Inspect() string {
	var parts []string
	for name, value := range si.FieldValues {
		parts = append(parts, fmt.Sprintf("%s: %s", name, value.Inspect()))
	}
	// Sort parts by field name for consistent output for direct fields.
	// Embedded values are more complex to inspect inline; could list their types or a summary.
	// For now, let's list direct fields and then embedded struct instances by type name.

	// Direct fields
	var directFieldParts []string
	for name, value := range si.FieldValues {
		directFieldParts = append(directFieldParts, fmt.Sprintf("%s: %s", name, value.Inspect()))
	}
	sort.Strings(directFieldParts) // Sort for consistent output

	// Embedded struct instances
	var embeddedParts []string
	if len(si.EmbeddedValues) > 0 {
		var embTypeNames []string
		for typeName := range si.EmbeddedValues {
			embTypeNames = append(embTypeNames, typeName)
		}
		sort.Strings(embTypeNames) // Sort embedded type names for consistent output

		for _, typeName := range embTypeNames {
			embInstance := si.EmbeddedValues[typeName]
			// Simple representation for embedded instance, could be more detailed
			embeddedParts = append(embeddedParts, fmt.Sprintf("%s: %s", typeName, embInstance.Inspect()))
		}
	}

	finalParts := append(directFieldParts, embeddedParts...)

	return fmt.Sprintf("%s { %s }", si.Definition.Name, strings.Join(finalParts, ", "))
}

// --- AliasDefinition Object ---
// AliasDefinition stores the definition of a type alias.
// e.g. "type MyInt int" or "type MyCustomString string"
type AliasDefinition struct {
	Name string // Name of the alias (e.g., "MyInt")
	// BaseTypeNode stores the AST expression for the base type.
	// This allows for deferred resolution or more complex type information later.
	// For "type MyInt int", BaseTypeNode would be an *ast.Ident{Name: "int"}.
	// For "type MyPoint Point", BaseTypeNode would be an *ast.Ident{Name: "Point"}.
	BaseTypeNode ast.Expr
	// BaseObjectType is the resolved ObjectType of the underlying type.
	// e.g., INTEGER_OBJ for "int", STRING_OBJ for "string".
	// If the alias is to another user-defined type (struct, another alias),
	// this would be the ObjectType of that definition (e.g. STRUCT_DEF_OBJ, ALIAS_DEF_OBJ).
	BaseObjectType ObjectType
	FileSet        *token.FileSet // FileSet for context
	IsExternal     bool           // True if this definition came from an imported package
	PackagePath    string         // Import path of the package if IsExternal is true
}

func (ad *AliasDefinition) Type() ObjectType { return ALIAS_DEF_OBJ }
func (ad *AliasDefinition) Inspect() string {
	// For inspection, we might want to show the base type.
	// This requires formatting the ast.Expr, which can be complex.
	// A simple representation:
	return fmt.Sprintf("type %s = %s (alias for %s)", ad.Name, astExprToString(ad.BaseTypeNode), ad.BaseObjectType)
}

// astExprToString converts an ast.Expr (typically representing a type) to a string.
// This is a simplified version for inspection purposes.
func astExprToString(expr ast.Expr) string {
	if expr == nil {
		return "<nil_type_expr>"
	}
	switch n := expr.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return astExprToString(n.X) + "." + n.Sel.Name
	case *ast.StarExpr:
		return "*" + astExprToString(n.X)
	case *ast.ArrayType:
		lenStr := ""
		if n.Len != nil {
			lenStr = astExprToString(n.Len)
		}
		return fmt.Sprintf("[%s]%s", lenStr, astExprToString(n.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", astExprToString(n.Key), astExprToString(n.Value))
	case *ast.InterfaceType:
		// TODO: More detailed interface inspection if needed
		return "interface{}"
	// Add other ast.Expr types as needed for type representation.
	default:
		return fmt.Sprintf("%T", n) // Fallback to Go type name
	}
}

// --- ConstrainedStringTypeDefinition Object ---
// Defines an "enum" like type based on strings.
// e.g. type Status string, with consts defining allowed values.
type ConstrainedStringTypeDefinition struct {
	Name          string
	AllowedValues map[string]struct{} // Set of allowed string values
	FileSet       *token.FileSet      // FileSet for context
	IsExternal    bool                // True if this definition came from an imported package
	PackagePath   string              // Import path of the package if IsExternal is true
}

func (csd *ConstrainedStringTypeDefinition) Type() ObjectType { return CONSTRAINED_STRING_DEF_OBJ }
func (csd *ConstrainedStringTypeDefinition) Inspect() string {
	var values []string
	for val := range csd.AllowedValues {
		values = append(values, fmt.Sprintf("%q", val))
	}
	sort.Strings(values) // For consistent inspection order
	return fmt.Sprintf("type %s string (enum: %s)", csd.Name, strings.Join(values, ", "))
}

// AddAllowedValue adds a string value to the set of allowed values for this definition.
func (csd *ConstrainedStringTypeDefinition) AddAllowedValue(value string) {
	if csd.AllowedValues == nil {
		csd.AllowedValues = make(map[string]struct{})
	}
	csd.AllowedValues[value] = struct{}{}
}

// IsAllowed checks if a given string value is part of the allowed set.
func (csd *ConstrainedStringTypeDefinition) IsAllowed(value string) bool {
	if csd.AllowedValues == nil {
		return false
	}
	_, ok := csd.AllowedValues[value]
	return ok
}

// --- ConstrainedStringInstance Object ---
// Represents an instance of a ConstrainedStringTypeDefinition.
type ConstrainedStringInstance struct {
	Definition *ConstrainedStringTypeDefinition
	Value      string
}

func (csi *ConstrainedStringInstance) Type() ObjectType { return CONSTRAINED_STRING_INSTANCE_OBJ }
func (csi *ConstrainedStringInstance) Inspect() string {
	return fmt.Sprintf("%s(%q)", csi.Definition.Name, csi.Value)
}

// HashKey implements the Hashable interface for ConstrainedStringInstance.
// This allows instances to be used as map keys.
func (csi *ConstrainedStringInstance) HashKey() (HashKey, error) {
	// Enums are distinct by their definition and value.
	// We can combine the hash of the definition's name and the value.
	// For simplicity, using string representation for now.
	// A more robust hash would involve definition pointer/ID.
	// The StrValue should be enough if HashKey collisions are rare.
	return HashKey{Type: csi.Type(), StrValue: csi.Definition.Name + ":" + csi.Value}, nil
}

// Compare implements the Comparable interface.
func (csi *ConstrainedStringInstance) Compare(other Object) (int, error) {
	otherCsi, ok := other.(*ConstrainedStringInstance)
	if !ok {
		return 0, fmt.Errorf("type mismatch: cannot compare %s with %s", csi.Type(), other.Type())
	}

	// Must be instances of the exact same enum definition
	if csi.Definition != otherCsi.Definition {
		return 0, fmt.Errorf("type mismatch: cannot compare instances of different enum types (%s vs %s)", csi.Definition.Name, otherCsi.Definition.Name)
	}

	if csi.Value < otherCsi.Value {
		return -1, nil
	}
	if csi.Value > otherCsi.Value {
		return 1, nil
	}
	return 0, nil
}
