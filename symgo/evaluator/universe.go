package evaluator

import (
	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

type universeScope struct {
	functions *intrinsics.Registry
	values    map[string]object.Object
}

func (u *universeScope) GetFunction(name string) (intrinsics.IntrinsicFunc, bool) {
	return u.functions.Get(name)
}

func (u *universeScope) GetValue(name string) (object.Object, bool) {
	val, ok := u.values[name]
	return val, ok
}

var universe *universeScope

func init() {
	funcs := intrinsics.New()
	funcs.Register("panic", intrinsics.BuiltinPanic)

	vals := make(map[string]object.Object)
	vals["nil"] = &object.Nil{}
	vals["true"] = object.TRUE
	vals["false"] = object.FALSE

	universe = &universeScope{
		functions: funcs,
		values:    vals,
	}
}
