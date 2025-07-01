package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	// "github.com/go-scan/go-scan/scanner" // For more detailed error reporting later
)

// parseInt64 is a helper function to parse a string to an int64.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 0, 64)
}

// Interpreter holds the state of the interpreter
type Interpreter struct {
	globalEnv *Environment // Global environment
}

// NewInterpreter creates a new Interpreter with a global environment.
func NewInterpreter() *Interpreter {
	env := NewEnvironment(nil)
	// Register built-in functions
	for name, builtin := range Builtins {
		env.Define(name, builtin)
	}
	return &Interpreter{
		globalEnv: env,
	}
}

// LoadAndRun loads a Go source file, parses it, and runs the specified entry point function.
func (i *Interpreter) LoadAndRun(filename string, entryPoint string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("error parsing file %s: %w", filename, err)
	}

	for _, declNode := range node.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, err := i.eval(tempDeclStmt, i.globalEnv)
			if err != nil {
				return fmt.Errorf("error evaluating global variable declaration at %d: %w", genDecl.Pos(), err)
			}
		}
	}

	entryFunc := findFunction(node, entryPoint)
	if entryFunc == nil {
		return fmt.Errorf("entry point function '%s' not found in %s", entryPoint, filename)
	}

	funcEnv := NewEnvironment(i.globalEnv)
	fmt.Printf("Executing function: %s\n", entryPoint)
	_, evalErr := i.evalBlockStatement(entryFunc.Body, funcEnv)
	return evalErr
}

func findFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Name.Name == name {
				return fn
			}
		}
	}
	return nil
}

func (i *Interpreter) eval(node ast.Node, env *Environment) (Object, error) {
	switch n := node.(type) {
	case *ast.File:
		var result Object
		var err error
		for _, decl := range n.Decls {
			if fnDecl, ok := decl.(*ast.FuncDecl); ok && fnDecl.Name.Name == "main" {
				result, err = i.evalBlockStatement(fnDecl.Body, env)
				if err != nil {
					return nil, fmt.Errorf("error evaluating main function in file: %w", err)
				}
			}
		}
		return result, nil
	case *ast.BlockStmt:
		return i.evalBlockStatement(n, env)
	case *ast.ExprStmt:
		return i.eval(n.X, env)
	case *ast.Ident:
		return evalIdentifier(n, env)
	case *ast.BasicLit:
		switch n.Kind {
		case token.STRING:
			// n.Value from ast.BasicLit includes the surrounding quotes and is source-escaped.
			// strconv.Unquote handles this correctly.
			unquotedVal, err := strconv.Unquote(n.Value)
			if err != nil {
				return nil, fmt.Errorf("error unquoting string literal %s: %w", n.Value, err)
			}
			return &String{Value: unquotedVal}, nil
		case token.INT:
			// n.Value for integers is the number as a string, e.g., "123", "0xFF"
			val, err := parseInt64(n.Value) // parseInt64 handles base detection
			if err != nil {
				return nil, fmt.Errorf("error parsing integer literal %s: %w", n.Value, err)
			}
			return &Integer{Value: val}, nil
		default:
			return nil, fmt.Errorf("unsupported literal type: %s (value: %s)", n.Kind, n.Value)
		}
	case *ast.DeclStmt:
		return i.evalDeclStmt(n, env)
	case *ast.BinaryExpr:
		return i.evalBinaryExpr(n, env)
	case *ast.UnaryExpr:
		return i.evalUnaryExpr(n, env)
	case *ast.ParenExpr:
		return i.eval(n.X, env)
	case *ast.IfStmt:
		return i.evalIfStmt(n, env)
	case *ast.AssignStmt:
		return i.evalAssignStmt(n, env)
	case *ast.CallExpr: // Added case for CallExpr
		return i.evalCallExpr(n, env)
	case *ast.CompositeLit:
		return i.evalCompositeLit(n, env)
	default:
		return nil, fmt.Errorf("unsupported AST node type: %T at %d", n, n.Pos())
	}
}

