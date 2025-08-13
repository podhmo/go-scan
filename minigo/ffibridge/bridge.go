package ffibridge

import (
	"github.com/podhmo/go-scan/minigo/object"
	"reflect"
)

// Pointer is a bridge for passing MiniGo pointers to Go functions
// that expect to mutate their arguments (like json.Unmarshal).
type Pointer struct {
	// The original MiniGo pointer object that needs to be updated.
	Source *object.Pointer

	// A temporary, mutable Go value that the Go function can write to.
	// This will be a pointer to a Go type, e.g., *map[string]any.
	Dest reflect.Value
}
