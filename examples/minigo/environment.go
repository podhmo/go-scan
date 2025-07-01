package main

// Environment stores variables and their values, and handles scope.
type Environment struct {
	store map[string]Object
	outer *Environment // For lexical scoping (enclosing environment)
}

// NewEnvironment creates a new Environment. If 'outer' is nil, it's a global environment.
func NewEnvironment(outer *Environment) *Environment {
	s := make(map[string]Object)
	return &Environment{store: s, outer: outer}
}

// Get retrieves a value by name from the current environment or its outer scopes.
func (e *Environment) Get(name string) (Object, bool) {
	obj, ok := e.store[name]
	if !ok && e.outer != nil { // If not found here, try the outer scope
		obj, ok = e.outer.Get(name)
	}
	return obj, ok
}

// Define binds a name to a value in the current environment only.
// This is used for variable declarations (`var x = ...`) and function parameters.
// It does not check outer scopes.
func (e *Environment) Define(name string, val Object) Object {
	e.store[name] = val
	return val
}

// Assign updates the value of an existing variable.
// It searches for the variable in the current environment and then in outer scopes.
// If the variable is found, it's updated, and (val, true) is returned.
// If the variable is not found in any scope, (nil, false) is returned, indicating an error.
func (e *Environment) Assign(name string, val Object) (Object, bool) {
	if _, ok := e.store[name]; ok {
		e.store[name] = val
		return val, true
	}
	if e.outer != nil {
		return e.outer.Assign(name, val)
	}
	return nil, false // Variable not found in any scope
}

// ExistsInCurrentScope checks if a name is defined in the current environment's store.
// It does not check outer scopes.
func (e *Environment) ExistsInCurrentScope(name string) bool {
	_, ok := e.store[name]
	return ok
}

// TODO:
// - Constant handling.
// - Constant handling.
// - Built-in variables/functions.
// - Scope resolution for function calls (closures).
// - Type information storage if the language becomes statically typed or for type checking.
