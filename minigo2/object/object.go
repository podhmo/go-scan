package object

import "fmt"

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types. More will be added later.
const (
	INTEGER_OBJ ObjectType = "INTEGER"
	BOOLEAN_OBJ ObjectType = "BOOLEAN"
	STRING_OBJ  ObjectType = "STRING"
	NULL_OBJ    ObjectType = "NULL"
)

// Object is the interface that all value types in our interpreter will implement.
type Object interface {
	// Type returns the type of the object.
	Type() ObjectType
	// Inspect returns a string representation of the object's value.
	Inspect() string
}

// --- Integer Object ---

// Integer represents an integer value.
type Integer struct {
	Value int64
}

// Type returns the type of the Integer object.
func (i *Integer) Type() ObjectType { return INTEGER_OBJ }

// Inspect returns a string representation of the Integer's value.
func (i *Integer) Inspect() string { return fmt.Sprintf("%d", i.Value) }

// --- String Object ---

// String represents a string value.
type String struct {
	Value string
}

// Type returns the type of the String object.
func (s *String) Type() ObjectType { return STRING_OBJ }

// Inspect returns a string representation of the String's value.
func (s *String) Inspect() string { return s.Value }

// --- Boolean Object ---

// Boolean represents a boolean value.
type Boolean struct {
	Value bool
}

// Type returns the type of the Boolean object.
func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }

// Inspect returns a string representation of the Boolean's value.
func (b *Boolean) Inspect() string { return fmt.Sprintf("%t", b.Value) }

// --- Null Object ---

// Null represents a null value.
type Null struct{}

// Type returns the type of the Null object.
func (n *Null) Type() ObjectType { return NULL_OBJ }

// Inspect returns a string representation of the Null's value.
func (n *Null) Inspect() string { return "null" }

// --- Global Instances ---

// Pre-create global instances for common values to save allocations.
var (
	TRUE  = &Boolean{Value: true}
	FALSE = &Boolean{Value: false}
	NULL  = &Null{}
)
