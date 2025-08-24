package object

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/podhmo/go-scan/scanner"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types for the symbolic engine.
const (
	INTEGER_OBJ      ObjectType = "INTEGER"
	BOOLEAN_OBJ      ObjectType = "BOOLEAN"
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
	SLICE_OBJ        ObjectType = "SLICE"
	MULTI_RETURN_OBJ ObjectType = "MULTI_RETURN"
)

// Object is the interface that all value types in our symbolic engine will implement.
type Object interface {
	// Type returns the type of the object.
	Type() ObjectType
	// Inspect returns a string representation of the object's value.
	Inspect() string
	// TypeInfo returns the underlying go-scan type information, if available.
	TypeInfo() *scanner.TypeInfo
	// SetTypeInfo sets the underlying go-scan type information.
	SetTypeInfo(*scanner.TypeInfo)
	// FieldType returns the field type information for the object.
	FieldType() *scanner.FieldType
	// SetFieldType sets the field type information for the object.
	SetFieldType(*scanner.FieldType)
}

// BaseObject provides a default implementation for the TypeInfo and FieldType methods.
type BaseObject struct {
	ResolvedTypeInfo *scanner.TypeInfo
	ResolvedFieldType *scanner.FieldType
}

// TypeInfo returns the stored type information.
func (b *BaseObject) TypeInfo() *scanner.TypeInfo {
	return b.ResolvedTypeInfo
}

// SetTypeInfo sets the stored type information.
func (b *BaseObject) SetTypeInfo(ti *scanner.TypeInfo) {
	b.ResolvedTypeInfo = ti
}

// FieldType returns the stored field type information.
func (b *BaseObject) FieldType() *scanner.FieldType {
	return b.ResolvedFieldType
}

// SetFieldType sets the stored field type information.
func (b *BaseObject) SetFieldType(ft *scanner.FieldType) {
	b.ResolvedFieldType = ft
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

// --- Boolean Object ---

// Boolean represents a boolean value.
type Boolean struct {
	BaseObject
	Value bool
}

// Type returns the type of the Boolean object.
func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }

// Inspect returns a string representation of the Boolean's value.
func (b *Boolean) Inspect() string { return fmt.Sprintf("%t", b.Value) }

var (
	// TRUE is the singleton true value.
	TRUE = &Boolean{Value: true}
	// FALSE is the singleton false value.
	FALSE = &Boolean{Value: false}
)

// --- Function Object ---

// Function represents a user-defined function in the code being analyzed.
type Function struct {
	BaseObject
	Name       *ast.Ident
	Parameters *ast.FieldList
	Body       *ast.BlockStmt
	Env        *Environment
	Decl       *ast.FuncDecl // The original declaration, for metadata like godoc.
	Package    *scanner.PackageInfo
	Receiver   Object // The receiver for a method call ("self" or "this").
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
	TypeName   string            // e.g., "net/http.ServeMux"
	State      map[string]Object // for mock or intrinsic state
	Underlying Object            // To hold the object that this instance wraps (e.g., for interface implementations)
}

// Type returns the type of the Instance object.
func (i *Instance) Type() ObjectType { return INSTANCE_OBJ }

// Inspect returns a string representation of the Instance.
func (i *Instance) Inspect() string {
	if i.Underlying != nil {
		return fmt.Sprintf("instance<%s, underlying=%s>", i.TypeName, i.Underlying.Inspect())
	}
	return fmt.Sprintf("instance<%s>", i.TypeName)
}

// --- Package Object ---

// Package represents an imported Go package.
type Package struct {
	BaseObject
	Name string
	Path string
	Env  *Environment // The environment containing all package-level declarations.

	// ScannedInfo holds the detailed scan result for the package, loaded lazily.
	ScannedInfo *scanner.PackageInfo
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
	Pos     token.Pos
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
	// If the placeholder is for a function, this holds its signature.
	UnderlyingFunc *scanner.FunctionInfo
	// The package context for the UnderlyingFunc.
	Package *scanner.PackageInfo
	// If the placeholder is for an interface method call, this holds the receiver.
	Receiver Object
	// If the placeholder is for an interface method call, this holds the method info.
	UnderlyingMethod *scanner.MethodInfo
	// For interface method calls, this holds the set of possible concrete field types
	// that the receiver variable could hold.
	PossibleConcreteTypes []*scanner.FieldType
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
	// PossibleConcreteTypes tracks the set of concrete field types that have been
	// assigned to this variable. This is used for precise analysis of interface method calls.
	PossibleConcreteTypes map[*scanner.FieldType]struct{}
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

// --- Slice Object ---

// Slice represents a slice literal. Its type is represented by a FieldType,
// which captures the slice structure (e.g., []User).
type Slice struct {
	BaseObject
	SliceFieldType *scanner.FieldType
}

// Type returns the type of the Slice object.
func (s *Slice) Type() ObjectType { return SLICE_OBJ }

// Inspect returns a string representation of the slice type.
func (s *Slice) Inspect() string {
	if s.SliceFieldType != nil {
		return s.SliceFieldType.String()
	}
	return "[]<unknown>"
}

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

// GetLocal retrieves an object by name from the local environment only.
func (e *Environment) GetLocal(name string) (Object, bool) {
	obj, ok := e.store[name]
	return obj, ok
}

// GetWithScope retrieves an object by name, returning the object, the environment
// where it was found, and a boolean indicating success.
func (e *Environment) GetWithScope(name string) (Object, *Environment, bool) {
	obj, ok := e.store[name]
	if ok {
		return obj, e, true
	}
	if e.outer != nil {
		return e.outer.GetWithScope(name)
	}
	return nil, nil, false
}

// IsGlobal returns true if the environment is a top-level (package) scope.
func (e *Environment) IsGlobal() bool {
	return e.outer == nil
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

// Walk iterates over all items in the environment and its outer scopes.
// If the callback function returns false, the walk is stopped.
func (e *Environment) Walk(fn func(name string, obj Object) bool) {
	for name, obj := range e.store {
		if !fn(name, obj) {
			return
		}
	}
	if e.outer != nil {
		e.outer.Walk(fn)
	}
}

// WalkLocal iterates over all items in the local scope of the environment only.
// If the callback function returns false, the walk is stopped.
func (e *Environment) WalkLocal(fn func(name string, obj Object) bool) {
	for name, obj := range e.store {
		if !fn(name, obj) {
			return
		}
	}
}

// --- MultiReturn Object ---

// MultiReturn is a special object type to represent multiple return values from a function.
// This is not a first-class object that a user can interact with, but an internal
// mechanism for the evaluator.
type MultiReturn struct {
	BaseObject
	Values []Object
}

// Type returns the type of the MultiReturn object.
func (mr *MultiReturn) Type() ObjectType { return MULTI_RETURN_OBJ }

// Inspect returns a string representation of the multi-return values.
func (mr *MultiReturn) Inspect() string {
	return "multi-return"
}

// --- Tracer Interface ---

// Tracer is an interface for instrumenting the symbolic execution process.
// An implementation can be passed to the interpreter to track which AST nodes
// are being evaluated.
type Tracer interface {
	Visit(node ast.Node)
}

// TracerFunc is an adapter to allow the use of ordinary functions as Tracers.
type TracerFunc func(node ast.Node)

// Visit calls f(node).
func (f TracerFunc) Visit(node ast.Node) {
	f(node)
}
