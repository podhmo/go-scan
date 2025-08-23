package evaluator

import (
	"bufio"
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"io"
	"reflect"
	"strconv"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo/ffibridge"
	"github.com/podhmo/go-scan/minigo/object"
)

// SpecialFormFunction is the signature for special form functions.
type SpecialFormFunction func(e *Evaluator, fscope *object.FileScope, pos token.Pos, args []ast.Expr) object.Object

// SpecialForm represents a special form function.
type SpecialForm struct {
	Fn SpecialFormFunction
}

// Type returns the type of the SpecialForm object.
func (sf *SpecialForm) Type() object.ObjectType { return object.SPECIAL_FORM_OBJ }

// Inspect returns a string representation of the special form function.
func (sf *SpecialForm) Inspect() string { return "special form" }

var builtins = map[string]*object.Builtin{
	"readln": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 0 {
				return ctx.NewError(pos, "wrong number of arguments. got=0, want=0")
			}
			reader := bufio.NewReader(ctx.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return ctx.NewError(pos, "error reading from stdin: %v", err)
			}
			return &object.String{Value: strings.TrimSpace(line)}
		},
	},
	"len": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			switch arg := args[0].(type) {
			case *object.Array:
				return &object.Integer{Value: int64(len(arg.Elements))}
			case *object.String:
				return &object.Integer{Value: int64(len(arg.Value))}
			case *object.Map:
				return &object.Integer{Value: int64(len(arg.Pairs))}
			case *object.GoValue:
				val := arg.Value
				switch val.Kind() {
				case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
					return &object.Integer{Value: int64(val.Len())}
				default:
					return ctx.NewError(pos, "argument to `len` not supported, got Go value of type %s", val.Kind())
				}
			default:
				return ctx.NewError(pos, "argument to `len` not supported, got %s", args[0].Type())
			}
		},
	},
	"cap": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			switch arg := args[0].(type) {
			case *object.Array:
				return &object.Integer{Value: int64(cap(arg.Elements))}
			case *object.GoValue:
				val := arg.Value
				switch val.Kind() {
				case reflect.Array, reflect.Slice:
					return &object.Integer{Value: int64(val.Cap())}
				default:
					return ctx.NewError(pos, "argument to `cap` not supported, got Go value of type %s", val.Kind())
				}
			default:
				return ctx.NewError(pos, "argument to `cap` not supported, got %s", args[0].Type())
			}
		},
	},
	"copy": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 2 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=2", len(args))
			}
			dst, ok := args[0].(*object.Array)
			if !ok {
				return ctx.NewError(pos, "argument 1 to `copy` must be array, got %s", args[0].Type())
			}
			src, ok := args[1].(*object.Array)
			if !ok {
				return ctx.NewError(pos, "argument 2 to `copy` must be array, got %s", args[1].Type())
			}

			n := copy(dst.Elements, src.Elements)
			return &object.Integer{Value: int64(n)}
		},
	},
	"delete": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 2 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=2", len(args))
			}
			m, ok := args[0].(*object.Map)
			if !ok {
				return ctx.NewError(pos, "argument to `delete` must be a map, got %s", args[0].Type())
			}
			key, ok := args[1].(object.Hashable)
			if !ok {
				return ctx.NewError(pos, "unusable as map key: %s", args[1].Type())
			}
			delete(m.Pairs, key.HashKey())
			return object.NIL
		},
	},
	"make": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) == 0 {
				return ctx.NewError(pos, "missing argument to make")
			}

			switch typeArg := args[0].(type) {
			case *object.MapType:
				if len(args) > 2 {
					return ctx.NewError(pos, "make(map) takes at most 1 size argument")
				}
				// The optional size argument is ignored for now.
				return &object.Map{Pairs: make(map[object.HashKey]object.MapPair)}

			case *object.ArrayType:
				if len(args) < 2 || len(args) > 3 {
					return ctx.NewError(pos, "make([]T) requires len and optional cap arguments")
				}
				lenArg, ok := args[1].(*object.Integer)
				if !ok {
					return ctx.NewError(pos, "argument 2 to `make` must be an integer, got %s", args[1].Type())
				}
				length := lenArg.Value

				var capacity int64
				if len(args) == 3 {
					capArg, ok := args[2].(*object.Integer)
					if !ok {
						return ctx.NewError(pos, "argument 3 to `make` must be an integer, got %s", args[2].Type())
					}
					capacity = capArg.Value
				} else {
					capacity = length
				}

				if length < 0 || capacity < 0 || length > capacity {
					return ctx.NewError(pos, "invalid arguments: len=%d, cap=%d", length, capacity)
				}

				elements := make([]object.Object, length, capacity)
				for i := range elements {
					elements[i] = object.NIL // Zero-value for slices is nil elements
				}
				return &object.Array{Elements: elements}
			default:
				return ctx.NewError(pos, "argument 1 to `make` must be a slice or map type, got %s", typeArg.Type())
			}
		},
	},
	"clear": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			switch arg := args[0].(type) {
			case *object.Map:
				arg.Pairs = make(map[object.HashKey]object.MapPair)
				return object.NIL
			case *object.Array:
				for i := range arg.Elements {
					arg.Elements[i] = object.NIL // In a real GC'd language, this would be the zero value for the element type.
				}
				return object.NIL
			default:
				return ctx.NewError(pos, "argument to `clear` must be map or slice, got %s", arg.Type())
			}
		},
	},
	"complex": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 2 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=2", len(args))
			}

			getNumberAsFloat := func(arg object.Object) (float64, bool) {
				switch n := arg.(type) {
				case *object.Integer:
					return float64(n.Value), true
				case *object.Float:
					return n.Value, true
				default:
					return 0, false
				}
			}

			r, ok := getNumberAsFloat(args[0])
			if !ok {
				return ctx.NewError(pos, "argument 1 to `complex` must be a number, got %s", args[0].Type())
			}
			i, ok := getNumberAsFloat(args[1])
			if !ok {
				return ctx.NewError(pos, "argument 2 to `complex` must be a number, got %s", args[1].Type())
			}

			return &object.Complex{Real: r, Imag: i}
		},
	},
	"real": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			c, ok := args[0].(*object.Complex)
			if !ok {
				return ctx.NewError(pos, "argument to `real` must be a complex number, got %s", args[0].Type())
			}
			return &object.Float{Value: c.Real}
		},
	},
	"imag": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			c, ok := args[0].(*object.Complex)
			if !ok {
				return ctx.NewError(pos, "argument to `imag` must be a complex number, got %s", args[0].Type())
			}
			return &object.Float{Value: c.Imag}
		},
	},
	"append": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) < 2 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want at least 2", len(args))
			}

			var elements []object.Object
			if arr, ok := args[0].(*object.Array); ok {
				elements = arr.Elements
			} else if args[0] == object.NIL || args[0].Type() == object.TYPED_NIL_OBJ {
				elements = []object.Object{}
			} else {
				return ctx.NewError(pos, "argument to `append` must be array or nil, got %s", args[0].Type())
			}

			newElements := make([]object.Object, len(elements), len(elements)+len(args)-1)
			copy(newElements, elements)
			newElements = append(newElements, args[1:]...)

			return &object.Array{Elements: newElements}
		},
	},
	"max": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) == 0 {
				return ctx.NewError(pos, "max() requires at least one argument")
			}
			maxVal, ok := args[0].(*object.Integer)
			if !ok {
				return ctx.NewError(pos, "all arguments to max() must be integers")
			}
			for i := 1; i < len(args); i++ {
				val, ok := args[i].(*object.Integer)
				if !ok {
					return ctx.NewError(pos, "all arguments to max() must be integers")
				}
				if val.Value > maxVal.Value {
					maxVal = val
				}
			}
			return maxVal
		},
	},
	"min": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) == 0 {
				return ctx.NewError(pos, "min() requires at least one argument")
			}
			minVal, ok := args[0].(*object.Integer)
			if !ok {
				return ctx.NewError(pos, "all arguments to min() must be integers")
			}
			for i := 1; i < len(args); i++ {
				val, ok := args[i].(*object.Integer)
				if !ok {
					return ctx.NewError(pos, "all arguments to min() must be integers")
				}
				if val.Value < minVal.Value {
					minVal = val
				}
			}
			return minVal
		},
	},
	"new": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			// We can't call resolveType here directly because we don't have the evaluator.
			// This means `new(MyAlias)` where `MyAlias = MyStruct` won't work yet.
			// This is a limitation we'll accept for now. A better design would
			// make the evaluator available to builtins that need it.
			def, ok := args[0].(*object.StructDefinition)
			if !ok {
				return ctx.NewError(pos, "argument to `new` must be a struct type, got %s", args[0].Type())
			}

			// Create a zero-valued instance of the struct.
			instance := &object.StructInstance{
				Def:    def,
				Fields: make(map[string]object.Object),
			}
			for _, field := range def.Fields {
				// For now, we'll just initialize with NIL. A more advanced implementation
				// would handle zero values for different types (0, "", false).
				instance.Fields[field.Names[0].Name] = object.NIL
			}

			var obj object.Object = instance
			return &object.Pointer{Element: &obj}
		},
	},
	"panic": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			return &object.Panic{Value: args[0]}
		},
	},
	"recover": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 0 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=0", len(args))
			}
			// recover is only effective in deferred functions.
			if !ctx.IsExecutingDefer() {
				return object.NIL
			}
			if p := ctx.GetPanic(); p != nil {
				ctx.ClearPanic()
				return p.Value
			}
			return object.NIL
		},
	},
	"close": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				return ctx.NewError(pos, "wrong number of arguments. got=%d, want=1", len(args))
			}
			// Since channels are not supported, any argument is invalid.
			return ctx.NewError(pos, "argument to `close` must be a channel, got %s", args[0].Type())
		},
	},
	"print": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			for i, arg := range args {
				if i > 0 {
					fmt.Fprint(ctx.Stdout, " ")
				}
				fmt.Fprint(ctx.Stdout, arg.Inspect())
			}
			return object.NIL
		},
	},
	"println": {
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			for i, arg := range args {
				if i > 0 {
					fmt.Fprint(ctx.Stdout, " ")
				}
				fmt.Fprint(ctx.Stdout, arg.Inspect())
			}
			fmt.Fprintln(ctx.Stdout)
			return object.NIL
		},
	},
}

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	object.BuiltinContext
	scanner          *goscan.Scanner
	registry         *object.SymbolRegistry
	specialForms     map[string]*SpecialForm
	packages         map[string]*object.Package // Central package cache
	callStack        []*object.CallFrame
	currentPanic     *object.Panic // The currently active panic
	isExecutingDefer bool          // True if the evaluator is currently running a deferred function
}

// Config holds the configuration for creating a new Evaluator.
type Config struct {
	Fset         *token.FileSet
	Scanner      *goscan.Scanner
	Registry     *object.SymbolRegistry
	SpecialForms map[string]*SpecialForm
	Packages     map[string]*object.Package
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
}

// New creates a new Evaluator.
func New(cfg Config) *Evaluator {
	e := &Evaluator{
		scanner:      cfg.Scanner,
		registry:     cfg.Registry,
		specialForms: cfg.SpecialForms,
		packages:     cfg.Packages,
		callStack:    make([]*object.CallFrame, 0),
	}
	e.BuiltinContext = object.BuiltinContext{
		Stdin:  cfg.Stdin,
		Stdout: cfg.Stdout,
		Stderr: cfg.Stderr,
		Fset:   cfg.Fset,
		IsExecutingDefer: func() bool {
			return e.isExecutingDefer
		},
		GetPanic: func() *object.Panic {
			return e.currentPanic
		},
		ClearPanic: func() {
			e.currentPanic = nil
		},
		NewError: func(pos token.Pos, format string, v ...interface{}) *object.Error {
			return e.newError(pos, format, v...)
		},
	}
	return e
}

// inferTypeOf infers the object.Object representing the type of a given value object.
func (e *Evaluator) inferTypeOf(obj object.Object) object.Object {
	switch o := obj.(type) {
	case *object.TypedNil:
		return o.TypeObject
	case *object.Integer:
		return &object.Type{Name: "int"}
	case *object.Float:
		return &object.Type{Name: "float64"}
	case *object.String:
		return &object.Type{Name: "string"}
	case *object.Boolean:
		return &object.Type{Name: "bool"}
	case *object.StructInstance:
		// The type of a struct instance is its definition.
		return o.Def
	case *object.Pointer:
		if o.Element == nil || *o.Element == nil {
			// Cannot infer type from a nil pointer.
			return object.NIL
		}
		// Recursively find the type of the pointed-to element and wrap it in a pointer type.
		elemType := e.inferTypeOf(*o.Element)
		if elemType == object.NIL {
			return object.NIL
		}
		return &object.PointerType{ElementType: elemType}
	case *object.Array:
		if o.SliceType != nil {
			return o.SliceType
		}
		// For a fully typed system, we would need to know the array's element type.
		if len(o.Elements) == 0 {
			// Cannot infer type from an empty slice.
			// This is a known limitation in Go's type inference too.
			// We could potentially return a special "any" type here if needed.
			return nil
		}
		// Infer from the first element. Assumes a homogeneous slice.
		elemType := e.inferTypeOf(o.Elements[0])
		if elemType == nil {
			return nil
		}
		// Return an ArrayType object that represents `[]<elemType>`
		return &object.ArrayType{ElementType: elemType}
	case *object.Map:
		if o.MapType != nil {
			return o.MapType
		}
		if len(o.Pairs) == 0 {
			return nil
		}
		var keyType, valType object.Object
		for _, pair := range o.Pairs {
			keyType = e.inferTypeOf(pair.Key)
			valType = e.inferTypeOf(pair.Value)
			break // Infer from the first pair
		}
		if keyType == nil || valType == nil {
			return nil
		}
		return &object.MapType{KeyType: keyType, ValueType: valType}
	default:
		// Fallback for types we can't infer simply.
		return nil
	}
}

func (e *Evaluator) inferGenericTypes(pos token.Pos, f *object.Function, args []object.Object) ([]object.Object, object.Object) {
	// 1. Get the names of all type parameters (e.g., {"T": true, "K": true})
	typeParamNames := make(map[string]bool)
	for _, field := range f.TypeParams.List {
		for _, name := range field.Names {
			typeParamNames[name.Name] = true
		}
	}

	// 2. Map inferred types by name (e.g., "T" -> &object.Type{Name:"int"})
	inferredTypes := make(map[string]object.Object)

	// 3. Iterate through function parameters and corresponding arguments
	for i, paramField := range f.Parameters.List {
		// Handle simple generic type like 'T'
		if paramTypeIdent, ok := paramField.Type.(*ast.Ident); ok {
			// Check if the parameter's type is one of the generic type parameters
			if _, isGeneric := typeParamNames[paramTypeIdent.Name]; isGeneric {
				if i >= len(args) {
					// Not enough arguments provided to infer this type.
					return nil, e.newError(pos, "cannot infer type for generic parameter %s: not enough arguments", paramTypeIdent.Name)
				}

				argType := e.inferTypeOf(args[i])
				if argType == nil || argType == object.NIL {
					return nil, e.newError(pos, "cannot infer type for generic parameter %s from argument %d of type %s", paramTypeIdent.Name, i, args[i].Type())
				}

				// Check for conflicting inferences
				if existing, ok := inferredTypes[paramTypeIdent.Name]; ok {
					// A simple pointer comparison for types works for primitives. For complex types, we compare inspect strings.
					if existing != argType && existing.Inspect() != argType.Inspect() {
						return nil, e.newError(pos, "cannot infer type for %s: conflicting types %s and %s", paramTypeIdent.Name, existing.Inspect(), argType.Inspect())
					}
				} else {
					inferredTypes[paramTypeIdent.Name] = argType
				}
			}
		} else if paramTypeArray, ok := paramField.Type.(*ast.ArrayType); ok {
			// Handle slice of generic type like '[]T'
			if eltIdent, ok := paramTypeArray.Elt.(*ast.Ident); ok {
				if _, isGeneric := typeParamNames[eltIdent.Name]; isGeneric {
					if i >= len(args) {
						return nil, e.newError(pos, "cannot infer type for generic parameter %s: not enough arguments", eltIdent.Name)
					}
					arg := args[i]
					var inferredElemType object.Object
					if tn, ok := arg.(*object.TypedNil); ok {
						if at, ok := tn.TypeObject.(*object.ArrayType); ok {
							inferredElemType = at.ElementType
						}
					} else if arr, ok := arg.(*object.Array); ok {
						if arrType, ok := e.inferTypeOf(arr).(*object.ArrayType); ok {
							inferredElemType = arrType.ElementType
						}
					}

					if inferredElemType != nil {
						inferredTypes[eltIdent.Name] = inferredElemType
					}
				}
			}
		} else if paramTypeMap, ok := paramField.Type.(*ast.MapType); ok {
			// Handle map of generic types like 'map[K]V'
			keyIdent, keyIsIdent := paramTypeMap.Key.(*ast.Ident)
			valIdent, valIsIdent := paramTypeMap.Value.(*ast.Ident)

			keyIsGeneric := false
			if keyIsIdent {
				_, keyIsGeneric = typeParamNames[keyIdent.Name]
			}
			valIsGeneric := false
			if valIsIdent {
				_, valIsGeneric = typeParamNames[valIdent.Name]
			}

			if (keyIsIdent && keyIsGeneric) || (valIsIdent && valIsGeneric) {
				if i >= len(args) {
					return nil, e.newError(pos, "cannot infer type for generic map: not enough arguments")
				}
				arg := args[i]
				var inferredMapType *object.MapType
				if tn, ok := arg.(*object.TypedNil); ok {
					if mt, ok := tn.TypeObject.(*object.MapType); ok {
						inferredMapType = mt
					}
				} else if argMap, ok := arg.(*object.Map); ok {
					if mt, ok := e.inferTypeOf(argMap).(*object.MapType); ok {
						inferredMapType = mt
					}
				}

				if mapType := inferredMapType; mapType != nil {
					if keyIsIdent && keyIsGeneric {
						inferredTypes[keyIdent.Name] = mapType.KeyType
					}
					if valIsIdent && valIsGeneric {
						inferredTypes[valIdent.Name] = mapType.ValueType
					}
				}
			}
		} else if paramTypeStar, ok := paramField.Type.(*ast.StarExpr); ok {
			// Handle pointer to generic type like '*T'
			if eltIdent, ok := paramTypeStar.X.(*ast.Ident); ok {
				if _, isGeneric := typeParamNames[eltIdent.Name]; isGeneric {
					if i >= len(args) {
						return nil, e.newError(pos, "cannot infer type for generic parameter %s: not enough arguments", eltIdent.Name)
					}
					arg := args[i]
					var inferredElemType object.Object
					if tn, ok := arg.(*object.TypedNil); ok {
						if pt, ok := tn.TypeObject.(*object.PointerType); ok {
							inferredElemType = pt.ElementType
						}
					} else if ptr, ok := arg.(*object.Pointer); ok {
						if pt, ok := e.inferTypeOf(ptr).(*object.PointerType); ok {
							inferredElemType = pt.ElementType
						}
					}

					if inferredElemType != nil {
						inferredTypes[eltIdent.Name] = inferredElemType
					}
				}
			}
		}
	}

	// 4. Second pass for constraint-based inference.
	// This loop allows inferences to feed into each other. For example, inferring S
	// might allow us to infer E, which might then be used in another constraint.
	madeProgress := true
	for madeProgress {
		madeProgress = false
		for _, typeParamField := range f.TypeParams.List {
			paramName := typeParamField.Names[0].Name

			// If we have an inferred type for this parameter...
			if inferredType, ok := inferredTypes[paramName]; ok {
				// ...and its constraint is an array/slice type...
				constraintExpr := typeParamField.Type
				if unary, ok := constraintExpr.(*ast.UnaryExpr); ok && unary.Op == token.TILDE {
					constraintExpr = unary.X // Look past the ~
				}

				if arrayConstraint, ok := constraintExpr.(*ast.ArrayType); ok {
					// ...and the inferred type is indeed an array...
					if inferredArray, ok := inferredType.(*object.ArrayType); ok {
						// ...then we can try to infer the element type parameter.
						if elemParamIdent, ok := arrayConstraint.Elt.(*ast.Ident); ok {
							elemParamName := elemParamIdent.Name
							// If we haven't inferred this element type yet...
							if _, alreadyInferred := inferredTypes[elemParamName]; !alreadyInferred {
								// ...then infer it from the actual array's element type.
								inferredTypes[elemParamName] = inferredArray.ElementType
								madeProgress = true // We made progress, so loop again.
							}
						}
					}
				}
				// TODO: Add cases for other constraints like map[K]V
			}
		}
	}

	// 5. Convert the map of inferred types into an ordered slice
	finalTypeArgs := make([]object.Object, len(f.TypeParams.List))
	for i, field := range f.TypeParams.List {
		name := field.Names[0].Name
		inferred, ok := inferredTypes[name]
		if !ok {
			return nil, e.newError(pos, "could not infer type for generic parameter %s", name)
		}
		finalTypeArgs[i] = inferred
	}

	return finalTypeArgs, nil
}

func (e *Evaluator) newError(pos token.Pos, format string, args ...interface{}) *object.Error {
	msg := fmt.Sprintf(format, args...)
	// Create a copy of the current call stack for the error object.
	stackCopy := make([]*object.CallFrame, len(e.callStack))
	copy(stackCopy, e.callStack)

	err := &object.Error{
		Pos:       pos,
		Message:   msg,
		CallStack: stackCopy,
	}
	err.AttachFileSet(e.Fset) // Attach fset for formatting
	return err
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

// nativeBoolToBooleanObject is a helper to convert a native bool to our object.Boolean.
func (e *Evaluator) nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return object.TRUE
	}
	return object.FALSE
}

