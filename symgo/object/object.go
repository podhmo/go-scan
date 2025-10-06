package object

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/podhmo/go-scan/scanner"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types for the symbolic engine.
const (
	INTEGER_OBJ               ObjectType = "INTEGER"
	FLOAT_OBJ                 ObjectType = "FLOAT"
	COMPLEX_OBJ               ObjectType = "COMPLEX"
	BOOLEAN_OBJ               ObjectType = "BOOLEAN"
	STRING_OBJ                ObjectType = "STRING"
	FUNCTION_OBJ              ObjectType = "FUNCTION"
	INSTANTIATED_FUNCTION_OBJ ObjectType = "INSTANTIATED_FUNCTION"
	TYPE_OBJ                  ObjectType = "TYPE"
	ERROR_OBJ                 ObjectType = "ERROR"
	SYMBOLIC_OBJ              ObjectType = "SYMBOLIC_PLACEHOLDER"
	RETURN_VALUE_OBJ          ObjectType = "RETURN_VALUE"
	PACKAGE_OBJ               ObjectType = "PACKAGE"
	INTRINSIC_OBJ             ObjectType = "INTRINSIC"
	INSTANCE_OBJ              ObjectType = "INSTANCE"
	VARIABLE_OBJ              ObjectType = "VARIABLE"
	POINTER_OBJ               ObjectType = "POINTER"
	STRUCT_OBJ                ObjectType = "STRUCT"
	NIL_OBJ                   ObjectType = "NIL"
	SLICE_OBJ                 ObjectType = "SLICE"
	MAP_OBJ                   ObjectType = "MAP"
	CHANNEL_OBJ               ObjectType = "CHANNEL"
	MULTI_RETURN_OBJ          ObjectType = "MULTI_RETURN"
	BREAK_OBJ                 ObjectType = "BREAK"
	CONTINUE_OBJ              ObjectType = "CONTINUE"
	FALLTHROUGH_OBJ           ObjectType = "FALLTHROUGH"
	VARIADIC_OBJ              ObjectType = "VARIADIC"
	UNRESOLVED_FUNCTION_OBJ   ObjectType = "UNRESOLVED_FUNCTION"
	UNRESOLVED_TYPE_OBJ       ObjectType = "UNRESOLVED_TYPE"
	PANIC_OBJ                 ObjectType = "PANIC"
	AMBIGUOUS_SELECTOR_OBJ    ObjectType = "AMBIGUOUS_SELECTOR"
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
	// Clone creates a shallow copy of the object.
	Clone() Object
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

// Clone creates a shallow copy.
func (s *String) Clone() Object {
	c := *s
	return &c
}

// Release returns the String object to the pool.
func (s *String) Release() {
	s.Value = ""
	stringPool.Put(s)
}

// --- Integer Object ---

// Integer represents an integer value.
type Integer struct {
	BaseObject
	Value int64
}

// Type returns the type of the Integer object.
func (i *Integer) Type() ObjectType { return INTEGER_OBJ }

// Inspect returns a string representation of the Integer's value.
func (i *Integer) Inspect() string { return strconv.FormatInt(i.Value, 10) }

// Clone creates a shallow copy.
func (i *Integer) Clone() Object {
	c := *i
	return &c
}

// Release returns the Integer object to the pool.
func (i *Integer) Release() {
	i.Value = 0
	integerPool.Put(i)
}

// --- Float Object ---

// Float represents a float value.
type Float struct {
	BaseObject
	Value float64
}

// Type returns the type of the Float object.
func (f *Float) Type() ObjectType { return FLOAT_OBJ }

// Inspect returns a string representation of the Float's value.
func (f *Float) Inspect() string { return strconv.FormatFloat(f.Value, 'f', -1, 64) }

// Clone creates a shallow copy.
func (f *Float) Clone() Object {
	c := *f
	return &c
}

// Release returns the Float object to the pool.
func (f *Float) Release() {
	f.Value = 0
	floatPool.Put(f)
}

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

// Clone creates a shallow copy.
func (c *Complex) Clone() Object {
	clone := *c
	return &clone
}

// --- Boolean Object ---

// Boolean represents a boolean value.
type Boolean struct {
	BaseObject
	Value bool
}

// Type returns the type of the Boolean object.
func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }

