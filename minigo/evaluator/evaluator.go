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
					elements[i] = object.NIL
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
					arg.Elements[i] = object.NIL
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
			} else if args[0] == object.NIL {
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
			def, ok := args[0].(*object.StructDefinition)
			if !ok {
				return ctx.NewError(pos, "argument to `new` must be a struct type, got %s", args[0].Type())
			}
			instance := &object.StructInstance{
				Def:    def,
				Fields: make(map[string]object.Object),
			}
			for _, field := range def.Fields {
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

type Evaluator struct {
	object.BuiltinContext
	scanner          *goscan.Scanner
	registry         *object.SymbolRegistry
	specialForms     map[string]*SpecialForm
	packages         map[string]*object.Package
	callStack        []*object.CallFrame
	currentPanic     *object.Panic
	isExecutingDefer bool
}

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
		IsExecutingDefer: func() bool { return e.isExecutingDefer },
		GetPanic:         func() *object.Panic { return e.currentPanic },
		ClearPanic:       func() { e.currentPanic = nil },
		NewError: func(pos token.Pos, format string, v ...interface{}) *object.Error {
			return e.newError(pos, format, v...)
		},
	}
	return e
}

func (e *Evaluator) inferTypeOf(obj object.Object) object.Object {
	switch o := obj.(type) {
	case *object.Integer:
		return &object.Type{Name: "int"}
	case *object.Float:
		return &object.Type{Name: "float64"}
	case *object.String:
		return &object.Type{Name: "string"}
	case *object.Boolean:
		return &object.Type{Name: "bool"}
	case *object.StructInstance:
		return o.Def
	case *object.Pointer:
		if o.Element == nil || *o.Element == nil {
			return object.NIL
		}
		elemType := e.inferTypeOf(*o.Element)
		if elemType == object.NIL {
			return object.NIL
		}
		return &object.PointerType{ElementType: elemType}
	case *object.Array:
		if o.SliceType != nil {
			return o.SliceType
		}
		if len(o.Elements) == 0 {
			return nil
		}
		elemType := e.inferTypeOf(o.Elements[0])
		if elemType == nil {
			return nil
		}
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
			break
		}
		if keyType == nil || valType == nil {
			return nil
		}
		return &object.MapType{KeyType: keyType, ValueType: valType}
	default:
		return nil
	}
}

func (e *Evaluator) inferGenericTypes(pos token.Pos, f *object.Function, args []object.Object) ([]object.Object, object.Object) {
	typeParamNames := make(map[string]bool)
	for _, field := range f.TypeParams.List {
		for _, name := range field.Names {
			typeParamNames[name.Name] = true
		}
	}
	inferredTypes := make(map[string]object.Object)
	for i, paramField := range f.Parameters.List {
		if paramTypeIdent, ok := paramField.Type.(*ast.Ident); ok {
			if _, isGeneric := typeParamNames[paramTypeIdent.Name]; isGeneric {
				if i >= len(args) {
					return nil, e.newError(pos, "cannot infer type for generic parameter %s: not enough arguments", paramTypeIdent.Name)
				}
				argType := e.inferTypeOf(args[i])
				if argType == nil || argType == object.NIL {
					return nil, e.newError(pos, "cannot infer type for generic parameter %s from argument %d of type %s", paramTypeIdent.Name, i, args[i].Type())
				}
				if existing, ok := inferredTypes[paramTypeIdent.Name]; ok {
					if existing != argType && existing.Inspect() != argType.Inspect() {
						return nil, e.newError(pos, "cannot infer type for %s: conflicting types %s and %s", paramTypeIdent.Name, existing.Inspect(), argType.Inspect())
					}
				} else {
					inferredTypes[paramTypeIdent.Name] = argType
				}
			}
		} else if paramTypeArray, ok := paramField.Type.(*ast.ArrayType); ok {
			if eltIdent, ok := paramTypeArray.Elt.(*ast.Ident); ok {
				if _, isGeneric := typeParamNames[eltIdent.Name]; isGeneric {
					if i >= len(args) {
						return nil, e.newError(pos, "cannot infer type for generic parameter %s: not enough arguments", eltIdent.Name)
					}
					arg := args[i]
					argArray, ok := arg.(*object.Array)
					if !ok {
						continue
					}
					inferredElemType := e.inferTypeOf(argArray)
					if arrType, ok := inferredElemType.(*object.ArrayType); ok {
						inferredTypes[eltIdent.Name] = arrType.ElementType
					}
				}
			}
		} else if paramTypeMap, ok := paramField.Type.(*ast.MapType); ok {
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
				argMap, ok := arg.(*object.Map)
				if !ok {
					continue
				}
				inferredMapType := e.inferTypeOf(argMap)
				if mapType, ok := inferredMapType.(*object.MapType); ok {
					if keyIsIdent && keyIsGeneric {
						inferredTypes[keyIdent.Name] = mapType.KeyType
					}
					if valIsIdent && valIsGeneric {
						inferredTypes[valIdent.Name] = mapType.ValueType
					}
				}
			}
		}
	}
	madeProgress := true
	for madeProgress {
		madeProgress = false
		for _, typeParamField := range f.TypeParams.List {
			paramName := typeParamField.Names[0].Name
			if inferredType, ok := inferredTypes[paramName]; ok {
				constraintExpr := typeParamField.Type
				if unary, ok := constraintExpr.(*ast.UnaryExpr); ok && unary.Op == token.TILDE {
					constraintExpr = unary.X
				}
				if arrayConstraint, ok := constraintExpr.(*ast.ArrayType); ok {
					if inferredArray, ok := inferredType.(*object.ArrayType); ok {
						if elemParamIdent, ok := arrayConstraint.Elt.(*ast.Ident); ok {
							elemParamName := elemParamIdent.Name
							if _, alreadyInferred := inferredTypes[elemParamName]; !alreadyInferred {
								inferredTypes[elemParamName] = inferredArray.ElementType
								madeProgress = true
							}
						}
					}
				}
			}
		}
	}
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
	stackCopy := make([]*object.CallFrame, len(e.callStack))
	copy(stackCopy, e.callStack)
	err := &object.Error{Pos: pos, Message: msg, CallStack: stackCopy}
	err.AttachFileSet(e.Fset)
	return err
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

func (e *Evaluator) nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return object.TRUE
	}
	return object.FALSE
}

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

func (e *Evaluator) evalMinusPrefixOperatorExpression(node ast.Node, right object.Object) object.Object {
	if right.Type() != object.INTEGER_OBJ {
		return e.newError(node.Pos(), "unknown operator: -%s", right.Type())
	}
	value := right.(*object.Integer).Value
	return &object.Integer{Value: -value}
}

func (e *Evaluator) evalPrefixExpression(node *ast.UnaryExpr, operator string, right object.Object) object.Object {
	switch operator {
	case "!":
		return e.evalBangOperatorExpression(right)
	case "-":
		return e.evalMinusPrefixOperatorExpression(node, right)
	case "+":
		if right.Type() != object.INTEGER_OBJ && right.Type() != object.FLOAT_OBJ {
			return e.newError(node.Pos(), "invalid operation: unary + on non-number %s", right.Type())
		}
		return right
	case "~":
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
		obj := e.evalCompositeLit(operand, env, fscope)
		if isError(obj) {
			return obj
		}
		return &object.Pointer{Element: &obj}
	default:
		return e.newError(node.Pos(), "cannot take the address of %T", node.X)
	}
}

