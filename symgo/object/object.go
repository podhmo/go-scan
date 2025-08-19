package object

import (
	"fmt"
	"go/ast"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types for the symbolic engine.
const (
	STRING_OBJ       ObjectType = "STRING"
	FUNCTION_OBJ     ObjectType = "FUNCTION"
	ERROR_OBJ        ObjectType = "ERROR"
	SYMBOLIC_OBJ     ObjectType = "SYMBOLIC_PLACEHOLDER"
	RETURN_VALUE_OBJ ObjectType = "RETURN_VALUE"
	PACKAGE_OBJ      ObjectType = "PACKAGE"
	INTRINSIC_OBJ    ObjectType = "INTRINSIC"
	SERVE_MUX_OBJ    ObjectType = "SERVE_MUX"
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
	Env        *Environment
	Decl       *ast.FuncDecl // The original declaration, for metadata like godoc.
}

// Type returns the type of the Function object.
func (f *Function) Type() ObjectType { return FUNCTION_OBJ }

// Inspect returns a string representation of the function.
func (f *Function) Inspect() string {
	return fmt.Sprintf("func %s() { ... }", f.Name.String())
}

// --- Intrinsic Object ---

// Intrinsic represents a built-in function that is implemented in Go.
type Intrinsic struct {
	// The Go function that implements the intrinsic's behavior.
	Fn func(args ...Object) Object
}

// Type returns the type of the Intrinsic object.
func (i *Intrinsic) Type() ObjectType { return INTRINSIC_OBJ }

// Inspect returns a string representation of the intrinsic function.
func (i *Intrinsic) Inspect() string { return "intrinsic function" }

// --- ServeMux Object ---

// ServeMux represents a net/http.ServeMux object.
type ServeMux struct{}

// Type returns the type of the ServeMux object.
func (s *ServeMux) Type() ObjectType { return SERVE_MUX_OBJ }

// Inspect returns a string representation of the ServeMux.
func (s *ServeMux) Inspect() string { return "ServeMux" }

// --- Package Object ---

// Package represents an imported Go package.
type Package struct {
	Name string
	Path string
	Env  *Environment // The environment containing all package-level declarations.
}

// Type returns the type of the Package object.
func (p *Package) Type() ObjectType { return PACKAGE_OBJ }

// Inspect returns a string representation of the package.
func (p *Package) Inspect() string {
	return fmt.Sprintf("package %s (%q)", p.Name, p.Path)
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

// --- Environment ---

// Environment holds the bindings for variables and functions.
type Environment struct {
	store map[string]Object
	outer *Environment
}

// NewEnvironment creates a new, top-level environment.
func NewEnvironment() *Environment {
	s := make(map[string]Object)
	return &Environment{store: s, outer: nil}
}

// NewEnclosedEnvironment creates a new environment that is enclosed by an outer one.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}

// Get retrieves an object by name from the environment, checking outer scopes if necessary.
func (e *Environment) Get(name string) (Object, bool) {
	obj, ok := e.store[name]
	if !ok && e.outer != nil {
		obj, ok = e.outer.Get(name)
	}
	return obj, ok
}

// Set stores an object by name in the current environment.
func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
	return val
}

// IsEmpty checks if the environment has any local bindings.
func (e *Environment) IsEmpty() bool {
	return len(e.store) == 0
}
