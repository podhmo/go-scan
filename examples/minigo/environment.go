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

// Set stores a value by name.
// For assignment semantics (like x = value):
// - If the variable exists in the current environment, update it.
// - Else, if it exists in an outer environment, update it there.
// - Else, create it in the current environment.
// For `var` declaration semantics, one would typically only set in the current scope
// and error if it already exists. This Set is more like Python's or JavaScript's assignment.
func (e *Environment) Set(name string, val Object) Object {
	// Try to find if the variable exists in the current or any outer scope.
	env := e
	for env != nil {
		if _, ok := env.store[name]; ok {
			// Variable found in this scope (or an outer scope that was previously current).
			env.store[name] = val
			return val
		}
		if env.outer == nil {
			// Reached the outermost scope and variable not found yet.
			break
		}
		env = env.outer
	}
	// If loop finished, 'name' was not found in any scope up to the outermost one checked by the loop.
	// Or, it was found and set.
	// If it was not found anywhere (env is now the outermost scope and it wasn't there, or loop broke earlier)
	// then we define it in the *original* current environment 'e'.
	// The above loop is for *updating*. If not updated, define in current.

	// Corrected logic:
	// 1. Check if it exists in the current scope. If so, update.
	// 2. Else, check outer scopes. If found, update the first one where it's found.
	// 3. Else (not found anywhere), create in the current scope.

	currentEnv := e
	for {
		if _, ok := currentEnv.store[name]; ok {
			currentEnv.store[name] = val // Found in this scope, update
			return val
		}
		if currentEnv.outer == nil {
			break // Reached outermost scope, not found
		}
		currentEnv = currentEnv.outer
	}
	// Not found in any scope, define in the original (innermost) scope.
	e.store[name] = val
	return val
}

// Define stores a value by name only in the current environment's store.
// It's used for `var` declarations, which should not affect outer scopes directly but can shadow them.
func (e *Environment) Define(name string, val Object) Object {
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