func (e *Evaluator) unwrapToInt64(obj object.Object) (int64, bool) {
	switch o := obj.(type) {
	case *object.Integer:
		return o.Value, true
	case *object.GoValue:
		switch o.Value.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return o.Value.Int(), true
		}
	}
	return 0, false
}

func (e *Evaluator) evalMixedIntInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal, ok1 := e.unwrapToInt64(left)
	if !ok1 {
		return e.newError(node.Pos(), "left operand is not a valid integer: %s", left.Type())
	}
	rightVal, ok2 := e.unwrapToInt64(right)
	if !ok2 {
		return e.newError(node.Pos(), "right operand is not a valid integer: %s", right.Type())
	}
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
		return e.newError(node.Pos(), "unknown integer operator: %s", operator)
	}
}

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

func (e *Evaluator) evalMixedStringInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal, ok1 := e.unwrapToString(left)
	if !ok1 {
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

func (e *Evaluator) nativeToValue(val reflect.Value) object.Object {
	if !val.IsValid() {
		return object.NIL
	}
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
	case uint8:
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
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &object.Integer{Value: val.Int()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &object.Integer{Value: int64(val.Uint())}
	case reflect.Float32, reflect.Float64:
		return &object.Float{Value: val.Float()}
	case reflect.Ptr, reflect.Interface:
		if val.IsNil() {
			goType := val.Type()
			if goType.Name() != "" {
				pkgPath := goType.PkgPath()
				if goType.Kind() == reflect.Ptr {
					pkgPath = goType.Elem().PkgPath()
				}
				typeName := goType.Name()
				if goType.Kind() == reflect.Ptr {
					typeName = goType.Elem().Name()
				}
				if pkgInfo, err := e.scanner.ScanPackage(context.Background(), pkgPath); err == nil {
					for _, ti := range pkgInfo.Types {
						if ti.Name == typeName {
							return &object.TypedNil{TypeInfo: ti}
						}
					}
				}
			}
			return object.NIL
		}
		return &object.GoValue{Value: val}
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		return &object.GoValue{Value: val}
	default:
		return &object.GoValue{Value: val}
	}
}

func (e *Evaluator) objectToReflectValue(obj object.Object, targetType reflect.Type) (reflect.Value, error) {
	if targetType.Kind() == reflect.Interface && targetType.NumMethod() == 0 {
		nativeVal, err := e.objectToNativeGoValue(obj)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("unsupported conversion from %s to interface{}: %w", obj.Type(), err)
		}
		if nativeVal == nil {
			return reflect.Zero(targetType), nil
		}
		val := reflect.ValueOf(nativeVal)
		if !val.Type().AssignableTo(targetType) {
			return reflect.Value{}, fmt.Errorf("value of type %T is not assignable to interface type %s", nativeVal, targetType)
		}
		return val, nil
	}
	if goVal, ok := obj.(*object.GoValue); ok {
		if goVal.Value.Type().AssignableTo(targetType) {
			return goVal.Value, nil
		}
		if goVal.Value.Type().ConvertibleTo(targetType) {
			return goVal.Value.Convert(targetType), nil
		}
	}
	switch o := obj.(type) {
	case *object.AstNode:
		nodeType := reflect.TypeOf(o.Node)
		if nodeType.AssignableTo(targetType) {
			return reflect.ValueOf(o.Node), nil
		}
		return reflect.Value{}, fmt.Errorf("cannot convert %s (from AstNode) to %s", nodeType, targetType)
	case *object.Integer:
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
		if o.Value.Type().AssignableTo(targetType) {
			return o.Value, nil
		}
		if o.Value.Type().ConvertibleTo(targetType) {
			return o.Value.Convert(targetType), nil
		}
		return reflect.Value{}, fmt.Errorf("GoValue of type %s is not assignable or convertible to %s", o.Value.Type(), targetType)
	case *object.Nil:
		switch targetType.Kind() {
		case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func:
			return reflect.Zero(targetType), nil
		}
		return reflect.Value{}, fmt.Errorf("cannot convert nil to non-nillable type %s", targetType)
	case *object.Array:
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
				tag = name
			}
			if tag == "-" {
				continue
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

func (e *Evaluator) evalInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return e.evalIntegerInfixExpression(node, operator, left, right)
	case (left.Type() == object.INTEGER_OBJ || left.Type() == object.GO_VALUE_OBJ) &&
		(right.Type() == object.INTEGER_OBJ || right.Type() == object.GO_VALUE_OBJ):
		return e.evalMixedIntInfixExpression(node, operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return e.evalStringInfixExpression(node, operator, left, right)
	case (left.Type() == object.STRING_OBJ || left.Type() == object.GO_VALUE_OBJ) &&
		(right.Type() == object.STRING_OBJ || right.Type() == object.GO_VALUE_OBJ):
		return e.evalMixedStringInfixExpression(node, operator, left, right)
	case (left.Type() == object.BOOLEAN_OBJ || left.Type() == object.GO_VALUE_OBJ) &&
		(right.Type() == object.BOOLEAN_OBJ || right.Type() == object.GO_VALUE_OBJ):
		return e.evalMixedBoolInfixExpression(node, operator, left, right)
	case operator == "==":
		return e.nativeBoolToBooleanObject(left == right)
	case operator == "!=":
		return e.nativeBoolToBooleanObject(left != right)
	case left.Type() != right.Type():
		return e.newError(node.Pos(), "type mismatch: %s %s %s", left.Type(), operator, right.Type())
	default:
		return e.newError(node.Pos(), "unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func (e *Evaluator) isTruthy(obj object.Object) bool {
	switch o := obj.(type) {
	case *object.Boolean:
		return o.Value
	case *object.GoValue:
		if val, ok := e.unwrapToBool(o); ok {
			return val
		}
		return o.Value.IsValid() && !o.Value.IsZero()
	case *object.Nil:
		return false
	default:
		return !isError(obj)
	}
}

func (e *Evaluator) evalIfElseExpression(ie *ast.IfStmt, env *object.Environment, fscope *object.FileScope) object.Object {
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

func (e *Evaluator) evalForStmt(fs *ast.ForStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	loopEnv := object.NewEnclosedEnvironment(env)
	var loopVars []string
	if fs.Init != nil {
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
		bodyEnv := object.NewEnclosedEnvironment(loopEnv)
		for _, varName := range loopVars {
			val, ok := loopEnv.Get(varName)
			if ok {
				bodyEnv.Set(varName, val)
			}
		}
		bodyResult := e.Eval(fs.Body, bodyEnv, fscope)
		if bodyResult != nil {
			rt := bodyResult.Type()
			if rt == object.BREAK_OBJ {
				break
			}
			if rt == object.CONTINUE_OBJ {
				if fs.Post != nil {
					if postResult := e.Eval(fs.Post, loopEnv, fscope); isError(postResult) {
						return postResult
					}
				}
				continue
			}
			if rt == object.ERROR_OBJ || rt == object.RETURN_VALUE_OBJ || rt == object.PANIC_OBJ {
				return bodyResult
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
	var loopErr object.Object
	yield := &object.Builtin{
		Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
			loopEnv := object.NewEnclosedEnvironment(env)
			keyIdent, _ := rs.Key.(*ast.Ident)
			if rs.Value == nil {
				if len(args) != 1 {
					loopErr = ctx.NewError(pos, "yield must be called with 1 argument for a single-variable range loop, got %d", len(args))
					return object.FALSE
				}
				if keyIdent != nil && keyIdent.Name != "_" {
					loopEnv.Set(keyIdent.Name, args[0])
				}
			} else {
				valIdent, _ := rs.Value.(*ast.Ident)
				if len(args) != 2 {
					loopErr = ctx.NewError(pos, "yield must be called with 2 arguments for a two-variable range loop, got %d", len(args))
					return object.FALSE
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
				keyIdent, _ := rs.Key.(*ast.Ident)
				if keyIdent.Name != "_" {
					loopEnv.Set(keyIdent.Name, &object.Integer{Value: int64(i)})
				}
			}
			if rs.Value != nil {
				valueIdent, _ := rs.Value.(*ast.Ident)
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
				keyIdent, _ := rs.Key.(*ast.Ident)
				if keyIdent.Name != "_" {
					loopEnv.Set(keyIdent.Name, e.nativeToValue(k))
				}
			}
			if rs.Value != nil {
				valueIdent, _ := rs.Value.(*ast.Ident)
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
				loopEnv.Set(valueIdent.Name, &object.Integer{Value: int64(r)})
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
			return []object.Object{evaluated}
		}
		result[i] = evaluated
	}
	return result
}

func (e *Evaluator) getZeroValueForResolvedType(typeObj object.Object) object.Object {
	switch rt := typeObj.(type) {
	case *object.GoType:
		ptr := reflect.New(rt.GoType)
		return &object.GoValue{Value: ptr.Elem()}
	case *object.StructDefinition:
		instance := &object.StructInstance{Def: rt, Fields: make(map[string]object.Object)}
		for _, field := range rt.Fields {
			for _, name := range field.Names {
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
	return object.NIL
}

func (e *Evaluator) getZeroValueForType(typeExpr ast.Expr, env *object.Environment, fscope *object.FileScope) object.Object {
	typeObj := e.Eval(typeExpr, env, fscope)
	if isError(typeObj) {
		return typeObj
	}
	resolvedType := e.resolveType(typeObj, env, fscope)
	if isError(resolvedType) {
		return resolvedType
	}
	return e.getZeroValueForResolvedType(resolvedType)
}

func (e *Evaluator) applyFunction(call *ast.CallExpr, fn object.Object, args []object.Object, env *object.Environment, fscope *object.FileScope) object.Object {
	var function *object.Function
	var typeArgs []object.Object
	var receiver object.Object
	switch f := fn.(type) {
	case *object.Function:
		if f.TypeParams != nil && len(f.TypeParams.List) > 0 {
			inferred, errObj := e.inferGenericTypes(call.Pos(), f, args)
			if errObj != nil {
				return errObj
			}
			function = f
			typeArgs = inferred
		} else {
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
	case *object.Builtin:
		var pos token.Pos
		if call != nil {
			pos = call.Pos()
		}
		e.BuiltinContext.Env = env
		e.BuiltinContext.FScope = fscope
		return f.Fn(&e.BuiltinContext, pos, args...)
	case *object.GoSourceFunction:
		astDecl := f.Func.AstDecl
		if astDecl == nil || astDecl.Body == nil {
			return e.newError(call.Pos(), "cannot call Go function without a body: %s", f.Func.Name)
		}
		function = &object.Function{
			Name:       astDecl.Name,
			TypeParams: astDecl.Type.TypeParams,
			Parameters: astDecl.Type.Params,
			Results:    astDecl.Type.Results,
			Body:       astDecl.Body,
			Env:        f.DefEnv,
		}
	default:
		return e.newError(call.Pos(), "not a function: %s", fn.Type())
	}
	var callPos token.Pos
	if call != nil {
		callPos = call.Pos()
	}
	paramCount := 0
	if function.Parameters != nil {
		for _, field := range function.Parameters.List {
			if len(field.Names) > 0 {
				paramCount += len(field.Names)
			} else {
				paramCount++
			}
		}
	}
	if function.IsVariadic() {
		if len(args) < paramCount-1 {
			return e.newError(callPos, "wrong number of arguments for variadic function. got=%d, want at least %d", len(args), paramCount-1)
		}
	} else {
		if paramCount != len(args) {
			return e.newError(callPos, "wrong number of arguments. got=%d, want=%d", len(args), paramCount)
		}
	}
	if function.TypeParams != nil && len(function.TypeParams.List) > 0 {
		if len(typeArgs) != len(function.TypeParams.List) {
			return e.newError(callPos, "wrong number of type arguments. got=%d, want=%d", len(typeArgs), len(function.TypeParams.List))
		}
	}
	if function.TypeParams != nil {
		var baseConstraintEnv *object.Environment
		if function.Env != nil {
			baseConstraintEnv = function.Env
		} else {
			baseConstraintEnv = env
		}
		constraintEnv := object.NewEnclosedEnvironment(baseConstraintEnv)
		e.bindTypeParams(constraintEnv, function.TypeParams, typeArgs)
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
	funcName := "<anonymous>"
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
	defer func() { e.callStack = e.callStack[:len(e.callStack)-1] }()
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
	evalFScope := fscope
	if function.FScope != nil {
		evalFScope = function.FScope
	}
	evaluated := e.Eval(function.Body, bodyEnv, evalFScope)
	isPanic := false
	if p, ok := evaluated.(*object.Panic); ok {
		isPanic = true
		e.currentPanic = p
	}
	for i := len(frame.Defers) - 1; i >= 0; i-- {
		e.executeDeferredCall(frame.Defers[i], fscope)
	}
	if e.currentPanic != nil {
		return e.currentPanic
	}
	if isPanic {
		return object.NIL
	}
	if ret, ok := evaluated.(*object.ReturnValue); ok && ret.Value == nil {
		if frame.NamedReturns != nil {
			return e.constructNamedReturnValue(function, frame.NamedReturns)
		}
		return &object.ReturnValue{Value: object.NIL}
	}
	return e.unwrapReturnValue(evaluated)
}

func (e *Evaluator) typesAreCompatible(concrete, constraint object.Object, approximate bool) bool {
	if concrete.Inspect() == constraint.Inspect() {
		return true
	}
	if approximate {
		if concrete.Inspect() == constraint.Inspect() {
			return true
		}
	}
	return false
}

func (e *Evaluator) checkTypeConstraint(pos token.Pos, concreteType, constraint object.Object, env *object.Environment, fscope *object.FileScope) *object.Error {
	resolvedConstraint := e.resolveType(constraint, env, fscope)
	if isError(resolvedConstraint) {
		return resolvedConstraint.(*object.Error)
	}
	ifaceDef, ok := resolvedConstraint.(*object.InterfaceDefinition)
	if !ok {
		return nil
	}
	if len(ifaceDef.TypeList) > 0 {
		for _, typeExpr := range ifaceDef.TypeList {
			isApproximate := false
			if unary, ok := typeExpr.(*ast.UnaryExpr); ok && unary.Op == token.TILDE {
				isApproximate = true
				typeExpr = unary.X
			}
			constraintTypeObj := e.Eval(typeExpr, env, fscope)
			if isError(constraintTypeObj) {
				return constraintTypeObj.(*object.Error)
			}
			if e.typesAreCompatible(concreteType, constraintTypeObj, isApproximate) {
				return nil
			}
		}
		return e.newError(pos, "type %s does not satisfy interface constraint %s", concreteType.Inspect(), ifaceDef.Name.Name)
	}
	return nil
}

func (e *Evaluator) constructNamedReturnValue(fn *object.Function, env *object.Environment) object.Object {
	numReturns := len(fn.Results.List)
	if numReturns == 0 {
		return &object.ReturnValue{Value: object.NIL}
	}
	values := make([]object.Object, 0, numReturns)
	for _, field := range fn.Results.List {
		for _, name := range field.Names {
			val, _ := env.Get(name.Name)
			values = append(values, val)
		}
	}
	if len(values) == 1 {
		return &object.ReturnValue{Value: values[0]}
	}
	return &object.ReturnValue{Value: &object.Tuple{Elements: values}}
}

func (e *Evaluator) ApplyFunction(call *ast.CallExpr, fn object.Object, args []object.Object, fscope *object.FileScope) object.Object {
	env := object.NewEnvironment()
	return e.applyFunction(call, fn, args, env, fscope)
}

func (e *Evaluator) extendMethodEnv(method *object.BoundMethod, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(method.Fn.Env)
	if instance, ok := method.Receiver.(*object.StructInstance); ok {
		e.bindTypeParams(env, instance.Def.TypeParams, instance.TypeArgs)
	}
	if method.Fn.Recv != nil && len(method.Fn.Recv.List) == 1 {
		recvField := method.Fn.Recv.List[0]
		if len(recvField.Names) > 0 {
			env.Set(recvField.Names[0].Name, method.Receiver)
		}
	}
	fn := method.Fn
	if fn.Parameters == nil {
		return env
	}
	if fn.IsVariadic() {
		for i, param := range fn.Parameters.List[:len(fn.Parameters.List)-1] {
			for _, paramName := range param.Names {
				env.Set(paramName.Name, args[i])
			}
		}
		lastParam := fn.Parameters.List[len(fn.Parameters.List)-1]
		variadicArgs := args[len(fn.Parameters.List)-1:]
		arr := &object.Array{Elements: make([]object.Object, len(variadicArgs))}
		for i, arg := range variadicArgs {
			arr.Elements[i] = arg
		}
		env.Set(lastParam.Names[0].Name, arr)
	} else {
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
	e.bindTypeParams(env, fn.TypeParams, typeArgs)
	if fn.Parameters == nil {
		return
	}
	if fn.IsVariadic() {
		for i, param := range fn.Parameters.List[:len(fn.Parameters.List)-1] {
			for _, paramName := range param.Names {
				env.Set(paramName.Name, args[i])
			}
		}
		lastParam := fn.Parameters.List[len(fn.Parameters.List)-1]
		variadicArgs := args[len(fn.Parameters.List)-1:]
		arr := &object.Array{Elements: make([]object.Object, len(variadicArgs))}
		for i, arg := range variadicArgs {
			arr.Elements[i] = arg
		}
		env.Set(lastParam.Names[0].Name, arr)
	} else {
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
				argIndex++
			}
		}
	}
}

func (e *Evaluator) executeDeferredCall(deferred *object.DeferredCall, fscope *object.FileScope) {
	fnObj := e.Eval(deferred.Call.Fun, deferred.Env, fscope)
	if isError(fnObj) {
		return
	}
	args := e.evalExpressions(deferred.Call.Args, deferred.Env, fscope, nil)
	if len(args) == 1 && isError(args[0]) {
		return
	}
	e.isExecutingDefer = true
	defer func() { e.isExecutingDefer = false }()
	switch f := fnObj.(type) {
	case *object.Function:
		for _, stmt := range f.Body.List {
			evaluated := e.Eval(stmt, deferred.Env, fscope)
			if p, isPanic := evaluated.(*object.Panic); isPanic {
				e.currentPanic = p
				return
			}
			if isError(evaluated) {
				return
			}
		}
	case *object.BoundMethod:
		extendedEnv := e.extendMethodEnv(f, args)
		e.Eval(f.Fn.Body, extendedEnv, fscope)
	case *object.Builtin:
		f.Fn(&e.BuiltinContext, deferred.Call.Pos(), args...)
	default:
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
	evalEnv := object.NewEnclosedEnvironment(alias.Env)
	for i, param := range alias.TypeParams.List {
		for _, paramName := range param.Names {
			evalEnv.SetType(paramName.Name, typeArgs[i])
		}
	}
	return e.Eval(alias.Underlying, evalEnv, nil)
}

func (e *Evaluator) resolveType(typeObj object.Object, env *object.Environment, fscope *object.FileScope) object.Object {
	alias, ok := typeObj.(*object.TypeAlias)
	if !ok {
		return typeObj
	}
	if alias.ResolvedType != nil {
		return alias.ResolvedType
	}
	originalName := alias.Name
	currentAlias := alias
	for {
		if currentAlias.TypeParams != nil && len(currentAlias.TypeParams.List) > 0 {
			return e.newError(currentAlias.Name.Pos(), "cannot use generic type %s without instantiation", currentAlias.Name.Name)
		}
		resolved := e.Eval(currentAlias.Underlying, currentAlias.Env, fscope)
		if isError(resolved) {
			return resolved
		}
		nextAlias, isAlias := resolved.(*object.TypeAlias)
		if !isAlias {
			if sd, ok := resolved.(*object.StructDefinition); ok {
				if sd.Name == nil {
					sd.Name = originalName
				}
			}
			alias.ResolvedType = resolved
			return resolved
		}
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

func (e *Evaluator) EvalToplevel(decls []object.DeclWithScope, env *object.Environment) object.Object {
	varDecls, constDecls := e.registerDecls(decls, env)
	result := e.evalInitializers(append(varDecls, constDecls...), env)
	if isError(result) {
		return result
	}
	return result
}

func (e *Evaluator) registerDecls(decls []object.DeclWithScope, env *object.Environment) (varDecls, constDecls []object.DeclWithScope) {
	for _, item := range decls {
		switch d := item.Decl.(type) {
		case *ast.FuncDecl:
			e.Eval(d, env, item.Scope)
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE, token.IMPORT:
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

func (e *Evaluator) evalInitializers(decls []object.DeclWithScope, env *object.Environment) object.Object {
	var result object.Object
	for _, item := range decls {
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
	case *object.ArrayType:
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
		case "int", "uint", "uint64":
			switch input := arg.(type) {
			case *object.Integer:
				return input
			case *object.Float:
				return &object.Integer{Value: int64(input.Value)}
			default:
				return e.newError(call.Pos(), "cannot convert %s to type %s", arg.Type(), typeName)
			}
		case "string":
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

func (e *Evaluator) evalFuncType(n *ast.FuncType, env *object.Environment, fscope *object.FileScope) object.Object {
	params := []object.Object{}
	if n.Params != nil {
		for _, p := range n.Params.List {
			pType := e.Eval(p.Type, env, fscope)
			if isError(pType) {
				return pType
			}
			if len(p.Names) > 0 {
				for i := 0; i < len(p.Names); i++ {
					params = append(params, pType)
				}
			} else {
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

func (e *Evaluator) flattenTypeUnion(expr ast.Expr) []ast.Expr {
	if be, ok := expr.(*ast.BinaryExpr); ok && be.Op == token.OR {
		left := e.flattenTypeUnion(be.X)
		right := e.flattenTypeUnion(be.Y)
		return append(left, right...)
	}
	return []ast.Expr{expr}
}

func (e *Evaluator) evalProgram(program *ast.File, env *object.Environment, fscope *object.FileScope) object.Object {
	var decls []object.DeclWithScope
	for _, decl := range program.Decls {
		decls = append(decls, object.DeclWithScope{Decl: decl, Scope: fscope})
	}
	result := e.EvalToplevel(decls, env)
	if isError(result) {
		return result
	}
	mainObj, ok := env.Get("main")
	if !ok {
		return object.NIL
	}
	mainFn, ok := mainObj.(*object.Function)
	if !ok {
		return e.newError(program.Pos(), "main is not a function, but %s", mainObj.Type())
	}
	return e.applyFunction(nil, mainFn, []object.Object{}, env, fscope)
}

func (e *Evaluator) Eval(node ast.Node, env *object.Environment, fscope *object.FileScope) object.Object {
	switch n := node.(type) {
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
		case *ast.IndexExpr:
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
			Recv:       n.Recv,
			TypeParams: n.Type.TypeParams,
			Parameters: n.Type.Params,
			Results:    n.Type.Results,
			Body:       n.Body,
			Env:        env,
		}
		def.Methods[n.Name.Name] = fn
		return nil
	case *ast.DeferStmt:
		if len(e.callStack) == 0 {
			return e.newError(n.Pos(), "defer is not allowed outside of a function")
		}
		deferred := &object.DeferredCall{
			Call: n.Call,
			Env:  env,
		}
		currentFrame := e.callStack[len(e.callStack)-1]
		currentFrame.Defers = append(currentFrame.Defers, deferred)
		return nil
	case *ast.ReturnStmt:
		var currentFrame *object.CallFrame
		if len(e.callStack) > 0 {
			currentFrame = e.callStack[len(e.callStack)-1]
		}
		if currentFrame != nil && currentFrame.NamedReturns != nil {
			if len(n.Results) > 0 {
				values := e.evalExpressions(n.Results, env, fscope, nil)
				if len(values) == 1 && isError(values[0]) {
					return values[0]
				}
				i := 0
				for _, field := range currentFrame.Fn.Results.List {
					for _, name := range field.Names {
						if i < len(values) {
							currentFrame.NamedReturns.Assign(name.Name, values[i])
							i++
						}
					}
				}
			}
			return &object.ReturnValue{Value: nil}
		}
		if len(n.Results) == 0 {
			return &object.ReturnValue{Value: object.NIL}
		}
		if len(n.Results) == 1 {
			val := e.Eval(n.Results[0], env, fscope)
			if isError(val) {
				return val
			}
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
	case *ast.ParenExpr:
		return e.Eval(n.X, env, fscope)
	case *ast.IndexExpr:
		left := e.Eval(n.X, env, fscope)
		if isError(left) {
			return left
		}
		index := e.Eval(n.Index, env, fscope)
		if isError(index) {
			return index
		}
		switch l := left.(type) {
		case *object.StructDefinition, *object.Function:
			return &object.InstantiatedType{GenericDef: left, TypeArgs: []object.Object{index}}
		case *object.TypeAlias:
			return e.instantiateTypeAlias(n.Pos(), l, []object.Object{index})
		default:
			return e.evalIndexExpression(n, left, index)
		}
	case *ast.IndexListExpr:
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
		switch function.(type) {
		case *object.Type, *object.ArrayType:
			args := e.evalExpressions(n.Args, env, fscope, nil)
			if len(args) == 1 && isError(args[0]) {
				return args[0]
			}
			return e.evalTypeConversion(n, function, args)
		}
		if sf, ok := function.(*SpecialForm); ok {
			return sf.Fn(e, fscope, n.Pos(), n.Args)
		}
		var args []object.Object
		if n.Ellipsis.IsValid() {
			if len(n.Args) == 0 {
				return e.newError(n.Pos(), "cannot use ... on empty argument list")
			}
			args = e.evalExpressions(n.Args[:len(n.Args)-1], env, fscope, nil)
			if len(args) > 0 && isError(args[len(args)-1]) {
				return args[len(args)-1]
			}
			lastArg := n.Args[len(n.Args)-1]
			sliceToSpread := e.Eval(lastArg, env, fscope)
			if isError(sliceToSpread) {
				return sliceToSpread
			}
			switch s := sliceToSpread.(type) {
			case *object.Array:
				args = append(args, s.Elements...)
			default:
				return e.newError(lastArg.Pos(), "cannot use ... on non-slice type %s", sliceToSpread.Type())
			}
		} else {
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
		operand := e.Eval(n.X, env, fscope)
		if isError(operand) {
			return operand
		}
		switch operand.(type) {
		case *object.StructDefinition, *object.Type, *object.PointerType, *object.ArrayType, *object.MapType, *object.InterfaceDefinition:
			return &object.PointerType{ElementType: operand}
		default:
			return e.evalDereferenceExpression(n, operand)
		}
	case *ast.UnaryExpr:
		if n.Op == token.AND {
			return e.evalAddressOfExpression(n, env, fscope)
		}
		right := e.Eval(n.X, env, fscope)
		if isError(right) {
			return right
		}
		return e.evalPrefixExpression(n, n.Op.String(), right)
	case *ast.BinaryExpr:
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
		left := e.Eval(n.X, env, fscope)
		if isError(left) {
			return left
		}
		right := e.Eval(n.Y, env, fscope)
		if isError(right) {
			return right
		}
		return e.evalInfixExpression(n, n.Op.String(), left, right)
	case *ast.Ident:
		return e.evalIdent(n, env, fscope)
	case *ast.BasicLit:
		return e.evalBasicLit(n)
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
		return &object.StructDefinition{
			Name:    nil,
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
				max = -1
			}
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
				continue
			case ".":
				fscope.DotImports = append(fscope.DotImports, path)
			default:
				fscope.Aliases[alias] = path
			}
		}
		return nil
	case token.CONST, token.VAR:
		var lastValues []ast.Expr
		for iotaValue, spec := range n.Specs {
			valueSpec := spec.(*ast.ValueSpec)
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
				continue
			}
			if n.Tok == token.CONST {
				if len(valueSpec.Values) == 0 {
					valueSpec.Values = lastValues
				} else {
					lastValues = valueSpec.Values
				}
			}
			for i, name := range valueSpec.Names {
				if valueSpec.Type != nil {
					typeObj := e.Eval(valueSpec.Type, env, fscope)
					if isError(typeObj) {
						return typeObj
					}
					if ifaceDef, ok := typeObj.(*object.InterfaceDefinition); ok {
						var concreteVal object.Object
						if len(valueSpec.Values) > i {
							concreteVal = e.Eval(valueSpec.Values[i], env, fscope)
							if isError(concreteVal) {
								return concreteVal
							}
							if concreteVal.Type() != object.NIL_OBJ {
								if errObj := e.checkImplements(valueSpec.Pos(), concreteVal, ifaceDef); errObj != nil {
									return errObj
								}
							}
						} else {
							concreteVal = object.NIL
						}
						env.Set(name.Name, &object.InterfaceInstance{Def: ifaceDef, Value: concreteVal})
						continue
					}
				}
				var val object.Object
				if len(valueSpec.Values) > i {
					iotaEnv := object.NewEnclosedEnvironment(env)
					iotaEnv.SetConstant("iota", &object.Integer{Value: int64(iotaValue)})
					val = e.Eval(valueSpec.Values[i], iotaEnv, fscope)
				} else if n.Tok == token.VAR {
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
							ptr := reflect.New(rt.GoType)
							val = &object.GoValue{Value: ptr.Elem()}
						case *object.StructDefinition:
							instance := &object.StructInstance{Def: rt, Fields: make(map[string]object.Object)}
							for _, field := range rt.Fields {
								zeroVal := e.getZeroValueForType(field.Type, env, fscope)
								for _, name := range field.Names {
									instance.Fields[name.Name] = zeroVal
								}
							}
							val = instance
						default:
							val = object.NIL
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
				} else {
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
			if typeSpec.Assign.IsValid() {
				alias := &object.TypeAlias{
					Name:       typeSpec.Name,
					TypeParams: typeSpec.TypeParams,
					Underlying: typeSpec.Type,
					Env:        env,
				}
				env.Set(typeSpec.Name.Name, alias)
			} else {
				switch t := typeSpec.Type.(type) {
				case *ast.StructType:
					fieldTags := make(map[string]string)
					for _, field := range t.Fields.List {
						if field.Tag == nil {
							continue
						}
						tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
						jsonTag := tag.Get("json")
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
						Name:    typeSpec.Name,
						Methods: &ast.FieldList{},
						TypeList: make([]ast.Expr, 0),
					}
					if t.Methods != nil {
						for _, field := range t.Methods.List {
							if len(field.Names) > 0 {
								def.Methods.List = append(def.Methods.List, field)
							} else {
								def.TypeList = append(def.TypeList, e.flattenTypeUnion(field.Type)...)
							}
						}
					}
					env.Set(typeSpec.Name.Name, def)
				default:
					alias := &object.TypeAlias{
						Name:       typeSpec.Name,
						TypeParams: nil,
						Underlying: typeSpec.Type,
						Env:        env,
					}
					env.Set(typeSpec.Name.Name, alias)
				}
			}
		}
		return nil
	}
	return nil
}

func (e *Evaluator) evalIndexExpression(node ast.Node, left, index object.Object) object.Object {
	switch l := left.(type) {
	case *object.StructDefinition:
		if l.TypeParams != nil && len(l.TypeParams.List) > 0 {
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
			return e.newError(node.Pos(), "runtime error: index out of range [%d] with length %d", idx, val.Len())
		}
		resultVal := val.Index(idx)
		return e.nativeToValue(resultVal)
	case reflect.Map:
		keyVal, err := e.objectToReflectValue(index, val.Type().Key())
		if err != nil {
			return e.newError(node.Pos(), "cannot use %s as type %s in map index: %v", index.Type(), val.Type().Key(), err)
		}
		resultVal := val.MapIndex(keyVal)
		if !resultVal.IsValid() {
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
		return object.NIL
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
	currentVal := e.Eval(n.X, env, fscope)
	if isError(currentVal) {
		return currentVal
	}
	integer, ok := currentVal.(*object.Integer)
	if !ok {
		return e.newError(n.Pos(), "cannot %s non-integer type %s", n.Tok, currentVal.Type())
	}
	var newVal int64
	if n.Tok == token.INC {
		newVal = integer.Value + 1
	} else {
		newVal = integer.Value - 1
	}
	return e.assignValue(n.X, &object.Integer{Value: newVal}, env, fscope)
}

func (e *Evaluator) evalAssignStmt(n *ast.AssignStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
		return e.evalSingleAssign(n, env, fscope)
	}
	if len(n.Lhs) > 1 && len(n.Rhs) == 1 {
		return e.evalMultiAssign(n, env, fscope)
	}
	if len(n.Lhs) > 0 && len(n.Lhs) == len(n.Rhs) {
		return e.evalDestructuringAssign(n, env, fscope)
	}
	return e.newError(n.Pos(), "assignment mismatch: %d variables but %d values", len(n.Lhs), len(n.Rhs))
}

func (e *Evaluator) evalDestructuringAssign(n *ast.AssignStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	values := make([]object.Object, len(n.Rhs))
	for i, rhsExpr := range n.Rhs {
		val := e.Eval(rhsExpr, env, fscope)
		if isError(val) {
			return val
		}
		values[i] = val
	}
	switch n.Tok {
	case token.ASSIGN:
		for i, lhsExpr := range n.Lhs {
			res := e.assignValue(lhsExpr, values[i], env, fscope)
			if isError(res) {
				return res
			}
		}
	case token.DEFINE:
		for i, lhsExpr := range n.Lhs {
			ident, ok := lhsExpr.(*ast.Ident)
			if !ok {
				return e.newError(lhsExpr.Pos(), "non-identifier on left side of :=")
			}
			if ident.Name == "_" {
				continue
			}
			if fn, ok := values[i].(*object.Function); ok {
				fn.Name = ident
			}
			env.Set(ident.Name, values[i])
		}
	default:
		return e.newError(n.Pos(), "unsupported assignment token: %s", n.Tok)
	}
	return nil
}

func (e *Evaluator) evalSingleAssign(n *ast.AssignStmt, env *object.Environment, fscope *object.FileScope) object.Object {
	val := e.Eval(n.Rhs[0], env, fscope)
	if isError(val) {
		return val
	}
	if ret, ok := val.(*object.ReturnValue); ok {
		val = ret.Value
	}
	if _, ok := val.(*object.Tuple); ok {
		return e.newError(n.Rhs[0].Pos(), "multi-value function call in single-value context")
	}
	lhs := n.Lhs[0]
	switch n.Tok {
	case token.ASSIGN:
		return e.assignValue(lhs, val, env, fscope)
	case token.DEFINE:
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			return e.newError(lhs.Pos(), "non-identifier on left side of :=")
		}
		if ident.Name == "_" {
			return nil
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
			return nil
		}
		if existing, ok := env.Get(lhsNode.Name); ok {
			if iface, isIface := existing.(*object.InterfaceInstance); isIface {
				if val.Type() != object.NIL_OBJ {
					if errObj := e.checkImplements(lhsNode.Pos(), val, iface.Def); errObj != nil {
						return errObj
					}
				}
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
	case token.ASSIGN:
		for i, lhsExpr := range n.Lhs {
			res := e.assignValue(lhsExpr, tuple.Elements[i], env, fscope)
			if isError(res) {
				return res
			}
		}
	case token.DEFINE:
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
	return nil
}

func (e *Evaluator) findFieldInStruct(instance *object.StructInstance, fieldName string) (object.Object, bool) {
	if val, ok := instance.Fields[fieldName]; ok {
		return val, true
	}
	for _, fieldDef := range instance.Def.Fields {
		if len(fieldDef.Names) == 0 {
			var typeName string
			switch t := fieldDef.Type.(type) {
			case *ast.Ident:
				typeName = t.Name
			case *ast.StarExpr:
				if ident, ok := t.X.(*ast.Ident); ok {
					typeName = ident.Name
				}
			}
			if typeName == "" {
				continue
			}
			embeddedObj, ok := instance.Fields[typeName]
			if !ok {
				continue
			}
			if ptr, ok := embeddedObj.(*object.Pointer); ok {
				if ptr.Element == nil || *ptr.Element == nil {
					continue
				}
				embeddedObj = *ptr.Element
			}
			embeddedInstance, ok := embeddedObj.(*object.StructInstance)
			if !ok {
				continue
			}
			if val, found := e.findFieldInStruct(embeddedInstance, fieldName); found {
				return val, true
			}
		}
	}
	return nil, false
}

func (e *Evaluator) constantInfoToObject(c *goscan.ConstantInfo) (object.Object, error) {
	if c.Name == "UintSize" && c.Value == "" {
		return &object.Integer{Value: 64}, nil
	}
	if c.ConstVal != nil {
		switch c.ConstVal.Kind() {
		case constant.String:
			if c.RawValue != "" {
				return &object.String{Value: c.RawValue}, nil
			}
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

func (e *Evaluator) findSymbolInPackageInfo(pkgInfo *goscan.Package, symbolName string, pkgEnv *object.Environment, fscope *object.FileScope) (object.Object, bool) {
	if t, ok := e.registry.LookupType(pkgInfo.Path, symbolName); ok {
		return &object.GoType{GoType: t}, true
	}
	for _, c := range pkgInfo.Constants {
		if c.Name == symbolName {
			obj, err := e.constantInfoToObject(c)
			if err != nil {
				return e.newError(token.NoPos, "could not convert constant %q: %v", symbolName, err), true
			}
			return obj, true
		}
	}
	for _, t := range pkgInfo.Types {
		if t.Name == symbolName {
			switch t.Kind {
			case goscan.StructKind:
				typeSpec, ok := t.Node.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				def := &object.StructDefinition{
					Name:      typeSpec.Name,
					Fields:    structType.Fields.List,
					Methods:   make(map[string]*object.Function),
					GoMethods: make(map[string]*goscan.FunctionInfo),
				}
				for _, f := range pkgInfo.Functions {
					if f.Receiver != nil && f.Receiver.Type.Definition != nil && f.Receiver.Type.Definition.Name == t.Name {
						def.GoMethods[f.Name] = f
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
	for _, f := range pkgInfo.Functions {
		if f.Name == symbolName {
			return &object.GoSourceFunction{Func: f, PkgPath: pkgInfo.Path, DefEnv: pkgEnv}, true
		}
	}
	return nil, false
}

func (e *Evaluator) updateMiniGoStructFromNative(ctx *object.BuiltinContext, src map[string]any, dst *object.StructInstance, visited map[uintptr]object.Object) object.Object {
	dstPtr := reflect.ValueOf(dst).Pointer()
	if _, ok := visited[dstPtr]; ok {
		return nil
	}
	visited[dstPtr] = dst
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
			continue
		}
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
			return e.nativeToValue(reflect.ValueOf(nativeValue)), nil
		}
	case *object.StructDefinition:
		nestedMap, ok := nativeValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("type mismatch")
		}
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
		return e.nativeToValue(reflect.ValueOf(nativeValue)), nil
	default:
		return e.nativeToValue(reflect.ValueOf(nativeValue)), nil
	}
}

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
				if isVariadic && i >= numIn-1 {
					targetType = funcType.In(numIn - 1).Elem()
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
			for i, arg := range args {
				if arr, ok := arg.(*object.Array); ok {
					goSlice := in[i]
					if goSlice.Kind() == reflect.Slice {
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
							return errObj
						}
					}
				}
			}
			numOut := funcType.NumOut()
			if numOut == 0 {
				return object.NIL
			}
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

func (e *Evaluator) evalMethodCall(n *ast.SelectorExpr, receiver object.Object, def *object.StructDefinition) object.Object {
	method, ok := def.Methods[n.Sel.Name]
	if !ok {
		return nil
	}
	isPointerReceiver := false
	if method.Recv != nil && len(method.Recv.List) > 0 {
		if _, ok := method.Recv.List[0].Type.(*ast.StarExpr); ok {
			isPointerReceiver = true
		}
	}
	if isPointerReceiver {
		if _, isPointer := receiver.(*object.Pointer); !isPointer {
			return e.newError(n.Pos(), "cannot call pointer method %s on value %s", n.Sel.Name, def.Name.Name)
		}
		return &object.BoundMethod{Fn: method, Receiver: receiver}
	}
	if ptr, isPointer := receiver.(*object.Pointer); isPointer {
		return &object.BoundMethod{Fn: method, Receiver: *ptr.Element}
	}
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
		switch concrete := l.Value.(type) {
		case *object.StructInstance:
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
		if method := e.evalMethodCall(n, l, l.Def); method != nil {
			if err, isErr := method.(*object.Error); isErr {
				return err
			}
			return method
		}
		if val, found := e.findFieldInStruct(l, n.Sel.Name); found {
			return val
		}
		return e.newError(n.Pos(), "undefined field or method '%s' on struct '%s'", n.Sel.Name, l.Def.Name.Name)
	case *object.Pointer:
		if l.Element == nil || *l.Element == nil {
			return e.newError(n.Pos(), "nil pointer dereference")
		}
		switch elem := (*l.Element).(type) {
		case *object.StructInstance:
			if method := e.evalMethodCall(n, l, elem.Def); method != nil {
				if err, isErr := method.(*object.Error); isErr {
					return err
				}
				return method
			}
			if val, found := e.findFieldInStruct(elem, n.Sel.Name); found {
				return val
			}
			return e.newError(n.Pos(), "undefined field or method '%s' on pointer to struct '%s'", n.Sel.Name, elem.Def.Name.Name)
		case *object.GoValue:
			return e.evalGoValueSelectorExpr(n, elem, n.Sel.Name)
		default:
			return e.newError(n.Pos(), "base of selector expression is not a pointer to a struct or Go value")
		}
	case *object.GoValue:
		return e.evalGoValueSelectorExpr(n, l, n.Sel.Name)
	case *object.TypedNil:
		typeInfo := l.TypeInfo
		pkg, err := e.scanner.ScanPackage(context.Background(), typeInfo.PkgPath)
		if err != nil {
			return e.newError(n.Pos(), "package not found for type %s: %v", typeInfo.Name, err)
		}
		obj, ok := e.findSymbolInPackageInfo(pkg, typeInfo.Name, nil, nil)
		if !ok {
			return e.newError(n.Pos(), "type definition not found for %s", typeInfo.Name)
		}
		def, ok := obj.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Pos(), "type %s is not a struct", typeInfo.Name)
		}
		if method, ok := def.GoMethods[n.Sel.Name]; ok {
			return &object.GoMethod{
				Recv: typeInfo,
				Func: method,
			}
		}
		return e.newError(n.Pos(), "undefined method %s for nil pointer of type %s", n.Sel.Name, typeInfo.Name)
	default:
		return e.newError(n.Pos(), "base of selector expression is not a package or struct")
	}
}

func (e *Evaluator) findSymbolInPackage(pkg *object.Package, symbolName *ast.Ident, pos token.Pos) object.Object {
	if member, ok := pkg.Members[symbolName.Name]; ok {
		return member
	}
	if symbol, ok := e.registry.Lookup(pkg.Path, symbolName.Name); ok {
		var member object.Object
		val := reflect.ValueOf(symbol)
		if val.Kind() == reflect.Func {
			member = e.WrapGoFunction(pos, val)
		} else {
			member = &object.GoValue{Value: val}
		}
		pkg.Members[symbolName.Name] = member
		return member
	}
	if t, ok := e.registry.LookupType(pkg.Path, symbolName.Name); ok {
		member := &object.GoType{GoType: t}
		pkg.Members[symbolName.Name] = member
		return member
	}
	if pkg.Env.IsEmpty() {
		cumulativePkgInfo, err := e.scanner.FindSymbolInPackage(context.Background(), pkg.Path, symbolName.Name)
		if err != nil {
			return e.newError(pos, "undefined: %s.%s (package scan failed: %v)", pkg.Name, symbolName.Name, err)
		}
		pkg.Info = cumulativePkgInfo
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
	if member, ok := pkg.Env.Get(symbolName.Name); ok {
		pkg.Members[symbolName.Name] = member
		return member
	}
	return e.newError(pos, "undefined: %s.%s", pkg.Name, symbolName.Name)
}

func (e *Evaluator) evalGoValueSelectorExpr(node ast.Node, goVal *object.GoValue, sel string) object.Object {
	val := goVal.Value
	var method reflect.Value
	method = val.MethodByName(sel)
	if !method.IsValid() && val.CanAddr() {
		method = val.Addr().MethodByName(sel)
	}
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
				if isVariadic {
					if len(args) < numIn-1 {
						return ctx.NewError(callPos, "wrong number of arguments for variadic method %s: got %d, want at least %d", sel, len(args), numIn-1)
					}
				} else {
					if len(args) != numIn {
						return ctx.NewError(callPos, "wrong number of arguments for method %s: got %d, want %d", sel, len(args), numIn)
					}
				}
				in := make([]reflect.Value, len(args))
				for i, arg := range args {
					var targetType reflect.Type
					if isVariadic && i >= numIn-1 {
						targetType = funcType.In(numIn - 1).Elem()
					} else {
						targetType = funcType.In(i)
					}
					val, err := e.objectToReflectValue(arg, targetType)
					if err != nil {
						return ctx.NewError(callPos, "argument %d type mismatch for method %s: %v", i+1, sel, err)
					}
					in[i] = val
				}
				results := method.Call(in)
				numOut := funcType.NumOut()
				if numOut == 0 {
					return object.NIL
				}
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
	return e.newError(node.Pos(), "undefined field or method '%s' on Go object of type %s", sel, val.Type())
}

func (e *Evaluator) evalCompositeLit(n *ast.CompositeLit, env *object.Environment, fscope *object.FileScope) object.Object {
	if n.Type == nil {
		return e.newError(n.Pos(), "untyped composite literal in context where type cannot be inferred")
	}
	typeObj := e.Eval(n.Type, env, fscope)
	if isError(typeObj) {
		return typeObj
	}
	return e.evalCompositeLitWithType(n, typeObj, env, fscope)
}

func (e *Evaluator) evalCompositeLitWithType(n *ast.CompositeLit, typeObj object.Object, env *object.Environment, fscope *object.FileScope) object.Object {
	resolvedType := e.resolveType(typeObj, env, fscope)
	if isError(resolvedType) {
		return resolvedType
	}
	switch def := resolvedType.(type) {
	case *object.InstantiatedType:
		structDef, ok := def.GenericDef.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Pos(), "cannot create composite literal of non-struct generic type %s", def.GenericDef.Type())
		}
		instanceObj := e.evalStructLiteral(n, structDef, env, fscope)
		if si, ok := instanceObj.(*object.StructInstance); ok {
			si.TypeArgs = def.TypeArgs
		}
		return instanceObj
	case *object.StructDefinition:
		instanceObj := e.evalStructLiteral(n, def, env, fscope)
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
		finalElements := make([]object.Object, len(elements))
		copy(finalElements, elements)
		return &object.Array{SliceType: def, Elements: finalElements}
	case *object.MapType:
		return e.evalMapLiteral(n, def, env, fscope)
	default:
		return e.newError(n.Pos(), "cannot create composite literal for type %s", resolvedType.Type())
	}
}

func (e *Evaluator) checkImplements(pos token.Pos, concrete object.Object, iface *object.InterfaceDefinition) object.Object {
	var concreteMethods map[string]*object.Function
	var concreteTypeName string
	switch c := concrete.(type) {
	case *object.StructInstance:
		concreteMethods = c.Def.Methods
		concreteTypeName = c.Def.Name.Name
	case *object.Pointer:
		if s, ok := (*c.Element).(*object.StructInstance); ok {
			concreteMethods = s.Def.Methods
			concreteTypeName = s.Def.Name.Name
		} else {
			if len(iface.Methods.List) > 0 {
				return e.newError(pos, "type %s cannot implement non-empty interface %s", (*c.Element).Type(), iface.Name.Name)
			}
			return nil
		}
	default:
		if len(iface.Methods.List) > 0 {
			return e.newError(pos, "type %s cannot implement non-empty interface %s", concrete.Type(), iface.Name.Name)
		}
		return nil
	}
	for _, ifaceMethodField := range iface.Methods.List {
		if len(ifaceMethodField.Names) == 0 {
			continue
		}
		methodName := ifaceMethodField.Names[0].Name
		ifaceFuncType, ok := ifaceMethodField.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		concreteMethod, ok := concreteMethods[methodName]
		if !ok {
			return e.newError(pos, "type %s does not implement %s (missing method %s)", concreteTypeName, iface.Name.Name, methodName)
		}
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
	}
	return nil
}

func (e *Evaluator) evalStructLiteral(n *ast.CompositeLit, def *object.StructDefinition, env *object.Environment, fscope *object.FileScope) object.Object {
	instance := &object.StructInstance{Def: def, Fields: make(map[string]object.Object)}
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
			fieldName := node.Name
			value := e.Eval(node, env, fscope)
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
	if pkg, ok := e.packages[path]; ok {
		return pkg
	}
	pkgObj := &object.Package{
		Name:    ident.Name,
		Path:    path,
		Env:     object.NewEnvironment(),
		Info:    nil,
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
	if fscope != nil {
		for _, path := range fscope.DotImports {
			dummyIdent := &ast.Ident{Name: "_"}
			pkg := e.resolvePackage(dummyIdent, path)
			val := e.findSymbolInPackage(pkg, n, n.Pos())
			if err, ok := val.(*object.Error); ok {
				if strings.Contains(err.Message, "undefined:") {
					continue
				}
			}
			return val
		}
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
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(n.Pos(), "could not unquote char literal %q", n.Value)
		}
		return &object.Integer{Value: int64(rune(s[0]))}
	default:
		return e.newError(n.Pos(), "unsupported literal type: %s", n.Kind)
	}
}

func (e *Evaluator) Scanner() *goscan.Scanner {
	return e.scanner
}