// Inspect returns a string representation of the Boolean's value.
func (b *Boolean) Inspect() string { return strconv.FormatBool(b.Value) }

// Clone creates a shallow copy.
func (b *Boolean) Clone() Object {
	clone := *b
	return &clone
}

// NewInteger creates a new Integer object from the pool.
func NewInteger(value int64) *Integer {
	obj := integerPool.Get().(*Integer)
	obj.Value = value
	return obj
}

// NewString creates a new String object from the pool.
func NewString(value string) *String {
	obj := stringPool.Get().(*String)
	obj.Value = value
	return obj
}

// NewFloat creates a new Float object from the pool.
func NewFloat(value float64) *Float {
	obj := floatPool.Get().(*Float)
	obj.Value = value
	return obj
}

// --- Function Object ---

// Function represents a user-defined function in the code being analyzed.
type Function struct {
	BaseObject
	Name        *ast.Ident
	Parameters  *ast.FieldList
	Body        *ast.BlockStmt
	Env         *Environment
	Decl        *ast.FuncDecl // The original declaration, for metadata like godoc.
	Lit         *ast.FuncLit  // The original function literal, for anonymous functions.
	Package     *scanner.PackageInfo
	Receiver    Object // The receiver for a method call ("self" or "this").
	ReceiverPos token.Pos
	Def         *scanner.FunctionInfo

	// BoundCallStack stores the evaluator's call stack at the point where this
	// function was passed as an argument. This is used to detect recursion
	// through higher-order functions.
	BoundCallStack []*CallFrame
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

// WithReceiver creates a new Function object with the receiver and its position bound.
func (f *Function) WithReceiver(receiver Object, pos token.Pos) *Function {
	newF := f.Clone().(*Function) // Creates a shallow copy
	newF.Receiver = receiver
	newF.ReceiverPos = pos
	return newF
}

// Clone creates a shallow copy of the Function object. This is essential for
// creating call-site-specific instances of a function (e.g., to bind a call stack)
// without polluting the globally cached function object.
func (f *Function) Clone() Object {
	newF := *f
	return &newF
}

// --- Intrinsic Object ---

// Intrinsic represents a built-in function that is implemented in Go.
type Intrinsic struct {
	BaseObject
	// The Go function that implements the intrinsic's behavior.
	Fn func(ctx context.Context, args ...Object) Object
}

// Type returns the type of the Intrinsic object.
func (i *Intrinsic) Type() ObjectType { return INTRINSIC_OBJ }

// Inspect returns a string representation of the intrinsic function.
func (i *Intrinsic) Inspect() string { return "intrinsic function" }

// Clone creates a shallow copy.
func (i *Intrinsic) Clone() Object {
	c := *i
	return &c
}

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

// Clone creates a shallow copy of the instance.
func (i *Instance) Clone() Object {
	c := *i
	// The State map and Underlying object are copied by reference, which is a shallow copy.
	return &c
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

// Clone creates a shallow copy.
func (p *Package) Clone() Object {
	c := *p
	return &c
}

// --- Error Object ---

// CallFrame represents a single frame in the symbolic execution call stack.
type CallFrame struct {
	Function    string
	Pos         token.Pos
	Fn          *Function
	Args        []Object
	ReceiverPos token.Pos
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
	Wrapped   Object       // For error wrapping (e.g., %w)
}

// Type returns the type of the Error object.
func (e *Error) Type() ObjectType { return ERROR_OBJ }

// Inspect returns a formatted string representation of the error, including the call stack.
func (e *Error) Inspect() string {
	return e.Error()
}

// Clone creates a shallow copy.
func (e *Error) Clone() Object {
	c := *e
	return &c
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
	// Generic field to hold an underlying object, used for error wrapping etc.
	Underlying Object
	// If the placeholder is for an interface method call, this holds the receiver.
	Receiver Object
	// For interface method calls, this holds the set of possible concrete field types
	// that the receiver variable could hold.
	PossibleConcreteTypes []*scanner.FieldType
	// Cache for the Inspect() result to avoid repeated string building
	inspectCache string
	cacheValid   bool

	// For symbolic slices/maps, we can sometimes know the length and capacity
	// even if the contents are unknown. -1 indicates unknown.
	Len int64
	Cap int64
}

// Type returns the type of the SymbolicPlaceholder object.
func (sp *SymbolicPlaceholder) Type() ObjectType { return SYMBOLIC_OBJ }

// Inspect returns a string representation of the symbolic placeholder.
func (sp *SymbolicPlaceholder) Inspect() string {
	if sp.cacheValid {
		return sp.inspectCache
	}

	var builder strings.Builder
	builder.WriteString("<Symbolic: ")
	builder.WriteString(sp.Reason)
	builder.WriteString(">")
	sp.inspectCache = builder.String()
	sp.cacheValid = true
	return sp.inspectCache
}

// Clone creates a shallow copy.
func (sp *SymbolicPlaceholder) Clone() Object {
	c := *sp
	c.cacheValid = false // Invalidate cache on clone
	return &c
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

// Clone creates a shallow copy.
func (rv *ReturnValue) Clone() Object {
	c := *rv
	return &c
}

// --- Variable Object ---

// Variable represents a declared variable in the environment.
// It holds a value and its resolved type information.
type Variable struct {
	BaseObject
	Name          string
	Value         Object
	Initializer   ast.Expr             // For lazy evaluation
	IsEvaluated   bool                 // For lazy evaluation
	DeclEnv       *Environment         // Environment where the variable was declared
	DeclPkg       *scanner.PackageInfo // Package where the variable was declared
	PossibleTypes map[string]struct{}  // Used for tracking possible types for interface variables
}

// Type returns the type of the Variable object.
func (v *Variable) Type() ObjectType { return VARIABLE_OBJ }

// Inspect returns a string representation of the variable's value.
func (v *Variable) Inspect() string {
	if v.Value == nil {
		return "<unevaluated var>"
	}
	return v.Value.Inspect()
}

// Clone creates a shallow copy.
func (v *Variable) Clone() Object {
	c := *v
	return &c
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

// Clone creates a shallow copy.
func (p *Pointer) Clone() Object {
	c := *p
	return &c
}

// --- Struct Object ---

// Struct represents a struct instance. Its type is represented by a TypeInfo.
type Struct struct {
	BaseObject
	StructType *scanner.TypeInfo
	Fields     map[string]Object
}

// Type returns the type of the Struct object.
func (s *Struct) Type() ObjectType { return STRUCT_OBJ }

// Inspect returns a string representation of the struct.
func (s *Struct) Inspect() string {
	var out bytes.Buffer
	pairs := []string{}
	// Sort keys for consistent output
	keys := make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := s.Fields[k]
		pairs = append(pairs, fmt.Sprintf("%s: %s", k, v.Inspect()))
	}

	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

// Clone creates a shallow copy of the struct. The Fields map is duplicated,
// but the values within are not.
func (s *Struct) Clone() Object {
	c := *s
	c.Fields = make(map[string]Object, len(s.Fields))
	for k, v := range s.Fields {
		c.Fields[k] = v // This is a shallow copy of the field values
	}
	return &c
}

// Get retrieves a field from the struct.
func (s *Struct) Get(name string) (Object, bool) {
	val, ok := s.Fields[name]
	return val, ok
}

// Set sets a field in the struct.
func (s *Struct) Set(name string, val Object) {
	if s.Fields == nil {
		s.Fields = make(map[string]Object)
	}
	s.Fields[name] = val
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

// Clone returns the singleton NIL instance, as it's immutable.
func (n *Nil) Clone() Object {
	return n
}

// --- Slice Object ---

// Slice represents a slice literal. Its type is represented by a FieldType,
// which captures the slice structure (e.g., []User).
type Slice struct {
	BaseObject
	Elements       []Object
	SliceFieldType *scanner.FieldType
	Len            int64
	Cap            int64
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

// Clone creates a shallow copy of the slice object.
// The elements themselves are not cloned.
func (s *Slice) Clone() Object {
	c := *s
	// The elements slice is copied, but it's a slice of interfaces, so it's a shallow copy.
	newElements := make([]Object, len(s.Elements))
	copy(newElements, s.Elements)
	c.Elements = newElements
	return &c
}

// --- Map Object ---

// Map represents a map literal. Its type is represented by a FieldType,
// which captures the map structure (e.g., map[string]int).
type Map struct {
	BaseObject
	MapFieldType *scanner.FieldType
	Pairs        map[Object]Object // Simplified representation for symbolic analysis
}

// Type returns the type of the Map object.
func (m *Map) Type() ObjectType { return MAP_OBJ }

// Inspect returns a string representation of the map type.
func (m *Map) Inspect() string {
	// If we have full TypeInfo for a named type, use that.
	if ti := m.TypeInfo(); ti != nil && ti.Name != "" {
		return ti.Name
	}
	if m.MapFieldType != nil {
		return m.MapFieldType.String()
	}
	return "map[<unknown>]<unknown>"
}

// Clone creates a shallow copy of the map object.
// The key/value pairs are not deep-cloned.
func (m *Map) Clone() Object {
	c := *m
	c.Pairs = make(map[Object]Object, len(m.Pairs))
	for k, v := range m.Pairs {
		c.Pairs[k] = v
	}
	return &c
}

// --- Channel Object ---

// Channel represents a channel object. Its type is represented by a FieldType,
// which captures the channel structure (e.g., chan int).
type Channel struct {
	BaseObject
	ChanFieldType *scanner.FieldType
}

// Type returns the type of the Channel object.
func (c *Channel) Type() ObjectType { return CHANNEL_OBJ }

// Inspect returns a string representation of the channel type.
func (c *Channel) Inspect() string {
	if c.ChanFieldType != nil {
		return c.ChanFieldType.String()
	}
	return "chan <unknown>"
}

// Clone creates a shallow copy.
func (c *Channel) Clone() Object {
	clone := *c
	return &clone
}

// --- Environment ---

// Environment holds the bindings for variables and functions.
type Environment struct {
	store map[string]Object
	outer *Environment
}

// Object pools for reusing common objects
var (
	envPool = sync.Pool{
		New: func() interface{} {
			return &Environment{
				store: make(map[string]Object),
				outer: nil,
			}
		},
	}
	integerPool = sync.Pool{
		New: func() interface{} {
			return &Integer{}
		},
	}
	stringPool = sync.Pool{
		New: func() interface{} {
			return &String{}
		},
	}
	floatPool = sync.Pool{
		New: func() interface{} {
			return &Float{}
		},
	}
)

// NewEnvironment creates a new, top-level environment.
func NewEnvironment() *Environment {
	env := envPool.Get().(*Environment)
	// Reset the environment state
	for k := range env.store {
		delete(env.store, k)
	}
	env.outer = nil
	return env
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

// Release returns the environment to the pool for reuse.
// Only call this on environments that are no longer needed.
func (e *Environment) Release() {
	// Only release if this is not an outer environment being used by others
	if e.outer == nil {
		envPool.Put(e)
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

// Clone creates a shallow copy.
func (mr *MultiReturn) Clone() Object {
	c := *mr
	return &c
}

// --- Break Object ---

// Break represents a break statement.
type Break struct {
	BaseObject
	Label string
}

// Type returns the type of the Break object.
func (b *Break) Type() ObjectType { return BREAK_OBJ }

// Inspect returns a string representation of the break statement.
func (b *Break) Inspect() string {
	if b.Label != "" {
		return fmt.Sprintf("break %s", b.Label)
	}
	return "break"
}

// Clone creates a shallow copy.
func (b *Break) Clone() Object {
	c := *b
	return &c
}

// --- Continue Object ---

// Continue represents a continue statement.
type Continue struct {
	BaseObject
	Label string
}

// Type returns the type of the Continue object.
func (c *Continue) Type() ObjectType { return CONTINUE_OBJ }

// Inspect returns a string representation of the continue statement.
func (c *Continue) Inspect() string {
	if c.Label != "" {
		return fmt.Sprintf("continue %s", c.Label)
	}
	return "continue"
}

// Clone creates a shallow copy.
func (c *Continue) Clone() Object {
	clone := *c
	return &clone
}

// --- Fallthrough Object ---

// Fallthrough represents a fallthrough statement.
type Fallthrough struct {
	BaseObject
}

// Type returns the type of the Fallthrough object.
func (f *Fallthrough) Type() ObjectType { return FALLTHROUGH_OBJ }

// Inspect returns a string representation of the fallthrough statement.
func (f *Fallthrough) Inspect() string { return "fallthrough" }

// Clone returns the singleton FALLTHROUGH instance.
func (f *Fallthrough) Clone() Object {
	return f
}

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

// Clone creates a shallow copy.
func (v *Variadic) Clone() Object {
	c := *v
	return &c
}

var ()

// --- InstantiatedFunction Object ---

// InstantiatedFunction represents a generic function that has been instantiated
// with concrete type arguments.
type InstantiatedFunction struct {
	*Function
	TypeArguments []ast.Expr
	TypeArgs      []*scanner.TypeInfo // Resolved type arguments
}

// Type returns the type of the InstantiatedFunction object.
func (f *InstantiatedFunction) Type() ObjectType { return INSTANTIATED_FUNCTION_OBJ }

// Inspect returns a string representation of the instantiated function.
func (f *InstantiatedFunction) Inspect() string {
	var args []string
	for _, arg := range f.TypeArgs {
		if arg != nil {
			args = append(args, arg.Name)
		} else {
			args = append(args, "<unresolved>")
		}
	}
	name := "<closure>"
	if f.Function.Name != nil {
		name = f.Function.Name.Name
	}
	return fmt.Sprintf("func %s[%s]() { ... }", name, strings.Join(args, ", "))
}

// Clone creates a shallow copy of the instantiated function.
func (f *InstantiatedFunction) Clone() Object {
	c := *f
	// Also clone the embedded Function to ensure the receiver isn't shared.
	if f.Function != nil {
		c.Function = f.Function.Clone().(*Function)
	}
	return &c
}

// --- Type Object ---

// Type represents a type value that can be stored in the environment.
// This is used to represent type parameters in generic functions.
type Type struct {
	BaseObject
	// TypeName is the string representation of the type, e.g., "int".
	TypeName string
	// ResolvedType is the detailed type information from the scanner.
	ResolvedType *scanner.TypeInfo
}

// Type returns the type of the Type object.
func (t *Type) Type() ObjectType { return TYPE_OBJ }

// Inspect returns a string representation of the type.
func (t *Type) Inspect() string {
	return t.TypeName
}

// Clone creates a shallow copy.
func (t *Type) Clone() Object {
	c := *t
	return &c
}

// --- Tracer Interface ---

// TraceEvent represents a single event in the evaluation trace.
type TraceEvent struct {
	Step int
	Node ast.Node
	Pkg  *scanner.PackageInfo
	Env  *Environment
}

// Tracer is an interface for instrumenting the symbolic execution process.
// An implementation can be passed to the interpreter to track which AST nodes
// are being evaluated.
type Tracer interface {
	Trace(event TraceEvent)
}

// ScanPolicyFunc is a function that determines whether a package should be scanned from source.
type ScanPolicyFunc func(importPath string) bool

// TracerFunc is an adapter to allow the use of ordinary functions as Tracers.
type TracerFunc func(event TraceEvent)

// Trace calls f(event).
func (f TracerFunc) Trace(event TraceEvent) {
	f(event)
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

// --- PanicError Object ---

// PanicError represents a panic that occurred during symbolic evaluation.
// It is also a Go error, so it can be returned as one.
type PanicError struct {
	BaseObject
	Value Object // The value passed to panic()
}

// Type returns the type of the PanicError object.
func (pe *PanicError) Type() ObjectType { return PANIC_OBJ }

// Inspect returns a string representation of the panic.
func (pe *PanicError) Inspect() string {
	return fmt.Sprintf("panic: %s", pe.Value.Inspect())
}

// Error returns a string representation of the panic, satisfying the error interface.
func (pe *PanicError) Error() string {
	return pe.Inspect()
}

// Clone creates a shallow copy.
func (pe *PanicError) Clone() Object {
	c := *pe
	return &c
}

// --- UnresolvedFunction Object ---

// UnresolvedFunction represents a function that could not be fully resolved
// at the time of symbol lookup, for example, because its package could not be scanned.
type UnresolvedFunction struct {
	BaseObject
	PkgPath  string
	FuncName string
}

// Type returns the type of the UnresolvedFunction object.
func (uf *UnresolvedFunction) Type() ObjectType { return UNRESOLVED_FUNCTION_OBJ }

// Inspect returns a string representation of the unresolved function.
func (uf *UnresolvedFunction) Inspect() string {
	return fmt.Sprintf("<Unresolved Function: %s.%s>", uf.PkgPath, uf.FuncName)
}

// Clone creates a shallow copy.
func (uf *UnresolvedFunction) Clone() Object {
	c := *uf
	return &c
}

// --- UnresolvedType Object ---

// UnresolvedType represents a type that could not be fully resolved
// at the time of symbol lookup, for example, because its package could not be scanned.
type UnresolvedType struct {
	BaseObject
	PkgPath  string
	TypeName string
}

// Type returns the type of the UnresolvedType object.
func (ut *UnresolvedType) Type() ObjectType { return UNRESOLVED_TYPE_OBJ }

// Inspect returns a string representation of the unresolved type.
func (ut *UnresolvedType) Inspect() string {
	return fmt.Sprintf("<Unresolved Type: %s.%s>", ut.PkgPath, ut.TypeName)
}

// Clone creates a shallow copy.
func (ut *UnresolvedType) Clone() Object {
	c := *ut
	return &c
}

// --- Global Instances ---
var (
	// TRUE is the singleton true value.
	TRUE = &Boolean{Value: true}
	// FALSE is the singleton false value.
	FALSE = &Boolean{Value: false}
	// NIL is the singleton nil value.
	NIL = &Nil{}
	// FALLTHROUGH is the singleton fallthrough value.
	FALLTHROUGH = &Fallthrough{}
)

// --- AmbiguousSelector Object ---

// AmbiguousSelector represents a selector expression (e.g., `x.Y`) where it's
// unclear if `Y` is a field or a method, typically due to unresolved embedded types.
// The resolution is deferred to the caller (e.g., a CallExpr or AssignStmt).
type AmbiguousSelector struct {
	BaseObject
	Receiver Object
	Sel      *ast.Ident
}

// Type returns the type of the AmbiguousSelector object.
func (as *AmbiguousSelector) Type() ObjectType { return AMBIGUOUS_SELECTOR_OBJ }

// Inspect returns a string representation of the ambiguous selector.
func (as *AmbiguousSelector) Inspect() string {
	return fmt.Sprintf("<Ambiguous Selector: %s.%s>", as.Receiver.Inspect(), as.Sel.Name)
}

// Clone creates a shallow copy.
func (as *AmbiguousSelector) Clone() Object {
	c := *as
	return &c
}
