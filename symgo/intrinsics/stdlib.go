package intrinsics

import (
	"context"
	"fmt"
	"strings"

	"github.com/podhmo/go-scan/symgo/object"
)

const (
	ReasonErrorsNew  = "value from errors.New"
	ReasonFmtErrorf  = "value from fmt.Errorf"
	ReasonErrorsAs   = "result of errors.As"
	ReasonPossibleAs = "possible result of errors.As into "
)

// unwrapVariable recursively unwraps a variable to get to its underlying value.
func unwrapVariable(obj object.Object) object.Object {
	for {
		if v, ok := obj.(*object.Variable); ok {
			if v.Value == nil {
				return obj // Return the unevaluated variable itself
			}
			obj = v.Value
		} else if rv, ok := obj.(*object.ReturnValue); ok {
			obj = rv.Value
		} else {
			break
		}
	}
	return obj
}

// unwrapErrorPlaceholder follows the chain of wrapped errors via the Underlying field
// of SymbolicPlaceholders that represent error values.
func unwrapErrorPlaceholder(obj object.Object) object.Object {
	sp, ok := obj.(*object.SymbolicPlaceholder)
	if !ok || (sp.Reason != ReasonErrorsNew && sp.Reason != ReasonFmtErrorf) {
		return nil
	}
	return sp.Underlying
}

// StdlibErrorsNew implements the symbolic behavior of errors.New.
func StdlibErrorsNew(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: fmt.Sprintf("wrong number of arguments for errors.New: got=%d, want=1", len(args))}
	}
	msg := unwrapVariable(args[0])
	return &object.SymbolicPlaceholder{
		Reason:     ReasonErrorsNew,
		Underlying: msg, // Store the string object
	}
}

// StdlibErrorsIs implements the symbolic behavior of errors.Is.
func StdlibErrorsIs(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 2 {
		return &object.Error{Message: fmt.Sprintf("wrong number of arguments for errors.Is: got=%d, want=2", len(args))}
	}

	errVal := unwrapVariable(args[0])
	targetVal := unwrapVariable(args[1])

	// If either is a generic symbolic value, the result is also symbolic.
	if sp, ok := errVal.(*object.SymbolicPlaceholder); ok && sp.Reason != ReasonFmtErrorf && sp.Reason != ReasonErrorsNew {
		return &object.SymbolicPlaceholder{Reason: "errors.Is on generic symbolic error"}
	}
	if sp, ok := targetVal.(*object.SymbolicPlaceholder); ok && sp.Reason != ReasonFmtErrorf && sp.Reason != ReasonErrorsNew {
		return &object.SymbolicPlaceholder{Reason: "errors.Is with generic symbolic target"}
	}

	// Walk the error chain.
	currentErr := errVal
	for currentErr != nil {
		if currentErr == targetVal {
			return object.TRUE
		}
		currentErr = unwrapErrorPlaceholder(currentErr)
	}

	return object.FALSE
}

// StdlibErrorsAs implements the symbolic behavior of errors.As.
func StdlibErrorsAs(ctx context.Context, args ...object.Object) object.Object {
	if len(args) != 2 {
		return &object.Error{Message: fmt.Sprintf("wrong number of arguments for errors.As: got=%d, want=2", len(args))}
	}

	arg1 := unwrapVariable(args[1])
	targetPtr, ok := arg1.(*object.Pointer)
	if !ok {
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("errors.As target was not a pointer, but %T", arg1)}
	}

	targetVar, ok := targetPtr.Value.(*object.Variable)
	if !ok {
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("errors.As pointer does not point to a variable, but to %T", targetPtr.Value)}
	}


	newVal := &object.SymbolicPlaceholder{
		Reason: ReasonPossibleAs + targetVar.Name,
	}
	newVal.SetFieldType(targetVar.FieldType())
	newVal.SetTypeInfo(targetVar.TypeInfo())
	targetVar.Value = newVal

	return &object.SymbolicPlaceholder{Reason: ReasonErrorsAs}
}

// StdlibFmtErrorf implements the symbolic behavior of fmt.Errorf.
func StdlibFmtErrorf(ctx context.Context, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: fmt.Sprintf("wrong number of arguments for fmt.Errorf: got=%d, want at least 1", len(args))}
	}

	format, ok := unwrapVariable(args[0]).(*object.String)
	if !ok {
		return &object.SymbolicPlaceholder{Reason: "fmt.Errorf with dynamic format string"}
	}

	var wrappedErr object.Object
	if strings.Contains(format.Value, "%w") {
		if len(args) < 2 {
			return &object.Error{Message: "fmt.Errorf with %w requires a wrapped error argument"}
		}
		wrappedErr = args[len(args)-1]
	}

	return &object.SymbolicPlaceholder{
		Reason:     ReasonFmtErrorf,
		Underlying: wrappedErr, // This will be nil if there's no wrapped error
	}
}