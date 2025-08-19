package object

import (
	"fmt"
	"go/ast"

	goscan "github.com/podhmo/go-scan"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types for the symbolic engine.
const (
	INTEGER_OBJ      ObjectType = "INTEGER"
	STRING_OBJ       ObjectType = "STRING"
	FUNCTION_OBJ     ObjectType = "FUNCTION"
	ERROR_OBJ        ObjectType = "ERROR"
	SYMBOLIC_OBJ     ObjectType = "SYMBOLIC_PLACEHOLDER"
	RETURN_VALUE_OBJ ObjectType = "RETURN_VALUE"
	PACKAGE_OBJ      ObjectType = "PACKAGE"
	INTRINSIC_OBJ    ObjectType = "INTRINSIC"
	INSTANCE_OBJ     ObjectType = "INSTANCE"
	VARIABLE_OBJ     ObjectType = "VARIABLE"
	POINTER_OBJ      ObjectType = "POINTER"
	NIL_OBJ          ObjectType = "NIL"
)

// Object is the interface that all value types in our symbolic engine will implement.
type Object interface {
	// Type returns the type of the object.
	Type() ObjectType
	// Inspect returns a string representation of the object's value.
	Inspect() string
	// TypeInfo returns the underlying go-scan type information, if available.
	// This is the bridge between the symbolic world and the static type world.
	TypeInfo() *goscan.TypeInfo
}

// BaseObject provides a default implementation for the TypeInfo method.
type BaseObject struct {
	ResolvedTypeInfo *goscan.TypeInfo
}

// TypeInfo returns the stored type information.
func (b *BaseObject) TypeInfo() *goscan.TypeInfo {
	return b.ResolvedTypeInfo
}

// --- String Object ---

// String represents a string value.
type String struct {
	BaseObject
	Value string
}

// Type returns the type of the String object.
func (s *String) Type() ObjectType { return STRING_OBJ }

// Inspect returns a string representation of the String's value.
func (s *String) Inspect() string { return s.Value }

// --- Integer Object ---

// Integer represents an integer value.
type Integer struct {
	BaseObject
	Value int64
}

// Type returns the type of the Integer object.
func (i *Integer) Type() ObjectType { return INTEGER_OBJ }

// Inspect returns a string representation of the Integer's value.
func (i *Integer) Inspect() string { return fmt.Sprintf("%d", i.Value) }

// --- Function Object ---

// Function represents a user-defined function in the code being analyzed.
type Function struct {
	BaseObject
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
	BaseObject
	// The Go function that implements the intrinsic's behavior.
	Fn func(args ...Object) Object
}

// Type returns the type of the Intrinsic object.
func (i *Intrinsic) Type() ObjectType { return INTRINSIC_OBJ }

// Inspect returns a string representation of the intrinsic function.
func (i *Intrinsic) Inspect() string { return "intrinsic function" }

// --- Instance Object ---

// Instance represents a symbolic instance of a particular type.
// It's used to track objects returned by intrinsics (like constructors)
// so that method calls on them can be resolved.
type Instance struct {
	BaseObject
	TypeName string            // e.g., "net/http.ServeMux"
	State    map[string]Object // for mock or intrinsic state
}

// Type returns the type of the Instance object.
func (i *Instance) Type() ObjectType { return INSTANCE_OBJ }

// Inspect returns a string representation of the Instance.
func (i *Instance) Inspect() string { return fmt.Sprintf("instance<%s>", i.TypeName) }

// --- Package Object ---

// Package represents an imported Go package.
type Package struct {
	BaseObject
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
	BaseObject
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
	BaseObject
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
	BaseObject
	Value Object
}

// Type returns the type of the ReturnValue object.
func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }

// Inspect returns a string representation of the wrapped value.
func (rv *ReturnValue) Inspect() string { return rv.Value.Inspect() }

// --- Variable Object ---

// Variable represents a declared variable in the environment.
// It holds a value and its resolved type information.
type Variable struct {
	BaseObject
	Name  string
	Value Object
}

// Type returns the type of the Variable object.
func (v *Variable) Type() ObjectType { return VARIABLE_OBJ }

// Inspect returns a string representation of the variable's value.
func (v *Variable) Inspect() string {
	return v.Value.Inspect()
}

// --- Pointer Object ---

// Pointer represents a pointer to another object.
type Pointer struct {
	BaseObject
	Value Object
}

// Type returns the type of the Pointer object.
func (p *Pointer) Type() ObjectType { return POINTER_OBJ }

// Inspect returns a string representation of the pointer.
func (p *Pointer) Inspect() string {
	return fmt.Sprintf("&%s", p.Value.Inspect())
}

// --- Nil Object ---

// Nil represents the nil value.
type Nil struct {
	BaseObject
}

// Type returns the type of the Nil object.
func (n *Nil) Type() ObjectType { return NIL_OBJ }

// Inspect returns a string representation of nil.
func (n *Nil) Inspect() string { return "nil" }

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
