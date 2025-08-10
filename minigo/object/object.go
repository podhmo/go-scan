package object

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"hash/fnv"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types. More will be added later.
const (
	INTEGER_OBJ              ObjectType = "INTEGER"
	BOOLEAN_OBJ              ObjectType = "BOOLEAN"
	STRING_OBJ               ObjectType = "STRING"
	NIL_OBJ                  ObjectType = "NIL"
	BREAK_OBJ                ObjectType = "BREAK"
	CONTINUE_OBJ             ObjectType = "CONTINUE"
	RETURN_VALUE_OBJ         ObjectType = "RETURN_VALUE"
	FUNCTION_OBJ             ObjectType = "FUNCTION"
	BUILTIN_OBJ              ObjectType = "BUILTIN"
	STRUCT_DEFINITION_OBJ    ObjectType = "STRUCT_DEFINITION"
	STRUCT_INSTANCE_OBJ      ObjectType = "STRUCT_INSTANCE"
	INTERFACE_DEFINITION_OBJ ObjectType = "INTERFACE_DEFINITION"
	INTERFACE_INSTANCE_OBJ   ObjectType = "INTERFACE_INSTANCE"
	BOUND_METHOD_OBJ         ObjectType = "BOUND_METHOD"
	POINTER_OBJ              ObjectType = "POINTER"
	ARRAY_OBJ                ObjectType = "ARRAY"
	MAP_OBJ                  ObjectType = "MAP"
	TUPLE_OBJ                ObjectType = "TUPLE"
	PACKAGE_OBJ              ObjectType = "PACKAGE"
	GO_VALUE_OBJ             ObjectType = "GO_VALUE"
	ERROR_OBJ                ObjectType = "ERROR"
)

// Hashable is an interface for objects that can be used as map keys.
type Hashable interface {
	// HashKey returns a unique key for the object, used for map lookups.
	HashKey() HashKey
}

// HashKey is used as a key in the internal hash map for Map objects.
// It's a combination of the object's type and its calculated hash value.
type HashKey struct {
	Type  ObjectType
	Value uint64
}

// CallFrame represents a single frame in the call stack.
type CallFrame struct {
	Pos       token.Pos
	Function  string // Name of the function
	IsBuiltin bool   // Whether the function is a Go builtin or user-defined
}

// Format formats the call frame into a readable string.
// fset is required to resolve the position to a file and line number.
func (cf *CallFrame) Format(fset *token.FileSet) string {
	position := fset.Position(cf.Pos)
	funcName := cf.Function
	if funcName == "" {
		funcName = "<script>"
	}

	sourceLine := getSourceLine(position.Filename, position.Line)
	if sourceLine != "" {
		sourceLine = "\n\t\t" + sourceLine
	}

	return fmt.Sprintf("\t%s:%d:%d:\tin %s%s", position.Filename, position.Line, position.Column, funcName, sourceLine)
}

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

// HashKey returns the hash key for an Integer.
func (i *Integer) HashKey() HashKey {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}
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

// HashKey returns the hash key for a String.
func (s *String) HashKey() HashKey {
	h := fnv.New64a()
	h.Write([]byte(s.Value))
	return HashKey{Type: s.Type(), Value: h.Sum64()}
}

// --- Boolean Object ---

// Boolean represents a boolean value.
type Boolean struct {
	Value bool
}

// Type returns the type of the Boolean object.
func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }

// Inspect returns a string representation of the Boolean's value.
func (b *Boolean) Inspect() string { return fmt.Sprintf("%t", b.Value) }

// HashKey returns the hash key for a Boolean.
func (b *Boolean) HashKey() HashKey {
	var value uint64
	if b.Value {
		value = 1
	}
	return HashKey{Type: b.Type(), Value: value}
}

// --- Nil Object ---

// Nil represents a nil value.
type Nil struct{}

// Type returns the type of the Nil object.
func (n *Nil) Type() ObjectType { return NIL_OBJ }

// Inspect returns a string representation of the Nil's value.
func (n *Nil) Inspect() string { return "nil" }

// --- Break Statement Object ---

// BreakStatement represents a break statement. It's a singleton.
type BreakStatement struct{}

