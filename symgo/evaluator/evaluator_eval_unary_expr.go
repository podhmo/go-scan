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