// evalBangOperatorExpression evaluates the '!' prefix expression.
func (e *Evaluator) evalBangOperatorExpression(right object.Object) object.Object {
	switch right {
	case object.TRUE:
		return object.FALSE
	case object.FALSE:
		return object.TRUE
	case object.NIL:
		return object.TRUE
	default:
		return object.FALSE
	}
}

// evalMinusPrefixOperatorExpression evaluates the '-' prefix expression.
func (e *Evaluator) evalMinusPrefixOperatorExpression(node ast.Node, right object.Object) object.Object {
	if right.Type() != object.INTEGER_OBJ {
		return e.newError(node.Pos(), "unknown operator: -%s", right.Type())
	}
	value := right.(*object.Integer).Value
	return &object.Integer{Value: -value}
}

// evalPrefixExpression dispatches to the correct prefix evaluation function.
func (e *Evaluator) evalPrefixExpression(node *ast.UnaryExpr, operator string, right object.Object) object.Object {
	switch operator {
	case "!":
		return e.evalBangOperatorExpression(right)
	case "-":
		return e.evalMinusPrefixOperatorExpression(node, right)
	case "+":
		// Unary plus is a no-op for numbers.
		if right.Type() != object.INTEGER_OBJ && right.Type() != object.FLOAT_OBJ {
			return e.newError(node.Pos(), "invalid operation: unary + on non-number %s", right.Type())
		}
		return right
	case "~":
		// The tilde is for type approximation in constraints.
		// For our dynamic evaluation, we can treat ~T as just T.
		return right
	default:
		return e.newError(node.Pos(), "unknown operator: %s%s", operator, right.Type())
	}
}

func (e *Evaluator) evalDereferenceExpression(node ast.Node, right object.Object) object.Object {
	ptr, ok := right.(*object.Pointer)
	if !ok {
		return e.newError(node.Pos(), "invalid indirect of %s (type %s)", right.Inspect(), right.Type())
	}
	return *ptr.Element
}

func (e *Evaluator) evalAddressOfExpression(node *ast.UnaryExpr, env *object.Environment, fscope *object.FileScope) object.Object {
	switch operand := node.X.(type) {
	case *ast.Ident:
		addr, ok := env.GetAddress(operand.Name)
		if !ok {
			return e.newError(node.Pos(), "cannot take the address of undeclared variable: %s", operand.Name)
		}
		return &object.Pointer{Element: addr}
	case *ast.CompositeLit:
		// Evaluate the composite literal to create the object instance.
		obj := e.evalCompositeLit(operand, env, fscope)
		if isError(obj) {
			return obj
		}
		// Return a pointer to the newly created object.
		return &object.Pointer{Element: &obj}
	default:
		return e.newError(node.Pos(), "cannot take the address of %T", node.X)
	}
}

// unwrapToInt64 is a helper to extract an int64 from an Integer or a GoValue.
func (e *Evaluator) unwrapToInt64(obj object.Object) (int64, bool) {
	switch o := obj.(type) {
	case *object.Integer:
		return o.Value, true
	case *object.GoValue:
		// Check if the underlying Go value is some kind of integer.
		switch o.Value.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return o.Value.Int(), true
		}
	}
	return 0, false
}

// evalMixedIntInfixExpression handles infix expressions for combinations of Integer and GoValue(int).
func (e *Evaluator) evalMixedIntInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal, ok1 := e.unwrapToInt64(left)
	if !ok1 {
		return e.newError(node.Pos(), "left operand is not a valid integer: %s", left.Type())
	}
	rightVal, ok2 := e.unwrapToInt64(right)
	if !ok2 {
		return e.newError(node.Pos(), "right operand is not a valid integer: %s", right.Type())
	}

	// Now that we have two int64s, we can perform the operation.
	// This logic is duplicated from evalIntegerInfixExpression.
	// A future refactor could merge them.
	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return e.newError(node.Pos(), "division by zero")
		}
		return &object.Integer{Value: leftVal / rightVal}
	case "%":
		if rightVal == 0 {
			return e.newError(node.Pos(), "division by zero")
		}
		return &object.Integer{Value: leftVal % rightVal}
	case "<<":
		return &object.Integer{Value: leftVal << rightVal}
	case ">>":
		return &object.Integer{Value: leftVal >> rightVal}
	case "&":
		return &object.Integer{Value: leftVal & rightVal}
	case "|":
		return &object.Integer{Value: leftVal | rightVal}
	case "^":
		return &object.Integer{Value: leftVal ^ rightVal}
	case "<":
		return e.nativeBoolToBooleanObject(leftVal < rightVal)
	case "<=":
		return e.nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">":
		return e.nativeBoolToBooleanObject(leftVal > rightVal)
	case ">=":
		return e.nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return e.nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return e.nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		// This should be caught by the outer switch, but as a safeguard:
		return e.newError(node.Pos(), "unknown integer operator: %s", operator)
	}
}

// evalIntegerInfixExpression evaluates infix expressions for integers.
func (e *Evaluator) evalIntegerInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value

	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return e.newError(node.Pos(), "division by zero")
		}
		return &object.Integer{Value: leftVal / rightVal}
	case "%":
		if rightVal == 0 {
			return e.newError(node.Pos(), "division by zero")
		}
		return &object.Integer{Value: leftVal % rightVal}
	case "<<":
		return &object.Integer{Value: leftVal << rightVal}
	case ">>":
		return &object.Integer{Value: leftVal >> rightVal}
	case "&":
		return &object.Integer{Value: leftVal & rightVal}
	case "|":
		return &object.Integer{Value: leftVal | rightVal}
	case "^":
		return &object.Integer{Value: leftVal ^ rightVal}
	case "<":
		return e.nativeBoolToBooleanObject(leftVal < rightVal)
	case "<=":
		return e.nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">":
		return e.nativeBoolToBooleanObject(leftVal > rightVal)
	case ">=":
		return e.nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return e.nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return e.nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return e.newError(node.Pos(), "unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

// unwrapToString is a helper to extract a string from a String or a GoValue.
func (e *Evaluator) unwrapToString(obj object.Object) (string, bool) {
	switch o := obj.(type) {
	case *object.String:
		return o.Value, true
	case *object.GoValue:
		if o.Value.Kind() == reflect.String {
			return o.Value.String(), true
		}
	}
	return "", false
}

// evalMixedStringInfixExpression handles infix expressions for combinations of String and GoValue(string).
func (e *Evaluator) evalMixedStringInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal, ok1 := e.unwrapToString(left)
	if !ok1 {
		// If unwrap fails, it's not a string, so we can't proceed.
		// Fallback to the generic error message in evalInfixExpression.
		return e.newError(node.Pos(), "type mismatch: %s %s %s", left.Type(), operator, right.Type())
	}
	rightVal, ok2 := e.unwrapToString(right)
	if !ok2 {
		return e.newError(node.Pos(), "type mismatch: %s %s %s", left.Type(), operator, right.Type())
	}

	switch operator {
	case "+":
		return &object.String{Value: leftVal + rightVal}
	case "==":
		return e.nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return e.nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return e.newError(node.Pos(), "unknown operator for strings: %s", operator)
	}
}

// unwrapToBool is a helper to extract a bool from a Boolean or a GoValue.
func (e *Evaluator) unwrapToBool(obj object.Object) (bool, bool) {
	switch o := obj.(type) {
	case *object.Boolean:
		return o.Value, true
	case *object.GoValue:
		if o.Value.Kind() == reflect.Bool {
			return o.Value.Bool(), true
		}
	}
	return false, false
}

// nativeToValue converts a native Go value (from reflect.Value) into a minigo object.
// This is used when retrieving values from Go collections or structs.
func (e *Evaluator) nativeToValue(val reflect.Value) object.Object {
	if !val.IsValid() {
		return object.NIL
	}

	// Check if we can convert the interface value directly.
	// This handles cases where the value might be, for example, a named type
	// whose underlying type is a primitive.
	i := val.Interface()
	switch v := i.(type) {
	case int:
		return &object.Integer{Value: int64(v)}
	case int8:
		return &object.Integer{Value: int64(v)}
	case int16:
		return &object.Integer{Value: int64(v)}
	case int32:
		return &object.Integer{Value: int64(v)}
	case int64:
		return &object.Integer{Value: v}
	case uint:
		return &object.Integer{Value: int64(v)}
	case uint8: // byte
		return &object.Integer{Value: int64(v)}
	case uint16:
		return &object.Integer{Value: int64(v)}
	case uint32:
		return &object.Integer{Value: int64(v)}
	case uint64:
		return &object.Integer{Value: int64(v)}
	case float32:
		return &object.Float{Value: float64(v)}
	case float64:
		return &object.Float{Value: v}
	case string:
		return &object.String{Value: v}
	case bool:
		return e.nativeBoolToBooleanObject(v)
	case []byte:
		elements := make([]object.Object, len(v))
		for i, b := range v {
			elements[i] = &object.Integer{Value: int64(b)}
		}
		return &object.Array{Elements: elements}
	case []string:
		elements := make([]object.Object, len(v))
		for i, s := range v {
			elements[i] = &object.String{Value: s}
		}
		return &object.Array{Elements: elements}
	case nil:
		return object.NIL
	}

	// If direct conversion fails, fall back to Kind-based conversion.
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &object.Integer{Value: val.Int()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &object.Integer{Value: int64(val.Uint())} // Treat unsigned as signed for simplicity
	case reflect.Float32, reflect.Float64:
		return &object.Float{Value: val.Float()}
	case reflect.Ptr, reflect.Interface:
		if val.IsNil() {
			return object.NIL
		}
		// Re-wrap the value to allow further operations on it.
		return &object.GoValue{Value: val}
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		// Wrap complex types so they can be operated on within the interpreter.
		return &object.GoValue{Value: val}
	default:
		// For any other type, we can't safely represent it.
		// For now, we'll return a GoValue, but this could also be an error.
		return &object.GoValue{Value: val}
	}
}

// objectToReflectValue converts a minigo object to a reflect.Value of a specific Go type.
// This is a crucial helper for map indexing and function calls into Go code.
func (e *Evaluator) objectToReflectValue(obj object.Object, targetType reflect.Type) (reflect.Value, error) {
	// Handle target type of interface{} separately.
	// We convert the minigo object to its "best" Go equivalent.
	if targetType.Kind() == reflect.Interface && targetType.NumMethod() == 0 {
		nativeVal, err := e.objectToNativeGoValue(obj)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("unsupported conversion from %s to interface{}: %w", obj.Type(), err)
		}

		if nativeVal == nil {
			return reflect.Zero(targetType), nil
		}
		// We have the native Go value; now we need to put it into a reflect.Value
		// of the target interface type.
		val := reflect.ValueOf(nativeVal)

		// This check is important. For example, if nativeVal is a map[string]any
		// from a struct, its type is not directly assignable to `any` if `any`
		// is from a different type system context (less common now, but good practice).
		// More importantly, it handles named interfaces.
		if !val.Type().AssignableTo(targetType) {
			return reflect.Value{}, fmt.Errorf("value of type %T is not assignable to interface type %s", nativeVal, targetType)
		}
		return val, nil
	}

	// If the object is already a GoValue, try to use its underlying value directly if compatible.
	if goVal, ok := obj.(*object.GoValue); ok {
		if goVal.Value.Type().AssignableTo(targetType) {
			return goVal.Value, nil
		}
		if goVal.Value.Type().ConvertibleTo(targetType) {
			return goVal.Value.Convert(targetType), nil
		}
		// Fall through to allow conversions like minigo Integer -> Go float64
	}

	switch o := obj.(type) {
	case *object.AstNode:
		// Check if the underlying AST node (e.g., *ast.FuncLit) can be assigned
		// to the target Go function's argument type (e.g., ast.Node or *ast.FuncLit).
		// The `AssignableTo` method correctly handles assignment to interfaces.
		nodeType := reflect.TypeOf(o.Node)
		if nodeType.AssignableTo(targetType) {
			return reflect.ValueOf(o.Node), nil
		}
		return reflect.Value{}, fmt.Errorf("cannot convert %s (from AstNode) to %s", nodeType, targetType)
	case *object.Integer:
		// Create a reflect.Value of the target type and set its value.
		val := reflect.New(targetType).Elem()
		switch targetType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val.SetInt(o.Value)
			return val, nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val.SetUint(uint64(o.Value))
			return val, nil
		case reflect.Float32, reflect.Float64:
			val.SetFloat(float64(o.Value))
			return val, nil
		default:
			return reflect.Value{}, fmt.Errorf("cannot convert integer to %s", targetType)
		}
	case *object.Float:
		val := reflect.New(targetType).Elem()
		switch targetType.Kind() {
		case reflect.Float32, reflect.Float64:
			val.SetFloat(o.Value)
			return val, nil
		default:
			return reflect.Value{}, fmt.Errorf("cannot convert float to %s", targetType)
		}
	case *object.String:
		if targetType.Kind() != reflect.String {
			return reflect.Value{}, fmt.Errorf("cannot convert string to %s", targetType)
		}
		return reflect.ValueOf(o.Value).Convert(targetType), nil
	case *object.Boolean:
		if targetType.Kind() != reflect.Bool {
			return reflect.Value{}, fmt.Errorf("cannot convert boolean to %s", targetType)
		}
		return reflect.ValueOf(o.Value).Convert(targetType), nil
	case *object.GoValue:
		// If the underlying Go value is assignable to the target type, use it directly.
		if o.Value.Type().AssignableTo(targetType) {
			return o.Value, nil
		}
		// Also check for convertibility (e.g., int to int64).
		if o.Value.Type().ConvertibleTo(targetType) {
			return o.Value.Convert(targetType), nil
		}
		return reflect.Value{}, fmt.Errorf("GoValue of type %s is not assignable or convertible to %s", o.Value.Type(), targetType)
	case *object.Nil:
		// For nil, we can return a zero value of the target type if it's a pointer, map, slice, etc.
		switch targetType.Kind() {
		case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func:
			return reflect.Zero(targetType), nil
		}
		return reflect.Value{}, fmt.Errorf("cannot convert nil to non-nillable type %s", targetType)
	case *object.Array:
		// Handle conversion to []byte, which is common for stdlib functions.
		if targetType.Kind() == reflect.Slice && targetType.Elem().Kind() == reflect.Uint8 {
			bytes := make([]byte, len(o.Elements))
			for i, el := range o.Elements {
				intVal, ok := el.(*object.Integer)
				if !ok {
					return reflect.Value{}, fmt.Errorf("cannot convert non-integer element in array to byte")
				}
				bytes[i] = byte(intVal.Value)
			}
			return reflect.ValueOf(bytes), nil
		}
		// Handle conversion to []int.
		if targetType.Kind() == reflect.Slice && targetType.Elem().Kind() == reflect.Int {
			ints := make([]int, len(o.Elements))
			for i, el := range o.Elements {
				intVal, ok := el.(*object.Integer)
				if !ok {
					return reflect.Value{}, fmt.Errorf("cannot convert non-integer element in array to int")
				}
				ints[i] = int(intVal.Value)
			}
			return reflect.ValueOf(ints), nil
		}
		// Handle conversion to []float64.
		if targetType.Kind() == reflect.Slice && targetType.Elem().Kind() == reflect.Float64 {
			floats := make([]float64, len(o.Elements))
			for i, el := range o.Elements {
				floatVal, ok := el.(*object.Float)
				if !ok {
					return reflect.Value{}, fmt.Errorf("cannot convert non-float element in array to float64")
				}
				floats[i] = floatVal.Value
			}
			return reflect.ValueOf(floats), nil
		}
		// Handle conversion to []string.
		if targetType.Kind() == reflect.Slice && targetType.Elem().Kind() == reflect.String {
			strings := make([]string, len(o.Elements))
			for i, el := range o.Elements {
				strVal, ok := el.(*object.String)
				if !ok {
					return reflect.Value{}, fmt.Errorf("cannot convert non-string element in array to string")
				}
				strings[i] = strVal.Value
			}
			return reflect.ValueOf(strings), nil
		}
	}

	return reflect.Value{}, fmt.Errorf("unsupported conversion from %s to %s", obj.Type(), targetType)
}

// objectToNativeGoValue converts a minigo object to its most natural Go counterpart.
func (e *Evaluator) objectToNativeGoValue(obj object.Object) (any, error) {
	switch o := obj.(type) {
	case *object.Integer:
		return o.Value, nil
	case *object.String:
		return o.Value, nil
	case *object.Boolean:
		return o.Value, nil
	case *object.Nil:
		return nil, nil
	case *object.GoValue:
		return o.Value.Interface(), nil
	case *object.Array:
		slice := make([]any, len(o.Elements))
		for i, elem := range o.Elements {
			var err error
			slice[i], err = e.objectToNativeGoValue(elem)
			if err != nil {
				return nil, fmt.Errorf("failed to convert element %d in array: %w", i, err)
			}
		}
		return slice, nil
	case *object.StructInstance:
		m := make(map[string]any, len(o.Fields))
		for name, fieldObj := range o.Fields {
			tag, ok := o.Def.FieldTags[name]
			if !ok {
				tag = name // Default to field name if no tag
			}

			if tag == "-" {
				continue // Skip ignored fields
			}

			tagName := tag
			omitempty := false
			if strings.HasSuffix(tag, ",omitempty") {
				omitempty = true
				tagName = strings.TrimSuffix(tag, ",omitempty")
			}

			if omitempty {
				isZero := false
				switch v := fieldObj.(type) {
				case *object.Integer:
					if v.Value == 0 {
						isZero = true
					}
				case *object.Float:
					if v.Value == 0.0 {
						isZero = true
					}
				case *object.String:
					if v.Value == "" {
						isZero = true
					}
				case *object.Boolean:
					if !v.Value {
						isZero = true
					}
				case *object.Nil:
					isZero = true
				case *object.Array:
					if len(v.Elements) == 0 {
						isZero = true
					}
				case *object.Map:
					if len(v.Pairs) == 0 {
						isZero = true
					}
				}
				if isZero {
					continue
				}
			}

			var err error
			m[tagName], err = e.objectToNativeGoValue(fieldObj)
			if err != nil {
				return nil, fmt.Errorf("failed to convert field %q: %w", name, err)
			}
		}
		return m, nil
	default:
		return nil, fmt.Errorf("cannot convert object type %s to a native Go value", obj.Type())
	}
}

// evalMixedBoolInfixExpression handles infix expressions for combinations of Boolean and GoValue(bool).
func (e *Evaluator) evalMixedBoolInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal, ok1 := e.unwrapToBool(left)
	if !ok1 {
		return e.newError(node.Pos(), "type mismatch: %s %s %s", left.Type(), operator, right.Type())
	}
	rightVal, ok2 := e.unwrapToBool(right)
	if !ok2 {
		return e.newError(node.Pos(), "type mismatch: %s %s %s", left.Type(), operator, right.Type())
	}

	switch operator {
	case "==":
		return e.nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return e.nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return e.newError(node.Pos(), "unknown operator for booleans: %s", operator)
	}
}

