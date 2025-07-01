package main

import (
	"bytes"
	"fmt"
	"strings"
)

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
	FUNCTION_OBJ     ObjectType = "FUNCTION"     // For user-defined functions
	BUILTIN_OBJ      ObjectType = "BUILTIN"      // For built-in functions
	ARRAY_OBJ        ObjectType = "ARRAY"        // For array/slice objects
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

// Add performs string concatenation.
func (s *String) Add(other Object) (Object, error) {
	otherStr, ok := other.(*String)
	if !ok {
		return nil, fmt.Errorf("type mismatch: cannot concatenate %s with %s", s.Type(), other.Type())
	}
	return &String{Value: s.Value + otherStr.Value}, nil
}

// TODO:
// - Implement other object types: Integer, Boolean, Null, Array, Hash, Function, etc.
// - Implement operations for these types (e.g., arithmetic for Integers, logical for Booleans).
// - Consider how to handle type errors for operations (e.g., "hello" + 5).
// - Implement `ReturnValue` and `Error` wrapper objects for flow control and error handling.
// - Implement `Hashable` interface for objects that can be keys in a hash map.
// - Implement `Callable` interface for function objects.

// --- Builtin Function Object ---

// BuiltinFunction defines the signature of a built-in function.
// It takes a variable number of Object arguments and returns an Object or an error.
type BuiltinFunction func(args ...Object) (Object, error)

// Builtin wraps a BuiltinFunction and implements the Object interface.
type Builtin struct {
	Fn BuiltinFunction
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin function" } // How it's represented if printed

// --- Array Object ---
type Array struct {
	Elements []Object
}

func (ao *Array) Type() ObjectType { return ARRAY_OBJ }
func (ao *Array) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range ao.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}
