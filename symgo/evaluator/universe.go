package evaluator

import (
	"fmt"

	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

type universeScope struct {
	functions *intrinsics.Registry
	values    map[string]object.Object
	types     map[string]object.Object
}

func (u *universeScope) GetFunction(name string) (intrinsics.IntrinsicFunc, bool) {
	return u.functions.Get(name)
}

func (u *universeScope) GetValue(name string) (object.Object, bool) {
	val, ok := u.values[name]
	return val, ok
}

func (u *universeScope) GetType(name string) (object.Object, bool) {
	t, ok := u.types[name]
	return t, ok
}

var universe *universeScope

func init() {
	funcs := intrinsics.New()
	funcs.Register("panic", intrinsics.BuiltinPanic)

	vals := make(map[string]object.Object)
	vals["nil"] = object.NIL
	vals["true"] = object.TRUE
	vals["false"] = object.FALSE

	types := make(map[string]object.Object)
	for _, name := range []string{
		"string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128",
		"bool", "byte", "rune", "error",
	} {
		types[name] = &object.SymbolicPlaceholder{Reason: fmt.Sprintf("built-in type %s", name)}
	}

	universe = &universeScope{
		functions: funcs,
		values:    vals,
		types:     types,
	}
}
