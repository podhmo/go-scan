package scanner

import (
	"go/token"
	"sync"
)

// IdentityMap provides a session-wide cache to ensure that each AST declaration
// maps to a single, unique semantic object (*FunctionInfo, *TypeInfo, etc.).
// This ensures that consumers of the scanner can rely on pointer equality for
// objects that represent the same piece of code.
type IdentityMap struct {
	mu        sync.RWMutex
	Functions map[token.Pos]*FunctionInfo
	Types     map[token.Pos]*TypeInfo
}

// NewIdentityMap creates a new, initialized IdentityMap.
func NewIdentityMap() *IdentityMap {
	return &IdentityMap{
		Functions: make(map[token.Pos]*FunctionInfo),
		Types:     make(map[token.Pos]*TypeInfo),
	}
}

// GetFunction retrieves a FunctionInfo from the map by its declaration position.
// It returns the function info and true if found, otherwise nil and false.
func (m *IdentityMap) GetFunction(pos token.Pos) (*FunctionInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	fn, ok := m.Functions[pos]
	return fn, ok
}

// SetFunction adds or updates a FunctionInfo in the map.
func (m *IdentityMap) SetFunction(pos token.Pos, fn *FunctionInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Functions[pos] = fn
}

// GetType retrieves a TypeInfo from the map by its declaration position.
// It returns the type info and true if found, otherwise nil and false.
func (m *IdentityMap) GetType(pos token.Pos) (*TypeInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	typ, ok := m.Types[pos]
	return typ, ok
}

// SetType adds or updates a TypeInfo in the map.
func (m *IdentityMap) SetType(pos token.Pos, typ *TypeInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Types[pos] = typ
}