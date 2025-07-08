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
