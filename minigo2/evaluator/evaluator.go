package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
	"strings"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo2/object"
)

var builtins = map[string]*object.Builtin{
	"len": {
		Fn: func(fset *token.FileSet, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				err := &object.Error{
					Pos:     pos,
					Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)),
				}
				err.AttachFileSet(fset)
				return err
			}
			switch arg := args[0].(type) {
			case *object.Array:
				return &object.Integer{Value: int64(len(arg.Elements))}
			case *object.String:
				return &object.Integer{Value: int64(len(arg.Value))}
			case *object.GoValue:
				val := arg.Value
				switch val.Kind() {
				case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
					return &object.Integer{Value: int64(val.Len())}
				default:
					err := &object.Error{
						Pos:     pos,
						Message: fmt.Sprintf("argument to `len` not supported, got Go value of type %s", val.Kind()),
					}
					err.AttachFileSet(fset)
					return err
				}
			default:
				err := &object.Error{
					Pos:     pos,
					Message: fmt.Sprintf("argument to `len` not supported, got %s", args[0].Type()),
				}
				err.AttachFileSet(fset)
				return err
			}
		},
	},
	"new": {
		Fn: func(fset *token.FileSet, pos token.Pos, args ...object.Object) object.Object {
			if len(args) != 1 {
				err := &object.Error{
					Pos:     pos,
					Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args)),
				}
				err.AttachFileSet(fset)
				return err
			}
			def, ok := args[0].(*object.StructDefinition)
			if !ok {
				err := &object.Error{
					Pos:     pos,
					Message: fmt.Sprintf("argument to `new` must be a type, got %s", args[0].Type()),
				}
				err.AttachFileSet(fset)
				return err
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
}

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	fset      *token.FileSet
	scanner   *goscan.Scanner
	registry  *object.SymbolRegistry
	callStack []object.CallFrame
}

// New creates a new Evaluator.
func New(fset *token.FileSet, scanner *goscan.Scanner, registry *object.SymbolRegistry) *Evaluator {
	return &Evaluator{
		fset:      fset,
		scanner:   scanner,
		registry:  registry,
		callStack: make([]object.CallFrame, 0),
	}
}

