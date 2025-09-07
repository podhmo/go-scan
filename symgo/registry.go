package symgo

import (
	"context"
	"fmt"
	"sync"

	"github.com/podhmo/go-scan/scanner"
)

// InterfaceInfo holds the state for a single interface type.
type InterfaceInfo struct {
	// The type information for the interface itself.
	Interface *scanner.TypeInfo

	// A map of method names that have been called on this interface.
	// The value is true if the method has been called.
	CalledMethods map[string]bool

	// A slice of concrete types that are known to implement this interface.
	Implementations []*scanner.TypeInfo
}

// InterfaceRegistry tracks all known interfaces, their implementations,
// and the methods that have been called on them. It is safe for concurrent use.
type InterfaceRegistry struct {
	mu         sync.RWMutex
	// interfaces maps a fully qualified interface name (ID) to its info.
	interfaces map[string]*InterfaceInfo
}

// NewInterfaceRegistry creates a new, empty registry.
func NewInterfaceRegistry() *InterfaceRegistry {
	return &InterfaceRegistry{
		interfaces: make(map[string]*InterfaceInfo),
	}
}

// getInterfaceID returns the fully qualified ID for an interface type.
func getInterfaceID(ifaceType *scanner.TypeInfo) string {
	if ifaceType == nil || ifaceType.PkgPath == "" {
		return "" // Should not happen for valid types
	}
	return fmt.Sprintf("%s.%s", ifaceType.PkgPath, ifaceType.Name)
}

// registerInterface is the internal, non-locking version of RegisterInterface.
func (r *InterfaceRegistry) registerInterface(ifaceType *scanner.TypeInfo) *InterfaceInfo {
	id := getInterfaceID(ifaceType)
	if id == "" {
		return nil
	}

	if info, exists := r.interfaces[id]; exists {
		return info
	}

	info := &InterfaceInfo{
		Interface:     ifaceType,
		CalledMethods: make(map[string]bool),
		Implementations: []*scanner.TypeInfo{},
	}
	r.interfaces[id] = info
	return info
}

// RegisterInterface ensures an interface is tracked in the registry.
func (r *InterfaceRegistry) RegisterInterface(ctx context.Context, ifaceType *scanner.TypeInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerInterface(ifaceType)
}


// OnImplementationFound registers a concrete type as an implementation of an interface.
// It returns the list of methods that have already been called on the interface,
// so the evaluator can retroactively apply them to the new implementation.
func (r *InterfaceRegistry) OnImplementationFound(ctx context.Context, ifaceType, implType *scanner.TypeInfo) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	info := r.registerInterface(ifaceType) // Ensure the interface is tracked
	if info == nil {
		return nil
	}

	// Avoid duplicate registrations
	for _, existingImpl := range info.Implementations {
		// This should be a robust check based on type ID.
		if getInterfaceID(existingImpl) == getInterfaceID(implType) {
			return nil
		}
	}

	info.Implementations = append(info.Implementations, implType)

	// Return list of already called methods for retroactive analysis
	called := make([]string, 0, len(info.CalledMethods))
	for method := range info.CalledMethods {
		called = append(called, method)
	}
	return called
}

// OnInterfaceMethodCall records that a method was called on an interface.
// It returns the list of concrete types that currently implement the interface,
// so the evaluator can apply the call to them.
func (r *InterfaceRegistry) OnInterfaceMethodCall(ctx context.Context, ifaceType *scanner.TypeInfo, methodName string) []*scanner.TypeInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	info := r.registerInterface(ifaceType)
	if info == nil {
		return nil
	}

	info.CalledMethods[methodName] = true

	// Return a copy of the current implementations
	impls := make([]*scanner.TypeInfo, len(info.Implementations))
	copy(impls, info.Implementations)
	return impls
}

// GetAllInterfaces returns all tracked interfaces.
func (r *InterfaceRegistry) GetAllInterfaces(ctx context.Context) []*scanner.TypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*scanner.TypeInfo, 0, len(r.interfaces))
	for _, info := range r.interfaces {
		all = append(all, info.Interface)
	}
	return all
}
