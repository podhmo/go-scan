package symgo

import (
	"context"
	"sync"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// TypeRelations manages the relationships between interfaces and their implementers.
// It is safe for concurrent use.
type TypeRelations struct {
	mu           sync.RWMutex
	resolver     scanner.PackageResolver
	interfaces   map[string]*scanner.TypeInfo
	structs      map[string]*scanner.TypeInfo
	implementers map[string][]*scanner.TypeInfo     // key: interface qualified name, value: list of implementing structs
	pendingCalls map[string][]*object.InterfaceCall // key: interface qualified name
}

// NewTypeRelations creates a new, empty TypeRelations registry.
func NewTypeRelations(resolver scanner.PackageResolver) *TypeRelations {
	return &TypeRelations{
		resolver:     resolver,
		interfaces:   make(map[string]*scanner.TypeInfo),
		structs:      make(map[string]*scanner.TypeInfo),
		implementers: make(map[string][]*scanner.TypeInfo),
		pendingCalls: make(map[string][]*object.InterfaceCall),
	}
}

// qualifiedName returns the fully qualified name for a type, which is used as a unique key.
func qualifiedName(t *scanner.TypeInfo) string {
	if t == nil {
		return ""
	}
	return t.PkgPath + "." + t.Name
}

// AddType registers a new type and checks for new implementation relationships.
// It returns a list of any newly discovered implementation pairs.
func (r *TypeRelations) AddType(ctx context.Context, t *scanner.TypeInfo) []object.ImplementationPair {
	r.mu.Lock()
	defer r.mu.Unlock()

	var newPairs []object.ImplementationPair

	qname := qualifiedName(t)
	if qname == "." { // Invalid type info
		return nil
	}

	if t.Kind == scanner.InterfaceKind {
		if _, exists := r.interfaces[qname]; exists {
			return nil // already registered
		}
		r.interfaces[qname] = t

		// Check if any existing structs implement this new interface.
		for _, s := range r.structs {
			if goscan.ImplementsContext(ctx, s, t, r.resolver) {
				r.implementers[qname] = append(r.implementers[qname], s)
				newPairs = append(newPairs, object.ImplementationPair{Struct: s, Interface: t})
			}
		}
	} else if t.Kind == scanner.StructKind {
		if _, exists := r.structs[qname]; exists {
			return nil // already registered
		}
		r.structs[qname] = t

		// Check if this new struct implements any existing interfaces.
		for iqname, i := range r.interfaces {
			if goscan.ImplementsContext(ctx, t, i, r.resolver) {
				r.implementers[iqname] = append(r.implementers[iqname], t)
				newPairs = append(newPairs, object.ImplementationPair{Struct: t, Interface: i})
			}
		}
	}
	return newPairs
}

// AddPendingCall records that a method was called on an interface.
func (r *TypeRelations) AddPendingCall(iface *scanner.TypeInfo, methodName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	qname := qualifiedName(iface)
	r.pendingCalls[qname] = append(r.pendingCalls[qname], &object.InterfaceCall{MethodName: methodName})
}

// GetPendingCalls returns all recorded calls for a given interface.
func (r *TypeRelations) GetPendingCalls(iface *scanner.TypeInfo) []*object.InterfaceCall {
	r.mu.RLock()
	defer r.mu.RUnlock()

	qname := qualifiedName(iface)
	calls := r.pendingCalls[qname]
	result := make([]*object.InterfaceCall, len(calls))
	copy(result, calls)
	return result
}

// GetImplementers returns all known structs that implement the given interface.
func (r *TypeRelations) GetImplementers(iface *scanner.TypeInfo) []*scanner.TypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Return a copy to avoid race conditions on the slice.
	result := make([]*scanner.TypeInfo, len(r.implementers[qualifiedName(iface)]))
	copy(result, r.implementers[qualifiedName(iface)])
	return result
}
