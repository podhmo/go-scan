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
	// In symbolic execution, we treat panic as a distinct control flow object.
	return &object.PanicError{Value: args[0]}
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
		var length, capacity int64
		if len(args) < 2 {
			return &object.Error{Message: "make for slice expects at least 2 arguments"}
		}
		if l, ok := args[1].(*object.Integer); ok {
			length = l.Value
		} else {
			// If length is not a constant, it's symbolic.
			length = -1 // Use -1 to indicate symbolic length
		}

		if len(args) >= 3 {
			if c, ok := args[2].(*object.Integer); ok {
				capacity = c.Value
			} else {
				capacity = -1 // Symbolic capacity
			}
		} else {
			capacity = length // If cap is omitted, it's equal to len.
		}

		slice := &object.Slice{
			SliceFieldType: fieldType,
			Len:            length,
			Cap:            capacity,
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

	// If the argument is a return value, unwrap it to get the actual object.
	if ret, ok := args[0].(*object.ReturnValue); ok {
		args[0] = ret.Value
	}

	arg := args[0]
	// Recursively unwrap variables to get to the underlying object.
	for {
		v, ok := arg.(*object.Variable)
		if !ok {
			break
		}
		arg = v.Value
	}

	switch arg := arg.(type) {
	case *object.Slice:
		// If the slice itself has a concrete length, use it.
		if arg.Len >= 0 {
			return &object.Integer{Value: arg.Len}
		}
		// Otherwise, it's symbolic.
		return &object.SymbolicPlaceholder{Reason: "len on symbolic slice"}
	case *object.String:
		return &object.Integer{Value: int64(len(arg.Value))}
	case *object.Map:
		return &object.Integer{Value: int64(len(arg.Pairs))}
	case *object.SymbolicPlaceholder:
		// If we have a symbolic placeholder with length info (e.g., from an out-of-policy `make`), use it.
		if arg.Len != -1 {
			return &object.Integer{Value: arg.Len}
		}
		return &object.SymbolicPlaceholder{Reason: "len on symbolic value"}
	case *object.UnresolvedFunction:
		// This can happen if `len` is called on a variable from an unscanned
		// package that is mis-identified as a function. Instead of crashing,
		// return a symbolic placeholder for the length.
		return &object.SymbolicPlaceholder{Reason: "len on unresolved function"}
	default:
		return &object.Error{Message: fmt.Sprintf("argument to `len` not supported, got %s", arg.Type())}
	}
}

// BuiltinCap is the intrinsic function for the built-in `cap`.
func BuiltinCap(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: cap expects 1"}
	}
	arg := args[0]
	// Recursively unwrap variables to get to the underlying object.
	for {
		v, ok := arg.(*object.Variable)
		if !ok {
			break
		}
		arg = v.Value
	}

	switch arg := arg.(type) {
	case *object.Slice:
		if arg.Cap >= 0 {
			return &object.Integer{Value: arg.Cap}
		}
		return &object.SymbolicPlaceholder{Reason: "cap on symbolic slice"}
	case *object.SymbolicPlaceholder:
		if arg.Cap != -1 {
			return &object.Integer{Value: arg.Cap}
		}
		return &object.SymbolicPlaceholder{Reason: "cap on symbolic value"}
	default:
		return &object.Error{Message: fmt.Sprintf("argument to `cap` not supported, got %s", arg.Type())}
	}
}

// BuiltinNew is the intrinsic function for the built-in `new`.
func BuiltinNew(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: "wrong number of arguments: new expects 1"}
	}

	typeArg := args[0]
	var pointee object.Object

	switch t := typeArg.(type) {
	case *object.Type:
		// For a resolved type, create a symbolic instance of it.
		instance := &object.Instance{
			TypeName: t.TypeName,
		}
		instance.SetTypeInfo(t.ResolvedType)
		pointee = instance

	case *object.UnresolvedType:
		// For an unresolved type, create a symbolic placeholder for an instance of it.
		// This placeholder can then be used in subsequent operations.
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("instance of unresolved type %s.%s", t.PkgPath, t.TypeName),
		}
		pointee = placeholder

	case *object.UnresolvedFunction:
		// If we try to new an unresolved function type, it's valid. We can't know
		// the "zero value" but we can return a placeholder for the pointer's pointee.
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("instance of unresolved function %s.%s", t.PkgPath, t.FuncName),
		}
		pointee = placeholder

	default:
		// Fallback for other types, like a symbolic placeholder representing a type.
		if _, ok := typeArg.(*object.SymbolicPlaceholder); ok {
			// If `new` is called on a placeholder, return a pointer to another placeholder.
			return &object.Pointer{Value: &object.SymbolicPlaceholder{Reason: "pointer to " + typeArg.Inspect()}}
		}
		return &object.Error{Message: fmt.Sprintf("invalid argument for new: expected a type, got %s", typeArg.Type())}
	}

	// The `new` built-in function returns a pointer to the allocated object.
	return &object.Pointer{Value: pointee}
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
