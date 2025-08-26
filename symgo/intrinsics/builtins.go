package intrinsics

import (
	"fmt"

	"github.com/podhmo/go-scan/symgo/object"
)

// BuiltinPanic is the intrinsic function for the built-in `panic`.
func BuiltinPanic(args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{
			Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)),
		}
	}
	// In symbolic execution, we treat panic as an error that stops execution.
	// The message of the panic is wrapped in an Error object.
	return &object.Error{
		Message: fmt.Sprintf("panic: %s", args[0].Inspect()),
	}
}
