package main

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	// "github.com/go-scan/go-scan/scanner" // For more detailed error reporting later
)

// formatErrorWithContext creates a detailed error message including file, line, column, and source code.
func formatErrorWithContext(fset *token.FileSet, pos token.Pos, originalErr error, customMsg string) error {
	baseErrMsg := ""
	if originalErr != nil {
		baseErrMsg = originalErr.Error()
	}

	if pos == token.NoPos {
		if customMsg != "" {
			if originalErr != nil {
				return fmt.Errorf("%s: %w", customMsg, originalErr)
			}
			return errors.New(customMsg)
		}
		if originalErr != nil { // Return original error if no custom message and no pos
			return originalErr
		}
		return errors.New("unknown error") // Should not happen if originalErr is always provided
	}

	position := fset.Position(pos)
	filename := position.Filename
	line := position.Line
	column := position.Column

	var sourceLine string
	file, err := os.Open(filename)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for i := 1; scanner.Scan(); i++ {
			if i == line {
				sourceLine = strings.TrimSpace(scanner.Text())
				break
			}
		}
		if err := scanner.Err(); err != nil {
			sourceLine = fmt.Sprintf("[Error reading source line: %v]", err)
		}
	} else {
		sourceLine = fmt.Sprintf("[Error opening source file: %v]", err)
	}

	detailMsg := fmt.Sprintf("Error in %s at line %d, column %d", filename, line, column)
	if customMsg != "" {
		detailMsg = fmt.Sprintf("%s: %s", customMsg, detailMsg)
	}

	if sourceLine != "" {
		if baseErrMsg != "" {
			return fmt.Errorf("%s\n  Source: %s\n  Details: %s", detailMsg, sourceLine, baseErrMsg)
		}
		return fmt.Errorf("%s\n  Source: %s", detailMsg, sourceLine)
	}

	if baseErrMsg != "" {
		return fmt.Errorf("%s\n  Details: %s", detailMsg, baseErrMsg)
	}
	return errors.New(detailMsg)
}

// parseInt64 is a helper function to parse a string to an int64.
// It's defined here to keep the main eval function cleaner.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 0, 64)
}

// Interpreter holds the state of the interpreter
type Interpreter struct {
	globalEnv *Environment
	FileSet   *token.FileSet
}

func NewInterpreter() *Interpreter {
	env := NewEnvironment(nil)
	i := &Interpreter{
		globalEnv: env,
		FileSet:   token.NewFileSet(),
	}

	builtins := GetBuiltinFmtFunctions()
	for name, builtin := range builtins {
		env.Define(name, builtin)
	}
	builtinsStrings := GetBuiltinStringsFunctions()
	for name, builtin := range builtinsStrings {
		env.Define(name, builtin)
	}
	return i
}

// LoadAndRun loads a Go source file, parses it, and runs the specified entry point function.
// Temporarily reverting parts of LoadAndRun to simplify debugging parameter issues.
func (i *Interpreter) LoadAndRun(filename string, entryPoint string) error {
	node, err := parser.ParseFile(i.FileSet, filename, nil, parser.ParseComments)
	if err != nil {
		return formatErrorWithContext(i.FileSet, token.NoPos, err, fmt.Sprintf("Error parsing file %s", filename))
	}

	// Process top-level declarations (functions and global vars)
	// Store all function declarations first
	for _, declNode := range node.Decls {
		if fnDecl, ok := declNode.(*ast.FuncDecl); ok {
			_, evalErr := i.evalFuncDecl(fnDecl, i.globalEnv)
			if evalErr != nil {
				return evalErr
			}
		}
	}
	// Then evaluate global variable declarations
	for _, declNode := range node.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				return formatErrorWithContext(i.FileSet, genDecl.Pos(), evalErr, "Error evaluating global variable declaration")
			}
		}
	}

	// Get the entry function *object* from the global environment
	entryFuncObj, ok := i.globalEnv.Get(entryPoint)
	if !ok {
	    return formatErrorWithContext(i.FileSet, token.NoPos, fmt.Errorf("entry point function '%s' not found in global environment", entryPoint), "Setup error")
	}

	userEntryFunc, ok := entryFuncObj.(*UserDefinedFunction)
	if !ok {
	    return formatErrorWithContext(i.FileSet, token.NoPos, fmt.Errorf("entry point '%s' is not a user-defined function (type: %s)", entryPoint, entryFuncObj.Type()), "Setup error")
	}

	fmt.Printf("Executing entry point function: %s\n", entryPoint)
	// For main/entry point, we assume no arguments are passed.
	result, errApply := i.applyUserDefinedFunction(userEntryFunc, []Object{}, token.NoPos)
	if errApply != nil {
		if errObj, isErrObj := errApply.(*Error); isErrObj {
			return fmt.Errorf("Runtime error in %s: %s", entryPoint, errObj.Message)
		}
		return errApply
	}

	if result != nil && result.Type() != NULL_OBJ {
		fmt.Printf("Entry point '%s' finished, result: %s\n", entryPoint, result.Inspect())
	} else {
		fmt.Printf("Entry point '%s' finished.\n", entryPoint)
	}
	return nil
}


