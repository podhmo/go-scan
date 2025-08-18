package stdslices

import (
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	"go/token"
	"sort"
)

// Install registers the native `slices` functions with the minigo interpreter.
func Install(interp *minigo.Interpreter) {
	interp.Register("slices", map[string]any{
		"Sort":    builtinSort(),
		"Clone":   builtinClone(),
		"Equal":   builtinEqual(),
		"Compare": builtinCompare(),
	})
}

func builtinSort() *object.Builtin {
	return &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments for slices.Sort, got=%d, want=1", len(args))
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return ctx.NewError(pos, "argument to slices.Sort must be a slice, got %s", args[0].Type())
			}
			if len(arr.Elements) == 0 {
				return object.NIL // sorting empty slice is a no-op
			}

			// Use sort.Slice which is flexible. We compare based on the type of the first element.
			switch arr.Elements[0].(type) {
			case *object.Integer:
				sort.Slice(arr.Elements, func(i, j int) bool {
					return arr.Elements[i].(*object.Integer).Value < arr.Elements[j].(*object.Integer).Value
				})
			case *object.Float:
				sort.Slice(arr.Elements, func(i, j int) bool {
					return arr.Elements[i].(*object.Float).Value < arr.Elements[j].(*object.Float).Value
				})
			case *object.String:
				sort.Slice(arr.Elements, func(i, j int) bool {
					return arr.Elements[i].(*object.String).Value < arr.Elements[j].(*object.String).Value
				})
			default:
				return ctx.NewError(pos, "slices.Sort not supported for slice of %s", arr.Elements[0].Type())
			}
			return object.NIL
		},
	}
}

func builtinClone() *object.Builtin {
	return &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments for slices.Clone, got=%d, want=1", len(args))
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return ctx.NewError(pos, "argument to slices.Clone must be a slice, got %s", args[0].Type())
			}

			newElements := make([]object.Object, len(arr.Elements))
			copy(newElements, arr.Elements)
			return &object.Array{Elements: newElements}
		},
	}
}

func builtinEqual() *object.Builtin {
	return &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 2 {
				return ctx.NewError(pos, "wrong number of arguments for slices.Equal, got=%d, want=2", len(args))
			}
			arr1, ok1 := args[0].(*object.Array)
			arr2, ok2 := args[1].(*object.Array)
			if !ok1 || !ok2 {
				return ctx.NewError(pos, "arguments to slices.Equal must be slices")
			}

			if len(arr1.Elements) != len(arr2.Elements) {
				return object.FALSE
			}

			for i := range arr1.Elements {
				// This is a simplified equality check. It may not work for all object types.
				// It relies on the basic types having value equality.
				if !isObjectEqual(arr1.Elements[i], arr2.Elements[i]) {
					return object.FALSE
				}
			}

			return object.TRUE
		},
	}
}

func builtinCompare() *object.Builtin {
	return &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 2 {
				return ctx.NewError(pos, "wrong number of arguments for slices.Compare, got=%d, want=2", len(args))
			}
			arr1, ok1 := args[0].(*object.Array)
			arr2, ok2 := args[1].(*object.Array)
			if !ok1 || !ok2 {
				return ctx.NewError(pos, "arguments to slices.Compare must be slices")
			}

			minLen := len(arr1.Elements)
			if len(arr2.Elements) < minLen {
				minLen = len(arr2.Elements)
			}

			for i := 0; i < minLen; i++ {
				cmp := compareObjects(arr1.Elements[i], arr2.Elements[i])
				if cmp != 0 {
					return &object.Integer{Value: int64(cmp)}
				}
			}

			if len(arr1.Elements) < len(arr2.Elements) {
				return &object.Integer{Value: -1}
			}
			if len(arr1.Elements) > len(arr2.Elements) {
				return &object.Integer{Value: 1}
			}
			return &object.Integer{Value: 0}
		},
	}
}

func isObjectEqual(a, b object.Object) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch a := a.(type) {
	case *object.Integer:
		return a.Value == b.(*object.Integer).Value
	case *object.Float:
		return a.Value == b.(*object.Float).Value
	case *object.String:
		return a.Value == b.(*object.String).Value
	case *object.Boolean:
		return a.Value == b.(*object.Boolean).Value
	case *object.Nil:
		return true
	default:
		// Pointer equality for other types
		return a == b
	}
}

func compareObjects(a, b object.Object) int {
	// Simplified comparison, assuming homogeneous comparable types.
	switch a := a.(type) {
	case *object.Integer:
		bVal := b.(*object.Integer).Value
		if a.Value < bVal {
			return -1
		}
		if a.Value > bVal {
			return 1
		}
		return 0
	case *object.String:
		bVal := b.(*object.String).Value
		if a.Value < bVal {
			return -1
		}
		if a.Value > bVal {
			return 1
		}
		return 0
	}
	return 0 // Cannot compare
}
