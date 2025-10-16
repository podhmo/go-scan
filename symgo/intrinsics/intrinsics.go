package intrinsics

import (
	"context"

	"github.com/podhmo/go-scan/symgo/object"
)

// IntrinsicFunc is the type for a function that can be registered as an intrinsic.
type IntrinsicFunc func(ctx context.Context, args ...object.Object) object.Object

// Registry holds the registered intrinsic functions in a layered stack.
// This allows for temporary intrinsics to be pushed for a specific scope
// and then popped, restoring the previous state.
type Registry struct {
	layers []map[string]IntrinsicFunc
}

// New creates a new, empty registry with a single base layer.
func New() *Registry {
	return &Registry{
		layers: []map[string]IntrinsicFunc{make(map[string]IntrinsicFunc)},
	}
}

// Register adds a new intrinsic function to the top-most layer of the registry.
// The key is typically the fully qualified function name (e.g., "fmt.Sprintf").
func (r *Registry) Register(key string, fn IntrinsicFunc) {
	topLayer := r.layers[len(r.layers)-1]
	topLayer[key] = fn
}

// Get retrieves an intrinsic function from the registry by its key, searching
// from the top-most layer down to the base layer.
func (r *Registry) Get(key string) (IntrinsicFunc, bool) {
	for i := len(r.layers) - 1; i >= 0; i-- {
		if fn, ok := r.layers[i][key]; ok {
			return fn, true
		}
	}
	return nil, false
}

// Push adds a new, empty layer to the top of the registry stack.
// This is used to create a new scope for temporary intrinsics.
func (r *Registry) Push() {
	r.layers = append(r.layers, make(map[string]IntrinsicFunc))
}

// Pop removes the top-most layer from the registry stack.
// It does not remove the base layer.
func (r *Registry) Pop() {
	if len(r.layers) > 1 {
		r.layers = r.layers[:len(r.layers)-1]
	}
}
