// Package ffibridge provides helper types for the Foreign Function Interface (FFI)
// between the MiniGo interpreter and native Go code.
//
// The core challenge in any FFI is bridging the gap between the interpreter's
// internal object model and the host language's type system. This package
// contains data structures that act as a "bridge" for complex scenarios.
//
// Currently, it provides a bridge for passing mutable pointers from MiniGo to Go,
// which is essential for functions like `json.Unmarshal`.
//
// In the future, this package could be extended to handle other complex FFI
// scenarios. For example, to support Go channels, a `ChannelBridge` could be
// created:
//
//	// ChannelBridge could wrap a Go channel, allowing MiniGo code to send
//	// and receive data from it. (NOT IMPLEMENTED)
//	type ChannelBridge struct {
//	    SourceChan reflect.Value // The native Go channel
//	    // ... methods to convert between Go and MiniGo values ...
//	}
//
// Other potential uses include:
// - Handling function pointers or callbacks from Go to MiniGo.
// - More complex memory management or ownership transfer helpers.
package ffibridge

import (
	"reflect"

	"github.com/podhmo/go-scan/minigo/object"
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
