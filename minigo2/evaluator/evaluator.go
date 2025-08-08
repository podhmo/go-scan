package evaluator

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/podhmo/go-scan/minigo2/object"
)

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	fset      *token.FileSet
	callStack []object.CallFrame
}

// New creates a new Evaluator.
func New(fset *token.FileSet) *Evaluator {
	return &Evaluator{
		fset:      fset,
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
	case object.NULL:
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

// evalStringInfixExpression evaluates infix expressions for strings.
func (e *Evaluator) evalStringInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	if operator != "+" {
		return e.newError(node.Pos(), "unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}

	return &object.String{Value: leftVal + rightVal}
}

// evalInfixExpression dispatches to the correct infix evaluation function based on type.
func (e *Evaluator) evalInfixExpression(node ast.Node, operator string, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return e.evalIntegerInfixExpression(node, operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return e.evalStringInfixExpression(node, operator, left, right)
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
	switch obj {
	case object.NULL, object.FALSE:
		return false
	default:
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
		return object.NULL
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

	return object.NULL
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

	return object.NULL
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
		if len(fn.Parameters) != len(args) {
			return e.newError(call.Pos(), "wrong number of arguments. got=%d, want=%d", len(args), len(fn.Parameters))
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

	default:
		return e.newError(call.Pos(), "not a function: %s", fn.Type())
	}
}

func (e *Evaluator) extendFunctionEnv(fn *object.Function, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(fn.Env)
	for paramIdx, param := range fn.Parameters {
		env.Set(param.Name, args[paramIdx])
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
	case *ast.BranchStmt:
		return e.evalBranchStmt(n, env)
	case *ast.DeclStmt:
		return e.Eval(n.Decl, env)
	case *ast.FuncDecl:
		params := []*ast.Ident{}
		if n.Type.Params != nil {
			for _, field := range n.Type.Params.List {
				for _, name := range field.Names {
					params = append(params, name)
				}
			}
		}
		fn := &object.Function{Name: n.Name, Parameters: params, Body: n.Body, Env: env}
		env.Set(n.Name.Name, fn)
		return nil
	case *ast.ReturnStmt:
		var val object.Object = object.NULL
		if len(n.Results) > 0 {
			val = e.Eval(n.Results[0], env)
			if isError(val) {
				return val
			}
		}
		return &object.ReturnValue{Value: val}
	case *ast.GenDecl:
		return e.evalGenDecl(n, env)
	case *ast.AssignStmt:
		return e.evalAssignStmt(n, env)

	// Expressions
	case *ast.ParenExpr:
		return e.Eval(n.X, env)
	case *ast.FuncLit:
		params := []*ast.Ident{}
		if n.Type.Params != nil {
			for _, field := range n.Type.Params.List {
				for _, name := range field.Names {
					params = append(params, name)
				}
			}
		}
		return &object.Function{Parameters: params, Body: n.Body, Env: env}
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
	case *ast.UnaryExpr:
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
	var lastValues []ast.Expr // For const value carry-over

	for iotaValue, spec := range n.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec: // var, const
			// Handle const value carry-over
			if n.Tok == token.CONST {
				if len(s.Values) == 0 {
					s.Values = lastValues
				} else {
					lastValues = s.Values
				}
			}

			for i, name := range s.Names {
				var val object.Object
				if i < len(s.Values) {
					// Create a temporary environment for iota evaluation.
					iotaEnv := object.NewEnclosedEnvironment(env)
					iotaEnv.SetConstant("iota", &object.Integer{Value: int64(iotaValue)})
					val = e.Eval(s.Values[i], iotaEnv)
				} else {
					// This should be handled by the parser, but as a safeguard.
					return e.newError(name.Pos(), "missing value in declaration")
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
			}
		case *ast.TypeSpec: // type
			structType, ok := s.Type.(*ast.StructType)
			if !ok {
				return e.newError(s.Pos(), "unsupported type declaration: not a struct")
			}
			def := &object.StructDefinition{
				Name:   s.Name,
				Fields: structType.Fields.List,
			}
			env.Set(s.Name.Name, def)
		}
	}
	return nil
}

func (e *Evaluator) evalAssignStmt(n *ast.AssignStmt, env *object.Environment) object.Object {
	val := e.Eval(n.Rhs[0], env)
	if isError(val) {
		return val
	}

	switch n.Tok {
	case token.ASSIGN:
		switch lhs := n.Lhs[0].(type) {
		case *ast.Ident:
			if _, ok := env.GetConstant(lhs.Name); ok {
				return e.newError(lhs.Pos(), "cannot assign to constant %s", lhs.Name)
			}
			if !env.Assign(lhs.Name, val) {
				return e.newError(lhs.Pos(), "undeclared variable: %s", lhs.Name)
			}
		case *ast.SelectorExpr:
			obj := e.Eval(lhs.X, env)
			if isError(obj) {
				return obj
			}
			instance, ok := obj.(*object.StructInstance)
			if !ok {
				return e.newError(lhs.Pos(), "assignment to non-struct field")
			}
			instance.Fields[lhs.Sel.Name] = val
		default:
			return e.newError(n.Pos(), "unsupported assignment target")
		}
	case token.DEFINE:
		ident, ok := n.Lhs[0].(*ast.Ident)
		if !ok {
			return e.newError(n.Pos(), "non-identifier on left side of :=")
		}
		if fn, ok := val.(*object.Function); ok {
			fn.Name = ident
		}
		env.Set(ident.Name, val)
	}
	return val
}

func (e *Evaluator) evalSelectorExpr(n *ast.SelectorExpr, env *object.Environment) object.Object {
	left := e.Eval(n.X, env)
	if isError(left) {
		return left
	}

	instance, ok := left.(*object.StructInstance)
	if !ok {
		return e.newError(n.Pos(), "base of selector expression is not a struct")
	}

	if val, ok := instance.Fields[n.Sel.Name]; ok {
		return val
	}

	return e.newError(n.Sel.Pos(), "undefined field '%s' on struct", n.Sel.Name)
}

func (e *Evaluator) evalCompositeLit(n *ast.CompositeLit, env *object.Environment) object.Object {
	defObj := e.Eval(n.Type, env)
	if isError(defObj) {
		return defObj
	}
	def, ok := defObj.(*object.StructDefinition)
	if !ok {
		return e.newError(n.Type.Pos(), "not a struct type in composite literal")
	}

	instance := &object.StructInstance{Def: def, Fields: make(map[string]object.Object)}
	for _, elt := range n.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return e.newError(elt.Pos(), "unsupported literal element")
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

func (e *Evaluator) evalIdent(n *ast.Ident, env *object.Environment) object.Object {
	if val, ok := env.Get(n.Name); ok {
		return val
	}
	switch n.Name {
	case "true":
		return object.TRUE
	case "false":
		return object.FALSE
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