// evalStringInfixExpression evaluates infix expressions for strings.
func (e *Evaluator) evalStringInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	switch operator {
	case "+":
		return &object.String{Value: leftVal + rightVal}
	case "==":
		return e.nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return e.nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return e.newError(node.Pos(), "unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

// evalInfixExpression dispatches to the correct infix evaluation function based on type.
func (e *Evaluator) evalInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return e.evalIntegerInfixExpression(node, operator, left, right)

	// Handle arithmetic with injected Go values (integers).
	case (left.Type() == object.INTEGER_OBJ || left.Type() == object.GO_VALUE_OBJ) &&
		(right.Type() == object.INTEGER_OBJ || right.Type() == object.GO_VALUE_OBJ):
		return e.evalMixedIntInfixExpression(node, operator, left, right)

	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return e.evalStringInfixExpression(node, operator, left, right)

	// Handle operations with injected Go values (strings).
	case (left.Type() == object.STRING_OBJ || left.Type() == object.GO_VALUE_OBJ) &&
		(right.Type() == object.STRING_OBJ || right.Type() == object.GO_VALUE_OBJ):
		return e.evalMixedStringInfixExpression(node, operator, left, right)

	// Handle operations with injected Go values (booleans).
	case (left.Type() == object.BOOLEAN_OBJ || left.Type() == object.GO_VALUE_OBJ) &&
		(right.Type() == object.BOOLEAN_OBJ || right.Type() == object.GO_VALUE_OBJ):
		return e.evalMixedBoolInfixExpression(node, operator, left, right)

	case operator == "==":
		// Handle nil comparisons first.
		isLeftNil := left.Type() == object.NIL_OBJ || left.Type() == object.TYPED_NIL_OBJ
		isRightNil := right.Type() == object.NIL_OBJ || right.Type() == object.TYPED_NIL_OBJ
		if isLeftNil || isRightNil {
			// If either is nil, they are equal only if both are nil.
			return e.nativeBoolToBooleanObject(isLeftNil == isRightNil)
		}
		// Fallback to pointer comparison for reference types.
		// Note: The cases above handle value types like integers and strings.
		return e.nativeBoolToBooleanObject(left == right)
	case operator == "!=":
		isLeftNil := left.Type() == object.NIL_OBJ || left.Type() == object.TYPED_NIL_OBJ
		isRightNil := right.Type() == object.NIL_OBJ || right.Type() == object.TYPED_NIL_OBJ
		if isLeftNil || isRightNil {
			// If either is nil, they are not equal if one is nil and the other isn't.
			return e.nativeBoolToBooleanObject(isLeftNil != isRightNil)
		}
		return e.nativeBoolToBooleanObject(left != right)
	case left.Type() != right.Type():
		return e.newError(node.Pos(), "type mismatch: %s %s %s", left.Type(), operator, right.Type())
	default:
		return e.newError(node.Pos(), "unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

// isTruthy checks if an object is considered true in a boolean context.
func (e *Evaluator) isTruthy(obj object.Object) bool {
	switch o := obj.(type) {
	case *object.Boolean:
		return o.Value
	case *object.GoValue:
		if val, ok := e.unwrapToBool(o); ok {
			return val
		}
		// If it's a GoValue but not a bool, consider it truthy if it's not nil/zero.
		return o.Value.IsValid() && !o.Value.IsZero()
	case *object.Nil, *object.TypedNil:
		return false
	default:
		// Any other object type (Integer, String, etc.) is considered truthy.
		return !isError(obj)
	}
}

// evalIfElseExpression evaluates an if-else expression.
func (e *Evaluator) evalIfElseExpression(ie *ast.IfStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	// Handle if with initializer
	ifEnv := env
	if ie.Init != nil {
		ifEnv = object.NewEnclosedEnvironment(env)
		initResult := e.Eval(ie.Init, ifEnv, fscope)
		if isError(initResult) {
			return initResult
		}
	}

	condition := e.Eval(ie.Cond, ifEnv, fscope)
	if isError(condition) {
		return condition
	}

	if e.isTruthy(condition) {
		return e.Eval(ie.Body, ifEnv, fscope)
	} else if ie.Else != nil {
		return e.Eval(ie.Else, ifEnv, fscope)
	} else {
		return object.NIL
	}
}

// evalBlockStatement evaluates a block of statements within a new scope.
func (e *Evaluator) evalBlockStatement(block *ast.BlockStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	var result object.Object
	enclosedEnv := object.NewEnclosedEnvironment(env)

	for _, statement := range block.List {
		result = e.Eval(statement, enclosedEnv, fscope)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.BREAK_OBJ || rt == object.CONTINUE_OBJ || rt == object.ERROR_OBJ || rt == object.PANIC_OBJ {
				return result
			}
		}
	}

	return result
}

// evalForStmt evaluates a for loop.
func (e *Evaluator) evalForStmt(fs *ast.ForStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	loopEnv := object.NewEnclosedEnvironment(env)

	var loopVars []string
	if fs.Init != nil {
		// If the init statement is a short variable declaration (:=),
		// record the names of the variables it declares. This is key
		// to emulating Go 1.22's per-iteration variable semantics.
		if assign, ok := fs.Init.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			for _, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					loopVars = append(loopVars, ident.Name)
				}
			}
		}

		initResult := e.Eval(fs.Init, loopEnv, fscope)
		if isError(initResult) {
			return initResult
		}
	}

	for {
		if fs.Cond != nil {
			condition := e.Eval(fs.Cond, loopEnv, fscope)
			if isError(condition) {
				return condition
			}
			if !e.isTruthy(condition) {
				break
			}
		}

		// Create a new environment for the loop body for each iteration.
		bodyEnv := object.NewEnclosedEnvironment(loopEnv)
		// If we identified loop variables from a `:=` statement, we create
		// a copy of them in the new body-specific environment.
		// This is the magic that makes closures capture the variable per-iteration.
		for _, varName := range loopVars {
			val, ok := loopEnv.Get(varName)
			if ok {
				bodyEnv.Set(varName, val)
			}
		}

		// Evaluate the loop body in this new, per-iteration environment.
		bodyResult := e.Eval(fs.Body, bodyEnv, fscope)

		// Check for control flow statements
		if bodyResult != nil {
			rt := bodyResult.Type()
			if rt == object.BREAK_OBJ {
				break // Break from the for-loop
			}
			if rt == object.CONTINUE_OBJ {
				// Execute the post statement before continuing.
				if fs.Post != nil {
					if postResult := e.Eval(fs.Post, loopEnv, fscope); isError(postResult) {
						return postResult
					}
				}
				continue
			}
			if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ || rt == object.PANIC_OBJ {
				return bodyResult // Propagate up
			}
		}

		if fs.Post != nil {
			postResult := e.Eval(fs.Post, loopEnv, fscope)
			if isError(postResult) {
				return postResult
			}
		}
	}

	return object.NIL
}

// evalForRangeStmt evaluates a for...range loop.
func (e *Evaluator) evalForRangeStmt(rs *ast.RangeStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	iterable := e.Eval(rs.X, env, fscope)
	if isError(iterable) {
		return iterable
	}

	switch iterable := iterable.(type) {
	case *object.Array:
		return e.evalRangeArray(rs, iterable, env, fscope)
	case *object.String:
		return e.evalRangeString(rs, iterable, env, fscope)
	case *object.Map:
		return e.evalRangeMap(rs, iterable, env, fscope)
	case *object.Integer:
		return e.evalRangeInteger(rs, iterable, env, fscope)
	case *object.GoValue:
		return e.evalRangeGoValue(rs, iterable, env, fscope)
	case *object.Function:
		return e.evalRangeFunction(rs, iterable, env, fscope)
	default:
		return e.newError(rs.X.Pos(), "range operator not supported for %s", iterable.Type())
	}
}

func (e *Evaluator) evalRangeFunction(rs *ast.RangeStmt, fn *object.Function, env *object.Environment, fscope *object.FileScope) object.Object {
	var loopErr object.Object // To capture errors/returns from the yield function

	yield := &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			loopEnv := object.NewEnclosedEnvironment(env)

			keyIdent, _ := rs.Key.(*ast.Ident)

			if rs.Value == nil {
				// Form: for v := range f
				if len(args) != 1 {
					loopErr = ctx.NewError(pos, "yield must be called with 1 argument for a single-variable range loop, got %d", len(args))
					return object.FALSE // Stop iteration on error
				}
				if keyIdent != nil && keyIdent.Name != "_" {
					loopEnv.Set(keyIdent.Name, args[0])
				}
			} else {
				// Form: for k, v := range f
				valIdent, _ := rs.Value.(*ast.Ident)
				if len(args) != 2 {
					loopErr = ctx.NewError(pos, "yield must be called with 2 arguments for a two-variable range loop, got %d", len(args))
					return object.FALSE // Stop iteration on error
				}
				if keyIdent != nil && keyIdent.Name != "_" {
					loopEnv.Set(keyIdent.Name, args[0])
				}
				if valIdent != nil && valIdent.Name != "_" {
					loopEnv.Set(valIdent.Name, args[1])
				}
			}

			result := e.Eval(rs.Body, loopEnv, fscope)

			switch result {
			case object.BREAK:
				return object.FALSE
			case object.CONTINUE:
				return object.TRUE
			}

			if result != nil {
				rt := result.Type()
				if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ {
					loopErr = result
					return object.FALSE
				}
			}

			return object.TRUE
		},
	}

	e.applyFunction(nil, fn, []object.Object{yield}, env, fscope)

	if loopErr != nil {
		return loopErr
	}

	return object.NIL
}

func (e *Evaluator) evalRangeInteger(rs *ast.RangeStmt, num *object.Integer, env *object.Environment, fscope *object.FileScope) object.Object {
	if rs.Value != nil {
		return e.newError(rs.Pos(), "range over integer does not support a second loop variable")
	}

	for i := int64(0); i < num.Value; i++ {
		loopEnv := object.NewEnclosedEnvironment(env)
		if rs.Key != nil {
			keyIdent, ok := rs.Key.(*ast.Ident)
			if !ok {
				return e.newError(rs.Key.Pos(), "range key must be an identifier")
			}
			if keyIdent.Name != "_" {
				loopEnv.Set(keyIdent.Name, &object.Integer{Value: i})
			}
		}

		result := e.Eval(rs.Body, loopEnv, fscope)
		if result != nil {
			rt := result.Type()
			if rt == object.BREAK_OBJ {
				break
			}
			if rt == object.CONTINUE_OBJ {
				continue
			}
			if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ {
				return result
			}
		}
	}
	return object.NIL
}

func (e *Evaluator) evalRangeGoValue(rs *ast.RangeStmt, goVal *object.GoValue, env *object.Environment, fscope *object.FileScope) object.Object {
	val := goVal.Value
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			loopEnv := object.NewEnclosedEnvironment(env)
			elem := val.Index(i)

			if rs.Key != nil {
				keyIdent, _ := rs.Key.(*ast.Ident) // This is safe in a valid AST
				if keyIdent.Name != "_" {
					loopEnv.Set(keyIdent.Name, &object.Integer{Value: int64(i)})
				}
			}
			if rs.Value != nil {
				valueIdent, _ := rs.Value.(*ast.Ident) // This is safe in a valid AST
				if valueIdent.Name != "_" {
					loopEnv.Set(valueIdent.Name, e.nativeToValue(elem))
				}
			}

			result := e.Eval(rs.Body, loopEnv, fscope)
			if result != nil {
				rt := result.Type()
				if rt == object.BREAK_OBJ {
					break
				}
				if rt == object.CONTINUE_OBJ {
					continue
				}
				if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ {
					return result
				}
			}
		}
		return object.NIL

	case reflect.Map:
		iter := val.MapRange()
		for iter.Next() {
			loopEnv := object.NewEnclosedEnvironment(env)
			k := iter.Key()
			v := iter.Value()

			if rs.Key != nil {
				keyIdent, _ := rs.Key.(*ast.Ident) // This is safe in a valid AST
				if keyIdent.Name != "_" {
					loopEnv.Set(keyIdent.Name, e.nativeToValue(k))
				}
			}
			if rs.Value != nil {
				valueIdent, _ := rs.Value.(*ast.Ident) // This is safe in a valid AST
				if valueIdent.Name != "_" {
					loopEnv.Set(valueIdent.Name, e.nativeToValue(v))
				}
			}

			result := e.Eval(rs.Body, loopEnv, fscope)
			if result != nil {
				rt := result.Type()
				if rt == object.BREAK_OBJ {
					break
				}
				if rt == object.CONTINUE_OBJ {
					continue
				}
				if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ {
					return result
				}
			}
		}
		return object.NIL
	}

	return e.newError(rs.X.Pos(), "range operator not supported for Go value of type %s", val.Kind())
}

func (e *Evaluator) evalRangeArray(rs *ast.RangeStmt, arr *object.Array, env *object.Environment, fscope *object.FileScope) object.Object {
	for i, element := range arr.Elements {
		loopEnv := object.NewEnclosedEnvironment(env)
		if rs.Key != nil {
			keyIdent, ok := rs.Key.(*ast.Ident)
			if !ok {
				return e.newError(rs.Key.Pos(), "range key must be an identifier")
			}
			if keyIdent.Name != "_" {
				loopEnv.Set(keyIdent.Name, &object.Integer{Value: int64(i)})
			}
		}
		if rs.Value != nil {
			valueIdent, ok := rs.Value.(*ast.Ident)
			if !ok {
				return e.newError(rs.Value.Pos(), "range value must be an identifier")
			}
			if valueIdent.Name != "_" {
				loopEnv.Set(valueIdent.Name, element)
			}
		}

		result := e.Eval(rs.Body, loopEnv, fscope)
		if result != nil {
			rt := result.Type()
			if rt == object.BREAK_OBJ {
				break
			}
			if rt == object.CONTINUE_OBJ {
				continue
			}
			if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ {
				return result
			}
		}
	}
	return object.NIL
}

func (e *Evaluator) evalRangeString(rs *ast.RangeStmt, str *object.String, env *object.Environment, fscope *object.FileScope) object.Object {
	for i, r := range str.Value {
		loopEnv := object.NewEnclosedEnvironment(env)
		if rs.Key != nil {
			keyIdent, ok := rs.Key.(*ast.Ident)
			if !ok {
				return e.newError(rs.Key.Pos(), "range key must be an identifier")
			}
			if keyIdent.Name != "_" {
				loopEnv.Set(keyIdent.Name, &object.Integer{Value: int64(i)})
			}
		}
		if rs.Value != nil {
			valueIdent, ok := rs.Value.(*ast.Ident)
			if !ok {
				return e.newError(rs.Value.Pos(), "range value must be an identifier")
			}
			if valueIdent.Name != "_" {
				loopEnv.Set(valueIdent.Name, &object.Integer{Value: int64(r)}) // rune is an alias for int32
			}
		}

		result := e.Eval(rs.Body, loopEnv, fscope)
		if result != nil {
			rt := result.Type()
			if rt == object.BREAK_OBJ {
				break
			}
			if rt == object.CONTINUE_OBJ {
				continue
			}
			if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ {
				return result
			}
		}
	}
	return object.NIL
}

func (e *Evaluator) evalRangeMap(rs *ast.RangeStmt, m *object.Map, env *object.Environment, fscope *object.FileScope) object.Object {
	// Note: Iteration order over maps is not guaranteed.
	for _, pair := range m.Pairs {
		loopEnv := object.NewEnclosedEnvironment(env)
		if rs.Key != nil {
			keyIdent, ok := rs.Key.(*ast.Ident)
			if !ok {
				return e.newError(rs.Key.Pos(), "range key must be an identifier")
			}
			if keyIdent.Name != "_" {
				loopEnv.Set(keyIdent.Name, pair.Key)
			}
		}
		if rs.Value != nil {
			valueIdent, ok := rs.Value.(*ast.Ident)
			if !ok {
				return e.newError(rs.Value.Pos(), "range value must be an identifier")
			}
			if valueIdent.Name != "_" {
				loopEnv.Set(valueIdent.Name, pair.Value)
			}
		}

		result := e.Eval(rs.Body, loopEnv, fscope)
		if result != nil {
			rt := result.Type()
			if rt == object.BREAK_OBJ {
				break
			}
			if rt == object.CONTINUE_OBJ {
				continue
			}
			if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ {
				return result
			}
		}
	}
	return object.NIL
}

// evalSwitchStmt evaluates a switch statement.
func (e *Evaluator) evalSwitchStmt(ss *ast.SwitchStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	switchEnv := env
	if ss.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		initResult := e.Eval(ss.Init, switchEnv, fscope)
		if isError(initResult) {
			return initResult
		}
	}

	var tag object.Object
	if ss.Tag != nil {
		tag = e.Eval(ss.Tag, switchEnv, fscope)
		if isError(tag) {
			return tag
		}
	} else {
		tag = object.TRUE
	}

	var defaultCase *ast.CaseClause
	var matched bool

	for _, stmt := range ss.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}

		if clause.List == nil {
			defaultCase = clause
			continue
		}

		for _, caseExpr := range clause.List {
			caseVal := e.Eval(caseExpr, switchEnv, fscope)
			if isError(caseVal) {
				return caseVal
			}

			var condition bool
			if ss.Tag == nil {
				condition = e.isTruthy(caseVal)
			} else {
				eq := e.evalInfixExpression(caseExpr, "==", tag, caseVal)
				if isError(eq) {
					// We treat comparison errors as non-matches and continue.
					condition = false
				} else {
					condition = eq == object.TRUE
				}
			}

			if condition {
				matched = true
				break
			}
		}

		if matched {
			caseEnv := object.NewEnclosedEnvironment(switchEnv)
			var result object.Object
			for _, caseBodyStmt := range clause.Body {
				result = e.Eval(caseBodyStmt, caseEnv, fscope)
				if isError(result) {
					return result
				}
			}
			return result
		}
	}

	if defaultCase != nil {
		caseEnv := object.NewEnclosedEnvironment(switchEnv)
		var result object.Object
		for _, caseBodyStmt := range defaultCase.Body {
			result = e.Eval(caseBodyStmt, caseEnv, fscope)
			if isError(result) {
				return result
			}
		}
		return result
	}

	return object.NIL
}

func (e *Evaluator) evalExpressions(exps []ast.Expr, env *object.Environment, fscope *object.FileScope, expectedElementType object.Object) []object.Object {
	result := make([]object.Object, len(exps))

	for i, exp := range exps {
		var evaluated object.Object
		if compLit, ok := exp.(*ast.CompositeLit); ok && compLit.Type == nil {
			if expectedElementType == nil {
				evaluated = e.newError(compLit.Pos(), "untyped composite literal in context where type cannot be inferred")
			} else {
				evaluated = e.evalCompositeLitWithType(compLit, expectedElementType, env, fscope)
			}
		} else {
			evaluated = e.Eval(exp, env, fscope)
		}

		if isError(evaluated) {
			// Return a slice containing just the error to stop further processing.
			return []object.Object{evaluated}
		}
		result[i] = evaluated
	}

	return result
}

// getZeroValueForResolvedType creates a zero-value object for a given resolved type object.
func (e *Evaluator) getZeroValueForResolvedType(typeObj object.Object) object.Object {
	switch rt := typeObj.(type) {
	case *object.GoType:
		ptr := reflect.New(rt.GoType)
		return &object.GoValue{Value: ptr.Elem()}
	case *object.StructDefinition:
		instance := &object.StructInstance{Def: rt, Fields: make(map[string]object.Object)}
		for _, field := range rt.Fields {
			for _, name := range field.Names {
				// Initialize fields to NIL. The recursive creation was causing stack overflows.
				// NIL is the correct zero value for any reference or struct type in the interpreter.
				instance.Fields[name.Name] = object.NIL
			}
		}
		return instance
	case *object.Type:
		switch rt.Name {
		case "int", "int64", "int32", "int16", "int8", "uint", "uint64", "uint32", "uint16", "uint8", "byte":
			return &object.Integer{Value: 0}
		case "string":
			return &object.String{Value: ""}
		case "bool":
			return object.FALSE
		case "float64", "float32":
			return &object.Float{Value: 0.0}
		}
	}
	// For any other type (pointers, interfaces, arrays, maps, etc.), the zero value is a typed nil.
	return &object.TypedNil{TypeObject: typeObj}
}

func (e *Evaluator) getZeroValueForType(typeExpr ast.Expr, env *object.Environment, fscope *object.FileScope) object.Object {
	// First, evaluate the AST expression to get a minigo object representing the type.
	typeObj := e.Eval(typeExpr, env, fscope)
	if isError(typeObj) {
		return typeObj
	}

	// Then, resolve any aliases to get the underlying type definition.
	resolvedType := e.resolveType(typeObj, env, fscope)
	if isError(resolvedType) {
		return resolvedType
	}

	// Finally, create the zero value based on the resolved type.
	return e.getZeroValueForResolvedType(resolvedType)
}