// Type returns the type of the BreakStatement object.
func (bs *BreakStatement) Type() ObjectType { return BREAK_OBJ }

// Inspect returns a string representation of the BreakStatement.
func (bs *BreakStatement) Inspect() string { return "break" }

// --- Continue Statement Object ---

// ContinueStatement represents a continue statement. It's a singleton.
type ContinueStatement struct{}

// Type returns the type of the ContinueStatement object.
func (cs *ContinueStatement) Type() ObjectType { return CONTINUE_OBJ }

// Inspect returns a string representation of the ContinueStatement.
func (cs *ContinueStatement) Inspect() string { return "continue" }

// --- Return Value Object ---

// ReturnValue represents the value being returned from a function.
// It wraps another object to signal the "return" state.
type ReturnValue struct {
	Value Object
}

// Type returns the type of the ReturnValue object.
func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }

// Inspect returns a string representation of the wrapped value.
func (rv *ReturnValue) Inspect() string { return rv.Value.Inspect() }

// --- Function Object ---

// Function represents a user-defined function or method.
type Function struct {
	Name       *ast.Ident
	Recv       *ast.FieldList // nil for regular functions
	Parameters *ast.FieldList
	Results    *ast.FieldList
	Body       *ast.BlockStmt
	Env        *Environment
}

// IsVariadic returns true if the function is variadic.
func (f *Function) IsVariadic() bool {
	if f.Parameters == nil || len(f.Parameters.List) == 0 {
		return false
	}
	lastParam := f.Parameters.List[len(f.Parameters.List)-1]
	_, ok := lastParam.Type.(*ast.Ellipsis)
	return ok
}

// Type returns the type of the Function object.
func (f *Function) Type() ObjectType { return FUNCTION_OBJ }

// Inspect returns a string representation of the function.
func (f *Function) Inspect() string {
	var out bytes.Buffer

	params := []string{}
	if f.Parameters != nil {
		for _, p := range f.Parameters.List {
			paramStr := []string{}
			for _, name := range p.Names {
				paramStr = append(paramStr, name.String())
			}
			// This is a simplified representation; we don't show the type.
			// A more advanced version might use format.Node.
			params = append(params, strings.Join(paramStr, ", "))
		}
	}

	out.WriteString("func")
	if f.Name != nil {
		out.WriteString(" ")
		out.WriteString(f.Name.String())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") { ... }")

	return out.String()
}

// Copy creates a shallow copy of the struct instance.
func (si *StructInstance) Copy() *StructInstance {
	newFields := make(map[string]Object, len(si.Fields))
	for k, v := range si.Fields {
		// Note: This is a shallow copy of the field values.
		newFields[k] = v
	}
	return &StructInstance{
		Def:    si.Def,
		Fields: newFields,
	}
}

// --- Builtin Function Object ---

// BuiltinContext provides the necessary context for a built-in function to execute.
// This interface is implemented by the evaluator and passed to built-in functions,
// avoiding a direct dependency from the object package to the evaluator package.
type BuiltinContext interface {
	Stdin() io.Reader
	Stdout() io.Writer
	Stderr() io.Writer
	Fset() *token.FileSet
	NewError(pos token.Pos, format string, args ...interface{}) *Error
}

// BuiltinFunction is the signature for built-in functions.
// It receives the execution context, the position of the call, and the evaluated arguments.
type BuiltinFunction func(ctx BuiltinContext, pos token.Pos, args ...Object) Object

// Builtin represents a built-in function.
type Builtin struct {
	Fn BuiltinFunction
}

// Type returns the type of the Builtin object.
func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }

// Inspect returns a string representation of the built-in function.
func (b *Builtin) Inspect() string { return "builtin function" }

// --- Struct Definition Object ---

// StructDefinition represents the definition of a struct type.
type StructDefinition struct {
	Name    *ast.Ident
	Fields  []*ast.Field
	Methods map[string]*Function
}

// Type returns the type of the StructDefinition object.
func (sd *StructDefinition) Type() ObjectType { return STRUCT_DEFINITION_OBJ }

// Inspect returns a string representation of the struct definition.
func (sd *StructDefinition) Inspect() string {
	return fmt.Sprintf("struct %s", sd.Name.String())
}

// --- Interface Definition Object ---

