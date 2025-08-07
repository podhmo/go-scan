package object

import "fmt"

// ObjectType is a string representation of an object's type.
type ObjectType string

// Define the basic object types. More will be added later.
const (
	INTEGER_OBJ  ObjectType = "INTEGER"
	BOOLEAN_OBJ  ObjectType = "BOOLEAN"
	STRING_OBJ   ObjectType = "STRING"
	NULL_OBJ     ObjectType = "NULL"
	BREAK_OBJ    ObjectType = "BREAK"
	CONTINUE_OBJ ObjectType = "CONTINUE"
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