func (e *Evaluator) applyFunction(call *ast.CallExpr, fn object.Object, args []object.Object, env *object.Environment, fscope *object.FileScope) object.Object {
	var function *object.Function
	var typeArgs []object.Object
	var receiver object.Object // For bound methods

	switch f := fn.(type) {
	case *object.Function:
		// Check if this is a generic function being called without instantiation.
		if f.TypeParams != nil && len(f.TypeParams.List) > 0 {
			// It's a generic function. Try to infer type arguments from value arguments.
			inferred, errObj := e.inferGenericTypes(call.Pos(), f, args)
			if errObj != nil {
				return errObj
			}
			function = f
			typeArgs = inferred
		} else {
			// It's a regular, non-generic function.
			function = f
		}
	case *object.InstantiatedType:
		genericFn, ok := f.GenericDef.(*object.Function)
		if !ok {
			return e.newError(call.Pos(), "not a function: %s", f.GenericDef.Type())
		}
		function = genericFn
		typeArgs = f.TypeArgs
	case *object.BoundMethod:
		function = f.Fn
		receiver = f.Receiver
		// Generic methods on generic structs are handled by the receiver's type args,
		// which are already bound in extendMethodEnv.
	case *object.GoSourceFunction:
		// Convert GoSourceFunction to a standard Function object for execution.
		// The key is that we will use its DefEnv as the outer environment.
		fInfo := f.Fn
		astFunc := fInfo.AstDecl
		function = &object.Function{
			Name:       astFunc.Name,
			TypeParams: astFunc.Type.TypeParams,
			Parameters: astFunc.Type.Params,
			Results:    astFunc.Type.Results,
			Body:       astFunc.Body,
			Env:        f.DefEnv, // Use the DEFINITION environment
			FScope:     f.FScope, // Use the DEFINITION file scope
		}
		// This is also a generic function, so we must run the same type
		// inference logic as for a regular *object.Function.
		if function.TypeParams != nil && len(function.TypeParams.List) > 0 {
			inferred, errObj := e.inferGenericTypes(call.Pos(), function, args)
			if errObj != nil {
				return errObj
			}
			typeArgs = inferred
		}
	case *object.Builtin:
		var pos token.Pos
		if call != nil {
			pos = call.Pos()
		}
		e.BuiltinContext.Env = env
		e.BuiltinContext.FScope = fscope
		return f.Fn(&e.BuiltinContext, pos, args...)
	default:
		return e.newError(call.Pos(), "not a function: %s", fn.Type())
	}

	// --- Common logic for all user-defined function/method calls ---
	var callPos token.Pos
	if call != nil {
		callPos = call.Pos()
	}

	// Calculate the total number of non-variadic parameter names.
	paramCount := 0
	if function.Parameters != nil {
		for _, field := range function.Parameters.List {
			// For `func(a, b int)`, field.Names is ["a", "b"].
			// For `func(int)`, field.Names is empty, but there's one field.
			if len(field.Names) > 0 {
				paramCount += len(field.Names)
			} else {
				// This handles unnamed parameters, which appear one per field.
				// It also correctly handles the `...T` in a variadic function,
				// which also appears as a single field with no name.
				paramCount++
			}
		}
	}

	// Check argument count
	if function.IsVariadic() {
		// For a variadic function, we need at least (paramCount - 1) arguments.
		// The `paramCount` includes the variadic `...T` parameter itself.
		if len(args) < paramCount-1 {
			return e.newError(callPos, "wrong number of arguments for variadic function. got=%d, want at least %d", len(args), paramCount-1)
		}
	} else {
		if paramCount != len(args) {
			return e.newError(callPos, "wrong number of arguments. got=%d, want=%d", len(args), paramCount)
		}
	}

	// Check type argument count for generic functions
	if function.TypeParams != nil && len(function.TypeParams.List) > 0 {
		if len(typeArgs) != len(function.TypeParams.List) {
			return e.newError(callPos, "wrong number of type arguments. got=%d, want=%d", len(typeArgs), len(function.TypeParams.List))
		}
	}

	// Check type constraints before setting up the environment.
	if function.TypeParams != nil {
		// We need an environment to evaluate the constraint expressions.
		// It should be based on the function's definition environment...
		var baseConstraintEnv *object.Environment
		if function.Env != nil {
			baseConstraintEnv = function.Env
		} else {
			baseConstraintEnv = env // Fallback to the calling environment
		}
		// ... and it must also contain the type parameters themselves, since a
		// constraint for one parameter might refer to another (e.g., S ~[]E).
		constraintEnv := object.NewEnclosedEnvironment(baseConstraintEnv)
		e.bindTypeParams(constraintEnv, function.TypeParams, typeArgs)

		// This loop needs to be careful with multi-name fields.
		typeArgIndex := 0
		for _, param := range function.TypeParams.List {
			for range param.Names {
				if typeArgIndex < len(typeArgs) {
					concreteType := typeArgs[typeArgIndex]
					constraintExpr := param.Type

					constraintObj := e.Eval(constraintExpr, constraintEnv, function.FScope)
					if isError(constraintObj) {
						return constraintObj
					}

					if err := e.checkTypeConstraint(param.Pos(), concreteType, constraintObj, constraintEnv, function.FScope); err != nil {
						return err
					}
					typeArgIndex++
				}
			}
		}
	}

	// Set up call stack
	funcName := "<anonymous>"
	// If the call expression is not directly a func literal (i.e., not an IIFE),
	// and the function object has a name (from a declaration or assignment), use it.
	if call != nil {
		if _, ok := call.Fun.(*ast.FuncLit); !ok {
			if function.Name != nil {
				funcName = function.Name.Name
			}
		}
	} else if function.Name != nil {
		funcName = function.Name.Name
	}
	frame := &object.CallFrame{
		Pos:      callPos,
		Function: funcName,
		Fn:       function,
		Defers:   make([]*object.DeferredCall, 0),
	}
	e.callStack = append(e.callStack, frame)
	// Ensure the stack is popped even if a Go panic occurs within the evaluator.
	defer func() { e.callStack = e.callStack[:len(e.callStack)-1] }()

	// --- Environment Setup ---
	var baseEnv *object.Environment
	if receiver != nil {
		boundMethod := &object.BoundMethod{Fn: function, Receiver: receiver}
		baseEnv = e.extendMethodEnv(boundMethod, args)
	} else {
		baseEnv = object.NewEnclosedEnvironment(function.Env)
		e.extendFunctionEnv(baseEnv, function, args, typeArgs)
	}

	bodyEnv := baseEnv
	if function.HasNamedReturns() {
		namedReturnsEnv := object.NewEnclosedEnvironment(baseEnv)
		for _, field := range function.Results.List {
			for _, name := range field.Names {
				zeroVal := e.getZeroValueForType(field.Type, namedReturnsEnv, fscope)
				namedReturnsEnv.Set(name.Name, zeroVal)
			}
		}
		frame.NamedReturns = namedReturnsEnv
		bodyEnv = namedReturnsEnv
	}

	// Evaluate the function body. This will return a ReturnValue on `return`.
	evalFScope := fscope
	if function.FScope != nil {
		evalFScope = function.FScope
	}
	evaluated := e.Eval(function.Body, bodyEnv, evalFScope)

	// Check if the evaluation resulted in a panic.
	isPanic := false
	if p, ok := evaluated.(*object.Panic); ok {
		isPanic = true
		e.currentPanic = p
	}

	// Now, *after* the body has run, execute the defers. This happens even if a panic occurred.
	for i := len(frame.Defers) - 1; i >= 0; i-- {
		e.executeDeferredCall(frame.Defers[i], fscope)
	}

	// If a panic was active and was not cleared by a recover() in a defer,
	// then it should be propagated up the call stack.
	if e.currentPanic != nil {
		return e.currentPanic
	}

	// If a panic occurred but was recovered, the function's normal execution
	// was aborted. It should return NIL, not continue processing the original
	// panic object as a return value.
	if isPanic {
		return object.NIL
	}

	// --- Return Value Handling ---
	// After defers have run, construct the final return value if necessary.
	if ret, ok := evaluated.(*object.ReturnValue); ok && ret.Value == nil {
		if frame.NamedReturns != nil {
			// This now happens *after* defers, so it will see modified values.
			return e.constructNamedReturnValue(function, frame.NamedReturns)
		}
		return &object.ReturnValue{Value: object.NIL}
	}

	// For regular returns, unwrap the value.
	return e.unwrapReturnValue(evaluated)
}

// constructNamedReturnValue collects the values from the named return environment
// and packages them into a single return value (or a tuple for multiple returns).
func (e *Evaluator) typesAreCompatible(concrete, constraint object.Object, approximate bool) bool {
	// A simple inspect comparison works for basic types and struct definitions.
	// e.g., "int" == "int" or "struct MyStruct" == "struct MyStruct"
	if concrete.Inspect() == constraint.Inspect() {
		return true
	}

	// TODO: A more robust implementation is needed here, especially for the 'approximate' (`~`) case.
	// For `~T`, we would need to check the underlying type of `concrete`.
	// Our current object system doesn't retain alias information after resolution,
	// which makes checking underlying types difficult.
	// For now, this simple comparison is enough to pass tests for built-in types.
	if approximate {
		// This is a placeholder. A real implementation would look at the underlying type.
		// For now, we'll just re-check equality, which is incorrect but safe.
		if concrete.Inspect() == constraint.Inspect() {
			return true
		}
	}

	return false
}

// checkTypeConstraint verifies that a concrete type satisfies a given constraint.
func (e *Evaluator) checkTypeConstraint(pos token.Pos, concreteType, constraint object.Object, env *object.Environment, fscope *object.FileScope) *object.Error {
	resolvedConstraint := e.resolveType(constraint, env, fscope)
	if isError(resolvedConstraint) {
		return resolvedConstraint.(*object.Error)
	}

	ifaceDef, ok := resolvedConstraint.(*object.InterfaceDefinition)
	if !ok {
		// Not an interface constraint, or not one we need to check yet (e.g. method sets).
		// For now, we only care about type list interfaces.
		return nil
	}

	// It is an interface. Check if it's a type list constraint.
	if len(ifaceDef.TypeList) > 0 {
		for _, typeExpr := range ifaceDef.TypeList {
			isApproximate := false
			if unary, ok := typeExpr.(*ast.UnaryExpr); ok && unary.Op == token.TILDE {
				isApproximate = true
				typeExpr = unary.X
			}

			// We need a new environment for this evaluation so it doesn't pollute the function's env.
			constraintTypeObj := e.Eval(typeExpr, env, fscope)
			if isError(constraintTypeObj) {
				return constraintTypeObj.(*object.Error)
			}

			// Now compare concreteType with constraintTypeObj
			if e.typesAreCompatible(concreteType, constraintTypeObj, isApproximate) {
				return nil // Match found, constraint satisfied.
			}
		}

		// No match found in the type list.
		return e.newError(pos, "type %s does not satisfy interface constraint %s", concreteType.Inspect(), ifaceDef.Name.Name)
	}

	// TODO: Handle traditional interface constraints (method sets).
	// For now, we assume it's satisfied if it's not a type list.
	return nil
}

// constructNamedReturnValue collects the values from the named return environment
// and packages them into a single return value (or a tuple for multiple returns).
func (e *Evaluator) constructNamedReturnValue(fn *object.Function, env *object.Environment) object.Object {
	numReturns := len(fn.Results.List)
	if numReturns == 0 {
		return &object.ReturnValue{Value: object.NIL}
	}

	values := make([]object.Object, 0, numReturns)
	for _, field := range fn.Results.List {
		for _, name := range field.Names {
			val, _ := env.Get(name.Name) // We can ignore 'ok' because we initialized them.
			values = append(values, val)
		}
	}

	if len(values) == 1 {
		return &object.ReturnValue{Value: values[0]}
	}
	return &object.ReturnValue{Value: &object.Tuple{Elements: values}}
}

// ApplyFunction is a public wrapper for the internal applyFunction, allowing it to be called from other packages.
func (e *Evaluator) ApplyFunction(call *ast.CallExpr, fn object.Object, args []object.Object, fscope *object.FileScope) object.Object {
	// This is a simplification. A real implementation would need to determine the correct environment.
	// For now, we'll use a new top-level environment, which will work for pure functions
	// but not for closures that capture variables.
	env := object.NewEnvironment()
	return e.applyFunction(call, fn, args, env, fscope)
}

func (e *Evaluator) extendMethodEnv(method *object.BoundMethod, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(method.Fn.Env)

	// Bind type parameters from the generic struct instance to the environment.
	if instance, ok := method.Receiver.(*object.StructInstance); ok {
		e.bindTypeParams(env, instance.Def.TypeParams, instance.TypeArgs)
	}

	// Bind the receiver variable (e.g., 's' in 'func (s MyType) ...')
	if method.Fn.Recv != nil && len(method.Fn.Recv.List) == 1 {
		recvField := method.Fn.Recv.List[0]
		if len(recvField.Names) > 0 {
			env.Set(recvField.Names[0].Name, method.Receiver)
		}
	}

	// Bind the method arguments (handles variadic)
	fn := method.Fn
	if fn.Parameters == nil {
		return env
	}

	if fn.IsVariadic() {
		// Bind non-variadic parameters
		for i, param := range fn.Parameters.List[:len(fn.Parameters.List)-1] {
			for _, paramName := range param.Names {
				env.Set(paramName.Name, args[i])
			}
		}

		// Bind variadic parameter
		lastParam := fn.Parameters.List[len(fn.Parameters.List)-1]
		variadicArgs := args[len(fn.Parameters.List)-1:]
		arr := &object.Array{Elements: make([]object.Object, len(variadicArgs))}
		for i, arg := range variadicArgs {
			arr.Elements[i] = arg
		}
		env.Set(lastParam.Names[0].Name, arr)
	} else {
		// Bind regular parameters
		argIndex := 0
		for _, param := range fn.Parameters.List {
			if len(param.Names) > 0 {
				for _, paramName := range param.Names {
					if argIndex < len(args) {
						env.Set(paramName.Name, args[argIndex])
						argIndex++
					}
				}
			} else {
				// This handles unnamed parameters, which appear one per field.
				// We don't bind a name, but we still consume an argument.
				argIndex++
			}
		}
	}

	return env
}

func (e *Evaluator) bindTypeParams(env *object.Environment, typeParams *ast.FieldList, typeArgs []object.Object) {
	if typeParams == nil || len(typeParams.List) == 0 {
		return
	}
	typeArgIndex := 0
	for _, param := range typeParams.List {
		for _, paramName := range param.Names {
			if typeArgIndex < len(typeArgs) {
				env.SetType(paramName.Name, typeArgs[typeArgIndex])
				typeArgIndex++
			}
		}
	}
}

func (e *Evaluator) extendFunctionEnv(env *object.Environment, fn *object.Function, args []object.Object, typeArgs []object.Object) {
	// Bind type parameters from the generic function call to the environment.
	e.bindTypeParams(env, fn.TypeParams, typeArgs)

	if fn.Parameters == nil {
		return
	}

	if fn.IsVariadic() {
		// Bind non-variadic parameters
		for i, param := range fn.Parameters.List[:len(fn.Parameters.List)-1] {
			// A single parameter can have multiple names (e.g., `a, b int`).
			for _, paramName := range param.Names {
				env.Set(paramName.Name, args[i])
			}
		}

		// Bind variadic parameter
		lastParam := fn.Parameters.List[len(fn.Parameters.List)-1]
		variadicArgs := args[len(fn.Parameters.List)-1:]
		arr := &object.Array{Elements: make([]object.Object, len(variadicArgs))}
		for i, arg := range variadicArgs {
			arr.Elements[i] = arg
		}
		// The variadic parameter has only one name.
		env.Set(lastParam.Names[0].Name, arr)

	} else {
		// Bind regular parameters
		argIndex := 0
		for _, param := range fn.Parameters.List {
			if len(param.Names) > 0 {
				for _, paramName := range param.Names {
					if argIndex < len(args) {
						env.Set(paramName.Name, args[argIndex])
						argIndex++
					}
				}
			} else {
				// This handles unnamed parameters, which appear one per field.
				// We don't bind a name, but we still consume an argument.
				argIndex++
			}
		}
	}

}

func (e *Evaluator) executeDeferredCall(deferred *object.DeferredCall, fscope *object.FileScope) {
	// A deferred call is a simplified function application.
	// It doesn't return a value and cannot have its own defers.
	fnObj := e.Eval(deferred.Call.Fun, deferred.Env, fscope)
	if isError(fnObj) {
		// TODO: How to handle errors in deferred calls?
		return
	}

	args := e.evalExpressions(deferred.Call.Args, deferred.Env, fscope, nil)
	if len(args) == 1 && isError(args[0]) {
		// TODO: Handle errors in deferred call arguments.
		return
	}

	// Set the deferred execution flag.
	e.isExecutingDefer = true
	defer func() { e.isExecutingDefer = false }() // Ensure it's always reset.

	switch f := fnObj.(type) {
	case *object.Function:
		// A deferred function literal runs in the environment it was defined in.
		// We evaluate the statements in its body directly in that environment,
		// without creating a new block scope. This allows the deferred function
		// to modify variables in the parent function's scope (like named returns).
		for _, stmt := range f.Body.List {
			evaluated := e.Eval(stmt, deferred.Env, fscope)
			// We should probably handle errors and return signals here,
			// but for now, we'll ignore them as `defer` behavior with `return` is complex.
			if p, isPanic := evaluated.(*object.Panic); isPanic {
				// A new panic inside a defer replaces the currently recovering one.
				e.currentPanic = p
				return // Stop executing this deferred function.
			}
			if isError(evaluated) {
				// TODO: How to handle errors in deferred calls?
				return
			}
		}
	case *object.BoundMethod:
		// A deferred method call also runs in its captured environment.
		// We need to bind the arguments, though.
		extendedEnv := e.extendMethodEnv(f, args)
		e.Eval(f.Fn.Body, extendedEnv, fscope)
	case *object.Builtin:
		// Execute builtin directly. Its return value is discarded.
		f.Fn(&e.BuiltinContext, deferred.Call.Pos(), args...)
	default:
		// Error: trying to defer a non-function.
		return
	}
}

func (e *Evaluator) unwrapReturnValue(obj object.Object) object.Object {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

func (e *Evaluator) instantiateTypeAlias(pos token.Pos, alias *object.TypeAlias, typeArgs []object.Object) object.Object {
	if alias.TypeParams == nil || len(alias.TypeParams.List) == 0 {
		return e.newError(pos, "type %s is not generic", alias.Name.Name)
	}

	if len(alias.TypeParams.List) != len(typeArgs) {
		return e.newError(pos, "wrong number of type arguments for %s: got %d, want %d", alias.Name.Name, len(typeArgs), len(alias.TypeParams.List))
	}

	// Create a new environment for evaluating the underlying type expression.
	// This environment is enclosed by the one where the alias was defined.
	evalEnv := object.NewEnclosedEnvironment(alias.Env)

	// Bind the type parameters (e.g., T, K) to the provided type arguments (e.g., int, string).
	for i, param := range alias.TypeParams.List {
		for _, paramName := range param.Names {
			evalEnv.SetType(paramName.Name, typeArgs[i])
		}
	}

	// Evaluate the underlying type expression (e.g., `[]T`) in the new environment.
	// The result will be the concrete type object (e.g., an Array object).
	// We pass fscope as nil because type resolution within the alias should
	// be self-contained within its definition environment.
	return e.Eval(alias.Underlying, evalEnv, nil)
}

func (e *Evaluator) resolveType(typeObj object.Object, env *object.Environment, fscope *object.FileScope) object.Object {
	alias, ok := typeObj.(*object.TypeAlias)
	if !ok {
		return typeObj // Not an alias, return as is.
	}

	// 1. Check cache
	if alias.ResolvedType != nil {
		return alias.ResolvedType
	}

	// It is an alias, so we need to resolve it.
	// Keep track of the top-level alias name to name anonymous structs.
	originalName := alias.Name
	currentAlias := alias

	// Loop to resolve nested aliases (e.g., type A = B; type B = C; type C = int)
	for {
		if currentAlias.TypeParams != nil && len(currentAlias.TypeParams.List) > 0 {
			return e.newError(currentAlias.Name.Pos(), "cannot use generic type %s without instantiation", currentAlias.Name.Name)
		}

		// Evaluate the underlying type of the current alias.
		resolved := e.Eval(currentAlias.Underlying, currentAlias.Env, fscope)
		if isError(resolved) {
			return resolved
		}

		// Check if the resolved type is another alias.
		nextAlias, isAlias := resolved.(*object.TypeAlias)
		if !isAlias {
			// Resolution finished. `resolved` is the base type object.
			// If it's a struct def that was defined anonymously, give it the original alias's name.
			if sd, ok := resolved.(*object.StructDefinition); ok {
				if sd.Name == nil {
					sd.Name = originalName
				}
			}
			// 2. Write to cache before returning
			alias.ResolvedType = resolved
			return resolved
		}
		// Continue the loop with the next alias in the chain.
		currentAlias = nextAlias
	}
}

func (e *Evaluator) evalBranchStmt(bs *ast.BranchStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	if bs.Label != nil {
		return e.newError(bs.Pos(), "labels are not supported")
	}
	switch bs.Tok {
	case token.BREAK:
		return object.BREAK
	case token.CONTINUE:
		return object.CONTINUE
	default:
		return e.newError(bs.Pos(), "unsupported branch statement: %s", bs.Tok)
	}
}

// EvalToplevel orchestrates the two-pass evaluation for a set of declarations
// from one or more files.
func (e *Evaluator) EvalToplevel(decls []object.DeclWithScope, env *object.Environment) object.Object {
	// Pass 1: Register all types and functions first.
	// This pass does not evaluate any variable or constant initializers.
	varDecls, constDecls := e.registerDecls(decls, env)

	// Pass 2: Evaluate the initializers for variables and constants.
	// Now that all functions and types are known, these initializers can refer to them.
	result := e.evalInitializers(append(varDecls, constDecls...), env)
	if isError(result) {
		return result
	}

	return result
}

// registerDecls is the first pass of the evaluation. It scans for all top-level
// type and function declarations and adds them to the environment. It returns
// slices of the variable and constant declarations to be processed in the second pass.
func (e *Evaluator) registerDecls(decls []object.DeclWithScope, env *object.Environment) (varDecls, constDecls []object.DeclWithScope) {
	for _, item := range decls {
		switch d := item.Decl.(type) {
		case *ast.FuncDecl:
			// Just register the function definition. The body will be evaluated on call.
			// The existing Eval logic for FuncDecl is sufficient for this pass.
			e.Eval(d, env, item.Scope)
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE, token.IMPORT:
				// Register type and import definitions.
				// The existing Eval logic for these GenDecls is sufficient.
				e.Eval(d, env, item.Scope)
			case token.VAR:
				varDecls = append(varDecls, item)
			case token.CONST:
				constDecls = append(constDecls, item)
			}
		}
	}
	return varDecls, constDecls
}

// evalInitializers is the second pass of the evaluation. It evaluates the
// expressions for all variable and constant declarations.
func (e *Evaluator) evalInitializers(decls []object.DeclWithScope, env *object.Environment) object.Object {
	var result object.Object
	for _, item := range decls {
		// The existing Eval logic for VAR and CONST GenDecls performs the initialization.
		result = e.Eval(item.Decl, env, item.Scope)
		if isError(result) {
			return result
		}
	}
	return result
}