// InterfaceDefinition represents the definition of an interface type.
type InterfaceDefinition struct {
	Name    *ast.Ident
	Methods *ast.FieldList
}

// Type returns the type of the InterfaceDefinition object.
func (id *InterfaceDefinition) Type() ObjectType { return INTERFACE_DEFINITION_OBJ }

// Inspect returns a string representation of the interface definition.
func (id *InterfaceDefinition) Inspect() string {
	var out bytes.Buffer
	methods := []string{}
	if id.Methods != nil {
		for _, method := range id.Methods.List {
			if len(method.Names) > 0 {
				// This is a simplified representation. A full one would need to format the type.
				methods = append(methods, method.Names[0].Name+"()")
			}
		}
	}
	out.WriteString("interface { ")
	out.WriteString(strings.Join(methods, "; "))
	out.WriteString(" }")
	return out.String()
}

// --- Interface Instance Object ---

// InterfaceInstance represents a value that is stored in a variable of an interface type.
// It holds a reference to the interface definition and the concrete object that implements it.
type InterfaceInstance struct {
	Def   *InterfaceDefinition
	Value Object
}

// Type returns the type of the InterfaceInstance object.
func (ii *InterfaceInstance) Type() ObjectType { return INTERFACE_INSTANCE_OBJ }

// Inspect returns a string representation of the underlying concrete value.
func (ii *InterfaceInstance) Inspect() string {
	if ii.Value == nil || ii.Value.Type() == NIL_OBJ {
		return "nil"
	}
	return ii.Value.Inspect()
}

// --- Bound Method Object ---

// BoundMethod represents a method that is bound to a specific receiver instance.
type BoundMethod struct {
	Fn       *Function
	Receiver Object
}

// Type returns the type of the BoundMethod object.
func (bm *BoundMethod) Type() ObjectType { return BOUND_METHOD_OBJ }

// Inspect returns a string representation of the bound method.
func (bm *BoundMethod) Inspect() string {
	// Similar to Function.Inspect, but we could indicate it's a method.
	return fmt.Sprintf("method %s()", bm.Fn.Name.String())
}

// --- Struct Instance Object ---

// StructInstance represents an instance of a struct.
type StructInstance struct {
	Def    *StructDefinition
	Fields map[string]Object
}

// Type returns the type of the StructInstance object.
func (si *StructInstance) Type() ObjectType { return STRUCT_INSTANCE_OBJ }

// Inspect returns a string representation of the struct instance.
func (si *StructInstance) Inspect() string {
	var out bytes.Buffer
	fields := []string{}
	for k, v := range si.Fields {
		fields = append(fields, fmt.Sprintf("%s: %s", k, v.Inspect()))
	}

	out.WriteString(si.Def.Name.String())
	out.WriteString("{")
	out.WriteString(strings.Join(fields, ", "))
	out.WriteString("}")

	return out.String()
}

// --- Pointer Object ---

// Pointer represents a pointer to another object.
type Pointer struct {
	Element *Object
}

// Type returns the type of the Pointer object.
func (p *Pointer) Type() ObjectType { return POINTER_OBJ }

// Inspect returns a string representation of the Pointer's value.
func (p *Pointer) Inspect() string {
	return fmt.Sprintf("0x%x", p.Element)
}

// --- Array Object ---

// Array represents an array data structure.
type Array struct {
	Elements []Object
}

// Type returns the type of the Array object.
func (a *Array) Type() ObjectType { return ARRAY_OBJ }

// Inspect returns a string representation of the Array's elements.
func (a *Array) Inspect() string {
	var out bytes.Buffer

	elements := []string{}
	for _, e := range a.Elements {
		elements = append(elements, e.Inspect())
	}

	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")

	return out.String()
}

// --- Map Object ---

// MapPair represents a key-value pair in a Map object.
type MapPair struct {
	Key   Object
	Value Object
}

// Map represents a map data structure.
type Map struct {
	Pairs map[HashKey]MapPair
}

// Type returns the type of the Map object.
func (m *Map) Type() ObjectType { return MAP_OBJ }

