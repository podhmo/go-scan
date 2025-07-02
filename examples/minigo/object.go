package main

import (
	"fmt"
	"go/ast"
	"go/token" // Import the token package
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
	BUILTIN_FUNCTION_OBJ ObjectType = "BUILTIN_FUNCTION" // For built-in functions
	BREAK_OBJ        ObjectType = "BREAK"        // For break statements
	CONTINUE_OBJ     ObjectType = "CONTINUE"      // For continue statements
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
	NULL     = &Null{}               // Global instance for Null
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
	Name       string // Optional: for debugging and representation
	Parameters []*ast.Ident
	Body       *ast.BlockStmt
	Env        *Environment // Closure: the environment where the function was defined
	FileSet    *token.FileSet // FileSet for error reporting context
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