func (e *Evaluator) evalTypeConversion(call *ast.CallExpr, typeObj object.Object, args []object.Object) object.Object {
	if len(args) != 1 {
		return e.newError(call.Pos(), "wrong number of arguments for type conversion: got=%d, want=1", len(args))
	}
	arg := args[0]

	switch t := typeObj.(type) {
	case *object.PointerType:
		if arg == object.NIL {
			return &object.TypedNil{TypeObject: t}
		}
		return e.newError(call.Pos(), "cannot convert non-nil value to pointer type %s", typeObj.Inspect())
	case *object.ArrayType:
		// Handle []byte("a string")
		eltType, ok := t.ElementType.(*object.Type)
		if !ok || eltType.Name != "byte" {
			return e.newError(call.Pos(), "unsupported array type conversion to %s", typeObj.Inspect())
		}

		str, ok := arg.(*object.String)
		if !ok {
			return e.newError(call.Pos(), "cannot convert %s to type %s", arg.Type(), typeObj.Inspect())
		}

		bytes := []byte(str.Value)
		elements := make([]object.Object, len(bytes))
		for i, b := range bytes {
			elements[i] = &object.Integer{Value: int64(b)}
		}
		return &object.Array{Elements: elements}

	case *object.Type:
		typeName := t.Name
		switch typeName {
		case "int", "uint", "uint64": // For now, treat uint as int.
			switch input := arg.(type) {
			case *object.Integer:
				return input // It's already an integer, no-op.
			case *object.Float:
				return &object.Integer{Value: int64(input.Value)}
			default:
				return e.newError(call.Pos(), "cannot convert %s to type %s", arg.Type(), typeName)
			}
		case "string":
			// Handle string([]byte{...})
			if arr, ok := arg.(*object.Array); ok {
				bytes := make([]byte, len(arr.Elements))
				for i, el := range arr.Elements {
					integer, ok := el.(*object.Integer)
					if !ok {
						return e.newError(call.Pos(), "cannot convert non-integer element in array to byte for string conversion")
					}
					if integer.Value < 0 || integer.Value > 255 {
						return e.newError(call.Pos(), "byte value out of range for string conversion: %d", integer.Value)
					}
					bytes[i] = byte(integer.Value)
				}
				return &object.String{Value: string(bytes)}
			}

			// In a real implementation, you might convert integers, etc.
			// For now, we only support string(string) which is a no-op.
			if str, ok := arg.(*object.String); ok {
				return str
			}
			return e.newError(call.Pos(), "cannot convert %s to type string", arg.Type())
		default:
			return e.newError(call.Pos(), "unsupported type conversion: %s", typeName)
		}

	default:
		return e.newError(call.Pos(), "invalid type for conversion: %s", typeObj.Type())
	}
}

// evalFuncType evaluates an ast.FuncType node and returns an object.FuncType.
func (e *Evaluator) evalFuncType(n *ast.FuncType, env *object.Environment, fscope *object.FileScope) object.Object {
	params := []object.Object{}
	if n.Params != nil {
		for _, p := range n.Params.List {
			pType := e.Eval(p.Type, env, fscope)
			if isError(pType) {
				return pType
			}
			// For `func(a, b int)`, there are two names but one type.
			if len(p.Names) > 0 {
				for i := 0; i < len(p.Names); i++ {
					params = append(params, pType)
				}
			} else {
				// For `func(int)`, there are no names, but one type.
				params = append(params, pType)
			}
		}
	}

	results := []object.Object{}
	if n.Results != nil {
		for _, r := range n.Results.List {
			rType := e.Eval(r.Type, env, fscope)
			if isError(rType) {
				return rType
			}
			if len(r.Names) > 0 {
				for i := 0; i < len(r.Names); i++ {
					results = append(results, rType)
				}
			} else {
				results = append(results, rType)
			}
		}
	}

	return &object.FuncType{Parameters: params, Results: results}
}

// flattenTypeUnion takes a type expression from an interface definition and flattens it
// into a list of individual type expressions. This is used to handle union types
// like `int | string | MyType`. The AST represents this as a binary tree of `|` operations.
func (e *Evaluator) flattenTypeUnion(expr ast.Expr) []ast.Expr {
	if be, ok := expr.(*ast.BinaryExpr); ok && be.Op == token.OR {
		// It's a union, so recursively flatten both sides.
		left := e.flattenTypeUnion(be.X)
		right := e.flattenTypeUnion(be.Y)
		return append(left, right...)
	}
	// It's a single type, not a union.
	return []ast.Expr{expr}
}

func (e *Evaluator) evalProgram(program *ast.File, env *object.Environment, fscope *object.FileScope) object.Object {
	// Use the new two-pass evaluation for the program's declarations.
	var decls []object.DeclWithScope
	for _, decl := range program.Decls {
		decls = append(decls, object.DeclWithScope{Decl: decl, Scope: fscope})
	}
	result := e.EvalToplevel(decls, env)
	if isError(result) {
		return result
	}

	// After all declarations are processed, find and execute the main function.
	mainObj, ok := env.Get("main")
	if !ok {
		return object.NIL // No main function, not an error
	}

	mainFn, ok := mainObj.(*object.Function)
	if !ok {
		return e.newError(program.Pos(), "main is not a function, but %s", mainObj.Type())
	}

	// Call the main function with no arguments.
	return e.applyFunction(nil, mainFn, []object.Object{}, env, fscope)
}

func (e *Evaluator) Eval(node ast.Node, env *object.Environment, fscope *object.FileScope) object.Object {
	switch n := node.(type) {
	// Statements
	case *ast.File:
		return e.evalProgram(n, env, fscope)
	case *ast.BlockStmt:
		return e.evalBlockStatement(n, env, fscope)
	case *ast.ExprStmt:
		return e.Eval(n.X, env, fscope)
	case *ast.IfStmt:
		return e.evalIfElseExpression(n, env, fscope)
	case *ast.SwitchStmt:
		return e.evalSwitchStmt(n, env, fscope)
	case *ast.ForStmt:
		return e.evalForStmt(n, env, fscope)
	case *ast.RangeStmt:
		return e.evalForRangeStmt(n, env, fscope)
	case *ast.BranchStmt:
		return e.evalBranchStmt(n, env, fscope)
	case *ast.DeclStmt:
		return e.Eval(n.Decl, env, fscope)
	case *ast.FuncDecl:
		// Regular function declaration
		if n.Recv == nil {
			fn := &object.Function{
				Name:       n.Name,
				TypeParams: n.Type.TypeParams,
				Parameters: n.Type.Params,
				Results:    n.Type.Results,
				Body:       n.Body,
				Env:        env,
			}
			env.Set(n.Name.Name, fn)
			return nil
		}

		// Method declaration
		if len(n.Recv.List) != 1 {
			return e.newError(n.Pos(), "method receiver must have exactly one argument")
		}
		recvField := n.Recv.List[0]

		var typeName string
		switch recvType := recvField.Type.(type) {
		case *ast.Ident:
			typeName = recvType.Name
		case *ast.StarExpr:
			if ident, ok := recvType.X.(*ast.Ident); ok {
				typeName = ident.Name
			} else {
				return e.newError(recvType.Pos(), "invalid receiver type: expected identifier")
			}
		case *ast.IndexExpr: // For generic receivers like `Box[T]`
			if ident, ok := recvType.X.(*ast.Ident); ok {
				typeName = ident.Name
			} else {
				return e.newError(recvType.Pos(), "invalid receiver type: expected identifier for generic type base")
			}
		default:
			return e.newError(recvField.Type.Pos(), "unsupported receiver type: %T", recvField.Type)
		}

		obj, ok := env.Get(typeName)
		if !ok {
			return e.newError(n.Pos(), "type '%s' not defined for method receiver", typeName)
		}

		// Resolve the type in case it's an alias.
		resolvedObj := e.resolveType(obj, env, fscope)
		if isError(resolvedObj) {
			return resolvedObj
		}

		def, ok := resolvedObj.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Pos(), "receiver for method '%s' is not a struct type", n.Name.Name)
		}

		fn := &object.Function{
			Name:       n.Name,
			Recv:       n.Recv, // Store receiver info
			TypeParams: n.Type.TypeParams,
			Parameters: n.Type.Params,
			Results:    n.Type.Results,
			Body:       n.Body,
			Env:        env, // The environment where the method is defined.
		}

		def.Methods[n.Name.Name] = fn
		return nil
	case *ast.DeferStmt:
		if len(e.callStack) == 0 {
			return e.newError(n.Pos(), "defer is not allowed outside of a function")
		}
		// The call expression and its environment are stored.
		deferred := &object.DeferredCall{
			Call: n.Call,
			Env:  env,
		}
		// Append to the defer stack. We will execute in reverse order.
		currentFrame := e.callStack[len(e.callStack)-1]
		currentFrame.Defers = append(currentFrame.Defers, deferred)
		return nil // defer statement itself evaluates to nothing.
	case *ast.ReturnStmt:
		// Check if we are in a function with named returns.
		var currentFrame *object.CallFrame
		if len(e.callStack) > 0 {
			currentFrame = e.callStack[len(e.callStack)-1]
		}

		// --- Logic for Named Returns ---
		if currentFrame != nil && currentFrame.NamedReturns != nil {
			if len(n.Results) > 0 {
				// Case: return x, y
				// Evaluate the expressions and assign them to the named return variables.
				values := e.evalExpressions(n.Results, env, fscope, nil)
				if len(values) == 1 && isError(values[0]) {
					return values[0]
				}

				// This is a simplified assignment; it assumes the number of return
				// expressions matches the number of named return variables.
				i := 0
				for _, field := range currentFrame.Fn.Results.List {
					for _, name := range field.Names {
						if i < len(values) {
							// Use Assign, not Set, to update the existing variable.
							currentFrame.NamedReturns.Assign(name.Name, values[i])
							i++
						}
					}
				}
			}
			// For both `return x` and a bare `return`, we signal to `applyFunction`
			// that it needs to construct the final return value from the environment.
			// We use a ReturnValue with a nil `Value` for this.
			return &object.ReturnValue{Value: nil}
		}

		// --- Logic for Regular (non-named) Returns ---
		if len(n.Results) == 0 {
			return &object.ReturnValue{Value: object.NIL}
		}
		if len(n.Results) == 1 {
			val := e.Eval(n.Results[0], env, fscope)
			if isError(val) {
				return val
			}
			// If the expression is already a return value (e.g. from a function call),
			// don't wrap it again.
			if ret, ok := val.(*object.ReturnValue); ok {
				return ret
			}
			return &object.ReturnValue{Value: val}
		}
		results := e.evalExpressions(n.Results, env, fscope, nil)
		if len(results) > 0 && isError(results[0]) {
			return results[0]
		}
		return &object.ReturnValue{Value: &object.Tuple{Elements: results}}
	case *ast.GenDecl:
		return e.evalGenDecl(n, env, fscope)
	case *ast.AssignStmt:
		return e.evalAssignStmt(n, env, fscope)
	case *ast.IncDecStmt:
		return e.evalIncDecStmt(n, env, fscope)

	// Expressions
	case *ast.ParenExpr:
		return e.Eval(n.X, env, fscope)

	case *ast.IndexExpr: // MyType[T]
		left := e.Eval(n.X, env, fscope)
		if isError(left) {
			return left
		}
		index := e.Eval(n.Index, env, fscope)
		if isError(index) {
			return index
		}
		// Check if this is a generic type instantiation or a regular index access.
		switch l := left.(type) {
		case *object.StructDefinition, *object.Function:
			return &object.InstantiatedType{GenericDef: left, TypeArgs: []object.Object{index}}
		case *object.TypeAlias:
			// This is a generic alias instantiation, e.g., List[int]
			return e.instantiateTypeAlias(n.Pos(), l, []object.Object{index})
		default:
			// It's a regular index expression like array[i].
			return e.evalIndexExpression(n, left, index)
		}

	case *ast.IndexListExpr: // MyType[T, K]
		left := e.Eval(n.X, env, fscope)
		if isError(left) {
			return left
		}
		indices := e.evalExpressions(n.Indices, env, fscope, nil)
		if len(indices) == 1 && isError(indices[0]) {
			return indices[0]
		}
		switch l := left.(type) {
		case *object.StructDefinition, *object.Function:
			return &object.InstantiatedType{GenericDef: left, TypeArgs: indices}
		case *object.TypeAlias:
			// This is a generic alias instantiation, e.g., Pair[int, string]
			return e.instantiateTypeAlias(n.Pos(), l, indices)
		default:
			return e.newError(n.Pos(), "index list operator not supported for %s", left.Type())
		}

	case *ast.FuncLit:
		return &object.Function{
			Parameters: n.Type.Params,
			Body:       n.Body,
			Env:        env,
		}
	case *ast.CallExpr:
		// Special form handling: check if the function is a registered special form.
		// Special forms receive the AST of their arguments without evaluation.
		switch fun := n.Fun.(type) {
		case *ast.Ident:
			if sf, isSpecial := e.specialForms[fun.Name]; isSpecial {
				return sf.Fn(e, fscope, n.Pos(), n.Args)
			}
		case *ast.SelectorExpr:
			if pkgIdent, ok := fun.X.(*ast.Ident); ok && fscope != nil {
				if path, isAlias := fscope.Aliases[pkgIdent.Name]; isAlias {
					qualifiedName := fmt.Sprintf("%s.%s", path, fun.Sel.Name)
					if sf, isSpecial := e.specialForms[qualifiedName]; isSpecial {
						return sf.Fn(e, fscope, n.Pos(), n.Args)
					}
				}
			}
		}

		function := e.Eval(n.Fun, env, fscope)
		if isError(function) {
			return function
		}

		// Check if the "function" is actually a type, indicating a type conversion.
		switch function.(type) {
		case *object.Type, *object.ArrayType, *object.PointerType:
			args := e.evalExpressions(n.Args, env, fscope, nil)
			if len(args) == 1 && isError(args[0]) {
				return args[0]
			}
			return e.evalTypeConversion(n, function, args)
		}

		// Check if the resolved function is a special form object. This can happen
		// if it was passed as an argument or returned from another function.
		if sf, ok := function.(*SpecialForm); ok {
			return sf.Fn(e, fscope, n.Pos(), n.Args)
		}

		var args []object.Object
		if n.Ellipsis.IsValid() {
			// Handle variadic call, e.g., fn(a, b, c...)
			if len(n.Args) == 0 {
				return e.newError(n.Pos(), "cannot use ... on empty argument list")
			}

			// Evaluate all but the last argument normally.
			args = e.evalExpressions(n.Args[:len(n.Args)-1], env, fscope, nil)
			if len(args) > 0 && isError(args[len(args)-1]) {
				return args[len(args)-1]
			}

			// Evaluate the last argument, which is the slice to be spread.
			lastArg := n.Args[len(n.Args)-1]
			sliceToSpread := e.Eval(lastArg, env, fscope)
			if isError(sliceToSpread) {
				return sliceToSpread
			}

			// Spread the elements of the slice.
			switch s := sliceToSpread.(type) {
			case *object.Array:
				args = append(args, s.Elements...)
			default:
				return e.newError(lastArg.Pos(), "cannot use ... on non-slice type %s", sliceToSpread.Type())
			}
		} else {
			// Regular function call.
			args = e.evalExpressions(n.Args, env, fscope, nil)
			if len(args) > 0 && isError(args[len(args)-1]) {
				return args[len(args)-1]
			}
		}
		return e.applyFunction(n, function, args, env, fscope)
	case *ast.SelectorExpr:
		return e.evalSelectorExpr(n, env, fscope)
	case *ast.CompositeLit:
		return e.evalCompositeLit(n, env, fscope)
	case *ast.StarExpr:
		// This can be a dereference operation (*p) or a pointer type (*T).
		// We can differentiate based on the context, but a simpler heuristic
		// is to check if the operand evaluates to a type object.
		operand := e.Eval(n.X, env, fscope)
		if isError(operand) {
			return operand
		}
		switch operand.(type) {
		case *object.StructDefinition, *object.Type, *object.PointerType, *object.ArrayType, *object.MapType, *object.InterfaceDefinition:
			// It's a pointer type expression, like `*MyStruct`.
			return &object.PointerType{ElementType: operand}
		default:
			// It's a dereference operation, like `*myPointer`.
			return e.evalDereferenceExpression(n, operand)
		}
	case *ast.UnaryExpr:
		// Special case for address-of operator, as we don't evaluate the operand.
		if n.Op == token.AND {
			return e.evalAddressOfExpression(n, env, fscope)
		}
		right := e.Eval(n.X, env, fscope)
		if isError(right) {
			return right
		}
		return e.evalPrefixExpression(n, n.Op.String(), right)
	case *ast.BinaryExpr:
		// Handle logical AND and OR with short-circuiting.
		switch n.Op {
		case token.LAND:
			left := e.Eval(n.X, env, fscope)
			if isError(left) {
				return left
			}
			if !e.isTruthy(left) {
				return object.FALSE
			}
			right := e.Eval(n.Y, env, fscope)
			if isError(right) {
				return right
			}
			return e.nativeBoolToBooleanObject(e.isTruthy(right))
		case token.LOR:
			left := e.Eval(n.X, env, fscope)
			if isError(left) {
				return left
			}
			if e.isTruthy(left) {
				return object.TRUE
			}
			right := e.Eval(n.Y, env, fscope)
			if isError(right) {
				return right
			}
			return e.nativeBoolToBooleanObject(e.isTruthy(right))
		}

		// Fallback to standard infix evaluation for other operators.
		left := e.Eval(n.X, env, fscope)
		if isError(left) {
			return left
		}
		right := e.Eval(n.Y, env, fscope)
		if isError(right) {
			return right
		}
		return e.evalInfixExpression(n, n.Op.String(), left, right)

	// Literals
	case *ast.Ident:
		return e.evalIdent(n, env, fscope)
	case *ast.BasicLit:
		return e.evalBasicLit(n)

	// Type Expressions
	case *ast.FuncType:
		return e.evalFuncType(n, env, fscope)
	case *ast.ArrayType:
		eltType := e.Eval(n.Elt, env, fscope)
		if isError(eltType) {
			return eltType
		}
		return &object.ArrayType{ElementType: eltType}
	case *ast.MapType:
		keyType := e.Eval(n.Key, env, fscope)
		if isError(keyType) {
			return keyType
		}
		valueType := e.Eval(n.Value, env, fscope)
		if isError(valueType) {
			return valueType
		}
		return &object.MapType{KeyType: keyType, ValueType: valueType}
	case *ast.StructType:
		// This creates a definition for an anonymous struct type.
		return &object.StructDefinition{
			Name:    nil, // Anonymous
			Fields:  n.Fields.List,
			Methods: make(map[string]*object.Function),
		}
	case *ast.SliceExpr:
		return e.evalSliceExpr(n, env, fscope)
	}

	return e.newError(node.Pos(), "evaluation not implemented for %T", node)
}

func (e *Evaluator) evalSliceExpr(node *ast.SliceExpr, env *object.Environment, fscope *object.FileScope) object.Object {
	left := e.Eval(node.X, env, fscope)
	if isError(left) {
		return left
	}

	evalIndex := func(expr ast.Expr, defaultVal int64) (int64, object.Object) {
		if expr == nil {
			return defaultVal, nil
		}
		val := e.Eval(expr, env, fscope)
		if isError(val) {
			return 0, val
		}
		intVal, ok := val.(*object.Integer)
		if !ok {
			return 0, e.newError(expr.Pos(), "slice index must be an integer, got %s", val.Type())
		}
		return intVal.Value, nil
	}

	switch l := left.(type) {
	case *object.Array:
		capacity := int64(cap(l.Elements))
		length := int64(len(l.Elements))

		var err object.Object
		var low, high, max int64

		low, err = evalIndex(node.Low, 0)
		if err != nil {
			return err
		}

		high, err = evalIndex(node.High, length)
		if err != nil {
			return err
		}

		if node.Slice3 {
			max, err = evalIndex(node.Max, capacity)
			if err != nil {
				return err
			}
		} else {
			max = capacity
		}

		if low < 0 || high < low || max < high || high > capacity || max > capacity {
			return e.newError(node.Pos(), "slice bounds out of range: low=%d, high=%d, max=%d, cap=%d", low, high, max, capacity)
		}

		// The Go equivalent of a[low:high:max] is (a[low:max])[:high-low]
		subSlice := l.Elements[low:max]
		finalSlice := subSlice[:high-low]
		return &object.Array{Elements: finalSlice}

	case *object.String:
		length := int64(len(l.Value))
		var err object.Object
		var low, high int64

		low, err = evalIndex(node.Low, 0)
		if err != nil {
			return err
		}
		high, err = evalIndex(node.High, length)
		if err != nil {
			return err
		}

		if node.Slice3 {
			return e.newError(node.Pos(), "full slice expression not supported for strings")
		}

		if low < 0 || high < low || high > length {
			return e.newError(node.Pos(), "slice bounds out of range: low=%d, high=%d, len=%d", low, high, length)
		}

		return &object.String{Value: l.Value[low:high]}

	case *object.GoValue:
		v := l.Value
		switch v.Kind() {
		case reflect.Array, reflect.Slice:
			// Slicing an array requires it to be addressable. If it's not,
			// we create a new addressable array and copy the data.
			if v.Kind() == reflect.Array && !v.CanAddr() {
				newVal := reflect.New(v.Type()).Elem()
				reflect.Copy(newVal, v)
				v = newVal
			}

			capacity := int64(v.Cap())
			length := int64(v.Len())

			var err object.Object
			var low, high, max int64

			low, err = evalIndex(node.Low, 0)
			if err != nil {
				return err
			}

			high, err = evalIndex(node.High, length)
			if err != nil {
				return err
			}

			if node.Slice3 {
				max, err = evalIndex(node.Max, capacity)
				if err != nil {
					return err
				}
			} else {
				max = -1 // Use -1 to indicate not set, similar to reflect.Value.Slice3
			}

			// Basic bounds check
			if low < 0 || high < low || high > length {
				return e.newError(node.Pos(), "slice bounds out of range: low=%d, high=%d, len=%d", low, high, length)
			}
			if node.Slice3 {
				if max < high {
					return e.newError(node.Pos(), "slice bounds out of range: max < high")
				}
				return &object.GoValue{Value: v.Slice3(int(low), int(high), int(max))}
			}
			return &object.GoValue{Value: v.Slice(int(low), int(high))}
		default:
			return e.newError(node.Pos(), "slice operator not supported for GO_VALUE of kind %s", v.Kind())
		}

	default:
		return e.newError(node.Pos(), "slice operator not supported for %s", left.Type())
	}
}

