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

// GetAllEntries returns all entries from the current environment and its outer scopes.
// Entries in inner scopes shadow those in outer scopes.
func (e *Environment) GetAllEntries() map[string]Object {
	allEntries := make(map[string]Object)

	// Collect entries from outer scopes first
	if e.outer != nil {
		outerEntries := e.outer.GetAllEntries()
		for name, obj := range outerEntries {
			allEntries[name] = obj
		}
	}

	// Add/overwrite with entries from the current scope
	for name, obj := range e.store {
		allEntries[name] = obj
	}
	return allEntries
}

// SetType defines a type in the current environment.
// Type definitions (like StructDefinition or DefinedType) are also Objects.
func (e *Environment) SetType(name string, typeDef Object) {
	// For now, types are stored in the same map as variables/functions.
	// Consider a separate map if type namespace needs to be distinct.
	e.store[name] = typeDef
}

// ResolveType retrieves a type definition by name from the environment.
// It checks the current and outer scopes.
func (e *Environment) ResolveType(name string) (Object, bool) {
	// For now, types are retrieved using the same mechanism as variables/functions.
	// The caller is responsible for asserting the Object is a type definition (e.g., *StructDefinition, *DefinedType).
	obj, ok := e.Get(name)
	if !ok {
		return nil, false
	}

	// Basic check: is the object something that represents a type?
	switch obj.(type) {
	case *StructDefinition, *DefinedType:
		return obj, true
	default:
		// It's some other kind of object (e.g., an Integer variable with the same name as a potential type)
		// This indicates a name collision or misuse if a type was expected.
		// For now, we return it and let the caller decide.
		// A stricter ResolveType might return (nil, false) here.
		return obj, true // Found an object, but maybe not a type definition. Caller must verify.
	}
}
