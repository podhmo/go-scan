package evaluator

import (
	"context"
	"go/ast"
	"go/token"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalBinaryExpr(ctx context.Context, node *ast.BinaryExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	leftObj := e.Eval(ctx, node.X, env, pkg)
	if isError(leftObj) {
		return leftObj
	}
	rightObj := e.Eval(ctx, node.Y, env, pkg)
	if isError(rightObj) {
		return rightObj
	}

	left := e.forceEval(ctx, leftObj, pkg)
	if isError(left) {
		return left
	}
	right := e.forceEval(ctx, rightObj, pkg)
	if isError(right) {
		return right
	}

	lType := left.Type()
	rType := right.Type()

	// If the expression was a function call, unwrap the return value.
	if l, ok := left.(*object.ReturnValue); ok {
		left = l.Value
	}
	if r, ok := right.(*object.ReturnValue); ok {
		right = r.Value
	}

	switch {
	case lType == object.INTEGER_OBJ && rType == object.INTEGER_OBJ:
		return e.evalIntegerInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.STRING_OBJ && rType == object.STRING_OBJ:
		return e.evalStringInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.COMPLEX_OBJ || rType == object.COMPLEX_OBJ:
		return e.evalComplexInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.FLOAT_OBJ || rType == object.FLOAT_OBJ:
		return e.evalFloatInfixExpression(ctx, node.Pos(), node.Op, left, right)
	default:
		return &object.SymbolicPlaceholder{Reason: "binary expression"}
	}
}

func (e *Evaluator) evalFloatInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
	if _, ok := left.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "float operation with symbolic operand"}
	}
	if _, ok := right.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "float operation with symbolic operand"}
	}

	var lval, rval float64

	switch l := left.(type) {
	case *object.Float:
		lval = l.Value
	case *object.Integer:
		lval = float64(l.Value)
	default:
		return e.newError(ctx, pos, "invalid left operand for float expression: %s", left.Type())
	}

	switch r := right.(type) {
	case *object.Float:
		rval = r.Value
	case *object.Integer:
		rval = float64(r.Value)
	default:
		return e.newError(ctx, pos, "invalid right operand for float expression: %s", right.Type())
	}

	switch op {
	case token.ADD:
		return &object.Float{Value: lval + rval}
	case token.SUB:
		return &object.Float{Value: lval - rval}
	case token.MUL:
		return &object.Float{Value: lval * rval}
	case token.QUO:
		return &object.Float{Value: lval / rval}
	case token.EQL:
		return nativeBoolToBooleanObject(lval == rval)
	case token.NEQ:
		return nativeBoolToBooleanObject(lval != rval)
	case token.LSS:
		return nativeBoolToBooleanObject(lval < rval)
	case token.LEQ:
		return nativeBoolToBooleanObject(lval <= rval)
	case token.GTR:
		return nativeBoolToBooleanObject(lval > rval)
	case token.GEQ:
		return nativeBoolToBooleanObject(lval >= rval)
	default:
		return e.newError(ctx, pos, "unknown float operator: %s", op)
	}
}

func (e *Evaluator) evalComplexInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
	if _, ok := left.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "complex operation with symbolic operand"}
	}
	if _, ok := right.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "complex operation with symbolic operand"}
	}

	var lval, rval complex128

	switch l := left.(type) {
	case *object.Complex:
		lval = l.Value
	case *object.Float:
		lval = complex(l.Value, 0)
	case *object.Integer:
		lval = complex(float64(l.Value), 0)
	default:
		return e.newError(ctx, pos, "invalid left operand for complex expression: %s", left.Type())
	}

	switch r := right.(type) {
	case *object.Complex:
		rval = r.Value
	case *object.Float:
		rval = complex(r.Value, 0)
	case *object.Integer:
		rval = complex(float64(r.Value), 0)
	default:
		return e.newError(ctx, pos, "invalid right operand for complex expression: %s", right.Type())
	}

	switch op {
	case token.ADD:
		return &object.Complex{Value: lval + rval}
	case token.SUB:
		return &object.Complex{Value: lval - rval}
	case token.MUL:
		return &object.Complex{Value: lval * rval}
	case token.QUO:
		return &object.Complex{Value: lval / rval}
	case token.EQL:
		return nativeBoolToBooleanObject(lval == rval)
	case token.NEQ:
		return nativeBoolToBooleanObject(lval != rval)
	default:
		return e.newError(ctx, pos, "unknown complex operator: %s", op)
	}
}

func (e *Evaluator) evalIntegerInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value

	switch op {
	case token.ADD:
		return &object.Integer{Value: leftVal + rightVal}
	case token.SUB:
		return &object.Integer{Value: leftVal - rightVal}
	case token.MUL:
		return &object.Integer{Value: leftVal * rightVal}
	case token.QUO:
		if rightVal == 0 {
			return &object.SymbolicPlaceholder{Reason: "division by zero"}
		}
		return &object.Integer{Value: leftVal / rightVal}

	// Placeholders for operators that are not fully evaluated
	case token.REM: // %
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.SHL: // <<
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.SHR: // >>
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.AND: // &
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.OR: // |
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.XOR: // ^
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}

	case token.EQL: // ==
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case token.NEQ: // !=
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case token.LSS: // <
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case token.LEQ: // <=
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case token.GTR: // >
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case token.GEQ: // >=
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	default:
		return e.newError(ctx, pos, "unknown integer operator: %s", op)
	}
}

func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return object.TRUE
	}
	return object.FALSE
}

func (e *Evaluator) evalStringInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	switch op {
	case token.ADD:
		return &object.String{Value: leftVal + rightVal}
	case token.EQL:
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case token.NEQ:
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return e.newError(ctx, pos, "unknown string operator: %s", op)
	}
}