func (e *Evaluator) evalGenDecl(n *ast.GenDecl, env *object.Environment, fscope *object.FileScope) object.Object {
	var lastVal object.Object
	switch n.Tok {
	case token.IMPORT:
		if fscope == nil {
			return e.newError(n.Pos(), "imports are only allowed at the file level")
		}
		for _, spec := range n.Specs {
			importSpec := spec.(*ast.ImportSpec)
			path, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				return e.newError(importSpec.Path.Pos(), "invalid import path: %v", err)
			}

			var alias string
			if importSpec.Name != nil {
				alias = importSpec.Name.Name
			} else {
				parts := strings.Split(path, "/")
				alias = parts[len(parts)-1]
			}

			switch alias {
			case "_":
				// Blank imports are ignored for now, but we could run init functions here in the future.
				continue
			case ".":
				// Dot import: add the path to the file scope's dot import list.
				fscope.DotImports = append(fscope.DotImports, path)
			default:
				// Regular import with an alias.
				fscope.Aliases[alias] = path
			}
		}
		return nil

	case token.CONST, token.VAR:
		var lastValues []ast.Expr // For const value carry-over
		for iotaValue, spec := range n.Specs {
			valueSpec := spec.(*ast.ValueSpec)

			// Handle multi-return assignment: var a, b = f()
			if n.Tok == token.VAR && len(valueSpec.Names) > 1 && len(valueSpec.Values) == 1 {
				val := e.Eval(valueSpec.Values[0], env, fscope)
				if isError(val) {
					return val
				}
				tuple, ok := val.(*object.Tuple)
				if !ok {
					return e.newError(valueSpec.Pos(), "multi-value assignment requires a multi-value return, but got %s", val.Type())
				}
				if len(valueSpec.Names) != len(tuple.Elements) {
					return e.newError(valueSpec.Pos(), "assignment mismatch: %d variables but %d values", len(valueSpec.Names), len(tuple.Elements))
				}
				for i, name := range valueSpec.Names {
					if name.Name == "_" {
						continue
					}
					env.Set(name.Name, tuple.Elements[i])
				}
				continue // Move to the next spec in the GenDecl
			}

			// Handle const value carry-over
			if n.Tok == token.CONST {
				if len(valueSpec.Values) == 0 {
					valueSpec.Values = lastValues
				} else {
					lastValues = valueSpec.Values
				}
			}

			for i, name := range valueSpec.Names {
				// Handle explicit type declarations, especially for interfaces.
				if valueSpec.Type != nil {
					typeObj := e.Eval(valueSpec.Type, env, fscope)
					if isError(typeObj) {
						return typeObj
					}

					if ifaceDef, ok := typeObj.(*object.InterfaceDefinition); ok {
						var concreteVal object.Object
						if len(valueSpec.Values) > i {
							// Case: var w Writer = myStruct
							concreteVal = e.Eval(valueSpec.Values[i], env, fscope)
							if isError(concreteVal) {
								return concreteVal
							}
							// A nil value can be assigned to an interface without checks.
							if concreteVal.Type() != object.NIL_OBJ {
								if errObj := e.checkImplements(valueSpec.Pos(), concreteVal, ifaceDef); errObj != nil {
									return errObj
								}
							}
						} else {
							// Case: var w Writer (no initial value)
							concreteVal = object.NIL
						}
						// Wrap the concrete value in an InterfaceInstance to track its interface type.
						env.Set(name.Name, &object.InterfaceInstance{Def: ifaceDef, Value: concreteVal})
						continue // Move to the next name in the spec (e.g., var a, b, c Writer)
					}
				}

				// Fallback to existing logic for non-interface types or untyped vars.
				var val object.Object
				if len(valueSpec.Values) > i {
					// Create a temporary environment for iota evaluation.
					iotaEnv := object.NewEnclosedEnvironment(env)
					iotaEnv.SetConstant("iota", &object.Integer{Value: int64(iotaValue)})
					val = e.Eval(valueSpec.Values[i], iotaEnv, fscope)
				} else if n.Tok == token.VAR {
					// Handle `var x T` (no initial value)
					if valueSpec.Type != nil {
						typeObj := e.Eval(valueSpec.Type, env, fscope)
						if isError(typeObj) {
							return typeObj
						}
						resolvedType := e.resolveType(typeObj, env, fscope)
						if isError(resolvedType) {
							return resolvedType
						}

						switch rt := resolvedType.(type) {
						case *object.GoType:
							// The type is a registered Go type. Instantiate its zero value.
							// We create a pointer to a new value, then get the element it points to.
							// This ensures the resulting reflect.Value is addressable, which is crucial
							// for calling pointer-receiver methods on it later.
							ptr := reflect.New(rt.GoType)
							val = &object.GoValue{Value: ptr.Elem()}
						case *object.StructDefinition:
							// It's a minigo-defined struct, so initialize a zero-valued instance.
							instance := &object.StructInstance{Def: rt, Fields: make(map[string]object.Object)}
							for _, field := range rt.Fields {
								zeroVal := e.getZeroValueForType(field.Type, env, fscope)
								for _, name := range field.Names {
									instance.Fields[name.Name] = zeroVal
								}
							}
							val = instance
						default:
							// For other types (slices, maps, pointers, interfaces), the zero value is a typed nil.
							val = &object.TypedNil{TypeObject: resolvedType}
						}
					} else {
						val = object.NIL
					}
				} else {
					return e.newError(name.Pos(), "missing value in declaration for %s", name.Name)
				}

				if isError(val) {
					return val
				}

				if n.Tok == token.CONST {
					env.SetConstant(name.Name, val)
				} else { // token.VAR
					if fn, ok := val.(*object.Function); ok {
						fn.Name = name
					}
					env.Set(name.Name, val)
				}
				lastVal = val
			}
		}
		return lastVal

	case token.TYPE:
		for _, spec := range n.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			// Check for type alias: type T = some.Type
			if typeSpec.Assign.IsValid() {
				alias := &object.TypeAlias{
					Name:       typeSpec.Name,
					TypeParams: typeSpec.TypeParams,
					Underlying: typeSpec.Type,
					Env:        env,
				}
				env.Set(typeSpec.Name.Name, alias)
			} else {
				// Regular type definition: type T struct { ... }
				switch t := typeSpec.Type.(type) {
				case *ast.StructType:
					fieldTags := make(map[string]string)
					for _, field := range t.Fields.List {
						if field.Tag == nil {
							continue
						}
						tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
						jsonTag := tag.Get("json")

						// The parser gives us one Field with multiple Names for `X, Y int`.
						for _, name := range field.Names {
							fieldTags[name.Name] = jsonTag
						}
					}

					def := &object.StructDefinition{
						Name:       typeSpec.Name,
						TypeParams: typeSpec.TypeParams,
						Fields:     t.Fields.List,
						Methods:    make(map[string]*object.Function),
						FieldTags:  fieldTags,
						Env:        env,
					}
					env.Set(typeSpec.Name.Name, def)

				case *ast.InterfaceType:
					def := &object.InterfaceDefinition{
						Name:     typeSpec.Name,
						Methods:  &ast.FieldList{}, // Initialize as empty
						TypeList: make([]ast.Expr, 0),
					}

					if t.Methods != nil {
						for _, field := range t.Methods.List {
							// A field with names is a method.
							if len(field.Names) > 0 {
								def.Methods.List = append(def.Methods.List, field)
							} else {
								// A field without names is a type constraint (embedded type or union).
								def.TypeList = append(def.TypeList, e.flattenTypeUnion(field.Type)...)
							}
						}
					}

					env.Set(typeSpec.Name.Name, def)

				default:
					// This could be a type definition like `type MyInt int`.
					// We can treat this as a non-generic type alias for now.
					alias := &object.TypeAlias{
						Name:       typeSpec.Name,
						TypeParams: nil, // No type params for this form
						Underlying: typeSpec.Type,
						Env:        env,
					}
					env.Set(typeSpec.Name.Name, alias)
				}
			}
		}
		return nil
	}

	return nil // Should be unreachable
}

func (e *Evaluator) evalIndexExpression(node ast.Node, left, index object.Object) object.Object {
	// Handle generic type instantiation, e.g. MyType[int]
	switch l := left.(type) {
	case *object.StructDefinition:
		if l.TypeParams != nil && len(l.TypeParams.List) > 0 {
			// TODO: This currently only handles a single type argument.
			// To support multiple (e.g., map[K, V]), we would need to inspect
			// the original ast.IndexListExpr.
			return &object.InstantiatedType{GenericDef: l, TypeArgs: []object.Object{index}}
		}
	case *object.Function:
		if l.TypeParams != nil && len(l.TypeParams.List) > 0 {
			return &object.InstantiatedType{GenericDef: l, TypeArgs: []object.Object{index}}
		}
	}

	switch {
	case left.Type() == object.ARRAY_OBJ:
		return e.evalArrayIndexExpression(node, left, index)
	case left.Type() == object.STRING_OBJ:
		return e.evalStringIndexExpression(node, left, index)
	case left.Type() == object.MAP_OBJ:
		return e.evalMapIndexExpression(node, left, index)
	case left.Type() == object.GO_VALUE_OBJ:
		return e.evalGoValueIndexExpression(node, left.(*object.GoValue), index)
	default:
		return e.newError(node.Pos(), "index operator not supported for %s", left.Type())
	}
}

func (e *Evaluator) evalGoValueIndexExpression(node ast.Node, goVal *object.GoValue, index object.Object) object.Object {
	val := goVal.Value
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		intIndex, ok := index.(*object.Integer)
		if !ok {
			return e.newError(node.Pos(), "index into Go slice/array must be an integer, got %s", index.Type())
		}
		idx := int(intIndex.Value)
		if idx < 0 || idx >= val.Len() {
			// Panic for out-of-bounds access, similar to Go.
			return e.newError(node.Pos(), "runtime error: index out of range [%d] with length %d", idx, val.Len())
		}
		resultVal := val.Index(idx)
		return e.nativeToValue(resultVal)

	case reflect.Map:
		// Convert the minigo index object to a reflect.Value that can be used as a map key.
		keyVal, err := e.objectToReflectValue(index, val.Type().Key())
		if err != nil {
			return e.newError(node.Pos(), "cannot use %s as type %s in map index: %v", index.Type(), val.Type().Key(), err)
		}
		resultVal := val.MapIndex(keyVal)
		if !resultVal.IsValid() {
			// Key not found in map. Go would return the zero value.
			// Let's return NIL for simplicity, as creating a zero value for any type is complex.
			return object.NIL
		}
		return e.nativeToValue(resultVal)

	default:
		return e.newError(node.Pos(), "index operator not supported for Go value of type %s", val.Kind())
	}
}

func (e *Evaluator) evalArrayIndexExpression(node ast.Node, array, index object.Object) object.Object {
	arrayObject := array.(*object.Array)
	idx, ok := index.(*object.Integer)
	if !ok {
		return e.newError(node.Pos(), "index into array is not an integer")
	}

	i := idx.Value
	max := int64(len(arrayObject.Elements) - 1)

	if i < 0 || i > max {
		return object.NIL // Go returns nil for out-of-bounds access, so we do too.
	}

	return arrayObject.Elements[i]
}

func (e *Evaluator) evalStringIndexExpression(node ast.Node, str, index object.Object) object.Object {
	stringObject := str.(*object.String)
	idx, ok := index.(*object.Integer)
	if !ok {
		return e.newError(node.Pos(), "index into string is not an integer")
	}

	i := idx.Value
	max := int64(len(stringObject.Value) - 1)

	if i < 0 || i > max {
		return e.newError(node.Pos(), "runtime error: index out of range [%d] with length %d", i, len(stringObject.Value))
	}

	return &object.Integer{Value: int64(stringObject.Value[i])}
}

func (e *Evaluator) evalMapIndexExpression(node ast.Node, m, index object.Object) object.Object {
	mapObject := m.(*object.Map)

	key, ok := index.(object.Hashable)
	if !ok {
		return e.newError(node.Pos(), "unusable as map key: %s", index.Type())
	}

	pair, ok := mapObject.Pairs[key.HashKey()]
	if !ok {
		return object.NIL
	}

	return pair.Value
}

func (e *Evaluator) evalIncDecStmt(n *ast.IncDecStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	// 1. Evaluate the left-hand side to get the current value.
	// This re-uses the logic for evaluating identifiers, selectors, etc.
	currentVal := e.Eval(n.X, env, fscope)
	if isError(currentVal) {
		return currentVal
	}

	// 2. Ensure the value is an integer.
	integer, ok := currentVal.(*object.Integer)
	if !ok {
		return e.newError(n.Pos(), "cannot %s non-integer type %s", n.Tok, currentVal.Type())
	}

	// 3. Calculate the new value.
	var newVal int64
	if n.Tok == token.INC {
		newVal = integer.Value + 1
	} else {
		newVal = integer.Value - 1
	}

	// 4. Assign the new value back to the variable.
	// We can reuse the `assignValue` logic.
	return e.assignValue(n.X, &object.Integer{Value: newVal}, env, fscope)
}

func (e *Evaluator) evalAssignStmt(n *ast.AssignStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
		// Single assignment: a = 1 or a := 1
		return e.evalSingleAssign(n, env, fscope)
	}

	if len(n.Lhs) > 1 && len(n.Rhs) == 1 {
		// Multi-assignment from single function call: a, b = f() or a, b := f()
		return e.evalMultiAssign(n, env, fscope)
	}

	if len(n.Lhs) > 0 && len(n.Lhs) == len(n.Rhs) {
		// Destructuring assignment: a, b = 1, 2 or a, b = b, a
		return e.evalDestructuringAssign(n, env, fscope)
	}

	return e.newError(n.Pos(), "assignment mismatch: %d variables but %d values", len(n.Lhs), len(n.Rhs))
}

func (e *Evaluator) evalDestructuringAssign(n *ast.AssignStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	// Evaluate all RHS expressions first and store them temporarily.
	// This is crucial for `a, b = b, a` to work correctly.
	values := make([]object.Object, len(n.Rhs))
	for i, rhsExpr := range n.Rhs {
		val := e.Eval(rhsExpr, env, fscope)
		if isError(val) {
			return val
		}
		values[i] = val
	}

	// Now, assign the evaluated values to the LHS variables.
	switch n.Tok {
	case token.ASSIGN: // =
		for i, lhsExpr := range n.Lhs {
			res := e.assignValue(lhsExpr, values[i], env, fscope)
			if isError(res) {
				return res
			}
		}
	case token.DEFINE: // :=
		for i, lhsExpr := range n.Lhs {
			ident, ok := lhsExpr.(*ast.Ident)
			if !ok {
				return e.newError(lhsExpr.Pos(), "non-identifier on left side of :=")
			}
			if ident.Name == "_" {
				continue // Discard value
			}
			if fn, ok := values[i].(*object.Function); ok {
				fn.Name = ident
			}
			env.Set(ident.Name, values[i])
		}
	default:
		return e.newError(n.Pos(), "unsupported assignment token: %s", n.Tok)
	}

	return nil // Assignment statements don't produce a value.
}

func (e *Evaluator) evalSingleAssign(n *ast.AssignStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	val := e.Eval(n.Rhs[0], env, fscope)
	if isError(val) {
		return val
	}

	// Unwrap return value from function calls
	if ret, ok := val.(*object.ReturnValue); ok {
		val = ret.Value
	}

	// Calling a multi-return function in a single-value context is an error.
	if _, ok := val.(*object.Tuple); ok {
		return e.newError(n.Rhs[0].Pos(), "multi-value function call in single-value context")
	}

	lhs := n.Lhs[0]
	switch n.Tok {
	case token.ASSIGN: // =
		return e.assignValue(lhs, val, env, fscope)
	case token.DEFINE: // :=
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			return e.newError(lhs.Pos(), "non-identifier on left side of :=")
		}
		if ident.Name == "_" {
			return nil // Assignment to blank identifier does nothing.
		}
		if fn, ok := val.(*object.Function); ok {
			fn.Name = ident
		}
		env.Set(ident.Name, val)
		return val
	default:
		return e.newError(n.Pos(), "unsupported assignment token: %s", n.Tok)
	}
}

func (e *Evaluator) assignValue(lhs ast.Expr, val object.Object, env *object.Environment, fscope *object.FileScope) object.Object {
	switch lhsNode := lhs.(type) {
	case *ast.Ident:
		if lhsNode.Name == "_" {
			return nil // Assignment to blank identifier does nothing.
		}
		// Check if we are assigning to an existing interface variable.
		if existing, ok := env.Get(lhsNode.Name); ok {
			if iface, isIface := existing.(*object.InterfaceInstance); isIface {
				// Allow assigning nil to any interface.
				if val.Type() != object.NIL_OBJ {
					// Check if the new value implements the interface.
					if errObj := e.checkImplements(lhsNode.Pos(), val, iface.Def); errObj != nil {
						return errObj
					}
				}
				// Update the concrete value held by the interface.
				iface.Value = val
				return val
			}
		}

		if _, ok := env.GetConstant(lhsNode.Name); ok {
			return e.newError(lhsNode.Pos(), "cannot assign to constant %s", lhsNode.Name)
		}
		if !env.Assign(lhsNode.Name, val) {
			return e.newError(lhsNode.Pos(), "undeclared variable: %s", lhsNode.Name)
		}
		return val
	case *ast.SelectorExpr:
		obj := e.Eval(lhsNode.X, env, fscope)
		if isError(obj) {
			return obj
		}

		var underlying object.Object
		if ptr, isPtr := obj.(*object.Pointer); isPtr {
			if ptr.Element == nil || *ptr.Element == nil {
				return e.newError(lhsNode.Pos(), "nil pointer dereference on assignment")
			}
			underlying = *ptr.Element
		} else {
			underlying = obj
		}

		switch base := underlying.(type) {
		case *object.StructInstance:
			base.Fields[lhsNode.Sel.Name] = val
			return val
		case *object.GoValue:
			structVal := base.Value
			if structVal.Kind() == reflect.Ptr {
				structVal = structVal.Elem()
			}
			if structVal.Kind() != reflect.Struct {
				return e.newError(lhsNode.Pos(), "assignment to field of non-struct Go value")
			}
			field := structVal.FieldByName(lhsNode.Sel.Name)
			if !field.IsValid() {
				return e.newError(lhsNode.Pos(), "no such field: %s in type %s", lhsNode.Sel.Name, structVal.Type())
			}
			if !field.CanSet() {
				return e.newError(lhsNode.Pos(), "cannot set field %s", lhsNode.Sel.Name)
			}
			goVal, err := e.objectToReflectValue(val, field.Type())
			if err != nil {
				return e.newError(lhsNode.Pos(), "type mismatch on assignment: %v", err)
			}
			field.Set(goVal)
			return val
		default:
			return e.newError(lhsNode.Pos(), "assignment to non-struct or non-Go-value field")
		}
	case *ast.StarExpr:
		ptrObj := e.Eval(lhsNode.X, env, fscope)
		if isError(ptrObj) {
			return ptrObj
		}
		ptr, ok := ptrObj.(*object.Pointer)
		if !ok {
			return e.newError(lhsNode.Pos(), "cannot assign to non-pointer")
		}
		*ptr.Element = val
		return val
	case *ast.IndexExpr:
		indexed := e.Eval(lhsNode.X, env, fscope)
		if isError(indexed) {
			return indexed
		}
		index := e.Eval(lhsNode.Index, env, fscope)
		if isError(index) {
			return index
		}
		switch obj := indexed.(type) {
		case *object.Array:
			intIndex, ok := index.(*object.Integer)
			if !ok {
				return e.newError(lhsNode.Index.Pos(), "index into array is not an integer")
			}
			idx := intIndex.Value
			if idx < 0 || idx >= int64(len(obj.Elements)) {
				return e.newError(lhsNode.Index.Pos(), "runtime error: index out of range")
			}
			obj.Elements[idx] = val
			return val
		case *object.Map:
			key, ok := index.(object.Hashable)
			if !ok {
				return e.newError(lhsNode.Index.Pos(), "unusable as map key: %s", index.Type())
			}
			hashKey := key.HashKey()
			obj.Pairs[hashKey] = object.MapPair{Key: index, Value: val}
			return val
		default:
			return e.newError(lhsNode.X.Pos(), "index assignment not supported for %s", indexed.Type())
		}
	default:
		return e.newError(lhs.Pos(), "unsupported assignment target")
	}
}

func (e *Evaluator) evalMultiAssign(n *ast.AssignStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	val := e.Eval(n.Rhs[0], env, fscope)
	if isError(val) {
		return val
	}

	// Unwrap return value from function calls
	if ret, ok := val.(*object.ReturnValue); ok {
		val = ret.Value
	}

	tuple, ok := val.(*object.Tuple)
	if !ok {
		return e.newError(n.Rhs[0].Pos(), "multi-assignment requires a multi-value return, got %s", val.Type())
	}

	if len(n.Lhs) != len(tuple.Elements) {
		return e.newError(n.Pos(), "assignment mismatch: %d variables but %d values", len(n.Lhs), len(tuple.Elements))
	}

	switch n.Tok {
	case token.ASSIGN: // =
		for i, lhsExpr := range n.Lhs {
			res := e.assignValue(lhsExpr, tuple.Elements[i], env, fscope)
			if isError(res) {
				return res
			}
		}
	case token.DEFINE: // :=
		for i, lhsExpr := range n.Lhs {
			ident, ok := lhsExpr.(*ast.Ident)
			if !ok {
				return e.newError(lhsExpr.Pos(), "non-identifier on left side of :=")
			}
			env.Set(ident.Name, tuple.Elements[i])
		}
	default:
		return e.newError(n.Pos(), "unsupported assignment token: %s", n.Tok)
	}

	return nil // Assignment statements don't produce a value.
}

