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

// Set stores a value by name in the current environment.
// It does not check outer scopes; it always sets in the current scope.
// This is typically used for 'var x = ...' or parameter binding.
// For assignment 'x = ...', one might want different semantics (e.g., update in existing scope).
func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
	return val
}

// TODO:
// - Consider methods for 'Define' (for 'var', which cannot re-declare in same scope)
//   vs 'Assign' (for 'x = y', which should find 'x' in current or outer scopes).
// - Constant handling.
// - Built-in variables/functions.
// - Scope resolution for function calls (closures).
// - Type information storage if the language becomes statically typed or for type checking.
