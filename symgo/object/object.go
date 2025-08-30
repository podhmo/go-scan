package object

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types for the symbolic engine.
const (
	INTEGER_OBJ      ObjectType = "INTEGER"
	FLOAT_OBJ        ObjectType = "FLOAT"
	COMPLEX_OBJ      ObjectType = "COMPLEX"
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
	MAP_OBJ          ObjectType = "MAP"
	MULTI_RETURN_OBJ ObjectType = "MULTI_RETURN"
	BREAK_OBJ        ObjectType = "BREAK"
	CONTINUE_OBJ     ObjectType = "CONTINUE"
	VARIADIC_OBJ     ObjectType = "VARIADIC"
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
	ResolvedTypeInfo  *scanner.TypeInfo
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
func (s *String) Inspect() string { return fmt.Sprintf("%q", s.Value) }

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

// --- Float Object ---

// Float represents a float value.
type Float struct {
	BaseObject
	Value float64
}

// Type returns the type of the Float object.
func (f *Float) Type() ObjectType { return FLOAT_OBJ }

// Inspect returns a string representation of the Float's value.
func (f *Float) Inspect() string { return fmt.Sprintf("%f", f.Value) }

// --- Complex Object ---

// Complex represents a complex number value.
type Complex struct {
	BaseObject
	Value complex128
}

// Type returns the type of the Complex object.
func (c *Complex) Type() ObjectType { return COMPLEX_OBJ }

// Inspect returns a string representation of the Complex's value.
func (c *Complex) Inspect() string { return fmt.Sprintf("%v", c.Value) }

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
	Def        *scanner.FunctionInfo
}

// Type returns the type of the Function object.
func (f *Function) Type() ObjectType { return FUNCTION_OBJ }

// Inspect returns a string representation of the function.
func (f *Function) Inspect() string {
	name := "<nil>"
	if f.Name != nil {
		name = f.Name.String()
	}
	return fmt.Sprintf("func %s() { ... }", name)
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

// CallFrame represents a single frame in the call stack.
type CallFrame struct {
	Pos      token.Pos
	Function string // Name of the function for stack traces
}

// Format formats the call frame into a readable string.
// fset is required to resolve the position to a file and line number.
func (cf *CallFrame) Format(fset *token.FileSet) string {
	position := fset.Position(cf.Pos)
	funcName := cf.Function
	if funcName == "" {
		funcName = "<script>"
	}

	sourceLine, err := getSourceLine(position.Filename, position.Line)
	formattedSourceLine := ""
	if err == nil && sourceLine != "" {
		formattedSourceLine = "\n\t\t" + sourceLine
	}

	return fmt.Sprintf("\t%s:%d:%d:\tin %s%s", position.Filename, position.Line, position.Column, funcName, formattedSourceLine)
}

// Error represents an error that occurred during symbolic evaluation.
type Error struct {
	BaseObject
	Message   string
	Pos       token.Pos
	CallStack []*CallFrame
	fset      *token.FileSet // FileSet to resolve positions
}

// Type returns the type of the Error object.
func (e *Error) Type() ObjectType { return ERROR_OBJ }

// Inspect returns a formatted string representation of the error, including the call stack.
func (e *Error) Inspect() string {
	return e.Error()
}

// Error returns a formatted string representation of the error, including the call stack,
// satisfying the Go `error` interface.
func (e *Error) Error() string {
	var out bytes.Buffer

	out.WriteString("symgo runtime error: ")
	out.WriteString(e.Message)

	// Print the source line of the error itself
	if e.fset != nil && e.Pos.IsValid() {
		position := e.fset.Position(e.Pos)
		sourceLine, err := getSourceLine(position.Filename, position.Line)
		out.WriteString(fmt.Sprintf("\n\t%s:%d:%d:", position.Filename, position.Line, position.Column))
		if err == nil && sourceLine != "" {
			out.WriteString("\n\t\t" + sourceLine)
		}
	}

	out.WriteString("\n")

	// Print the call stack in reverse order (most recent call first)
	if e.fset != nil {
		for i := len(e.CallStack) - 1; i >= 0; i-- {
			out.WriteString(e.CallStack[i].Format(e.fset))
			out.WriteString("\n")
		}
	}

	return out.String()
}

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
	Elements       []Object
	SliceFieldType *scanner.FieldType
}