func (i *Interpreter) applyUserDefinedFunction(fn *UserDefinedFunction, args []Object, callPos token.Pos) (Object, error) {
	if len(args) != len(fn.Parameters) {
		errMsg := fmt.Sprintf("wrong number of arguments for function %s: expected %d, got %d", fn.Name, len(fn.Parameters), len(args))
		return nil, formatErrorWithContext(i.FileSet, callPos, errors.New(errMsg), "Function call error")
	}

	funcEnv := NewEnvironment(fn.Env) // Closure: fn.Env is the lexical scope

	for idx, paramIdent := range fn.Parameters {
		funcEnv.Define(paramIdent.Name, args[idx])
	}

	evaluated, errEval := i.evalBlockStatement(fn.Body, funcEnv)
	if errEval != nil {
		return nil, errEval
	}

	if retVal, ok := evaluated.(*ReturnValue); ok {
		if retVal.Value == nil {
			return NULL, nil
		}
		return retVal.Value, nil
	}
	return NULL, nil
}

// findFunction is likely unused but kept for now.
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
					return nil, err
				}
			}
		}
		return result, nil

	case *ast.BlockStmt:
		return i.evalBlockStatement(n, env)

	case *ast.ExprStmt:
		return i.eval(n.X, env)

	case *ast.Ident:
		return evalIdentifier(n, env, i.FileSet)

	case *ast.BasicLit:
		switch n.Kind {
		case token.STRING:
			return &String{Value: n.Value[1 : len(n.Value)-1]}, nil
		case token.INT:
			val, err := parseInt64(n.Value)
			if err != nil {
				return nil, formatErrorWithContext(i.FileSet, n.Pos(), err, fmt.Sprintf("Could not parse integer literal '%s'", n.Value))
			}
			return &Integer{Value: val}, nil
		default:
			return nil, formatErrorWithContext(i.FileSet, n.Pos(), fmt.Errorf("unsupported literal type: %s", n.Kind), fmt.Sprintf("Unsupported literal value: %s", n.Value))
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

	case *ast.CallExpr:
		return i.evalCallExpr(n, env)

	case *ast.SelectorExpr:
		return i.evalSelectorExpr(n, env)

	case *ast.ReturnStmt:
		return i.evalReturnStmt(n, env)

	case *ast.FuncDecl:
		return i.evalFuncDecl(n, env)

	case *ast.FuncLit:
		return i.evalFuncLit(n, env)

	default:
		return nil, formatErrorWithContext(i.FileSet, n.Pos(), fmt.Errorf("unsupported AST node type: %T", n), fmt.Sprintf("Unsupported AST node value: %+v", n))
	}
}

func (i *Interpreter) evalSelectorExpr(node *ast.SelectorExpr, env *Environment) (Object, error) {
	xIdent, ok := node.X.(*ast.Ident)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, node.X.Pos(), fmt.Errorf("X is not an identifier, got %T", node.X), "Unsupported selector expression")
	}
	fullName := xIdent.Name + "." + node.Sel.Name
	if val, ok := env.Get(fullName); ok {
		return val, nil
	}
	return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("undefined selector: %s", fullName), "")
}

func (i *Interpreter) evalBlockStatement(block *ast.BlockStmt, env *Environment) (Object, error) {
	var result Object
	var err error

	for _, stmt := range block.List {
		result, err = i.eval(stmt, env)
		if err != nil {
			return nil, err
		}
		switch res := result.(type) {
		case *ReturnValue:
			return res, nil
		case *Error:
			return res, nil
		}
	}
	return result, nil
}

