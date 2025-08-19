package intrinsics

import "github.com/podhmo/go-scan/symgo/object"

// IntrinsicFunc is the type for a function that can be registered as an intrinsic.
type IntrinsicFunc func(args ...object.Object) object.Object

// Registry holds the registered intrinsic functions.
type Registry struct {
	store map[string]IntrinsicFunc
}

// New creates a new, empty registry.
func New() *Registry {
	return &Registry{store: make(map[string]IntrinsicFunc)}
}

// Register adds a new intrinsic function to the registry.
// The key is typically the fully qualified function name (e.g., "fmt.Sprintf").
func (r *Registry) Register(key string, fn IntrinsicFunc) {
	r.store[key] = fn
}

// Get retrieves an intrinsic function from the registry by its key.
func (r *Registry) Get(key string) (IntrinsicFunc, bool) {
	fn, ok := r.store[key]
	return fn, ok
}