// Type returns the type of the Slice object.
func (s *Slice) Type() ObjectType { return SLICE_OBJ }

// Inspect returns a string representation of the slice type.
func (s *Slice) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range s.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// --- Map Object ---

// Map represents a map literal. Its type is represented by a FieldType,
// which captures the map structure (e.g., map[string]int).
type Map struct {
	BaseObject
	MapFieldType *scanner.FieldType
}

// Type returns the type of the Map object.
func (m *Map) Type() ObjectType { return MAP_OBJ }

// Inspect returns a string representation of the map type.
func (m *Map) Inspect() string {
	if m.MapFieldType != nil {
		return m.MapFieldType.String()
	}
	return "map[<unknown>]<unknown>"
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

// Set stores an object by name in the environment, walking up to outer scopes
// to find where the variable is defined.
func (e *Environment) Set(name string, val Object) Object {
	if _, ok := e.store[name]; ok {
		e.store[name] = val
		return val
	}
	if e.outer != nil {
		return e.outer.Set(name, val)
	}
	// If not found anywhere, define it in the current (innermost) scope.
	e.store[name] = val
	return val
}

// SetLocal stores an object by name in the local (current) environment only.
// This is used for `:=` declarations.
func (e *Environment) SetLocal(name string, val Object) Object {
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

// --- Break Object ---

// Break represents a break statement.
type Break struct {
	BaseObject
}

// Type returns the type of the Break object.
func (b *Break) Type() ObjectType { return BREAK_OBJ }

// Inspect returns a string representation of the break statement.
func (b *Break) Inspect() string { return "break" }

// --- Continue Object ---

// Continue represents a continue statement.
type Continue struct {
	BaseObject
}

// Type returns the type of the Continue object.
func (c *Continue) Type() ObjectType { return CONTINUE_OBJ }

// Inspect returns a string representation of the continue statement.
func (c *Continue) Inspect() string { return "continue" }

// --- Variadic Object ---

// Variadic represents a variadic argument expansion (e.g., `slice...`).
type Variadic struct {
	BaseObject
	Value Object // The slice being expanded
}

// Type returns the type of the Variadic object.
func (v *Variadic) Type() ObjectType { return VARIADIC_OBJ }

// Inspect returns a string representation of the variadic expansion.
func (v *Variadic) Inspect() string {
	if v.Value != nil {
		return fmt.Sprintf("...%s", v.Value.Inspect())
	}
	return "..."
}

var (
	// BREAK is the singleton break value.
	BREAK = &Break{}
	// CONTINUE is the singleton continue value.
	CONTINUE = &Continue{}
)

// --- Tracer Interface ---

// Tracer is an interface for instrumenting the symbolic execution process.
// An implementation can be passed to the interpreter to track which AST nodes
// are being evaluated.
type Tracer interface {
	Visit(node ast.Node)
}

// ScanPolicyFunc is a function that determines whether a package should be scanned from source.
type ScanPolicyFunc func(importPath string) bool

// TracerFunc is an adapter to allow the use of ordinary functions as Tracers.
type TracerFunc func(node ast.Node)

// Visit calls f(node).
func (f TracerFunc) Visit(node ast.Node) {
	f(node)
}

// getSourceLine reads a specific line from a file. It returns the line and any error encountered.
func getSourceLine(filename string, lineNum int) (string, error) {
	if filename == "" || lineNum <= 0 {
		return "", nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentLine := 1
	for scanner.Scan() {
		if currentLine == lineNum {
			return strings.TrimSpace(scanner.Text()), nil
		}
		currentLine++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil // Line not found is not considered an error here.
}

// AttachFileSet attaches a FileSet to the error object, which is necessary
// for formatting the call stack.
func (e *Error) AttachFileSet(fset *token.FileSet) {
	e.fset = fset
}

// --- Global Instances ---

// Pre-create global instances for common values to save allocations.
var (
	NIL = &Nil{}
)
