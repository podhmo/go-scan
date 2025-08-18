package object

import (
	"fmt"
	"go/ast"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types for the symbolic engine.
const (
	STRING_OBJ      ObjectType = "STRING"
	FUNCTION_OBJ    ObjectType = "FUNCTION"
	ERROR_OBJ       ObjectType = "ERROR"
	SYMBOLIC_OBJ    ObjectType = "SYMBOLIC_PLACEHOLDER"
	RETURN_VALUE_OBJ ObjectType = "RETURN_VALUE"
)

// Object is the interface that all value types in our symbolic engine will implement.
type Object interface {
	// Type returns the type of the object.
	Type() ObjectType
	// Inspect returns a string representation of the object's value.
	Inspect() string
}

// --- String Object ---

// String represents a string value.
type String struct {
	Value string
}

// Type returns the type of the String object.
func (s *String) Type() ObjectType { return STRING_OBJ }

// Inspect returns a string representation of the String's value.
func (s *String) Inspect() string { return s.Value }

// --- Function Object ---

// Function represents a user-defined function in the code being analyzed.
type Function struct {
	Name       *ast.Ident
	Parameters *ast.FieldList
	Body       *ast.BlockStmt
	// TODO: Add Scope *scope.Scope when scope package is defined
}

// Type returns the type of the Function object.
func (f *Function) Type() ObjectType { return FUNCTION_OBJ }

// Inspect returns a string representation of the function.
func (f *Function) Inspect() string {
	return fmt.Sprintf("func %s() { ... }", f.Name.String())
}

// --- Error Object ---

// Error represents an error that occurred during symbolic evaluation.
type Error struct {
	Message string
}

// Type returns the type of the Error object.
func (e *Error) Type() ObjectType { return ERROR_OBJ }

// Inspect returns a string representation of the error.
func (e *Error) Inspect() string { return "Error: " + e.Message }

// --- SymbolicPlaceholder Object ---

// SymbolicPlaceholder represents a value that cannot be determined at analysis time.
// This is a key component of the symbolic execution engine.
type SymbolicPlaceholder struct {
	// Reason describes why this value is symbolic (e.g., "external function call", "complex expression").
	Reason string
}

// Type returns the type of the SymbolicPlaceholder object.
func (sp *SymbolicPlaceholder) Type() ObjectType { return SYMBOLIC_OBJ }

// Inspect returns a string representation of the symbolic placeholder.
func (sp *SymbolicPlaceholder) Inspect() string {
	return fmt.Sprintf("<Symbolic: %s>", sp.Reason)
}

// --- ReturnValue Object ---

// ReturnValue represents the value being returned from a function.
// It wraps another Object.
type ReturnValue struct {
	Value Object
}

// Type returns the type of the ReturnValue object.
func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }

// Inspect returns a string representation of the wrapped value.
func (rv *ReturnValue) Inspect() string { return rv.Value.Inspect() }