func (i *Interpreter) evalFuncDecl(fd *ast.FuncDecl, env *Environment) (Object, error) {
	params := []*ast.Ident{}
	if fd.Type.Params != nil && fd.Type.Params.List != nil {
		for _, field := range fd.Type.Params.List {
			if field.Names != nil {
				for _, name := range field.Names {
					params = append(params, name)
				}
			}
		}
	}

	function := &UserDefinedFunction{
		Name:       fd.Name.Name,
		Parameters: params,
		Body:       fd.Body,
		Env:        env,
	}

	if fd.Name != nil && fd.Name.Name != "" {
		env.Define(fd.Name.Name, function)
		return nil, nil
	}
	return nil, formatErrorWithContext(i.FileSet, fd.Pos(), fmt.Errorf("function declaration must have a name"), "")
}

func (i *Interpreter) evalFuncLit(fl *ast.FuncLit, env *Environment) (Object, error) {
	params := []*ast.Ident{}
	if fl.Type.Params != nil && fl.Type.Params.List != nil {
		for _, field := range fl.Type.Params.List {
			if field.Names != nil {
				for _, name := range field.Names {
					params = append(params, name)
				}
			}
		}
	}

	return &UserDefinedFunction{
		Name:       "",
		Parameters: params,
		Body:       fl.Body,
		Env:        env,
	}, nil
}

func (i *Interpreter) evalReturnStmt(rs *ast.ReturnStmt, env *Environment) (Object, error) {
	if len(rs.Results) == 0 {
		return &ReturnValue{Value: NULL}, nil
	}

	if len(rs.Results) > 1 {
		return nil, formatErrorWithContext(i.FileSet, rs.Pos(), fmt.Errorf("multiple return values not supported"), "")
	}

	val, err := i.eval(rs.Results[0], env)
	if err != nil {
		return nil, err
	}
	return &ReturnValue{Value: val}, nil
}

func (i *Interpreter) evalDeclStmt(declStmt *ast.DeclStmt, env *Environment) (Object, error) {
	genDecl, ok := declStmt.Decl.(*ast.GenDecl)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, declStmt.Pos(), fmt.Errorf("unsupported declaration type: %T", declStmt.Decl), "")
	}

	if genDecl.Tok != token.VAR {
		return nil, formatErrorWithContext(i.FileSet, genDecl.Pos(), fmt.Errorf("unsupported declaration token: %s (expected VAR)", genDecl.Tok), "")
	}

	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			return nil, formatErrorWithContext(i.FileSet, spec.Pos(), fmt.Errorf("unsupported spec type in var declaration: %T", spec), "")
		}

		for idx, nameIdent := range valueSpec.Names {
			varName := nameIdent.Name
			if len(valueSpec.Values) > idx {
				val, err := i.eval(valueSpec.Values[idx], env)
				if err != nil {
					return nil, err
				}
				env.Define(varName, val)
			} else {
				if valueSpec.Type == nil {
					return nil, formatErrorWithContext(i.FileSet, valueSpec.Pos(), fmt.Errorf("variable '%s' declared without initializer must have a type", varName), "")
				}

				var zeroVal Object
				switch T := valueSpec.Type.(type) {
				case *ast.Ident:
					switch T.Name {
					case "string":
						zeroVal = &String{Value: ""}
					case "int":
						zeroVal = &Integer{Value: 0}
					case "bool":
						zeroVal = FALSE
					default:
						return nil, formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported type '%s' for uninitialized variable '%s'", T.Name, varName), "")
					}
				case *ast.InterfaceType:
					if T.Methods == nil || len(T.Methods.List) == 0 {
						zeroVal = NULL
					} else {
						return nil, formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported specific interface type for uninitialized variable '%s'", varName), "")
					}
				default:
					return nil, formatErrorWithContext(i.FileSet, valueSpec.Type.Pos(), fmt.Errorf("unsupported type expression for zero value for variable '%s': %T", varName, valueSpec.Type), "")
				}
				env.Define(varName, zeroVal)
			}
		}
	}
	return nil, nil
}