// evalCompositeLit evaluates a composite literal, e.g., []string{"a", "b"}
func (i *Interpreter) evalCompositeLit(node *ast.CompositeLit, env *Environment) (Object, error) {
	// For now, we only support array literals like []string{"a", "b"}
	// We need to check node.Type to ensure it's an array type we support.
	// Example: ast.ArrayType{ Elt: ast.Ident{Name: "string"} }
	// This part can be expanded later for other composite literals (structs, maps).

	arrayType, ok := node.Type.(*ast.ArrayType)
	if !ok {
		// Only supporting array literals directly for now, not map or struct literals.
		// Also, not handling array literals like [...]int{1,2,3} which have a fixed size from an expression.
		return nil, fmt.Errorf("unsupported composite literal type: expected array type, got %T at %d", node.Type, node.Type.Pos())
	}

	// Check the element type of the array. For now, let's be specific for `[]string`.
	// A more general approach would map type names to our ObjectTypes.
	if ident, ok := arrayType.Elt.(*ast.Ident); !ok || ident.Name != "string" {
		// TODO: Support other array types like []int, []bool later.
		elementTypeString := "unknown"
		if typeIdent, isIdent := arrayType.Elt.(*ast.Ident); isIdent {
			elementTypeString = typeIdent.Name
		}
		return nil, fmt.Errorf("unsupported array element type: only 'string' is currently supported, got '%s' at %d",
			elementTypeString, arrayType.Elt.Pos())
	}

	elements, err := i.evalExpressions(node.Elts, env)
	if err != nil {
		return nil, err // Error evaluating one of the elements
	}

	// Ensure all evaluated elements are of the correct type (String for []string)
	for _, el := range elements {
		if _, ok := el.(*String); !ok {
			return nil, fmt.Errorf("array literal element type mismatch: expected STRING, got %s for element '%s' at %d",
				el.Type(), el.Inspect(), node.Pos()) // Position might need refinement to point to specific element
		}
	}

	return &Array{Elements: elements}, nil
}


func (i *Interpreter) evalBlockStatement(block *ast.BlockStmt, env *Environment) (Object, error) {
	var result Object
	var err error
	for _, stmt := range block.List {
		result, err = i.eval(stmt, env)
		if err != nil {
			return nil, err
		}
		// TODO: Handle ReturnValue
	}
	return result, nil
}

func (i *Interpreter) evalDeclStmt(declStmt *ast.DeclStmt, env *Environment) (Object, error) {
	genDecl, ok := declStmt.Decl.(*ast.GenDecl)
	if !ok {
		return nil, fmt.Errorf("unsupported declaration type: %T at %d", declStmt.Decl, declStmt.Pos())
	}
	if genDecl.Tok != token.VAR {
		return nil, fmt.Errorf("unsupported declaration token: %s (expected VAR) at %d", genDecl.Tok, genDecl.Pos())
	}
	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			return nil, fmt.Errorf("unsupported spec type in var declaration: %T at %d", spec, spec.Pos())
		}
		for idx, nameIdent := range valueSpec.Names {
			varName := nameIdent.Name
			if len(valueSpec.Values) > idx {
				val, err := i.eval(valueSpec.Values[idx], env)
				if err != nil {
					return nil, fmt.Errorf("error evaluating value for var %s: %w", varName, err)
				}
				env.Define(varName, val)
			} else {
				if valueSpec.Type == nil {
					return nil, fmt.Errorf("variable '%s' declared without initializer must have a type at %d", varName, valueSpec.Pos())
				}
				var zeroVal Object
				typeIdent, okType := valueSpec.Type.(*ast.Ident)
				if !okType {
					return nil, fmt.Errorf("unsupported type expression for zero value for variable '%s': %T at %d", varName, valueSpec.Type, valueSpec.Type.Pos())
				}
				switch typeIdent.Name {
				case "string":
					zeroVal = &String{Value: ""}
				case "int":
					zeroVal = &Integer{Value: 0}
				case "bool":
					zeroVal = FALSE
				default:
					return nil, fmt.Errorf("unsupported type '%s' for uninitialized variable '%s' at %d", typeIdent.Name, varName, typeIdent.Pos())
				}
				env.Define(varName, zeroVal)
			}
		}
	}
	return nil, nil
}

func evalIdentifier(ident *ast.Ident, env *Environment) (Object, error) {
	switch ident.Name {
	case "true":
		return TRUE, nil
	case "false":
		return FALSE, nil
	}
	if val, ok := env.Get(ident.Name); ok {
		return val, nil
	}
	if builtin := GetBuiltinByName(ident.Name); builtin != nil {
		return builtin, nil
	}
	return nil, fmt.Errorf("identifier not found: %s at %d", ident.Name, ident.Pos())
}

func (i *Interpreter) evalBinaryExpr(node *ast.BinaryExpr, env *Environment) (Object, error) {
	left, err := i.eval(node.X, env)
	if err != nil {
		return nil, err
	}
	right, err := i.eval(node.Y, env)
	if err != nil {
		return nil, err
	}
	switch {
	case left.Type() == STRING_OBJ && right.Type() == STRING_OBJ:
		return evalStringBinaryExpr(node.Op, left.(*String), right.(*String))
	case left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ:
		return evalIntegerBinaryExpr(node.Op, left.(*Integer), right.(*Integer), node.Pos())
	case left.Type() == BOOLEAN_OBJ && right.Type() == BOOLEAN_OBJ:
		if node.Op == token.EQL || node.Op == token.NEQ {
			return evalBooleanBinaryExpr(node.Op, left.(*Boolean), right.(*Boolean), node.Pos())
		}
		return nil, fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s at %d", left.Type(), node.Op, right.Type(), node.Pos())
	default:
		return nil, fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s at %d", left.Type(), node.Op, right.Type(), node.Pos())
	}
}

