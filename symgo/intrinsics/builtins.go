package intrinsics

import (
	"context"
	"fmt"

	"github.com/podhmo/go-scan/symgo/object"
)

// BuiltinPanic is the intrinsic function for the built-in `panic`.
func BuiltinPanic(ctx context.Context, args ...object.Object) object.Object {
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

// BuiltinMake is the intrinsic function for the built-in `make`.
func BuiltinMake(ctx context.Context, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{
			Message: fmt.Sprintf("wrong number of arguments. got=%d, want=at least 1", len(args)),
		}
	}

	typeArg := args[0]
	if typeArg == nil {
		return &object.Error{Message: "make's first argument cannot be nil"}
	}

	fieldType := typeArg.FieldType()
	if fieldType == nil {
		// Fallback if type information is not available.
		return &object.SymbolicPlaceholder{Reason: "make(...) call with untyped arg"}
	}

	if fieldType.IsChan {
		ch := &object.Channel{
			ChanFieldType: fieldType,
		}
		ch.SetFieldType(fieldType)
		// The resolved type info for a channel is itself, essentially.
		// No deeper resolution is needed for the type itself.
		ch.SetTypeInfo(typeArg.TypeInfo())
		return ch
	}

	if fieldType.IsSlice {
		slice := &object.Slice{
			SliceFieldType: fieldType,
		}
		slice.SetFieldType(fieldType)
		slice.SetTypeInfo(typeArg.TypeInfo())
		return slice
	}

	if fieldType.IsMap {
		m := &object.Map{
			MapFieldType: fieldType,
		}
		m.SetFieldType(fieldType)
		m.SetTypeInfo(typeArg.TypeInfo())
		return m
	}

	// Fallback for other types or when type info is not available
	return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("make(%s) call", fieldType.String())}
}

// BuiltinAppend is the intrinsic function for the built-in `append`.
func BuiltinAppend(ctx context.Context, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "wrong number of arguments: append needs at least 1"}
	}
	// In symbolic execution, we just acknowledge the call and return a placeholder.
	return &object.SymbolicPlaceholder{Reason: "append(...) call"}
}

// BuiltinLen is the intrinsic function for the built-in `len`.
func BuiltinLen(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: len expects 1"}
	}
	return &object.SymbolicPlaceholder{Reason: "len(...) call"}
}

// BuiltinCap is the intrinsic function for the built-in `cap`.
func BuiltinCap(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: cap expects 1"}
	}
	return &object.SymbolicPlaceholder{Reason: "cap(...) call"}
}

// BuiltinNew is the intrinsic function for the built-in `new`.
func BuiltinNew(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: new expects 1"}
	}
	return &object.SymbolicPlaceholder{Reason: "new(...) call"}
}

// BuiltinCopy is the intrinsic function for the built-in `copy`.
func BuiltinCopy(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 2 {
		return &object.Error{Message: "wrong number of arguments: copy expects 2"}
	}
	return &object.SymbolicPlaceholder{Reason: "copy(...) call"} // Returns int
}

// BuiltinDelete is the intrinsic function for the built-in `delete`.
func BuiltinDelete(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 2 {
		return &object.Error{Message: "wrong number of arguments: delete expects 2"}
	}
	return object.NIL // delete does not return a value
}

// BuiltinClose is the intrinsic function for the built-in `close`.
func BuiltinClose(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: close expects 1"}
	}
	return object.NIL // close does not return a value
}

// BuiltinClear is the intrinsic function for the built-in `clear`.
func BuiltinClear(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: clear expects 1"}
	}
	return object.NIL // clear does not return a value
}

// BuiltinComplex is the intrinsic function for the built-in `complex`.
func BuiltinComplex(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 2 {
		return &object.Error{Message: "wrong number of arguments: complex expects 2"}
	}
	return &object.SymbolicPlaceholder{Reason: "complex(...) call"}
}

// BuiltinReal is the intrinsic function for the built-in `real`.
func BuiltinReal(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: real expects 1"}
	}
	return &object.SymbolicPlaceholder{Reason: "real(...) call"}
}

// BuiltinImag is the intrinsic function for the built-in `imag`.
func BuiltinImag(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: imag expects 1"}
	}
	return &object.SymbolicPlaceholder{Reason: "imag(...) call"}
}

// BuiltinMax is the intrinsic function for the built-in `max`.
func BuiltinMax(ctx context.Context, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Error{Message: "wrong number of arguments: max expects at least 1"}
	}
	return &object.SymbolicPlaceholder{Reason: "max(...) call"}
}

// BuiltinMin is the intrinsic function for the built-in `min`.
func BuiltinMin(ctx context.Context, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Error{Message: "wrong number of arguments: min expects at least 1"}
	}
	return &object.SymbolicPlaceholder{Reason: "min(...) call"}
}

// BuiltinPrint is the intrinsic function for the built-in `print`.
func BuiltinPrint(ctx context.Context, args ...object.Object) object.Object {
	// In symbolic execution, we can ignore this.
	return object.NIL
}

// BuiltinPrintln is the intrinsic function for the built-in `println`.
func BuiltinPrintln(ctx context.Context, args ...object.Object) object.Object {
	// In symbolic execution, we can ignore this.
	return object.NIL
}

// BuiltinRecover is the intrinsic function for the built-in `recover`.
func BuiltinRecover(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 0 {
		return &object.Error{Message: "wrong number of arguments: recover expects 0"}
	}
	// recover() returns nil if the goroutine is not panicking.
	// For symbolic execution, assuming it's not panicking is a safe default.
	return object.NIL
}