func evalIdentifier(ident *ast.Ident, env *Environment, fset *token.FileSet) (Object, error) {
	switch ident.Name {
	case "true":
		return TRUE, nil
	case "false":
		return FALSE, nil
	}
	if val, ok := env.Get(ident.Name); ok {
		return val, nil
	}
	return nil, formatErrorWithContext(fset, ident.Pos(), fmt.Errorf("identifier not found: %s", ident.Name), "")
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
		return evalStringBinaryExpr(node.Op, left.(*String), right.(*String), i.FileSet, node.Pos())
	case left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ:
		return evalIntegerBinaryExpr(node.Op, left.(*Integer), right.(*Integer), i.FileSet, node.Pos())
	case left.Type() == BOOLEAN_OBJ && right.Type() == BOOLEAN_OBJ:
		if node.Op == token.EQL || node.Op == token.NEQ {
			return evalBooleanBinaryExpr(node.Op, left.(*Boolean), right.(*Boolean), i.FileSet, node.Pos())
		}
		return nil, formatErrorWithContext(i.FileSet, node.Pos(),
			fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s", left.Type(), node.Op, right.Type()), "")
	default:
		return nil, formatErrorWithContext(i.FileSet, node.Pos(),
			fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s", left.Type(), node.Op, right.Type()), "")
	}
}

func evalIntegerBinaryExpr(op token.Token, left, right *Integer, fset *token.FileSet, pos token.Pos) (Object, error) {
	leftVal := left.Value
	rightVal := right.Value

	switch op {
	case token.ADD:
		return &Integer{Value: leftVal + rightVal}, nil
	case token.SUB:
		return &Integer{Value: leftVal - rightVal}, nil
	case token.MUL:
		return &Integer{Value: leftVal * rightVal}, nil
	case token.QUO:
		if rightVal == 0 {
			return nil, formatErrorWithContext(fset, pos, fmt.Errorf("division by zero"), "")
		}
		return &Integer{Value: leftVal / rightVal}, nil
	case token.REM:
		if rightVal == 0 {
			return nil, formatErrorWithContext(fset, pos, fmt.Errorf("division by zero (modulo)"), "")
		}
		return &Integer{Value: leftVal % rightVal}, nil
	case token.EQL:
		return nativeBoolToBooleanObject(leftVal == rightVal), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(leftVal != rightVal), nil
	case token.LSS:
		return nativeBoolToBooleanObject(leftVal < rightVal), nil
	case token.LEQ:
		return nativeBoolToBooleanObject(leftVal <= rightVal), nil
	case token.GTR:
		return nativeBoolToBooleanObject(leftVal > rightVal), nil
	case token.GEQ:
		return nativeBoolToBooleanObject(leftVal >= rightVal), nil
	default:
		return nil, formatErrorWithContext(fset, pos, fmt.Errorf("unknown operator for integers: %s", op), "")
	}
}

func evalStringBinaryExpr(op token.Token, left, right *String, fset *token.FileSet, pos token.Pos) (Object, error) {
	switch op {
	case token.EQL:
		return nativeBoolToBooleanObject(left.Value == right.Value), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(left.Value != right.Value), nil
	case token.ADD:
		return &String{Value: left.Value + right.Value}, nil
	default:
		return nil, formatErrorWithContext(fset, pos, fmt.Errorf("unknown operator for strings: %s (left: %q, right: %q)", op, left.Value, right.Value), "")
	}
}

func evalBooleanBinaryExpr(op token.Token, left, right *Boolean, fset *token.FileSet, pos token.Pos) (Object, error) {
	leftVal := left.Value
	rightVal := right.Value

	switch op {
	case token.EQL:
		return nativeBoolToBooleanObject(leftVal == rightVal), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(leftVal != rightVal), nil
	default:
		return nil, formatErrorWithContext(fset, pos, fmt.Errorf("unknown operator for booleans: %s", op), "")
	}
}

func (i *Interpreter) evalCallExpr(node *ast.CallExpr, env *Environment) (Object, error) {
	funcObj, err := i.eval(node.Fun, env)
	if err != nil {
		return nil, err
	}

	args := make([]Object, len(node.Args))
	for idx, argExpr := range node.Args {
		argVal, err := i.eval(argExpr, env)
		if err != nil {
			return nil, err
		}
		args[idx] = argVal
	}

	switch fn := funcObj.(type) {
	case *BuiltinFunction:
		return fn.Fn(env, args...)
	case *UserDefinedFunction:
		return i.applyUserDefinedFunction(fn, args, node.Fun.Pos())
	default:
		funcName := "unknown"
		if ident, ok := node.Fun.(*ast.Ident); ok {
			funcName = ident.Name
		} else if selExpr, ok := node.Fun.(*ast.SelectorExpr); ok {
			if xIdent, okX := selExpr.X.(*ast.Ident); okX {
				funcName = xIdent.Name + "." + selExpr.Sel.Name
			}
		}
		return nil, formatErrorWithContext(i.FileSet, node.Fun.Pos(), fmt.Errorf("cannot call non-function type %s (for function '%s')", funcObj.Type(), funcName), "")
	}
}

func (i *Interpreter) evalAssignStmt(assignStmt *ast.AssignStmt, env *Environment) (Object, error) {
	if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
		return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(),
			fmt.Errorf("unsupported assignment: expected 1 expression on LHS and 1 on RHS, got %d and %d", len(assignStmt.Lhs), len(assignStmt.Rhs)), "")
	}

	lhs := assignStmt.Lhs[0]
	ident, ok := lhs.(*ast.Ident)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, lhs.Pos(), fmt.Errorf("unsupported assignment LHS: expected identifier, got %T", lhs), "")
	}
	varName := ident.Name

	val, err := i.eval(assignStmt.Rhs[0], env)
	if err != nil {
		return nil, err
	}

	if assignStmt.Tok != token.ASSIGN {
		existingVal, ok := env.Get(varName)
		if !ok {
			return nil, formatErrorWithContext(i.FileSet, ident.Pos(), fmt.Errorf("cannot use %s on undeclared variable '%s'", assignStmt.Tok, varName), "")
		}

		var op token.Token
		switch assignStmt.Tok {
		case token.ADD_ASSIGN:
			op = token.ADD
		case token.SUB_ASSIGN:
			op = token.SUB
		case token.MUL_ASSIGN:
			op = token.MUL
		case token.QUO_ASSIGN:
			op = token.QUO
		case token.REM_ASSIGN:
			op = token.REM
		default:
			return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("unsupported assignment operator %s", assignStmt.Tok), "")
		}

		if existingInt, okE := existingVal.(*Integer); okE {
			if valInt, okV := val.(*Integer); okV {
				switch op {
				case token.ADD:
					val = &Integer{Value: existingInt.Value + valInt.Value}
				case token.SUB:
					val = &Integer{Value: existingInt.Value - valInt.Value}
				case token.MUL:
					val = &Integer{Value: existingInt.Value * valInt.Value}
				case token.QUO:
					if valInt.Value == 0 {
						return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("division by zero in %s", assignStmt.Tok), "")
					}
					val = &Integer{Value: existingInt.Value / valInt.Value}
				case token.REM:
					if valInt.Value == 0 {
						return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("division by zero (modulo) in %s", assignStmt.Tok), "")
					}
					val = &Integer{Value: existingInt.Value % valInt.Value}
				default:
					return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("unsupported operator %s for augmented integer assignment", op), "")
				}
			} else {
				return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("type mismatch for %s: existing value is INTEGER, new value is %s", assignStmt.Tok, val.Type()), "")
			}
		} else if existingString, okE := existingVal.(*String); okE && op == token.ADD {
			if valString, okV := val.(*String); okV {
				val = &String{Value: existingString.Value + valString.Value}
			} else {
				return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("type mismatch for string concatenation (+=): existing value is STRING, new value is %s", val.Type()), "")
			}
		} else {
			return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("unsupported type %s for augmented assignment operator %s", existingVal.Type(), assignStmt.Tok), "")
		}
	}

	if _, ok := env.Assign(varName, val); !ok {
		return nil, formatErrorWithContext(i.FileSet, ident.Pos(), fmt.Errorf("cannot assign to undeclared variable '%s'", varName), "")
	}
	return nil, nil
}

func (i *Interpreter) evalIfStmt(ifStmt *ast.IfStmt, env *Environment) (Object, error) {
	condition, err := i.eval(ifStmt.Cond, env)
	if err != nil {
		return nil, err
	}

	boolCond, ok := condition.(*Boolean)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, ifStmt.Cond.Pos(),
			fmt.Errorf("condition for if statement must be a boolean, got %s (type: %s)", condition.Inspect(), condition.Type()), "")
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
	case token.SUB:
		if operand.Type() == INTEGER_OBJ {
			value := operand.(*Integer).Value
			return &Integer{Value: -value}, nil
		}
		return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("unsupported type for negation: %s", operand.Type()), "")
	case token.NOT:
		switch operand {
		case TRUE:
			return FALSE, nil
		case FALSE:
			return TRUE, nil
		default:
			return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("unsupported type for logical NOT: %s", operand.Type()), "")
		}
	default:
		return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("unsupported unary operator: %s", node.Op), "")
	}
}
