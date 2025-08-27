package intrinsics

import (
	"fmt"

	"github.com/podhmo/go-scan/symgo/object"
)

// BuiltinLen is the intrinsic function for the built-in `len`.
func BuiltinLen(args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{
			Message: fmt.Sprintf("wrong number of arguments for len. got=%d, want=1", len(args)),
		}
	}
	switch arg := args[0].(type) {
	case *object.String:
		return &object.Integer{Value: int64(len(arg.Value))}
	case *object.Slice:
		return &object.Integer{Value: int64(len(arg.Elements))}
	case *object.SymbolicPlaceholder:
		return &object.SymbolicPlaceholder{Reason: "len of symbolic value"}
	default:
		return &object.Error{
			Message: fmt.Sprintf("unsupported argument type for len: %s", arg.Type()),
		}
	}
}

// BuiltinCopy is the intrinsic function for the built-in `copy`.
func BuiltinCopy(args ...object.Object) object.Object {
	// copy(dst, src)
	// For symbolic execution, we don't need to perform a real copy.
	// We can return the number of elements copied, which is symbolically the length of the source.
	if len(args) != 2 {
		return &object.Error{
			Message: fmt.Sprintf("wrong number of arguments for copy. got=%d, want=2", len(args)),
		}
	}
	return BuiltinLen(args[1]) // Return len(src)
}

// BuiltinPanic is the intrinsic function for the built-in `panic`.
func BuiltinPanic(args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{
			Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)),
		}
	}
	// In symbolic execution, we treat panic as an error that stops execution.
	// The message of the panic is wrapped in an Error object.
	var msg string
	if str, ok := args[0].(*object.String); ok {
		msg = str.Value
	} else {
		msg = args[0].Inspect()
	}
	return &object.Error{
		Message: fmt.Sprintf("panic: %s", msg),
	}
}