func (e *Evaluator) newError(pos token.Pos, format string, args ...interface{}) *object.Error {
	msg := fmt.Sprintf(format, args...)
	// Create a copy of the current call stack for the error object.
	stackCopy := make([]object.CallFrame, len(e.callStack))
	copy(stackCopy, e.callStack)

	err := &object.Error{
		Pos:       pos,
		Message:   msg,
		CallStack: stackCopy,
	}
	err.AttachFileSet(e.fset) // Attach fset for formatting
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

func (e *Evaluator) evalAddressOfExpression(node *ast.UnaryExpr, env *object.Environment) object.Object {
	switch operand := node.X.(type) {
	case *ast.Ident:
		addr, ok := env.GetAddress(operand.Name)
		if !ok {
			return e.newError(node.Pos(), "cannot take the address of undeclared variable: %s", operand.Name)
		}
		return &object.Pointer{Element: addr}
	case *ast.CompositeLit:
		// Evaluate the composite literal to create the object instance.
		obj := e.evalCompositeLit(operand, env)
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
	case ">":
		return e.nativeBoolToBooleanObject(leftVal > rightVal)
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
	case ">":
		return e.nativeBoolToBooleanObject(leftVal > rightVal)
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

// nativeToValue converts a native Go value (from reflect.Value) into a minigo2 object.
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
	case int64:
		return &object.Integer{Value: v}
	case string:
		return &object.String{Value: v}
	case bool:
		return e.nativeBoolToBooleanObject(v)
	case nil:
		return object.NIL
	}

	// If direct conversion fails, fall back to Kind-based conversion.
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return &object.Integer{Value: val.Int()}
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

// objectToReflectValue converts a minigo2 object to a reflect.Value of a specific Go type.
// This is a crucial helper for map indexing and function calls into Go code.
func (e *Evaluator) objectToReflectValue(obj object.Object, targetType reflect.Type) (reflect.Value, error) {
	// Handle target type of interface{} separately.
	// We convert the minigo2 object to its "best" Go equivalent.
	if targetType.Kind() == reflect.Interface && targetType.NumMethod() == 0 {
		var nativeVal any
		switch o := obj.(type) {
		case *object.Integer:
			nativeVal = o.Value
		case *object.String:
			nativeVal = o.Value
		case *object.Boolean:
			nativeVal = o.Value
		case *object.Nil:
			nativeVal = nil
		case *object.GoValue:
			nativeVal = o.Value.Interface()
		case *object.Array:
			// A simple conversion to []any. More complex conversions would need more logic.
			slice := make([]any, len(o.Elements))
			for i, elem := range o.Elements {
				// This is a recursive call, but it's safe because the target type is concrete.
				if val, err := e.objectToNativeGoValue(elem); err == nil {
					slice[i] = val
				} else {
					return reflect.Value{}, fmt.Errorf("cannot convert array element %d to Go value: %w", i, err)
				}
			}
			nativeVal = slice
		default:
			return reflect.Value{}, fmt.Errorf("unsupported conversion from %s to interface{}", obj.Type())
		}
		if nativeVal == nil {
			return reflect.Zero(targetType), nil
		}
		// We have the native Go value; now we need to put it into a reflect.Value
		// of the target interface type.
		val := reflect.ValueOf(nativeVal)
		if !val.Type().AssignableTo(targetType) {
			// This can happen if nativeVal is e.g. int64 and targetType is a named interface.
			// For interface{}, this should generally not fail.
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
		// Fall through to allow conversions like minigo2 Integer -> Go float64
	}

	switch o := obj.(type) {
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
	}

	return reflect.Value{}, fmt.Errorf("unsupported conversion from %s to %s", obj.Type(), targetType)
}

// objectToNativeGoValue converts a minigo2 object to its most natural Go counterpart.
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
		return e.nativeBoolToBooleanObject(left == right)
	case operator == "!=":
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
	case *object.Nil:
		return false
	default:
		// Any other object type (Integer, String, etc.) is considered truthy.
		return !isError(obj)
	}
}

// evalIfElseExpression evaluates an if-else expression.
func (e *Evaluator) evalIfElseExpression(ie *ast.IfStmt, env *object.Environment) object.Object {
	condition := e.Eval(ie.Cond, env)
	if isError(condition) {
		return condition
	}

	if e.isTruthy(condition) {
		return e.Eval(ie.Body, env)
	} else if ie.Else != nil {
		return e.Eval(ie.Else, env)
	} else {
		return object.NIL
	}
}

// evalBlockStatement evaluates a block of statements within a new scope.
func (e *Evaluator) evalBlockStatement(block *ast.BlockStmt, env *object.Environment) object.Object {
	var result object.Object
	enclosedEnv := object.NewEnclosedEnvironment(env)

	for _, statement := range block.List {
		result = e.Eval(statement, enclosedEnv)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.BREAK_OBJ || rt == object.CONTINUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
		}
	}

	return result
}

// evalForStmt evaluates a for loop.
func (e *Evaluator) evalForStmt(fs *ast.ForStmt, env *object.Environment) object.Object {
	loopEnv := object.NewEnclosedEnvironment(env)

	if fs.Init != nil {
		initResult := e.Eval(fs.Init, loopEnv)
		if isError(initResult) {
			return initResult
		}
	}

	for {
		if fs.Cond != nil {
			condition := e.Eval(fs.Cond, loopEnv)
			if isError(condition) {
				return condition
			}
			if !e.isTruthy(condition) {
				break
			}
		}

		bodyResult := e.Eval(fs.Body, loopEnv)
		if isError(bodyResult) {
			return bodyResult
		}

		if bodyResult != nil {
			if bodyResult.Type() == object.BREAK_OBJ {
				break
			}
			if bodyResult.Type() == object.CONTINUE_OBJ {
				if fs.Post != nil {
					postResult := e.Eval(fs.Post, loopEnv)
					if isError(postResult) {
						return postResult
					}
				}
				continue
			}
		}

		if fs.Post != nil {
			postResult := e.Eval(fs.Post, loopEnv)
			if isError(postResult) {
				return postResult
			}
		}
	}

	return object.NIL
}

// evalForRangeStmt evaluates a for...range loop.
func (e *Evaluator) evalForRangeStmt(rs *ast.RangeStmt, env *object.Environment) object.Object {
	iterable := e.Eval(rs.X, env)
	if isError(iterable) {
		return iterable
	}

	switch iterable := iterable.(type) {
	case *object.Array:
		return e.evalRangeArray(rs, iterable, env)
	case *object.String:
		return e.evalRangeString(rs, iterable, env)
	case *object.Map:
		return e.evalRangeMap(rs, iterable, env)
	case *object.GoValue:
		return e.evalRangeGoValue(rs, iterable, env)
	default:
		return e.newError(rs.X.Pos(), "range operator not supported for %s", iterable.Type())
	}
}