func evalIntegerBinaryExpr(op token.Token, left, right *Integer, pos token.Pos) (Object, error) {
	lVal, rVal := left.Value, right.Value
	switch op {
	case token.ADD:
		return &Integer{Value: lVal + rVal}, nil
	case token.SUB:
		return &Integer{Value: lVal - rVal}, nil
	case token.MUL:
		return &Integer{Value: lVal * rVal}, nil
	case token.QUO:
		if rVal == 0 {
			return nil, fmt.Errorf("division by zero at %d", pos)
		}
		return &Integer{Value: lVal / rVal}, nil
	case token.REM:
		if rVal == 0 {
			return nil, fmt.Errorf("division by zero (modulo) at %d", pos)
		}
		return &Integer{Value: lVal % rVal}, nil
	case token.EQL:
		return nativeBoolToBooleanObject(lVal == rVal), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(lVal != rVal), nil
	case token.LSS:
		return nativeBoolToBooleanObject(lVal < rVal), nil
	case token.LEQ:
		return nativeBoolToBooleanObject(lVal <= rVal), nil
	case token.GTR:
		return nativeBoolToBooleanObject(lVal > rVal), nil
	case token.GEQ:
		return nativeBoolToBooleanObject(lVal >= rVal), nil
	default:
		return nil, fmt.Errorf("unknown operator for integers: %s at %d", op, pos)
	}
}

func evalStringBinaryExpr(op token.Token, left, right *String) (Object, error) {
	switch op {
	case token.ADD:
		return &String{Value: left.Value + right.Value}, nil
	case token.EQL:
		return nativeBoolToBooleanObject(left.Value == right.Value), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(left.Value != right.Value), nil
	default:
		return nil, fmt.Errorf("unknown operator for strings: %s", op)
	}
}

func evalBooleanBinaryExpr(op token.Token, left, right *Boolean, pos token.Pos) (Object, error) {
	lVal, rVal := left.Value, right.Value
	switch op {
	case token.EQL:
		return nativeBoolToBooleanObject(lVal == rVal), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(lVal != rVal), nil
	default:
		return nil, fmt.Errorf("unknown operator for booleans: %s at %d", op, pos)
	}
}

func (i *Interpreter) evalAssignStmt(assignStmt *ast.AssignStmt, env *Environment) (Object, error) {
	// For now, only support simple single assignment: ident = value or ident := value
	if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
		return nil, fmt.Errorf("unsupported assignment: expected 1 expression on LHS and 1 on RHS, got %d and %d at %d",
			len(assignStmt.Lhs), len(assignStmt.Rhs), assignStmt.Pos())
	}

	lhs := assignStmt.Lhs[0]
	ident, ok := lhs.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("unsupported assignment LHS: expected identifier, got %T at %d", lhs, lhs.Pos())
	}
	varName := ident.Name

	if assignStmt.Tok == token.DEFINE { // Short variable declaration: x := val
		val, err := i.eval(assignStmt.Rhs[0], env)
		if err != nil {
			return nil, err
		}
		env.Define(varName, val)
		return nil, nil
	}

	// Regular assignment (=) or augmented assignments (+=, -=, etc.)
	rhsVal, err := i.eval(assignStmt.Rhs[0], env)
	if err != nil {
		return nil, err
	}

	finalValToAssign := rhsVal // Default for token.ASSIGN

	if assignStmt.Tok != token.ASSIGN { // Augmented assignment logic
		existingVal, موجود := env.Get(varName)
		if !موجود {
			return nil, fmt.Errorf("cannot use %s on undeclared variable '%s' at %d", assignStmt.Tok.String(), varName, ident.Pos())
		}

		var op token.Token
		switch assignStmt.Tok {
		case token.ADD_ASSIGN: op = token.ADD
		case token.SUB_ASSIGN: op = token.SUB
		case token.MUL_ASSIGN: op = token.MUL
		case token.QUO_ASSIGN: op = token.QUO
		case token.REM_ASSIGN: op = token.REM
		default:
			return nil, fmt.Errorf("internal error: unsupported augmented assignment token %s at %d", assignStmt.Tok, assignStmt.Pos())
		}

		// Perform the operation existingVal op rhsVal
		if existingInt, okE := existingVal.(*Integer); okE {
			if valInt, okV := rhsVal.(*Integer); okV {
				l, r := existingInt.Value, valInt.Value
				switch op {
				case token.ADD: finalValToAssign = &Integer{Value: l + r}
				case token.SUB: finalValToAssign = &Integer{Value: l - r}
				case token.MUL: finalValToAssign = &Integer{Value: l * r}
				case token.QUO:
					if r == 0 { return nil, fmt.Errorf("division by zero in %s at %d", assignStmt.Tok.String(), assignStmt.Pos()) }
					finalValToAssign = &Integer{Value: l / r}
				case token.REM:
					if r == 0 { return nil, fmt.Errorf("division by zero (modulo) in %s at %d", assignStmt.Tok.String(), assignStmt.Pos()) }
					finalValToAssign = &Integer{Value: l % r}
				// Default case for op not needed here as it's derived from assignStmt.Tok
				}
			} else {
				return nil, fmt.Errorf("type mismatch for augmented assignment: LHS is INTEGER, RHS is %s for var '%s' at %d", rhsVal.Type(), varName, assignStmt.Pos())
			}
		} else if existingString, okE := existingVal.(*String); okE && op == token.ADD { // String concatenation +=
			if valString, okV := rhsVal.(*String); okV {
				finalValToAssign = &String{Value: existingString.Value + valString.Value}
			} else {
				return nil, fmt.Errorf("type mismatch for string concatenation (+=): LHS is STRING, RHS is %s for var '%s' at %d", rhsVal.Type(), varName, assignStmt.Pos())
			}
		} else {
			return nil, fmt.Errorf("unsupported type %s for augmented assignment operator %s on var '%s' at %d", existingVal.Type(), assignStmt.Tok.String(), varName, assignStmt.Pos())
		}
	} // End augmented assignment logic

	// Assign the final value (either from simple '=' or computed from augmented)
	if _, okSet := env.Assign(varName, finalValToAssign); !okSet {
		return nil, fmt.Errorf("cannot assign to undeclared variable '%s' at %d", varName, ident.Pos())
	}

	return nil, nil // Assignment statement itself does not produce a value
}