// Inspect returns a string representation of the Map's pairs.
func (m *Map) Inspect() string {
	var out bytes.Buffer

	pairs := []string{}
	// Note: Iteration order over maps is not guaranteed.
	for _, pair := range m.Pairs {
		pairs = append(pairs, fmt.Sprintf("%s: %s", pair.Key.Inspect(), pair.Value.Inspect()))
	}

	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")

	return out.String()
}

// --- Tuple Object ---

// Tuple represents a tuple of objects, used for multiple return values.
type Tuple struct {
	Elements []Object
}

// Type returns the type of the Tuple object.
func (t *Tuple) Type() ObjectType { return TUPLE_OBJ }

// Inspect returns a string representation of the Tuple's elements.
func (t *Tuple) Inspect() string {
	var out bytes.Buffer

	elements := []string{}
	for _, e := range t.Elements {
		elements = append(elements, e.Inspect())
	}

	out.WriteString("(")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString(")")

	return out.String()
}

// --- FileScope ---

// FileScope holds the AST and file-specific import aliases for a single file.
type FileScope struct {
	AST        *ast.File
	Aliases    map[string]string // alias -> import path
	DotImports []string          // list of package paths for dot imports
}

// NewFileScope creates a new file scope.
func NewFileScope(ast *ast.File) *FileScope {
	return &FileScope{
		AST:        ast,
		Aliases:    make(map[string]string),
		DotImports: make([]string, 0),
	}
}

// --- Package Object ---

// Package represents an imported Go package.
type Package struct {
	Name    string
	Path    string
	Info    *scanner.PackageInfo
	Members map[string]Object
}

// Type returns the type of the Package object.
func (p *Package) Type() ObjectType { return PACKAGE_OBJ }

// Inspect returns a string representation of the package.
func (p *Package) Inspect() string {
	return fmt.Sprintf("package %s (%q)", p.Name, p.Path)
}

// --- GoValue Object ---

// GoValue wraps a native Go value (`reflect.Value`).
// This allows Go variables and functions to be injected into the interpreter.
type GoValue struct {
	Value reflect.Value
}

// Type returns the type of the GoValue object.
func (g *GoValue) Type() ObjectType { return GO_VALUE_OBJ }

// Inspect returns a string representation of the wrapped Go value.
func (g *GoValue) Inspect() string {
	// Check if the value is valid to prevent panics with IsNil.
	if !g.Value.IsValid() {
		return "<invalid Go value>"
	}
	// Check for nil pointers to avoid panics on .Interface().
	if g.Value.Kind() == reflect.Ptr && g.Value.IsNil() {
		return "nil"
	}
	// For other types, Interface() is safe.
	return fmt.Sprintf("%v", g.Value.Interface())
}

// --- Error Object ---

// Error represents a runtime error. It contains a message and a call stack.
type Error struct {
	Pos       token.Pos
	Message   string
	CallStack []CallFrame
	fset      *token.FileSet // FileSet to resolve positions
}

// Type returns the type of the Error object.
func (e *Error) Type() ObjectType { return ERROR_OBJ }

