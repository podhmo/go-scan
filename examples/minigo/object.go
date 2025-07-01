package main

import "fmt"

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define all possible object types our interpreter will handle.
const (
	STRING_OBJ       ObjectType = "STRING"
	INTEGER_OBJ      ObjectType = "INTEGER"      // Example for future use
	BOOLEAN_OBJ      ObjectType = "BOOLEAN"      // Example for future use
	NULL_OBJ         ObjectType = "NULL"         // Example for future use
	RETURN_VALUE_OBJ ObjectType = "RETURN_VALUE" // Special type to wrap return values
	ERROR_OBJ        ObjectType = "ERROR"        // To wrap errors as objects
	// FUNCTION_OBJ     // For user-defined functions
	BUILTIN_FUNCTION_OBJ ObjectType = "BUILTIN_FUNCTION" // For built-in functions
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
	TRUE  = &Boolean{Value: true}
	FALSE = &Boolean{Value: false}
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