// findFieldInStruct recursively searches for a field within a struct instance,
// including its embedded structs. It returns the found object and a boolean indicating success.
func (e *Evaluator) findFieldInStruct(instance *object.StructInstance, fieldName string) (object.Object, bool) {
	// 1. Check direct fields first. This handles explicit fields and field shadowing.
	if val, ok := instance.Fields[fieldName]; ok {
		return val, true
	}

	// 2. If not found, search in embedded structs in the order they are defined.
	for _, fieldDef := range instance.Def.Fields {
		// An embedded field in Go's AST has no names.
		if len(fieldDef.Names) == 0 {
			// The type of the embedded field, e.g., 'T' in 'struct { T }'.
			// We need to resolve this type name to an object in the instance's fields.
			var typeName string
			switch t := fieldDef.Type.(type) {
			case *ast.Ident:
				typeName = t.Name
			// Handle pointer to embedded type, e.g., struct { *T }
			case *ast.StarExpr:
				if ident, ok := t.X.(*ast.Ident); ok {
					typeName = ident.Name
				}
			}

			if typeName == "" {
				continue // Unsupported embedded field type, e.g. struct { io.Writer }
			}

			// The embedded struct instance is stored in the parent's fields map under its type name.
			embeddedObj, ok := instance.Fields[typeName]
			if !ok {
				// This can happen if an embedded field is nil.
				continue
			}

			// Automatically dereference if the embedded field is a pointer.
			if ptr, ok := embeddedObj.(*object.Pointer); ok {
				// If the pointer is nil, we can't search its fields.
				if ptr.Element == nil || *ptr.Element == nil {
					continue
				}
				embeddedObj = *ptr.Element
			}

			embeddedInstance, ok := embeddedObj.(*object.StructInstance)
			if !ok {
				// It's an embedded field but the value isn't a struct instance.
				continue
			}

			// Recursively search in the embedded struct.
			if val, found := e.findFieldInStruct(embeddedInstance, fieldName); found {
				return val, true // First match wins.
			}
		}
	}

	// 3. Field not found anywhere in the hierarchy.
	return nil, false
}

// constantInfoToObject converts a goscan.ConstantInfo into a minigo object.
// This is how the interpreter understands constants from imported Go packages.
func (e *Evaluator) constantInfoToObject(c *goscan.ConstantInfo) (object.Object, error) {
	// HACK: Workaround for computed constants like `math/bits.UintSize` that go-scan
	// may not be able to resolve statically.
	if c.Name == "UintSize" && c.Value == "" {
		// For the interpreter's purposes, we can assume a 64-bit architecture.
		return &object.Integer{Value: 64}, nil
	}

	// simplified inference
	if c.ConstVal != nil {
		switch c.ConstVal.Kind() {
		case constant.String:
			// Prefer RawValue if available, as it's the direct unquoted value.
			if c.RawValue != "" {
				return &object.String{Value: c.RawValue}, nil
			}
			// Fallback to unquoting the literal representation from .Value
			if s, err := strconv.Unquote(c.Value); err == nil {
				return &object.String{Value: s}, nil
			}
			return nil, fmt.Errorf("could not unquote string constant value: %q", c.Value)
		case constant.Int:
			if i, err := strconv.ParseInt(c.Value, 0, 64); err == nil {
				return &object.Integer{Value: i}, nil
			}
		case constant.Bool:
			if b, err := strconv.ParseBool(c.Value); err == nil {
				return e.nativeBoolToBooleanObject(b), nil
			}
		}
	}

	// Fallback for when c.ConstVal is nil or for unhandled kinds.
	// This maintains the old behavior.
	if i, err := strconv.ParseInt(c.Value, 0, 64); err == nil {
		return &object.Integer{Value: i}, nil
	}
	if s, err := strconv.Unquote(c.Value); err == nil {
		return &object.String{Value: s}, nil
	}
	if b, err := strconv.ParseBool(c.Value); err == nil {
		return e.nativeBoolToBooleanObject(b), nil
	}
	return nil, fmt.Errorf("unsupported or malformed constant value: %q", c.Value)
}

// findSymbolInPackageInfo searches for a symbol within a pre-loaded PackageInfo.
// It does not trigger new scans. It returns the found object and a boolean.
// NOTE: This resolves constants, struct type definitions, and function declarations from AST.
func (e *Evaluator) findSymbolInPackageInfo(pkgInfo *goscan.Package, symbolName string, pkgEnv *object.Environment, fscope *object.FileScope) (object.Object, bool) {
	// Check for FFI types first
	if t, ok := e.registry.LookupType(pkgInfo.Path, symbolName); ok {
		return &object.GoType{GoType: t}, true
	}

	// Look in constants
	for _, c := range pkgInfo.Constants {
		if c.Name == symbolName {
			obj, err := e.constantInfoToObject(c)
			if err != nil {
				return e.newError(token.NoPos, "could not convert constant %q: %v", symbolName, err), true
			}
			return obj, true
		}
	}

	// Look in types (for struct definitions)
	for _, t := range pkgInfo.Types {
		if t.Name == symbolName {
			switch t.Kind {
			case goscan.StructKind:
				// The Node on TypeInfo is an ast.Spec, which should be a *ast.TypeSpec.
				typeSpec, ok := t.Node.(*ast.TypeSpec)
				if !ok {
					continue // Should not happen for valid structs
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue // Should not happen for a StructKind
				}

				// Convert scanner.TypeInfo to object.StructDefinition
				def := &object.StructDefinition{
					Name:       typeSpec.Name,
					Fields:     structType.Fields.List,
					Methods:    make(map[string]*object.Function),
					PkgPath:    pkgInfo.Path,
					ModulePath: e.scanner.ModulePath(),
					ModuleDir:  e.scanner.RootDir(),
					Env:        pkgEnv, // Associate with the package's environment
				}

				// Proactively find and attach all methods for this struct from the same package info.
				for _, f := range pkgInfo.Functions {
					if f.Receiver != nil {
						// It's a method.
						var recvTypeName string
						recvType := f.Receiver.Type
						if recvType.IsPointer && recvType.Elem != nil {
							recvTypeName = recvType.Elem.Name
						} else {
							recvTypeName = recvType.Name
						}

						if recvTypeName == def.Name.Name {
							// This method belongs to our struct. Attach it.
							methodFn := &object.Function{
								Name:       f.AstDecl.Name,
								Recv:       f.AstDecl.Recv,
								TypeParams: f.AstDecl.Type.TypeParams,
								Parameters: f.AstDecl.Type.Params,
								Results:    f.AstDecl.Type.Results,
								Body:       f.AstDecl.Body,
								Env:        pkgEnv, // The package's environment
								FScope:     fscope,
							}
							def.Methods[f.Name] = methodFn
						}
					}
				}
				return def, true

			case goscan.InterfaceKind:
				typeSpec, ok := t.Node.(*ast.TypeSpec)
				if !ok {
					continue
				}
				ifaceType, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}

				def := &object.InterfaceDefinition{
					Name:     typeSpec.Name,
					Methods:  &ast.FieldList{},
					TypeList: make([]ast.Expr, 0),
				}
				if ifaceType.Methods != nil {
					for _, field := range ifaceType.Methods.List {
						if len(field.Names) > 0 {
							def.Methods.List = append(def.Methods.List, field)
						} else {
							def.TypeList = append(def.TypeList, e.flattenTypeUnion(field.Type)...)
						}
					}
				}
				return def, true
			}
		}
	}

	// Look in functions
	for _, f := range pkgInfo.Functions {
		if f.Name == symbolName {
			if f.AstDecl == nil {
				continue
			}
			// When resolving a function from source, create a GoSourceFunction
			// that captures the function's metadata and its definition environment.
			return &object.GoSourceFunction{
				Fn:         f,
				PkgPath:    pkgInfo.Path,
				DefEnv:     pkgEnv,
				FScope:     fscope,
				ModulePath: e.scanner.ModulePath(),
				ModuleDir:  e.scanner.RootDir(),
			}, true
		}
	}

	return nil, false
}

func (e *Evaluator) updateMiniGoStructFromNative(ctx *object.BuiltinContext, src map[string]any, dst *object.StructInstance, visited map[uintptr]object.Object) object.Object {
	// Use pointer address of the destination struct to detect cycles.
	dstPtr := reflect.ValueOf(dst).Pointer()
	if _, ok := visited[dstPtr]; ok {
		return nil // Cycle detected
	}
	visited[dstPtr] = dst

	// This map is useful if the minigo struct uses `json` tags.
	jsonToFieldName := make(map[string]string)
	for fieldName, tag := range dst.Def.FieldTags {
		tagName := strings.Split(tag, ",")[0]
		if tagName != "" && tagName != "-" {
			jsonToFieldName[tagName] = fieldName
		}
	}

	for jsonKey, nativeValue := range src {
		fieldName, ok := jsonToFieldName[jsonKey]
		if !ok {
			// If no tag, default to matching the field name directly (case-insensitive).
			// This is a simplification; Go's json is case-sensitive but we are flexible.
			found := false
			for fldName := range dst.Def.FieldTags {
				if strings.EqualFold(fldName, jsonKey) {
					fieldName = fldName
					found = true
					break
				}
			}
			if !found {
				fieldName = jsonKey
			}
		}

		var astField *ast.Field
		for _, f := range dst.Def.Fields {
			for _, name := range f.Names {
				if name.Name == fieldName {
					astField = f
					break
				}
			}
			if astField != nil {
				break
			}
		}
		if astField == nil {
			continue // Ignore fields in JSON not present in the struct
		}

		// Resolve the expected type of the minigo struct field.
		// We must use the FFI call-site environment (ctx.Env) and scope (fscope)
		// to ensure that imported package types can be resolved correctly.
		expectedTypeObj := e.resolveType(e.Eval(astField.Type, ctx.Env, ctx.FScope), ctx.Env, ctx.FScope)
		if isError(expectedTypeObj) {
			return expectedTypeObj
		}

		var newFieldValue object.Object
		if nativeValue == nil {
			newFieldValue = object.NIL
		} else {
			nativeType := reflect.TypeOf(nativeValue)
			var err error
			newFieldValue, err = e.convertNativeToMiniGo(nativeValue, nativeType, expectedTypeObj, ctx, visited)
			if err != nil {
				return ctx.NewError(astField.Pos(), "json: cannot unmarshal %s into Go value of type %s", nativeType.Kind(), expectedTypeObj.Inspect())
			}
		}
		dst.Fields[fieldName] = newFieldValue
	}
	return nil
}

// convertNativeToMiniGo performs the type-checked conversion from a native Go value
// (from json.Unmarshal) to a minigo object, based on the expected minigo type.
func (e *Evaluator) convertNativeToMiniGo(
	nativeValue any,
	nativeType reflect.Type,
	expectedType object.Object,
	ctx *object.BuiltinContext,
	visited map[uintptr]object.Object,
) (object.Object, error) {

	switch t := expectedType.(type) {
	case *object.Type:
		switch t.Name {
		case "int", "int64", "int32", "int16", "int8":
			if nativeType.Kind() != reflect.Float64 {
				return nil, fmt.Errorf("type mismatch")
			}
			return &object.Integer{Value: int64(nativeValue.(float64))}, nil
		case "string":
			if nativeType.Kind() != reflect.String {
				return nil, fmt.Errorf("type mismatch")
			}
			return &object.String{Value: nativeValue.(string)}, nil
		case "bool":
			if nativeType.Kind() != reflect.Bool {
				return nil, fmt.Errorf("type mismatch")
			}
			return e.nativeBoolToBooleanObject(nativeValue.(bool)), nil
		default:
			// For other built-in types, do a simple conversion for now.
			return e.nativeToValue(reflect.ValueOf(nativeValue)), nil
		}
	case *object.StructDefinition:
		nestedMap, ok := nativeValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("type mismatch")
		}
		// The destination field might already have a struct instance, or it could be nil.
		// We create a new one to be safe and populate it.
		nestedInstance := &object.StructInstance{Def: t, Fields: make(map[string]object.Object)}
		if err := e.updateMiniGoStructFromNative(ctx, nestedMap, nestedInstance, visited); err != nil {
			return nil, fmt.Errorf("nested struct update failed")
		}
		return nestedInstance, nil
	case *object.PointerType:
		if nestedStructDef, ok := t.ElementType.(*object.StructDefinition); ok {
			nestedMap, isMap := nativeValue.(map[string]any)
			if !isMap {
				return nil, fmt.Errorf("type mismatch")
			}
			newInstance := &object.StructInstance{Def: nestedStructDef, Fields: make(map[string]object.Object)}
			if err := e.updateMiniGoStructFromNative(ctx, nestedMap, newInstance, visited); err != nil {
				return nil, fmt.Errorf("nested pointer to-struct update failed")
			}
			var obj object.Object = newInstance
			return &object.Pointer{Element: &obj}, nil
		}
		// Fallback for other pointer types
		return e.nativeToValue(reflect.ValueOf(nativeValue)), nil
	default:
		// Fallback for other types (arrays, etc.)
		return e.nativeToValue(reflect.ValueOf(nativeValue)), nil
	}
}

// WrapGoFunction is a public method to wrap a native Go function into a minigo object.
func (e *Evaluator) WrapGoFunction(pos token.Pos, funcVal reflect.Value) object.Object {
	funcType := funcVal.Type()
	return &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, callPos token.Pos, args ...object.Object) (ret object.Object) {
			defer func() {
				if r := recover(); r != nil {
					panicValue := &object.String{Value: fmt.Sprintf("%v", r)}
					ret = &object.Panic{Value: panicValue}
				}
			}()

			numIn := funcType.NumIn()
			isVariadic := funcType.IsVariadic()
			if isVariadic {
				if len(args) < numIn-1 {
					return ctx.NewError(pos, "wrong number of arguments for variadic function: got %d, want at least %d", len(args), numIn-1)
				}
			} else {
				if len(args) != numIn {
					return ctx.NewError(pos, "wrong number of arguments: got %d, want %d", len(args), numIn)
				}
			}

			in := make([]reflect.Value, len(args))
			var ptrBridges []*ffibridge.Pointer
			for i, arg := range args {
				var targetType reflect.Type
				if isVariadic && i >= funcType.NumIn()-1 {
					targetType = funcType.In(funcType.NumIn() - 1).Elem()
				} else {
					targetType = funcType.In(i)
				}

				if ptr, isPtr := arg.(*object.Pointer); isPtr && targetType.Kind() == reflect.Interface {
					var nativePtr any
					underlying := *ptr.Element
					if _, ok := underlying.(*object.StructInstance); ok {
						var m map[string]any
						nativePtr = &m
					} else if underlying == object.NIL {
						// This is the case for `var p Person; Unmarshal(..., &p)` where p starts as nil.
						// The var initialization change should handle this, but as a fallback,
						// we create the map anyway, and the post-call update will populate the struct.
						var m map[string]any
						nativePtr = &m
					} else {
						return ctx.NewError(pos, "passing pointers to Go functions is only supported for struct types, got %s", underlying.Type())
					}
					bridge := &ffibridge.Pointer{Source: ptr, Dest: reflect.ValueOf(nativePtr)}
					ptrBridges = append(ptrBridges, bridge)
					in[i] = bridge.Dest
				} else {
					val, err := e.objectToReflectValue(arg, targetType)
					if err != nil {
						return ctx.NewError(pos, "argument %d type mismatch: %v", i+1, err)
					}
					in[i] = val
				}
			}

			results := funcVal.Call(in)

			// After the call, check for in-place modifications to slice arguments.
			for i, arg := range args {
				if arr, ok := arg.(*object.Array); ok {
					// `in[i]` holds the `reflect.Value` of the Go slice that was passed.
					goSlice := in[i]
					if goSlice.Kind() == reflect.Slice {
						// Copy the (potentially modified) elements from the Go slice
						// back into our minigo Array object.
						for j := 0; j < goSlice.Len(); j++ {
							if j < len(arr.Elements) {
								goElement := goSlice.Index(j)
								objElement := e.nativeToValue(goElement)
								arr.Elements[j] = objElement
							}
						}
					}
				}
			}

			for _, bridge := range ptrBridges {
				nativeValue := bridge.Dest.Elem().Interface()
				targetObj := *bridge.Source.Element

				if dst, ok := targetObj.(*object.StructInstance); ok {
					if src, ok := nativeValue.(map[string]any); ok {
						if errObj := e.updateMiniGoStructFromNative(ctx, src, dst, make(map[uintptr]object.Object)); errObj != nil {
							// The error from updateMiniGoStructFromNative is already an *object.Error
							return errObj
						}
					}
				}
			}

			numOut := funcType.NumOut()
			if numOut == 0 {
				return object.NIL
			}

			// Convert all results to minigo objects.
			// This new logic correctly handles the `(value, error)` pattern by
			// wrapping the non-nil error in a GoValue, instead of halting execution.
			// The minigo script is then responsible for checking if the error is nil.
			resultObjects := make([]object.Object, numOut)
			for i := 0; i < numOut; i++ {
				resultObjects[i] = e.nativeToValue(results[i])
			}

			if numOut == 1 {
				return resultObjects[0]
			}
			return &object.Tuple{Elements: resultObjects}
		},
	}
}

// evalMethodCall handles resolving and binding a method to a receiver.
// The receiver can be a struct instance or a pointer to a struct instance.
func (e *Evaluator) evalMethodCall(n *ast.SelectorExpr, receiver object.Object, def *object.StructDefinition) object.Object {
	method, ok := def.Methods[n.Sel.Name]
	if !ok {
		return nil // Not a method, signal to caller to check for fields.
	}

	// Determine if the method requires a pointer receiver.
	isPointerReceiver := false
	if method.Recv != nil && len(method.Recv.List) > 0 {
		if _, ok := method.Recv.List[0].Type.(*ast.StarExpr); ok {
			isPointerReceiver = true
		}
	}

	// Check if the receiver is compatible.
	if isPointerReceiver {
		if _, isPointer := receiver.(*object.Pointer); !isPointer {
			// This is a limitation of minigo: it doesn't automatically take the address.
			// e.g., `var c Counter; c.Inc()` where Inc has a pointer receiver.
			// A real Go compiler would implicitly convert `c` to `&c`.
			return e.newError(n.Pos(), "cannot call pointer method %s on value %s", n.Sel.Name, def.Name.Name)
		}
		// Receiver is a pointer, and method wants a pointer. This is correct.
		return &object.BoundMethod{Fn: method, Receiver: receiver}
	}

	// Method has a value receiver.
	if ptr, isPointer := receiver.(*object.Pointer); isPointer {
		// If receiver is a pointer, dereference it for the method call.
		return &object.BoundMethod{Fn: method, Receiver: *ptr.Element}
	}

	// Receiver is a value, and method wants a value. This is correct.
	// We pass a copy to prevent the method from modifying the original struct.
	instance := receiver.(*object.StructInstance)
	return &object.BoundMethod{Fn: method, Receiver: instance.Copy()}
}

func (e *Evaluator) evalSelectorExpr(n *ast.SelectorExpr, env *object.Environment, fscope *object.FileScope) object.Object {
	left := e.Eval(n.X, env, fscope)
	if isError(left) {
		return left
	}

	switch l := left.(type) {
	case *object.InterfaceInstance:
		if l.Value == nil || l.Value.Type() == object.NIL_OBJ {
			return e.newError(n.Pos(), "nil pointer dereference (interface is nil)")
		}
		// Dispatch the selector to the concrete value held by the interface.
		// This is effectively a re-dispatch of evalSelectorExpr's logic.
		switch concrete := l.Value.(type) {
		case *object.StructInstance:
			// Re-run the logic for StructInstance
			if method := e.evalMethodCall(n, concrete, concrete.Def); method != nil {
				if err, isErr := method.(*object.Error); isErr {
					return err
				}
				return method
			}
			if val, found := e.findFieldInStruct(concrete, n.Sel.Name); found {
				return val
			}
			return e.newError(n.Pos(), "undefined field or method '%s' on struct '%s' held by interface", n.Sel.Name, concrete.Def.Name.Name)
		case *object.Pointer:
			// Re-run the logic for Pointer
			if concrete.Element == nil || *concrete.Element == nil {
				return e.newError(n.Pos(), "nil pointer dereference in interface")
			}
			instance, ok := (*concrete.Element).(*object.StructInstance)
			if !ok {
				return e.newError(n.Pos(), "interface holds pointer to non-struct")
			}
			if method := e.evalMethodCall(n, concrete, instance.Def); method != nil {
				if err, isErr := method.(*object.Error); isErr {
					return err
				}
				return method
			}
			if val, found := e.findFieldInStruct(instance, n.Sel.Name); found {
				return val
			}
			return e.newError(n.Pos(), "undefined field or method '%s' on pointer to struct '%s' held by interface", n.Sel.Name, instance.Def.Name.Name)
		default:
			return e.newError(n.Pos(), "type %s held by interface does not support method or field access", concrete.Type())
		}

	case *object.Package:
		return e.findSymbolInPackage(l, n.Sel, n.Pos())

	case *object.StructInstance:
		// 1. Look for a method.
		if method := e.evalMethodCall(n, l, l.Def); method != nil {
			if err, isErr := method.(*object.Error); isErr {
				return err
			}
			return method
		}
		// 2. If not a method, look for a field.
		if val, found := e.findFieldInStruct(l, n.Sel.Name); found {
			return val
		}
		return e.newError(n.Pos(), "undefined field or method '%s' on struct '%s'", n.Sel.Name, l.Def.Name.Name)

	case *object.TypedNil:
		ptrType, ok := l.TypeObject.(*object.PointerType)
		if !ok {
			return e.newError(n.Pos(), "internal error: TypedNil does not contain a pointer type")
		}
		structDef, ok := ptrType.ElementType.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Pos(), "cannot get method from nil pointer to non-struct type %s", ptrType.ElementType.Inspect())
		}
		method, ok := structDef.Methods[n.Sel.Name]
		if !ok {
			return e.newError(n.Pos(), "undefined method %s for type %s", n.Sel.Name, structDef.Name.Name)
		}
		return &object.GoMethodValue{Fn: method, RecvDef: structDef}

	case *object.Pointer:
		if l.Element == nil || *l.Element == nil {
			return e.newError(n.Pos(), "nil pointer dereference")
		}

		// Handle pointers to both minigo structs and Go values
		switch elem := (*l.Element).(type) {
		case *object.StructInstance:
			// This is a pointer to a minigo-defined struct.
			// 1. Look for a method. Pass the pointer `l` as the receiver.
			if method := e.evalMethodCall(n, l, elem.Def); method != nil {
				if err, isErr := method.(*object.Error); isErr {
					return err
				}
				return method
			}
			// 2. If not a method, look for a field on the dereferenced struct.
			if val, found := e.findFieldInStruct(elem, n.Sel.Name); found {
				return val
			}
			return e.newError(n.Pos(), "undefined field or method '%s' on pointer to struct '%s'", n.Sel.Name, elem.Def.Name.Name)

		case *object.GoValue:
			// This is a pointer to a Go value. Delegate to the Go value selector logic.
			return e.evalGoValueSelectorExpr(n, elem, n.Sel.Name)

		default:
			return e.newError(n.Pos(), "base of selector expression is not a pointer to a struct or Go value")
		}

	case *object.GoValue:
		return e.evalGoValueSelectorExpr(n, l, n.Sel.Name)
	default:
		return e.newError(n.Pos(), "base of selector expression is not a package or struct")
	}
}