func (i *Interpreter) evalIfStmt(ifStmt *ast.IfStmt, env *Environment) (Object, error) {
	condition, err := i.eval(ifStmt.Cond, env)
	if err != nil {
		return nil, err
	}
	boolCond, ok := condition.(*Boolean)
	if !ok {
		return nil, fmt.Errorf("condition for if statement must be boolean, got %s (type: %s) at %d", condition.Inspect(), condition.Type(), ifStmt.Cond.Pos())
	}
	if boolCond.Value {
		return i.evalBlockStatement(ifStmt.Body, env)
	} else if ifStmt.Else != nil {
		return i.eval(ifStmt.Else, env)
	}
	return nil, nil
}

func (i *Interpreter) evalUnaryExpr(node *ast.UnaryExpr, env *Environment) (Object, error) {
	operand, err := i.eval(node.X, env)
	if err != nil {
		return nil, err
	}
	switch node.Op {
	case token.SUB: // Negation -
		if operand.Type() == INTEGER_OBJ {
			return &Integer{Value: -operand.(*Integer).Value}, nil
		}
		return nil, fmt.Errorf("unsupported type for negation: %s at %d", operand.Type(), node.Pos())
	case token.NOT: // Logical not !
		switch operand {
		case TRUE: return FALSE, nil
		case FALSE: return TRUE, nil
		default:
			return nil, fmt.Errorf("unsupported type for logical NOT: %s at %d", operand.Type(), node.Pos())
		}
	default:
		return nil, fmt.Errorf("unsupported unary operator: %s at %d", node.Op, node.Pos())
	}
}

// evalCallExpr evaluates a function call expression.
func (i *Interpreter) evalCallExpr(node *ast.CallExpr, env *Environment) (Object, error) {
	fnObj, err := i.eval(node.Fun, env)
	if err != nil {
		return nil, err
	}

	args, err := i.evalExpressions(node.Args, env)
	if err != nil {
		return nil, err
	}

	return i.applyFunction(fnObj, args, node.Fun.Pos())
}

// evalExpressions evaluates a slice of expressions.
func (i *Interpreter) evalExpressions(exprs []ast.Expr, env *Environment) ([]Object, error) {
	var result []Object
	for _, e := range exprs {
		evaluated, err := i.eval(e, env)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluated)
	}
	return result, nil
}

// applyFunction applies a function object (either user-defined or builtin) to a list of arguments.
func (i *Interpreter) applyFunction(fn Object, args []Object, callPos token.Pos) (Object, error) {
	switch fn := fn.(type) {
	case *Builtin:
		result, err := fn.Fn(args...)
		if err != nil {
			return nil, fmt.Errorf("error calling builtin '%s' at %d: %w", fn.Name, callPos, err)
		}
		return result, nil
	// TODO: Add case for *Function (user-defined functions)
	default:
		return nil, fmt.Errorf("not a function: %s (type: %s) at %d", fn.Inspect(), fn.Type(), callPos)
	}
}
