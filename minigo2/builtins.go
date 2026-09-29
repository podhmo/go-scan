package minigo2

import (
	"fmt"

	"github.com/podhmo/go-scan/minigo2/runtime"
)

// builtins returns the predeclared universe: builtin functions and builtin
// type names (as *TypeDef values so `int(x)` is a normal conversion call).
func builtins() *runtime.Env {
	env := runtime.NewEnv()

	bf := func(name string, fn func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error)) {
		env.Set(name, &runtime.BuiltinFunc{Name: name, Fn: fn})
	}

	bf("len", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		switch x := args[0].(type) {
		case *runtime.Cell:
			return lenOf(x.Elem)
		default:
			return lenOf(x)
		}
	})
	bf("cap", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		return lenOf(args[0])
	})
	bf("append", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		s, ok := args[0].(*runtime.Slice)
		if !ok {
			if args[0] == runtime.NIL {
				s = &runtime.Slice{}
			} else {
				return nil, fmt.Errorf("append on %T", args[0])
			}
		}
		return &runtime.Slice{Elems: append(append([]runtime.Value{}, s.Elems...), args[1:]...)}, nil
	})
	bf("copy", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		dst, ok1 := args[0].(*runtime.Slice)
		src, ok2 := args[1].(*runtime.Slice)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("copy on non-slice")
		}
		n := copy(dst.Elems, src.Elems)
		return int64(n), nil
	})
	bf("delete", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		m, ok := args[0].(*runtime.Map)
		if !ok {
			return nil, fmt.Errorf("delete on %T", args[0])
		}
		delete(m.Pairs, args[1])
		return runtime.NIL, nil
	})
	bf("make", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		td, ok := args[0].(*runtime.TypeDef)
		if !ok {
			return nil, fmt.Errorf("make of non-type %T", args[0])
		}
		switch td.Kind {
		case runtime.KindSlice:
			n := int64(0)
			if len(args) > 1 {
				n = args[1].(int64)
			}
			el := make([]runtime.Value, n)
			for i := range el {
				el[i] = runtime.NIL
			}
			return &runtime.Slice{Elems: el}, nil
		case runtime.KindMap:
			return &runtime.Map{Pairs: map[runtime.Value]runtime.Value{}}, nil
		case runtime.KindChan:
			// buffer capacity is not modeled: sends never block
			return &runtime.Chan{}, nil
		default:
			return nil, fmt.Errorf("make of kind %d", td.Kind)
		}
	})
	bf("new", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		return &runtime.Cell{Elem: runtime.NIL}, nil
	})
	bf("close", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		ch, ok := args[0].(*runtime.Chan)
		if !ok {
			if c, isCell := args[0].(*runtime.Cell); isCell {
				ch, ok = c.Elem.(*runtime.Chan)
			}
			if !ok {
				return nil, fmt.Errorf("close of non-channel %T", args[0])
			}
		}
		if ch.Closed {
			panic(&runtime.Panic{Value: "close of closed channel"})
		}
		ch.Closed = true
		return runtime.NIL, nil
	})
	bf("panic", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		panic(&runtime.Panic{Value: args[0]})
	})
	bf("recover", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		return v.Recover(), nil
	})
	bf("print", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		for _, a := range args {
			fmt.Print(display(a))
		}
		return runtime.NIL, nil
	})
	bf("println", func(v runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		parts := make([]any, len(args))
		for i, a := range args {
			parts[i] = display(a)
		}
		fmt.Println(parts...)
		return runtime.NIL, nil
	})

	// builtin type names
	for _, n := range []string{
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "string", "bool", "byte", "rune",
	} {
		env.Set(n, &runtime.TypeDef{Name: n, Kind: runtime.KindNamedBasic})
	}
	// any / error: predeclared interface typedefs (assertion + decl targets)
	env.Set("any", &runtime.TypeDef{Name: "any", Kind: runtime.KindInterface})
	env.Set("error", &runtime.TypeDef{Name: "error", Kind: runtime.KindInterface, MReqs: []string{"Error"}})
	return env
}

func lenOf(v runtime.Value) (runtime.Value, error) {
	switch x := v.(type) {
	case *runtime.Slice:
		return int64(len(x.Elems)), nil
	case *runtime.Map:
		return int64(len(x.Pairs)), nil
	case *runtime.Chan:
		return int64(len(x.Elems)), nil
	case string:
		return int64(len(x)), nil
	case runtime.Nil:
		return int64(0), nil // len(nil slice/map/chan) == 0
	default:
		return nil, fmt.Errorf("len of %T", v)
	}
}

func display(v runtime.Value) any {
	switch x := v.(type) {
	case runtime.Nil:
		return nil
	case *runtime.Cell:
		return display(x.Elem)
	case *runtime.Slice:
		parts := make([]any, len(x.Elems))
		for i, e := range x.Elems {
			parts[i] = display(e)
		}
		return parts
	case *runtime.Struct:
		return fmt.Sprintf("%s%+v", x.Def.Name, x.Fields)
	default:
		return x
	}
}