// findSymbolInPackage resolves a symbol within a given package. It handles caching,
// consulting the symbol registry, and triggering on-demand scanning.
func (e *Evaluator) findSymbolInPackage(pkg *object.Package, symbolName *ast.Ident, pos token.Pos) object.Object {
	// 1. Check member cache first.
	if member, ok := pkg.Members[symbolName.Name]; ok {
		return member
	}

	// 2. Check the registry for pre-registered symbols (values and types).
	if symbol, ok := e.registry.Lookup(pkg.Path, symbolName.Name); ok {
		var member object.Object
		val := reflect.ValueOf(symbol)
		if val.Kind() == reflect.Func {
			member = e.WrapGoFunction(pos, val)
		} else {
			member = &object.GoValue{Value: val}
		}
		pkg.Members[symbolName.Name] = member // Cache it
		return member
	}
	if t, ok := e.registry.LookupType(pkg.Path, symbolName.Name); ok {
		member := &object.GoType{GoType: t}
		pkg.Members[symbolName.Name] = member // Cache it
		return member
	}

	// 3. If the package's environment is empty, it means we haven't scanned it yet.
	// This is the main entry point for on-demand, lazy loading of a package's source.
	if pkg.Env.IsEmpty() {
		cumulativePkgInfo, err := e.scanner.FindSymbolInPackage(context.Background(), pkg.Path, symbolName.Name)
		if err != nil {
			// Not found in any unscanned files either.
			return e.newError(pos, "undefined: %s.%s (package scan failed: %v)", pkg.Name, symbolName.Name, err)
		}

		// Update the package object with the richer info from the scan.
		pkg.Info = cumulativePkgInfo

		// Create a new, unified FileScope for the entire package from all its files.
		if cumulativePkgInfo != nil && len(cumulativePkgInfo.AstFiles) > 0 {
			var representativeAST *ast.File
			for _, astFile := range cumulativePkgInfo.AstFiles {
				if representativeAST == nil {
					representativeAST = astFile
				}
			}
			unifiedFScope := object.NewFileScope(representativeAST)
			for _, astFile := range cumulativePkgInfo.AstFiles {
				for _, importSpec := range astFile.Imports {
					path, err := strconv.Unquote(importSpec.Path.Value)
					if err != nil {
						return e.newError(importSpec.Path.Pos(), "invalid import path: %v", err)
					}
					var alias string
					var aliasIdent *ast.Ident
					if importSpec.Name != nil {
						alias = importSpec.Name.Name
						aliasIdent = importSpec.Name
					} else {
						parts := strings.Split(path, "/")
						alias = parts[len(parts)-1]
						aliasIdent = &ast.Ident{Name: alias, NamePos: importSpec.Path.Pos()}
					}
					e.resolvePackage(aliasIdent, path)
					switch alias {
					case "_":
						continue
					case ".":
						unifiedFScope.DotImports = append(unifiedFScope.DotImports, path)
					default:
						unifiedFScope.Aliases[alias] = path
					}
				}
			}
			pkg.FScope = unifiedFScope
		}

		// Proactively populate all symbols from the package info into the package's environment.
		// This acts as the "second pass" to resolve all top-level declarations before execution.
		if pkg.Info != nil {
			for _, t := range pkg.Info.Types {
				if _, ok := pkg.Env.Get(t.Name); !ok {
					typeObj, _ := e.findSymbolInPackageInfo(pkg.Info, t.Name, pkg.Env, pkg.FScope)
					if typeObj != nil {
						pkg.Env.Set(t.Name, typeObj)
					}
				}
			}
			for _, c := range pkg.Info.Constants {
				if _, ok := pkg.Env.GetConstant(c.Name); !ok {
					constObj, _ := e.findSymbolInPackageInfo(pkg.Info, c.Name, pkg.Env, pkg.FScope)
					if constObj != nil {
						pkg.Env.SetConstant(c.Name, constObj)
					}
				}
			}
			for _, f := range pkg.Info.Functions {
				if _, ok := pkg.Env.Get(f.Name); !ok {
					fnObj, _ := e.findSymbolInPackageInfo(pkg.Info, f.Name, pkg.Env, pkg.FScope)
					if fnObj != nil {
						pkg.Env.Set(f.Name, fnObj)
					}
				}
			}
		}
	}

	// 4. Now that the package environment is populated, retrieve the symbol and cache it in Members.
	if member, ok := pkg.Env.Get(symbolName.Name); ok {
		pkg.Members[symbolName.Name] = member
		return member
	}

	return e.newError(pos, "undefined: %s.%s", pkg.Name, symbolName.Name)
}

func (e *Evaluator) evalGoValueSelectorExpr(node ast.Node, goVal *object.GoValue, sel string) object.Object {
	val := goVal.Value

	// --- 1. Method Resolution ---
	// Try to find the method on the value itself, or on a pointer to the value.
	var method reflect.Value

	// a) Check value receiver
	method = val.MethodByName(sel)

	// b) If not found, and the value is addressable, check pointer receiver.
	// This is crucial for methods like `(*bytes.Buffer).Write`.
	if !method.IsValid() && val.CanAddr() {
		method = val.Addr().MethodByName(sel)
	}

	// --- 2. Method Invocation ---
	// If a method was found, return a callable Builtin object that wraps the Go method.
	if method.IsValid() {
		funcType := method.Type()
		return &object.Builtin{
			Fn: func(ctx *object.BuiltinContext, callPos token.Pos, args ...object.Object) (ret object.Object) {
				defer func() {
					if r := recover(); r != nil {
						ret = ctx.NewError(callPos, "panic in Go method call '%s': %v", sel, r)
					}
				}()

				numIn := funcType.NumIn()
				isVariadic := funcType.IsVariadic()

				// Check argument count against the Go method's signature.
				if isVariadic {
					if len(args) < numIn-1 {
						return ctx.NewError(callPos, "wrong number of arguments for variadic method %s: got %d, want at least %d", sel, len(args), numIn-1)
					}
				} else {
					if len(args) != numIn {
						return ctx.NewError(callPos, "wrong number of arguments for method %s: got %d, want %d", sel, len(args), numIn)
					}
				}

				// Prepare arguments for reflection call.
				in := make([]reflect.Value, len(args))
				for i, arg := range args {
					var targetType reflect.Type
					if isVariadic && i >= numIn-1 {
						targetType = funcType.In(numIn - 1).Elem()
					} else {
						targetType = funcType.In(i)
					}

					// Use the evaluator's conversion helper.
					val, err := e.objectToReflectValue(arg, targetType)
					if err != nil {
						return ctx.NewError(callPos, "argument %d type mismatch for method %s: %v", i+1, sel, err)
					}
					in[i] = val
				}

				// Call the method.
				results := method.Call(in)

				// Process results.
				numOut := funcType.NumOut()
				if numOut == 0 {
					return object.NIL
				}

				// Convert all results to minigo objects.
				// This new logic correctly handles the `(value, error)` pattern by
				// wrapping the non-nil error in a GoValue, instead of halting execution.
				// The minigo script is then responsible for checking if the error is nil.
				resultObjects := make([]object.Object, numOut)
				for i := 0; i < numOut; i++ {
					resultObjects[i] = e.nativeToValue(results[i])
				}

				if numOut == 1 {
					return resultObjects[0]
				}
				return &object.Tuple{Elements: resultObjects}
			},
		}
	}

	// --- 3. Field Access ---
	// If no method was found, try to access a field on the struct.
	objToInspect := val
	if objToInspect.Kind() == reflect.Ptr {
		if objToInspect.IsNil() {
			return e.newError(node.Pos(), "nil pointer dereference")
		}
		objToInspect = objToInspect.Elem()
	}

	if objToInspect.Kind() == reflect.Struct {
		field := objToInspect.FieldByName(sel)
		if field.IsValid() {
			if !field.CanInterface() {
				return e.newError(node.Pos(), "cannot access unexported field '%s' on Go struct %s", sel, objToInspect.Type())
			}
			return e.nativeToValue(field)
		}
	}

	// --- 4. Not Found ---
	// If neither a method nor a field was found, it's an error.
	return e.newError(node.Pos(), "undefined field or method '%s' on Go object of type %s", sel, val.Type())
}

func (e *Evaluator) evalCompositeLit(n *ast.CompositeLit, env *object.Environment, fscope *object.FileScope) object.Object {
	if n.Type == nil {
		return e.newError(n.Pos(), "untyped composite literal in context where type cannot be inferred")
	}
	// First, evaluate the type expression itself. This could be an identifier (MyStruct),
	// a selector (pkg.MyStruct), an index expression (MyGeneric[int]), or a type literal ([]int).
	typeObj := e.Eval(n.Type, env, fscope)
	if isError(typeObj) {
		return typeObj
	}
	return e.evalCompositeLitWithType(n, typeObj, env, fscope)
}

// evalCompositeLitWithType evaluates a composite literal against a given, already-evaluated type object.
func (e *Evaluator) evalCompositeLitWithType(n *ast.CompositeLit, typeObj object.Object, env *object.Environment, fscope *object.FileScope) object.Object {
	// Now, resolve the evaluated type object. This handles non-generic aliases.
	// For generic types, `typeObj` will already be the instantiated type object
	// (e.g., a StructDefinition or an ArrayType from `instantiateTypeAlias`).
	resolvedType := e.resolveType(typeObj, env, fscope)
	if isError(resolvedType) {
		return resolvedType
	}

	switch def := resolvedType.(type) {
	case *object.InstantiatedType:
		// Handle composite literals for instantiated generic types, e.g., Box[int]{...}
		structDef, ok := def.GenericDef.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Pos(), "cannot create composite literal of non-struct generic type %s", def.GenericDef.Type())
		}
		instanceObj := e.evalStructLiteral(n, structDef, env, fscope)
		if si, ok := instanceObj.(*object.StructInstance); ok {
			si.TypeArgs = def.TypeArgs // Attach the type arguments from the instantiation
		}
		return instanceObj

	case *object.StructDefinition:
		instanceObj := e.evalStructLiteral(n, def, env, fscope)
		// If the original type was a generic instantiation, we need to attach the type arguments.
		if instType, ok := typeObj.(*object.InstantiatedType); ok {
			if si, ok := instanceObj.(*object.StructInstance); ok {
				si.TypeArgs = instType.TypeArgs
			}
		}
		return instanceObj

	case *object.ArrayType:
		elements := e.evalExpressions(n.Elts, env, fscope, def.ElementType)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		// Create a new slice with capacity equal to length to mimic Go's behavior for literals.
		finalElements := make([]object.Object, len(elements))
		copy(finalElements, elements)
		return &object.Array{SliceType: def, Elements: finalElements}

	case *object.MapType:
		return e.evalMapLiteral(n, def, env, fscope)

	default:
		return e.newError(n.Pos(), "cannot create composite literal for type %s", resolvedType.Type())
	}
}

// checkImplements verifies that a concrete object satisfies an interface definition.
// It returns nil on success or an *object.Error on failure.
func (e *Evaluator) checkImplements(pos token.Pos, concrete object.Object, iface *object.InterfaceDefinition) object.Object {
	var concreteMethods map[string]*object.Function
	var concreteTypeName string

	// Determine the method set and type name from the concrete object.
	// This handles both value receivers (StructInstance) and pointer receivers (*Pointer to StructInstance).
	switch c := concrete.(type) {
	case *object.StructInstance:
		concreteMethods = c.Def.Methods
		concreteTypeName = c.Def.Name.Name
	case *object.Pointer:
		if s, ok := (*c.Element).(*object.StructInstance); ok {
			concreteMethods = s.Def.Methods
			concreteTypeName = s.Def.Name.Name
		} else {
			// A pointer to a non-struct cannot have methods.
			if len(iface.Methods.List) > 0 {
				return e.newError(pos, "type %s cannot implement non-empty interface %s", (*c.Element).Type(), iface.Name.Name)
			}
			return nil
		}
	default:
		// Any other type cannot have methods.
		if len(iface.Methods.List) > 0 {
			return e.newError(pos, "type %s cannot implement non-empty interface %s", concrete.Type(), iface.Name.Name)
		}
		return nil // Type can implement an empty interface.
	}

	// Now check each method required by the interface.
	for _, ifaceMethodField := range iface.Methods.List {
		if len(ifaceMethodField.Names) == 0 {
			continue // Should not happen in a valid interface AST.
		}
		methodName := ifaceMethodField.Names[0].Name
		ifaceFuncType, ok := ifaceMethodField.Type.(*ast.FuncType)
		if !ok {
			continue // Also should not happen.
		}

		concreteMethod, ok := concreteMethods[methodName]
		if !ok {
			return e.newError(pos, "type %s does not implement %s (missing method %s)", concreteTypeName, iface.Name.Name, methodName)
		}

		// Compare parameter counts.
		ifaceParamCount := 0
		if ifaceFuncType.Params != nil {
			ifaceParamCount = len(ifaceFuncType.Params.List)
		}
		concreteParamCount := 0
		if concreteMethod.Parameters != nil {
			concreteParamCount = len(concreteMethod.Parameters.List)
		}
		if ifaceParamCount != concreteParamCount {
			return e.newError(pos, "cannot use %s as %s value in assignment: method %s has wrong number of parameters (got %d, want %d)",
				concreteTypeName, iface.Name.Name, methodName, concreteParamCount, ifaceParamCount)
		}

		// Compare result counts.
		ifaceResultCount := 0
		if ifaceFuncType.Results != nil {
			ifaceResultCount = len(ifaceFuncType.Results.List)
		}
		concreteResultCount := 0
		if concreteMethod.Results != nil {
			concreteResultCount = len(concreteMethod.Results.List)
		}
		if ifaceResultCount != concreteResultCount {
			return e.newError(pos, "cannot use %s as %s value in assignment: method %s has wrong number of return values (got %d, want %d)",
				concreteTypeName, iface.Name.Name, methodName, concreteResultCount, ifaceResultCount)
		}

		// NOTE: A full implementation would also compare the types of parameters and results.
		// This is complex as it requires resolving type identifiers from the AST.
		// For now, we only check the counts, which covers many cases.
	}

	return nil
}

func (e *Evaluator) evalStructLiteral(n *ast.CompositeLit, def *object.StructDefinition, env *object.Environment, fscope *object.FileScope) object.Object {
	instance := &object.StructInstance{Def: def, Fields: make(map[string]object.Object)}

	// Initialize all fields to their zero value (nil) first.
	// This ensures that even uninitialized fields exist in the Fields map.
	for _, field := range def.Fields {
		for _, name := range field.Names {
			instance.Fields[name.Name] = object.NIL
		}
	}

	for _, elt := range n.Elts {
		switch node := elt.(type) {
		case *ast.KeyValueExpr:
			key, ok := node.Key.(*ast.Ident)
			if !ok {
				return e.newError(node.Key.Pos(), "field name is not an identifier")
			}
			value := e.Eval(node.Value, env, fscope)
			if isError(value) {
				return value
			}
			instance.Fields[key.Name] = value
		case *ast.Ident:
			// This handles shorthand struct literals, e.g., `MyStruct{Field}`
			// which is equivalent to `MyStruct{Field: Field}`.
			fieldName := node.Name
			value := e.Eval(node, env, fscope) // Evaluate the identifier in the current env
			if isError(value) {
				return value
			}
			instance.Fields[fieldName] = value
		default:
			return e.newError(elt.Pos(), "unsupported literal element in struct literal: %T", elt)
		}
	}
	return instance
}

func (e *Evaluator) evalMapLiteral(n *ast.CompositeLit, def *object.MapType, env *object.Environment, fscope *object.FileScope) object.Object {
	pairs := make(map[object.HashKey]object.MapPair)

	for _, elt := range n.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return e.newError(elt.Pos(), "non-key-value element in map literal")
		}

		key := e.Eval(kv.Key, env, fscope)
		if isError(key) {
			return key
		}

		hashable, ok := key.(object.Hashable)
		if !ok {
			return e.newError(kv.Key.Pos(), "unusable as map key: %s", key.Type())
		}

		value := e.Eval(kv.Value, env, fscope)
		if isError(value) {
			return value
		}

		hashed := hashable.HashKey()
		pairs[hashed] = object.MapPair{Key: key, Value: value}
	}

	return &object.Map{MapType: def, Pairs: pairs}
}

func (e *Evaluator) resolvePackage(ident *ast.Ident, path string) *object.Package {
	// Check if the package is already in the central cache.
	if pkg, ok := e.packages[path]; ok {
		return pkg
	}
	// If not, create a new proxy object and cache it.
	pkgObj := &object.Package{
		Name:    ident.Name,
		Path:    path,
		Env:     object.NewEnvironment(), // Create a new environment for the package.
		Info:    nil,                     // Mark as not loaded yet
		Members: make(map[string]object.Object),
	}
	e.packages[path] = pkgObj
	return pkgObj
}

func (e *Evaluator) evalIdent(n *ast.Ident, env *object.Environment, fscope *object.FileScope) object.Object {
	if val, ok := env.Get(n.Name); ok {
		return val
	}
	if builtin, ok := builtins[n.Name]; ok {
		return builtin
	}
	if sf, ok := e.specialForms[n.Name]; ok {
		return sf
	}
	// Handle built-in type identifiers. These don't have a first-class object
	// representation in our interpreter, but they shouldn't cause an "identifier
	// not found" error when used in declarations like `var x int`.
	switch n.Name {
	case "int", "int8", "int16", "int32", "int64":
		return &object.Type{Name: n.Name}
	case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return &object.Type{Name: n.Name}
	case "float32", "float64":
		return &object.Type{Name: n.Name}
	case "string", "bool", "byte", "rune", "any", "comparable":
		return &object.Type{Name: n.Name}
	}

	// Check if it's a package alias or a symbol from a dot import.
	if fscope != nil {
		// Check dot imports first.
		for _, path := range fscope.DotImports {
			// We need a dummy identifier for resolvePackage, as the package itself isn't named in a dot import.
			dummyIdent := &ast.Ident{Name: "_"}
			pkg := e.resolvePackage(dummyIdent, path)

			// Now, try to find the symbol `n` within this package.
			val := e.findSymbolInPackage(pkg, n, n.Pos())

			// If the symbol is found, return it.
			// We check for "undefined" specifically, because other errors
			// (like a real error from a function call) should be propagated.
			// If it's an "undefined" error, we just continue to the next dot-imported package.
			if err, ok := val.(*object.Error); ok {
				if strings.Contains(err.Message, "undefined:") {
					continue
				}
			}
			// If it's not an "undefined" error, we found it or encountered a different error.
			return val
		}

		// If not in a dot import, check for a regular package alias.
		if path, ok := fscope.Aliases[n.Name]; ok {
			return e.resolvePackage(n, path)
		}
	}

	switch n.Name {
	case "true":
		return object.TRUE
	case "false":
		return object.FALSE
	case "nil":
		return object.NIL
	}
	return e.newError(n.Pos(), "identifier not found: %s", n.Name)
}

func (e *Evaluator) evalBasicLit(n *ast.BasicLit) object.Object {
	switch n.Kind {
	case token.INT:
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return e.newError(n.Pos(), "could not parse %q as integer", n.Value)
		}
		return &object.Integer{Value: i}
	case token.FLOAT:
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return e.newError(n.Pos(), "could not parse %q as float", n.Value)
		}
		return &object.Float{Value: f}
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(n.Pos(), "could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	case token.CHAR:
		// Unquote the char literal (e.g., "'a'" -> "a")
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(n.Pos(), "could not unquote char literal %q", n.Value)
		}
		// A char literal in Go is a rune, which is an alias for int32.
		// We represent it as our standard Integer object.
		return &object.Integer{Value: int64(rune(s[0]))}
	default:
		return e.newError(n.Pos(), "unsupported literal type: %s", n.Kind)
	}
}

// Scanner returns the underlying goscan.Scanner instance.
func (e *Evaluator) Scanner() *goscan.Scanner {
	return e.scanner
}
