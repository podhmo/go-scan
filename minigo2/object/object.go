package object

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"hash/fnv"
	"os"
	"strings"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types. More will be added later.
const (
	INTEGER_OBJ           ObjectType = "INTEGER"
	BOOLEAN_OBJ           ObjectType = "BOOLEAN"
	STRING_OBJ            ObjectType = "STRING"
	NULL_OBJ              ObjectType = "NULL"
	BREAK_OBJ             ObjectType = "BREAK"
	CONTINUE_OBJ          ObjectType = "CONTINUE"
	RETURN_VALUE_OBJ      ObjectType = "RETURN_VALUE"
	FUNCTION_OBJ          ObjectType = "FUNCTION"
	STRUCT_DEFINITION_OBJ ObjectType = "STRUCT_DEFINITION"
	STRUCT_INSTANCE_OBJ   ObjectType = "STRUCT_INSTANCE"
	ARRAY_OBJ             ObjectType = "ARRAY"
	MAP_OBJ               ObjectType = "MAP"
	ERROR_OBJ             ObjectType = "ERROR"
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
	Pos        token.Pos
	Function   string // Name of the function
	IsBuiltin  bool   // Whether the function is a Go builtin or user-defined
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

// --- Null Object ---

// Null represents a null value.
type Null struct{}

// Type returns the type of the Null object.
func (n *Null) Type() ObjectType { return NULL_OBJ }

// Inspect returns a string representation of the Null's value.
func (n *Null) Inspect() string { return "null" }

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

// Function represents a user-defined function.
type Function struct {
	Name       *ast.Ident
	Parameters []*ast.Ident
	Body       *ast.BlockStmt
	Env        *Environment
}

// Type returns the type of the Function object.
func (f *Function) Type() ObjectType { return FUNCTION_OBJ }

// Inspect returns a string representation of the function.
func (f *Function) Inspect() string {
	var out bytes.Buffer

	params := []string{}
	for _, p := range f.Parameters {
		params = append(params, p.String())
	}

	out.WriteString("func")
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") { ... }")

	return out.String()
}

// --- Struct Definition Object ---

// StructDefinition represents the definition of a struct type.
type StructDefinition struct {
	Name   *ast.Ident
	Fields []*ast.Field
}

// Type returns the type of the StructDefinition object.
func (sd *StructDefinition) Type() ObjectType { return STRUCT_DEFINITION_OBJ }

// Inspect returns a string representation of the struct definition.
func (sd *StructDefinition) Inspect() string {
	return fmt.Sprintf("struct %s", sd.Name.String())
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
	NULL     = &Null{}
	BREAK    = &BreakStatement{}
	CONTINUE = &ContinueStatement{}
)

// --- Environment ---

// Environment holds the bindings for variables and functions.
type Environment struct {
	store  map[string]Object
	consts map[string]Object // For immutable bindings
	outer  *Environment
}

// NewEnvironment creates a new, top-level environment.
func NewEnvironment() *Environment {
	s := make(map[string]Object)
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
// It checks constants first, then variables.
func (e *Environment) Get(name string) (Object, bool) {
	if obj, ok := e.consts[name]; ok {
		return obj, true
	}
	if obj, ok := e.store[name]; ok {
		return obj, ok
	}
	if e.outer != nil {
		return e.outer.Get(name)
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
// It returns the object that was set. This is used for `var` and `:=`.
func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
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
	// Constants in the current scope cannot be reassigned.
	if _, ok := e.consts[name]; ok {
		return false
	}

	// If the variable exists in the current scope's store, assign it.
	if _, ok := e.store[name]; ok {
		e.store[name] = val
		return true
	}

	// If not found locally, try assigning in the outer scope.
	if e.outer != nil {
		return e.outer.Assign(name, val)
	}

	// The variable was not found in any scope.
	return false
}
