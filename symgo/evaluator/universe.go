package evaluator

import (
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

type universeScope struct {
	objects map[string]object.Object
}

func (u *universeScope) Get(name string) (object.Object, bool) {
	obj, ok := u.objects[name]
	return obj, ok
}

// Walk iterates over all items in the universe scope.
// If the callback function returns false, the walk is stopped.
func (u *universeScope) Walk(fn func(name string, obj object.Object) bool) {
	for name, obj := range u.objects {
		if !fn(name, obj) {
			return
		}
	}
}

var universe *universeScope

func init() {
	objects := make(map[string]object.Object)

	// Built-in Functions
	// We pre-create the Intrinsic objects here to avoid allocating them on the hot path in evalIdent.
	builtins := map[string]intrinsics.IntrinsicFunc{
		"panic":   intrinsics.BuiltinPanic,
		"make":    intrinsics.BuiltinMake,
		"append":  intrinsics.BuiltinAppend,
		"len":     intrinsics.BuiltinLen,
		"cap":     intrinsics.BuiltinCap,
		"new":     intrinsics.BuiltinNew,
		"copy":    intrinsics.BuiltinCopy,
		"delete":  intrinsics.BuiltinDelete,
		"close":   intrinsics.BuiltinClose,
		"clear":   intrinsics.BuiltinClear,
		"complex": intrinsics.BuiltinComplex,
		"real":    intrinsics.BuiltinReal,
		"imag":    intrinsics.BuiltinImag,
		"max":     intrinsics.BuiltinMax,
		"min":     intrinsics.BuiltinMin,
		"print":   intrinsics.BuiltinPrint,
		"println": intrinsics.BuiltinPrintln,
		"recover": intrinsics.BuiltinRecover,
	}
	for name, fn := range builtins {
		objects[name] = &object.Intrinsic{Fn: fn}
	}

	// Built-in Values
	objects["nil"] = object.NIL
	objects["true"] = object.TRUE
	objects["false"] = object.FALSE

	// Built-in Types
	for _, name := range []string{
		"any", "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128",
		"bool", "byte", "rune", "error",
	} {
		// For built-in types, we create a complete TypeInfo object.
		// This is crucial for the resolver and for generic type instantiation.
		typeInfo := &scanner.TypeInfo{
			Name: name,
		}
		if name == "error" || name == "any" {
			typeInfo.Kind = scanner.InterfaceKind
		} else {
			// There is no specific "BasicKind". Using UnknownKind for built-in
			// primitive types is sufficient for the evaluator's purposes.
			typeInfo.Kind = scanner.UnknownKind
		}

		objects[name] = &object.Type{
			TypeName:     name,
			ResolvedType: typeInfo,
		}
	}

	universe = &universeScope{
		objects: objects,
	}
}
