package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalUnaryExpr(ctx context.Context, node *ast.UnaryExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	rightObj := e.Eval(ctx, node.X, env, pkg)
	if isError(rightObj) {
		return rightObj
	}

	// If the expression was a function call, unwrap the return value.
	if ret, ok := rightObj.(*object.ReturnValue); ok {
		rightObj = ret.Value
	}

	// For most unary operations, we need the concrete value.
	// But for the address-of operator (&), we must NOT evaluate, because we need
	// the variable/expression itself, not its value.
	var right object.Object
	if node.Op == token.AND {
		right = rightObj
	} else {
		right = e.forceEval(ctx, rightObj, pkg)
		if isError(right) {
			return right
		}
	}

	switch node.Op {
	case token.NOT:
		return e.evalBangOperatorExpression(right)
	case token.SUB, token.ADD, token.XOR:
		return e.evalNumericUnaryExpression(ctx, node.Op, right)
	case token.AND:
		// This is the address-of operator, not a typical unary op on a value.
		// It needs to be handled specially as it operates on identifiers/expressions, not resolved objects.
		// Re-evaluating node.X might be redundant but safer.
		val := e.Eval(ctx, node.X, env, pkg)
		if isError(val) {
			return val
		}
		ptr := &object.Pointer{Value: val}
		if originalFieldType := val.FieldType(); originalFieldType != nil {
			pointerFieldType := &scan.FieldType{
				IsPointer: true,
				Elem:      originalFieldType,
			}
			ptr.SetFieldType(pointerFieldType)
		}
		ptr.SetTypeInfo(val.TypeInfo())
		return ptr
	case token.ARROW: // <-
		// Channel receive `<-ch`
		chObj := e.Eval(ctx, node.X, env, pkg)
		if isError(chObj) {
			return chObj
		}

		// Unwrap if it's a variable
		if v, ok := chObj.(*object.Variable); ok {
			chObj = v.Value
		}

		if ch, ok := chObj.(*object.Channel); ok {
			if ch.ChanFieldType != nil && ch.ChanFieldType.Elem != nil {
				elemFieldType := ch.ChanFieldType.Elem
				resolvedType := e.resolver.ResolveType(ctx, elemFieldType)
				placeholder := &object.SymbolicPlaceholder{
					Reason: fmt.Sprintf("value received from channel of type %s", ch.ChanFieldType.String()),
				}
				placeholder.SetFieldType(elemFieldType)
				placeholder.SetTypeInfo(resolvedType)
				return placeholder
			}
		}
		// Fallback for untyped or non-channel objects
		return &object.SymbolicPlaceholder{Reason: "value received from non-channel or untyped object"}
	default:
		return e.newError(ctx, node.Pos(), "unknown unary operator: %s", node.Op)
	}
}

func (e *Evaluator) evalBangOperatorExpression(right object.Object) object.Object {
	// If the operand is a symbolic placeholder, the result is also a symbolic placeholder.
	if _, ok := right.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "result of ! on symbolic value"}
	}

	switch right {
	case object.TRUE:
		return object.FALSE
	case object.FALSE:
		return object.TRUE
	case object.NIL:
		return object.TRUE
	default:
		// In Go, `!` is only for booleans. For symbolic execution,
		// we might encounter other types. We'll treat them as "truthy"
		// (so !non-boolean is false), which is a common scripty behavior,
		// but a more rigorous implementation might error here.
		return object.FALSE
	}
}

func (e *Evaluator) evalNumericUnaryExpression(ctx context.Context, op token.Token, right object.Object) object.Object {
	// If the operand is a symbolic placeholder, the result is also a symbolic placeholder.
	if _, ok := right.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of unary operator %s on symbolic value", op)}
	}

	switch val := right.(type) {
	case *object.Integer:
		switch op {
		case token.SUB:
			return &object.Integer{Value: -val.Value}
		case token.ADD:
			return &object.Integer{Value: val.Value} // Unary plus is a no-op.
		case token.XOR:
			return &object.Integer{Value: ^val.Value} // Bitwise NOT.
		default:
			return e.newError(ctx, token.NoPos, "unhandled numeric unary operator for INTEGER: %s", op)
		}
	case *object.Float:
		switch op {
		case token.SUB:
			return &object.Float{Value: -val.Value}
		case token.ADD:
			return &object.Float{Value: val.Value}
		default:
			return e.newError(ctx, token.NoPos, "unhandled numeric unary operator for FLOAT: %s", op)
		}
	case *object.Complex:
		switch op {
		case token.SUB:
			return &object.Complex{Value: -val.Value}
		case token.ADD:
			return &object.Complex{Value: val.Value}
		default:
			return e.newError(ctx, token.NoPos, "unhandled numeric unary operator for COMPLEX: %s", op)
		}
	default:
		return e.newError(ctx, token.NoPos, "unary operator %s not supported for type %s", op, right.Type())
	}
}