// Inspect returns a formatted string representation of the error, including the call stack.
func (e *Error) Inspect() string {
	var out bytes.Buffer

	out.WriteString("runtime error: ")
	out.WriteString(e.Message)

	if e.fset != nil && e.Pos.IsValid() {
		position := e.fset.Position(e.Pos)
		sourceLine := getSourceLine(position.Filename, position.Line)
		out.WriteString(fmt.Sprintf("\n\t%s:%d:%d:", position.Filename, position.Line, position.Column))
		if sourceLine != "" {
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

// AttachFileSet attaches a FileSet to the error object, which is necessary
// for formatting the call stack. This is done this way because the FileSet

// is part of the evaluator, not the object system itself.
func (e *Error) AttachFileSet(fset *token.FileSet) {
	e.fset = fset
}

// Error makes it a valid Go error.
func (e *Error) Error() string {
	return e.Message
}

// getSourceLine reads a specific line from a file.
func getSourceLine(filename string, lineNum int) string {
	if filename == "" || lineNum <= 0 {
		return ""
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Sprintf("[Error opening source file: %v]", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentLine := 1
	for scanner.Scan() {
		if currentLine == lineNum {
			return strings.TrimSpace(scanner.Text())
		}
		currentLine++
	}
	return ""
}

// --- Global Instances ---

// Pre-create global instances for common values to save allocations.
var (
	TRUE     = &Boolean{Value: true}
	FALSE    = &Boolean{Value: false}
	NIL      = &Nil{}
	BREAK    = &BreakStatement{}
	CONTINUE = &ContinueStatement{}
)

// --- Environment ---

// SymbolRegistry holds registered Go symbols (functions, variables) that can be
// imported by scripts.
type SymbolRegistry struct {
	packages map[string]map[string]any
}

// NewSymbolRegistry creates a new, empty symbol registry.
func NewSymbolRegistry() *SymbolRegistry {
	return &SymbolRegistry{
		packages: make(map[string]map[string]any),
	}
}

// Register adds a collection of symbols to a given package path.
// If the package path already exists, the new symbols are merged with the existing ones.
func (r *SymbolRegistry) Register(pkgPath string, symbols map[string]any) {
	if _, ok := r.packages[pkgPath]; !ok {
		r.packages[pkgPath] = make(map[string]any)
	}
	for name, symbol := range symbols {
		r.packages[pkgPath][name] = symbol
	}
}

// Lookup finds a symbol in the registry by its package path and name.
func (r *SymbolRegistry) Lookup(pkgPath, name string) (any, bool) {
	if pkg, ok := r.packages[pkgPath]; ok {
		if symbol, ok := pkg[name]; ok {
			return symbol, true
		}
	}
	return nil, false
}

// GetAllFor returns all registered symbols for a given package path.
func (r *SymbolRegistry) GetAllFor(pkgPath string) (map[string]any, bool) {
	pkg, ok := r.packages[pkgPath]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent modification of the original map.
	clone := make(map[string]any, len(pkg))
	for k, v := range pkg {
		clone[k] = v
	}
	return clone, true
}

// Environment holds the bindings for variables and functions.
type Environment struct {
	store  map[string]*Object
	consts map[string]Object // Constants are immutable, so they don't need to be pointers.
	outer  *Environment
}

// NewEnvironment creates a new, top-level environment.
func NewEnvironment() *Environment {
	s := make(map[string]*Object)
	c := make(map[string]Object)
	return &Environment{store: s, consts: c, outer: nil}
}

// NewEnclosedEnvironment creates a new environment that is enclosed by an outer one.
func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}

// Get retrieves an object by name from the environment, checking outer scopes if necessary.
// It checks constants first, then variables. It dereferences the pointer from the store.
func (e *Environment) Get(name string) (Object, bool) {
	if obj, ok := e.consts[name]; ok {
		return obj, true
	}
	if objPtr, ok := e.store[name]; ok {
		return *objPtr, true
	}
	if e.outer != nil {
		return e.outer.Get(name)
	}
	return nil, false
}

// GetAddress retrieves the memory address of a variable in the environment.
func (e *Environment) GetAddress(name string) (*Object, bool) {
	if objPtr, ok := e.store[name]; ok {
		return objPtr, true
	}
	if e.outer != nil {
		return e.outer.GetAddress(name)
	}
	return nil, false
}

// GetConstant retrieves a constant by name, checking outer scopes.
func (e *Environment) GetConstant(name string) (Object, bool) {
	if obj, ok := e.consts[name]; ok {
		return obj, true
	}
	if e.outer != nil {
		return e.outer.GetConstant(name)
	}
	return nil, false
}

// Set stores an object by name in the current environment's scope.
// It stores a pointer to the object.
func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = &val
	return val
}

// SetConstant stores an immutable binding.
func (e *Environment) SetConstant(name string, val Object) Object {
	e.consts[name] = val
	return val
}

// Assign updates the value of an existing variable. It searches up through
// the enclosing environments. If the variable is found, it's updated and
// the function returns true. If it's not found, or if it's a constant,
// it returns false.
func (e *Environment) Assign(name string, val Object) bool {
	// Constants cannot be reassigned.
	if _, ok := e.consts[name]; ok {
		return false
	}

	// If the variable exists in the current scope's store, update it.
	if objPtr, ok := e.store[name]; ok {
		*objPtr = val
		return true
	}

	// If not found locally, try assigning in the outer scope.
	if e.outer != nil {
		return e.outer.Assign(name, val)
	}

	// The variable was not found in any scope.
	return false
}
