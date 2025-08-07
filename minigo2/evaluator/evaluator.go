package evaluator

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/podhmo/go-scan/minigo2/object"
)

// nativeBoolToBooleanObject is a helper to convert a native bool to our object.Boolean.
func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return object.TRUE
	}
	return object.FALSE
}

// evalBangOperatorExpression evaluates the '!' prefix expression.
func evalBangOperatorExpression(right object.Object) object.Object {
	switch right {
	case object.TRUE:
		return object.FALSE
	case object.FALSE:
		return object.TRUE
	case object.NULL:
		return object.TRUE
	default:
		return object.FALSE
	}
}

// evalMinusPrefixOperatorExpression evaluates the '-' prefix expression.
func evalMinusPrefixOperatorExpression(right object.Object) object.Object {
	if right.Type() != object.INTEGER_OBJ {
		return nil // Later, an error object
	}
	value := right.(*object.Integer).Value
	return &object.Integer{Value: -value}
}

// evalPrefixExpression dispatches to the correct prefix evaluation function.
func evalPrefixExpression(operator string, right object.Object) object.Object {
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	default:
		return nil // Later, an error object
	}
}

// evalIntegerInfixExpression evaluates infix expressions for integers.
func evalIntegerInfixExpression(operator string, left, right object.Object) object.Object {
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
		return &object.Integer{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return nil // Later, an error object
	}
}

// evalStringInfixExpression evaluates infix expressions for strings.
func evalStringInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	if operator != "+" {
		return nil // Later, an error object
	}

	return &object.String{Value: leftVal + rightVal}
}

// evalInfixExpression dispatches to the correct infix evaluation function based on type.
func evalInfixExpression(operator string, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	// Pointer comparison for booleans, as they are singletons.
	case operator == "==":
		return nativeBoolToBooleanObject(left == right)
	case operator == "!=":
		return nativeBoolToBooleanObject(left != right)
	default:
		return nil // Later, an error object
	}
}

// evalBlockStatement evaluates a block of statements within a new scope.
func evalBlockStatement(block *ast.BlockStmt, env *object.Environment) object.Object {
	var result object.Object
	enclosedEnv := object.NewEnclosedEnvironment(env)

	for _, statement := range block.List {
		result = Eval(statement, enclosedEnv)
		// NOTE: Later we would handle return values, errors, etc. here
	}

	return result
}

// Eval is the central function of the evaluator. It traverses the AST
// and returns the result of the evaluation as an object.Object.
func Eval(node ast.Node, env *object.Environment) object.Object {
	switch n := node.(type) {
	// Statements
	case *ast.BlockStmt:
		return evalBlockStatement(n, env)
	case *ast.ExprStmt:
		// For an expression statement, we evaluate the underlying expression.
		return Eval(n.X, env)
	case *ast.DeclStmt:
		return Eval(n.Decl, env)
	case *ast.GenDecl:
		if n.Tok == token.VAR {
			for _, spec := range n.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i, name := range valueSpec.Names {
					// Assuming `var x = val` format
					val := Eval(valueSpec.Values[i], env)
					env.Set(name.Name, val)
				}
			}
		}
		return nil // var declaration is a statement
	case *ast.AssignStmt:
		switch n.Tok {
		case token.ASSIGN: // x = y
			// Assuming single assignment for now: `x = val`
			val := Eval(n.Rhs[0], env)
			// TODO: Check for error object from val

			ident, ok := n.Lhs[0].(*ast.Ident)
			if !ok {
				// TODO: Return error, not supported assignment target
				return nil
			}

			if !env.Assign(ident.Name, val) {
				// TODO: Return error, undeclared variable
				return nil
			}
			return val // Assignment can be an expression

		case token.DEFINE: // x := y
			// Assuming single assignment for now: `x := val`
			val := Eval(n.Rhs[0], env)
			// TODO: Check for error object from val

			ident, ok := n.Lhs[0].(*ast.Ident)
			if !ok {
				// TODO: Return error, not supported assignment target
				return nil
			}

			env.Set(ident.Name, val)
			return val
		}

	// Expressions
	case *ast.ParenExpr:
		return Eval(n.X, env)
	case *ast.UnaryExpr:
		right := Eval(n.X, env)
		return evalPrefixExpression(n.Op.String(), right)
	case *ast.BinaryExpr:
		left := Eval(n.X, env)
		right := Eval(n.Y, env)
		return evalInfixExpression(n.Op.String(), left, right)

	// Literals
	case *ast.Ident:
		if val, ok := env.Get(n.Name); ok {
			return val
		}

		switch n.Name {
		case "true":
			return object.TRUE
		case "false":
			return object.FALSE
		}
	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			i, err := strconv.ParseInt(n.Value, 0, 64)
			if err != nil {
				// In a real interpreter, we'd return an error object.
				// For now, we return nil.
				return nil
			}
			return &object.Integer{Value: i}
		case token.STRING:
			s, err := strconv.Unquote(n.Value)
			if err != nil {
				// Return nil on error for now.
				return nil
			}
			return &object.String{Value: s}
		}
	}

	return nil
}