func (e *Evaluator) evalRangeGoValue(rs *ast.RangeStmt, goVal *object.GoValue, env *object.Environment) object.Object {
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

			result := e.Eval(rs.Body, loopEnv)
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

			result := e.Eval(rs.Body, loopEnv)
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

func (e *Evaluator) evalRangeArray(rs *ast.RangeStmt, arr *object.Array, env *object.Environment) object.Object {
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

		result := e.Eval(rs.Body, loopEnv)
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

func (e *Evaluator) evalRangeString(rs *ast.RangeStmt, str *object.String, env *object.Environment) object.Object {
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

		result := e.Eval(rs.Body, loopEnv)
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

func (e *Evaluator) evalRangeMap(rs *ast.RangeStmt, m *object.Map, env *object.Environment) object.Object {
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

		result := e.Eval(rs.Body, loopEnv)
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
func (e *Evaluator) evalSwitchStmt(ss *ast.SwitchStmt, env *object.Environment) object.Object {
	switchEnv := env
	if ss.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		initResult := e.Eval(ss.Init, switchEnv)
		if isError(initResult) {
			return initResult
		}
	}

	var tag object.Object
	if ss.Tag != nil {
		tag = e.Eval(ss.Tag, switchEnv)
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
			caseVal := e.Eval(caseExpr, switchEnv)
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
				result = e.Eval(caseBodyStmt, caseEnv)
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
			result = e.Eval(caseBodyStmt, caseEnv)
			if isError(result) {
				return result
			}
		}
		return result
	}

	return object.NIL
}

func (e *Evaluator) evalExpressions(exps []ast.Expr, env *object.Environment) []object.Object {
	var result []object.Object

	for _, exp := range exps {
		evaluated := e.Eval(exp, env)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

func (e *Evaluator) applyFunction(call *ast.CallExpr, fn object.Object, args []object.Object) object.Object {
	switch fn := fn.(type) {
	case *object.Function:
		// Check argument count before extending the environment
		if fn.IsVariadic() {
			if len(args) < len(fn.Parameters.List)-1 {
				return e.newError(call.Pos(), "wrong number of arguments for variadic function. got=%d, want at least %d", len(args), len(fn.Parameters.List)-1)
			}
		} else {
			if fn.Parameters != nil && len(fn.Parameters.List) != len(args) {
				return e.newError(call.Pos(), "wrong number of arguments. got=%d, want=%d", len(args), len(fn.Parameters.List))
			} else if fn.Parameters == nil && len(args) != 0 {
				return e.newError(call.Pos(), "wrong number of arguments. got=%d, want=0", len(args))
			}
		}

		funcName := "<anonymous>"
		if fn.Name != nil {
			funcName = fn.Name.Name
		}
		frame := object.CallFrame{Pos: call.Pos(), Function: funcName}
		e.callStack = append(e.callStack, frame)
		defer func() { e.callStack = e.callStack[:len(e.callStack)-1] }()

		extendedEnv := e.extendFunctionEnv(fn, args)
		evaluated := e.Eval(fn.Body, extendedEnv)
		return e.unwrapReturnValue(evaluated)

	case *object.BoundMethod:
		// Argument count check for methods
		if fn.Fn.IsVariadic() {
			if len(args) < len(fn.Fn.Parameters.List)-1 {
				return e.newError(call.Pos(), "wrong number of arguments for variadic method. got=%d, want at least %d", len(args), len(fn.Fn.Parameters.List)-1)
			}
		} else {
			if fn.Fn.Parameters != nil && len(fn.Fn.Parameters.List) != len(args) {
				return e.newError(call.Pos(), "wrong number of arguments for method. got=%d, want=%d", len(args), len(fn.Fn.Parameters.List))
			} else if fn.Fn.Parameters == nil && len(args) != 0 {
				return e.newError(call.Pos(), "wrong number of arguments for method. got=%d, want=0", len(args))
			}
		}

		funcName := "<anonymous>"
		if fn.Fn.Name != nil {
			funcName = fn.Fn.Name.Name
		}
		frame := object.CallFrame{Pos: call.Pos(), Function: funcName}
		e.callStack = append(e.callStack, frame)
		defer func() { e.callStack = e.callStack[:len(e.callStack)-1] }()

		extendedEnv := e.extendMethodEnv(fn, args)
		evaluated := e.Eval(fn.Fn.Body, extendedEnv)
		return e.unwrapReturnValue(evaluated)

	case *object.Builtin:
		return fn.Fn(e.fset, call.Pos(), args...)
	default:
		return e.newError(call.Pos(), "not a function: %s", fn.Type())
	}
}

func (e *Evaluator) extendMethodEnv(method *object.BoundMethod, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(method.Fn.Env)

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
		for i, param := range fn.Parameters.List {
			for _, paramName := range param.Names {
				env.Set(paramName.Name, args[i])
			}
		}
	}

	return env
}

func (e *Evaluator) extendFunctionEnv(fn *object.Function, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(fn.Env)
	if fn.Parameters == nil {
		return env
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
		for i, param := range fn.Parameters.List {
			for _, paramName := range param.Names {
				env.Set(paramName.Name, args[i])
			}
		}
	}

	return env
}

func (e *Evaluator) unwrapReturnValue(obj object.Object) object.Object {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

func (e *Evaluator) evalBranchStmt(bs *ast.BranchStmt, env *object.Environment) object.Object {
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

func (e *Evaluator) Eval(node ast.Node, env *object.Environment) object.Object {
	switch n := node.(type) {
	// Statements
	case *ast.BlockStmt:
		return e.evalBlockStatement(n, env)
	case *ast.ExprStmt:
		return e.Eval(n.X, env)
	case *ast.IfStmt:
		return e.evalIfElseExpression(n, env)
	case *ast.SwitchStmt:
		return e.evalSwitchStmt(n, env)
	case *ast.ForStmt:
		return e.evalForStmt(n, env)
	case *ast.RangeStmt:
		return e.evalForRangeStmt(n, env)
	case *ast.BranchStmt:
		return e.evalBranchStmt(n, env)
	case *ast.DeclStmt:
		return e.Eval(n.Decl, env)
	case *ast.FuncDecl:
		// Regular function declaration
		if n.Recv == nil {
			fn := &object.Function{
				Name:       n.Name,
				Parameters: n.Type.Params,
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
		default:
			return e.newError(recvField.Type.Pos(), "unsupported receiver type: %T", recvField.Type)
		}

		obj, ok := env.Get(typeName)
		if !ok {
			return e.newError(n.Pos(), "type '%s' not defined for method receiver", typeName)
		}

		def, ok := obj.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Pos(), "receiver for method '%s' is not a struct type", n.Name.Name)
		}

		fn := &object.Function{
			Name:       n.Name,
			Recv:       n.Recv, // Store receiver info
			Parameters: n.Type.Params,
			Body:       n.Body,
			Env:        env, // The environment where the method is defined.
		}

		def.Methods[n.Name.Name] = fn
		return nil
	case *ast.ReturnStmt:
		if len(n.Results) == 0 {
			return &object.ReturnValue{Value: object.NIL}
		}

		// Handle single vs. multiple return values
		if len(n.Results) == 1 {
			val := e.Eval(n.Results[0], env)
			if isError(val) {
				return val
			}
			return &object.ReturnValue{Value: val}
		}

		// Multiple return values are evaluated and wrapped in a Tuple.
		results := e.evalExpressions(n.Results, env)
		if len(results) > 0 && isError(results[0]) {
			// evalExpressions returns a single error if one occurs.
			return results[0]
		}
		return &object.ReturnValue{Value: &object.Tuple{Elements: results}}
	case *ast.GenDecl:
		return e.evalGenDecl(n, env)
	case *ast.AssignStmt:
		return e.evalAssignStmt(n, env)

	// Expressions
	case *ast.ParenExpr:
		return e.Eval(n.X, env)
	case *ast.IndexExpr:
		left := e.Eval(n.X, env)
		if isError(left) {
			return left
		}
		index := e.Eval(n.Index, env)
		if isError(index) {
			return index
		}
		return e.evalIndexExpression(n, left, index)
	case *ast.FuncLit:
		return &object.Function{
			Parameters: n.Type.Params,
			Body:       n.Body,
			Env:        env,
		}
	case *ast.CallExpr:
		function := e.Eval(n.Fun, env)
		if isError(function) {
			return function
		}
		args := e.evalExpressions(n.Args, env)
		if len(args) == 1 && isError(args[0]) {
			return args[0]
		}
		return e.applyFunction(n, function, args)
	case *ast.SelectorExpr:
		return e.evalSelectorExpr(n, env)
	case *ast.CompositeLit:
		return e.evalCompositeLit(n, env)
	case *ast.StarExpr:
		operand := e.Eval(n.X, env)
		if isError(operand) {
			return operand
		}
		return e.evalDereferenceExpression(n, operand)
	case *ast.UnaryExpr:
		// Special case for address-of operator, as we don't evaluate the operand.
		if n.Op == token.AND {
			return e.evalAddressOfExpression(n, env)
		}
		right := e.Eval(n.X, env)
		if isError(right) {
			return right
		}
		return e.evalPrefixExpression(n, n.Op.String(), right)
	case *ast.BinaryExpr:
		left := e.Eval(n.X, env)
		if isError(left) {
			return left
		}
		right := e.Eval(n.Y, env)
		if isError(right) {
			return right
		}
		return e.evalInfixExpression(n, n.Op.String(), left, right)

	// Literals
	case *ast.Ident:
		return e.evalIdent(n, env)
	case *ast.BasicLit:
		return e.evalBasicLit(n)
	}

	return e.newError(node.Pos(), "evaluation not implemented for %T", node)
}

func (e *Evaluator) evalGenDecl(n *ast.GenDecl, env *object.Environment) object.Object {
	var lastVal object.Object
	switch n.Tok {
	case token.IMPORT:
		for _, spec := range n.Specs {
			importSpec := spec.(*ast.ImportSpec)
			path, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				return e.newError(importSpec.Path.Pos(), "invalid import path: %v", err)
			}

			var pkgName string
			if importSpec.Name != nil {
				pkgName = importSpec.Name.Name
			} else {
				parts := strings.Split(path, "/")
				pkgName = parts[len(parts)-1]
			}

			// Handle dot import: load all symbols into the current environment.
			if pkgName == "." {
				// 1. Get symbols from the registry
				if symbols, ok := e.registry.GetAllFor(path); ok {
					for name, symbol := range symbols {
						var member object.Object
						val := reflect.ValueOf(symbol)
						if val.Kind() == reflect.Func {
							member = e.wrapGoFunction(importSpec.Pos(), val)
						} else {
							member = &object.GoValue{Value: val}
						}
						env.Set(name, member)
					}
				}

				// 2. Scan package for source-level symbols (like consts)
				pkgInfo, _ := e.scanner.ScanPackage(context.Background(), path)
				if pkgInfo != nil {
					for _, c := range pkgInfo.Constants {
						obj, err := e.constantInfoToObject(c)
						if err == nil {
							env.Set(c.Name, obj)
						}
					}
				}
				continue // Move to the next import spec
			}

			// Handle blank import: do nothing. Its side-effects are assumed to be handled.
			if pkgName == "_" {
				continue
			}

			// Regular import: create a proxy package object.
			pkgObj := &object.Package{
				Name:    pkgName,
				Path:    path,
				Info:    nil, // Mark as not loaded yet
				Members: make(map[string]object.Object),
			}
			env.Set(pkgName, pkgObj)
		}
		return nil

	case token.CONST, token.VAR:
		var lastValues []ast.Expr // For const value carry-over
		for iotaValue, spec := range n.Specs {
			valueSpec := spec.(*ast.ValueSpec)

			// Handle multi-return assignment: var a, b = f()
			if n.Tok == token.VAR && len(valueSpec.Names) > 1 && len(valueSpec.Values) == 1 {
				val := e.Eval(valueSpec.Values[0], env)
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
				var val object.Object
				if len(valueSpec.Values) > 0 {
					if i < len(valueSpec.Values) {
						// Create a temporary environment for iota evaluation.
						iotaEnv := object.NewEnclosedEnvironment(env)
						iotaEnv.SetConstant("iota", &object.Integer{Value: int64(iotaValue)})
						val = e.Eval(valueSpec.Values[i], iotaEnv)
					} else {
						return e.newError(name.Pos(), "missing value in declaration for %s", name.Name)
					}
				} else if n.Tok == token.VAR {
					// Handle `var x int` (no initial value) -> defaults to nil
					val = object.NIL
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
			typeSpec := spec.(*ast.TypeSpec)
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return e.newError(typeSpec.Pos(), "unsupported type declaration: not a struct")
			}
			def := &object.StructDefinition{
				Name:    typeSpec.Name,
				Fields:  structType.Fields.List,
				Methods: make(map[string]*object.Function), // Initialize methods map
			}
			env.Set(typeSpec.Name.Name, def)
		}
		return nil
	}

	return nil // Should be unreachable
}

func (e *Evaluator) evalIndexExpression(node ast.Node, left, index object.Object) object.Object {
	switch {
	case left.Type() == object.ARRAY_OBJ:
		return e.evalArrayIndexExpression(node, left, index)
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
		// Convert the minigo2 index object to a reflect.Value that can be used as a map key.
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

func (e *Evaluator) evalAssignStmt(n *ast.AssignStmt, env *object.Environment) object.Object {
	if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
		// Single assignment: a = 1 or a := 1
		return e.evalSingleAssign(n, env)
	}

	if len(n.Lhs) > 1 && len(n.Rhs) == 1 {
		// Multi-assignment: a, b = f() or a, b := f()
		return e.evalMultiAssign(n, env)
	}

	// Other cases like a, b = 1, 2 are not supported by Go's parser in this form
	// for `var` or `:=`, but let's be safe.
	return e.newError(n.Pos(), "unsupported assignment form: %d LHS values, %d RHS values", len(n.Lhs), len(n.Rhs))
}

func (e *Evaluator) evalSingleAssign(n *ast.AssignStmt, env *object.Environment) object.Object {
	val := e.Eval(n.Rhs[0], env)
	if isError(val) {
		return val
	}

	// Calling a multi-return function in a single-value context is an error.
	if _, ok := val.(*object.Tuple); ok {
		return e.newError(n.Rhs[0].Pos(), "multi-value function call in single-value context")
	}

	lhs := n.Lhs[0]
	switch n.Tok {
	case token.ASSIGN: // =
		return e.assignValue(lhs, val, env)
	case token.DEFINE: // :=
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			return e.newError(lhs.Pos(), "non-identifier on left side of :=")
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

func (e *Evaluator) assignValue(lhs ast.Expr, val object.Object, env *object.Environment) object.Object {
	switch lhsNode := lhs.(type) {
	case *ast.Ident:
		if _, ok := env.GetConstant(lhsNode.Name); ok {
			return e.newError(lhsNode.Pos(), "cannot assign to constant %s", lhsNode.Name)
		}
		if !env.Assign(lhsNode.Name, val) {
			return e.newError(lhsNode.Pos(), "undeclared variable: %s", lhsNode.Name)
		}
		return val
	case *ast.SelectorExpr:
		obj := e.Eval(lhsNode.X, env)
		if isError(obj) {
			return obj
		}
		// Automatically dereference pointers.
		if ptr, ok := obj.(*object.Pointer); ok {
			obj = *ptr.Element
		}
		instance, ok := obj.(*object.StructInstance)
		if !ok {
			return e.newError(lhsNode.Pos(), "assignment to non-struct field")
		}
		instance.Fields[lhsNode.Sel.Name] = val
		return val
	case *ast.StarExpr:
		ptrObj := e.Eval(lhsNode.X, env)
		if isError(ptrObj) {
			return ptrObj
		}
		ptr, ok := ptrObj.(*object.Pointer)
		if !ok {
			return e.newError(lhsNode.Pos(), "cannot assign to non-pointer")
		}
		*ptr.Element = val
		return val
	default:
		return e.newError(lhs.Pos(), "unsupported assignment target")
	}
}

func (e *Evaluator) evalMultiAssign(n *ast.AssignStmt, env *object.Environment) object.Object {
	val := e.Eval(n.Rhs[0], env)
	if isError(val) {
		return val
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
			res := e.assignValue(lhsExpr, tuple.Elements[i], env)
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

// constantInfoToObject converts a goscan.ConstantInfo into a minigo2 object.
// This is how the interpreter understands constants from imported Go packages.
func (e *Evaluator) constantInfoToObject(c *goscan.ConstantInfo) (object.Object, error) {
	// simplified inference
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
// NOTE: This resolves constants and struct type definitions. Functions must be pre-registered.
func (e *Evaluator) findSymbolInPackageInfo(pkgInfo *goscan.Package, symbolName string) (object.Object, bool) {
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
		if t.Name == symbolName && t.Kind == goscan.StructKind {
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
				Name:   typeSpec.Name,
				Fields: structType.Fields.List,
			}
			return def, true
		}
	}

	return nil, false
}

func (e *Evaluator) wrapGoFunction(pos token.Pos, funcVal reflect.Value) object.Object {
	funcType := funcVal.Type()
	return &object.Builtin{
		Fn: func(fset *token.FileSet, callPos token.Pos, args ...object.Object) object.Object {
			// Check arg count
			numIn := funcType.NumIn()
			isVariadic := funcType.IsVariadic()
			if isVariadic {
				if len(args) < numIn-1 {
					return e.newError(pos, "wrong number of arguments for variadic function: got %d, want at least %d", len(args), numIn-1)
				}
			} else {
				if len(args) != numIn {
					return e.newError(pos, "wrong number of arguments: got %d, want %d", len(args), numIn)
				}
			}

			// Convert args
			in := make([]reflect.Value, len(args))
			for i, arg := range args {
				var targetType reflect.Type
				if isVariadic && i >= funcType.NumIn()-1 {
					// For variadic part, target the element type of the slice
					targetType = funcType.In(funcType.NumIn() - 1).Elem()
				} else {
					targetType = funcType.In(i)
				}
				val, err := e.objectToReflectValue(arg, targetType)
				if err != nil {
					return e.newError(pos, "argument %d type mismatch: %v", i+1, err)
				}
				in[i] = val
			}

			// Call the function
			var results []reflect.Value
			// Use a deferred function to recover from panics in the called Go function.
			defer func() {
				if r := recover(); r != nil {
					// Create the error object, but don't assign it to results here
					// as it might be overwritten by the return.
					errObj := e.newError(pos, "panic in called Go function: %v", r)
					// Set results to a slice containing the error to ensure it's propagated.
					results = []reflect.Value{reflect.ValueOf(errObj)}
				}
			}()

			results = funcVal.Call(in)

			// Handle results
			numOut := funcType.NumOut()
			if numOut == 0 {
				return object.NIL
			}

			// Check for Go-style error return
			lastResult := results[len(results)-1]
			if lastResult.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
				if !lastResult.IsNil() {
					return e.newError(pos, "error from called Go function: %v", lastResult.Interface())
				}
			}

			// Convert all results to minigo2 objects. Replace nil error with NIL.
			resultObjects := make([]object.Object, numOut)
			for i := 0; i < numOut; i++ {
				if i == numOut-1 && lastResult.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) && results[i].IsNil() {
					resultObjects[i] = object.NIL
				} else {
					resultObjects[i] = e.nativeToValue(results[i])
				}
			}

			// If the function only ever returns one value, return it directly.
			if numOut == 1 {
				return resultObjects[0]
			}

			// Otherwise, always return a tuple.
			return &object.Tuple{Elements: resultObjects}
		},
	}
}

// evalStructSelector evaluates a selector expression on a struct instance.
// It checks for fields first, then methods.
func (e *Evaluator) evalStructSelector(n *ast.SelectorExpr, instance *object.StructInstance) object.Object {
	// Fields take precedence over methods.
	if val, found := e.findFieldInStruct(instance, n.Sel.Name); found {
		return val
	}
	// If no field, check for a method.
	if method, ok := instance.Def.Methods[n.Sel.Name]; ok {
		// Check if the method has a value or pointer receiver.
		recvType := method.Recv.List[0].Type
		if _, isPointer := recvType.(*ast.StarExpr); isPointer {
			// Pointer receiver, so bind the instance directly.
			return &object.BoundMethod{Fn: method, Receiver: instance}
		} else {
			// Value receiver, so bind a copy of the instance.
			return &object.BoundMethod{Fn: method, Receiver: instance.Copy()}
		}
	}
	return e.newError(n.Pos(), "undefined field or method '%s' on struct '%s'", n.Sel.Name, instance.Def.Name.Name)
}

func (e *Evaluator) evalSelectorExpr(n *ast.SelectorExpr, env *object.Environment) object.Object {
	left := e.Eval(n.X, env)
	if isError(left) {
		return left
	}

	switch l := left.(type) {
	case *object.Package:
		memberName := n.Sel.Name
		// 1. Check member cache first.
		if member, ok := l.Members[memberName]; ok {
			return member
		}

		// 2. Check the registry for pre-registered symbols.
		if symbol, ok := e.registry.Lookup(l.Path, memberName); ok {
			var member object.Object
			val := reflect.ValueOf(symbol)
			if val.Kind() == reflect.Func {
				member = e.wrapGoFunction(n.Sel.Pos(), val)
			} else {
				member = &object.GoValue{Value: val}
			}
			l.Members[memberName] = member // Cache it
			return member
		}

		// 3. Fallback to scanning source files for constants if not in registry.
		// Check already loaded info.
		if l.Info != nil {
			member, found := e.findSymbolInPackageInfo(l.Info, memberName)
			if found {
				if err, isErr := member.(*object.Error); isErr {
					return err // Propagate conversion errors
				}
				l.Members[memberName] = member // Cache it
				return member
			}
		}

		// 4. Symbol not in loaded info, try scanning remaining files.
		cumulativePkgInfo, err := e.scanner.FindSymbolInPackage(context.Background(), l.Path, memberName)
		if err != nil {
			// Not found in any unscanned files either. It's undefined.
			return e.newError(n.Sel.Pos(), "undefined: %s.%s", l.Name, memberName)
		}

		// 5. Symbol was found. `cumulativePkgInfo` contains everything scanned so far.
		// Update the package object with the richer info.
		l.Info = cumulativePkgInfo

		// 6. Now that info is updated, the symbol must be in it. Find it, cache it, return it.
		member, found := e.findSymbolInPackageInfo(l.Info, memberName)
		if !found {
			// This should be an impossible state if FindSymbolInPackage works correctly.
			return e.newError(n.Sel.Pos(), "internal inconsistency: symbol %s found by scanner but not in final package info", memberName)
		}
		if err, isErr := member.(*object.Error); isErr {
			return err
		}
		l.Members[memberName] = member // Cache it
		return member

	case *object.StructInstance:
		return e.evalStructSelector(n, l)

	case *object.Pointer:
		if l.Element == nil || *l.Element == nil {
			return e.newError(n.Pos(), "nil pointer dereference")
		}
		// Automatically dereference pointers for struct field/method access.
		if instance, ok := (*l.Element).(*object.StructInstance); ok {
			return e.evalStructSelector(n, instance)
		}
		return e.newError(n.Pos(), "base of selector expression is not a struct or pointer to struct")
	case *object.GoValue:
		return e.evalGoValueSelectorExpr(n, l, n.Sel.Name)
	default:
		return e.newError(n.Pos(), "base of selector expression is not a package or struct")
	}
}

func (e *Evaluator) evalGoValueSelectorExpr(node ast.Node, goVal *object.GoValue, sel string) object.Object {
	val := goVal.Value
	// Allow field access on pointers to structs
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return e.newError(node.Pos(), "nil pointer dereference")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return e.newError(node.Pos(), "base of selector expression is not a Go struct or pointer to struct")
	}

	field := val.FieldByName(sel)
	if !field.IsValid() {
		return e.newError(node.Pos(), "undefined field '%s' on Go struct %s", sel, val.Type())
	}
	if !field.CanInterface() {
		return e.newError(node.Pos(), "cannot access unexported field '%s' on Go struct %s", sel, val.Type())
	}

	return e.nativeToValue(field)
}

func (e *Evaluator) evalCompositeLit(n *ast.CompositeLit, env *object.Environment) object.Object {
	switch typ := n.Type.(type) {
	case *ast.Ident:
		// This handles struct literals, e.g., MyStruct{...}
		defObj := e.Eval(typ, env)
		if isError(defObj) {
			return defObj
		}
		def, ok := defObj.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Type.Pos(), "not a struct type in composite literal")
		}
		return e.evalStructLiteral(n, def, env)

	case *ast.SelectorExpr:
		// This handles struct literals with imported types, e.g., pkg.MyStruct{...}
		defObj := e.Eval(typ, env)
		if isError(defObj) {
			return defObj
		}
		def, ok := defObj.(*object.StructDefinition)
		if !ok {
			return e.newError(n.Type.Pos(), "selector does not resolve to a struct type in composite literal")
		}
		return e.evalStructLiteral(n, def, env)

	case *ast.ArrayType:
		// This handles array and slice literals, e.g., []int{...}
		elements := e.evalExpressions(n.Elts, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &object.Array{Elements: elements}

	case *ast.MapType:
		// This handles map literals, e.g., map[string]int{...}
		return e.evalMapLiteral(n, env)

	default:
		return e.newError(n.Type.Pos(), "unsupported type in composite literal: %T", n.Type)
	}
}

func (e *Evaluator) evalStructLiteral(n *ast.CompositeLit, def *object.StructDefinition, env *object.Environment) object.Object {
	instance := &object.StructInstance{Def: def, Fields: make(map[string]object.Object)}
	for _, elt := range n.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return e.newError(elt.Pos(), "unsupported literal element in struct literal")
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return e.newError(kv.Key.Pos(), "field name is not an identifier")
		}
		value := e.Eval(kv.Value, env)
		if isError(value) {
			return value
		}
		instance.Fields[key.Name] = value
	}
	return instance
}

func (e *Evaluator) evalMapLiteral(n *ast.CompositeLit, env *object.Environment) object.Object {
	pairs := make(map[object.HashKey]object.MapPair)

	for _, elt := range n.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return e.newError(elt.Pos(), "non-key-value element in map literal")
		}

		key := e.Eval(kv.Key, env)
		if isError(key) {
			return key
		}

		hashable, ok := key.(object.Hashable)
		if !ok {
			return e.newError(kv.Key.Pos(), "unusable as map key: %s", key.Type())
		}

		value := e.Eval(kv.Value, env)
		if isError(value) {
			return value
		}

		hashed := hashable.HashKey()
		pairs[hashed] = object.MapPair{Key: key, Value: value}
	}

	return &object.Map{Pairs: pairs}
}

func (e *Evaluator) evalIdent(n *ast.Ident, env *object.Environment) object.Object {
	if val, ok := env.Get(n.Name); ok {
		return val
	}
	if builtin, ok := builtins[n.Name]; ok {
		return builtin
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
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(n.Pos(), "could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	default:
		return e.newError(n.Pos(), "unsupported literal type: %s", n.Kind)
	}
}
