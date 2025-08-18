package scope

import "github.com/podhmo/go-scan/symgo/object"

// Scope holds the bindings for variables and functions.
type Scope struct {
	store map[string]object.Object
	outer *Scope
}

// NewScope creates a new, top-level scope.
func NewScope() *Scope {
	s := make(map[string]object.Object)
	return &Scope{store: s, outer: nil}
}

// NewEnclosedScope creates a new scope that is enclosed by an outer one.
func NewEnclosedScope(outer *Scope) *Scope {
	scope := NewScope()
	scope.outer = outer
	return scope
}

// Get retrieves an object by name from the scope, checking outer scopes if necessary.
func (s *Scope) Get(name string) (object.Object, bool) {
	obj, ok := s.store[name]
	if !ok && s.outer != nil {
		obj, ok = s.outer.Get(name)
	}
	return obj, ok
}

// Set stores an object by name in the current scope.
func (s *Scope) Set(name string, val object.Object) object.Object {
	s.store[name] = val
	return val
}
