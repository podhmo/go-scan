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
// It's defined here to keep the main eval function cleaner.
func parseInt64(s string) (int64, error) {
	// strconv.ParseInt can handle various bases, but Go literals are typically base 10,
	// hex (0x), octal (0o or 0), or binary (0b).
	// For simplicity, we'll assume base 0, which lets ParseInt auto-detect based on prefix.
	return strconv.ParseInt(s, 0, 64)
}

// Interpreter holds the state of the interpreter
type Interpreter struct {
	globalEnv *Environment // Global environment
}

// NewInterpreter creates a new Interpreter with a global environment.
func NewInterpreter() *Interpreter {
	return &Interpreter{
		globalEnv: NewEnvironment(nil),
	}
}

// LoadAndRun loads a Go source file, parses it, and runs the specified entry point function.
func (i *Interpreter) LoadAndRun(filename string, entryPoint string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("error parsing file %s: %w", filename, err)
	}

	// Process top-level declarations, particularly global variables.
	// We iterate through all declarations in the file. If a top-level declaration
	// is a general declaration (*ast.GenDecl) of variables (token.VAR),
	// we evaluate it in the global environment.
	for _, declNode := range node.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			// To use our existing eval logic, which expects *ast.DeclStmt for declarations,
			// we wrap the *ast.GenDecl in a temporary *ast.DeclStmt.
			// This is a bit of a workaround but avoids needing a separate eval path for global GenDecls
			// versus GenDecls found within function bodies (which are already inside DeclStmts).
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, err := i.eval(tempDeclStmt, i.globalEnv) // Evaluate VAR declaration in global scope
			if err != nil {
				// An error here means a global variable declaration failed.
				return fmt.Errorf("error evaluating global variable declaration at %d: %w", genDecl.Pos(), err)
			}
		}
	}

	entryFunc := findFunction(node, entryPoint)
	if entryFunc == nil {
		return fmt.Errorf("entry point function '%s' not found in %s", entryPoint, filename)
	}

	// Each run of a function gets its own environment, enclosed by the global one.
	// For simplicity now, the main function's environment is directly enclosed by global.
	// Later, we might want a file-level scope or package-level scope.
	funcEnv := NewEnvironment(i.globalEnv)
	// TODO: Process function parameters and arguments here.

	fmt.Printf("Executing function: %s\n", entryPoint) // For debugging
	_, evalErr := i.evalBlockStatement(entryFunc.Body, funcEnv)
	return evalErr // Return the error from evaluation
}

// findFunction searches for a function declaration with the given name in the AST.
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

// eval evaluates an AST node within a given environment.
func (i *Interpreter) eval(node ast.Node, env *Environment) (Object, error) {
	switch n := node.(type) {
	case *ast.File: // It's possible to receive a File node if we decide to eval the whole file.
		var result Object
		var err error
		for _, decl := range n.Decls {
			// For now, only evaluating function declarations if they are 'main' (handled by LoadAndRun)
			// or other top-level statements if we support them (e.g. global var declarations).
			// This part needs more thought for full file evaluation.
			if fnDecl, ok := decl.(*ast.FuncDecl); ok && fnDecl.Name.Name == "main" { // simplistic
				result, err = i.evalBlockStatement(fnDecl.Body, env)
				if err != nil {
					return nil, fmt.Errorf("error evaluating main function in file: %w", err)
				}
			}
		}
		return result, nil // Return last evaluated result or nil

	case *ast.BlockStmt:
		return i.evalBlockStatement(n, env)

	case *ast.ExprStmt: // e.g. a function call used as a statement
		return i.eval(n.X, env)

	case *ast.Ident:
		return evalIdentifier(n, env)

	case *ast.BasicLit:
		switch n.Kind {
		case token.STRING:
			// Go's string literals are already unescaped by the parser.
			// The Value field includes the quotes, so we strip them.
			return &String{Value: n.Value[1 : len(n.Value)-1]}, nil
		case token.INT:
			// Integer literal processing
			val, err := parseInt64(n.Value)
			if err != nil {
				return nil, fmt.Errorf("could not parse integer literal %s: %w", n.Value, err)
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

	case *ast.ParenExpr: // Handle parenthesized expressions
		return i.eval(n.X, env)

	case *ast.IfStmt:
		return i.evalIfStmt(n, env)

	case *ast.AssignStmt:
		return i.evalAssignStmt(n, env)

	// TODO: Add more cases for other AST node types:
	// *ast.CallExpr (for function calls)
	// *ast.ForStmt, *ast.ReturnStmt etc.

	default:
		return nil, fmt.Errorf("unsupported AST node type: %T at %d", n, n.Pos())
	}
}

// evalBlockStatement evaluates a block of statements.
// Returns the result of the last statement if it's a return or expression, or nil.
func (i *Interpreter) evalBlockStatement(block *ast.BlockStmt, env *Environment) (Object, error) {
	var result Object
	var err error

	for _, stmt := range block.List {
		// TODO: Handle return statements. If a return is evaluated, we should stop execution
		// of this block and propagate the return value up.
		result, err = i.eval(stmt, env)
		if err != nil {
			return nil, err // Propagate error
		}
		// If 'result' is a special ReturnValue object, propagate it immediately.
		// if retVal, ok := result.(*ReturnValue); ok {
		// return retVal, nil
		// }
	}
	// The result of a block is typically the result of its last statement,
	// but in Go, blocks themselves don't have values unless it's an expression block.
	// For now, we return the last evaluated object, which might be nil for declarations.
	return result, nil
}

// evalDeclStmt handles declarations like var x = "hello" or var x int.
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
			if len(valueSpec.Values) > idx { // Check if there's an initializer for this var
				val, err := i.eval(valueSpec.Values[idx], env)
				if err != nil {
					return nil, fmt.Errorf("error evaluating value for var %s: %w", varName, err)
				}
				env.Define(varName, val) // Use Define for declarations
			} else {
				// Handle uninitialized variable declaration, e.g., var x string, var n int
				if valueSpec.Type == nil {
					// This case (e.g. `var x`) is generally not allowed in Go unless it's a short var declaration
					// or part of a multi-var block where type can be inferred.
					// For MiniGo, let's require a type if there's no initializer.
					return nil, fmt.Errorf("variable '%s' declared without initializer must have a type at %d", varName, valueSpec.Pos())
				}

				var zeroVal Object
				typeIdent, ok := valueSpec.Type.(*ast.Ident)
				if !ok {
					// TODO: Handle more complex types like arrays, structs if they are added.
					return nil, fmt.Errorf("unsupported type expression for zero value for variable '%s': %T at %d", varName, valueSpec.Type, valueSpec.Type.Pos())
				}

				switch typeIdent.Name {
				case "string":
					zeroVal = &String{Value: ""}
				case "int": // Assuming "int" is the type name for our IntegerObject
					zeroVal = &Integer{Value: 0}
				case "bool": // Assuming "bool" is the type name for our BooleanObject
					zeroVal = FALSE // Use the global FALSE instance
				default:
					return nil, fmt.Errorf("unsupported type '%s' for uninitialized variable '%s' at %d", typeIdent.Name, varName, typeIdent.Pos())
				}
				env.Define(varName, zeroVal)
			}
		}
	}
	return nil, nil // var declaration statement does not produce a value itself
}

// evalIdentifier evaluates an identifier (variable lookup).
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
	// TODO: Check for built-in functions here if ident.Name matches one.
	return nil, fmt.Errorf("identifier not found: %s at %d", ident.Name, ident.Pos())
}

// evalBinaryExpr handles binary expressions like +, -, *, /, ==, !=, <, >.
func (i *Interpreter) evalBinaryExpr(node *ast.BinaryExpr, env *Environment) (Object, error) {
	left, err := i.eval(node.X, env)
	if err != nil {
		return nil, err
	}
	right, err := i.eval(node.Y, env)
	if err != nil {
		return nil, err
	}

	// Handle operations based on the types of left and right operands
	switch {
	case left.Type() == STRING_OBJ && right.Type() == STRING_OBJ:
		return evalStringBinaryExpr(node.Op, left.(*String), right.(*String))
	case left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ:
		return evalIntegerBinaryExpr(node.Op, left.(*Integer), right.(*Integer), node.Pos())
	case left.Type() == BOOLEAN_OBJ && right.Type() == BOOLEAN_OBJ:
		// Only specific operators are defined for booleans. Others lead to type mismatch.
		if node.Op == token.EQL || node.Op == token.NEQ {
			return evalBooleanBinaryExpr(node.Op, left.(*Boolean), right.(*Boolean), node.Pos())
		}
		// If operator is not == or != for booleans, it's a type mismatch.
		return nil, fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s at %d",
			left.Type(), node.Op, right.Type(), node.Pos())
	default:
		// This default handles cases where left/right types were not String, Integer, or Boolean pairs.
		return nil, fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s at %d",
			left.Type(), node.Op, right.Type(), node.Pos())
	}
}

// evalIntegerBinaryExpr handles binary expressions specifically for integers.
func evalIntegerBinaryExpr(op token.Token, left, right *Integer, pos token.Pos) (Object, error) {
	leftVal := left.Value
	rightVal := right.Value

	switch op {
	case token.ADD: // +
		return &Integer{Value: leftVal + rightVal}, nil
	case token.SUB: // -
		return &Integer{Value: leftVal - rightVal}, nil
	case token.MUL: // *
		return &Integer{Value: leftVal * rightVal}, nil
	case token.QUO: // /
		if rightVal == 0 {
			return nil, fmt.Errorf("division by zero at %d", pos)
		}
		return &Integer{Value: leftVal / rightVal}, nil
	case token.REM: // %
		if rightVal == 0 {
			return nil, fmt.Errorf("division by zero (modulo) at %d", pos)
		}
		return &Integer{Value: leftVal % rightVal}, nil
	case token.EQL: // ==
		return nativeBoolToBooleanObject(leftVal == rightVal), nil
	case token.NEQ: // !=
		return nativeBoolToBooleanObject(leftVal != rightVal), nil
	case token.LSS: // <
		return nativeBoolToBooleanObject(leftVal < rightVal), nil
	case token.LEQ: // <=
		return nativeBoolToBooleanObject(leftVal <= rightVal), nil
	case token.GTR: // >
		return nativeBoolToBooleanObject(leftVal > rightVal), nil
	case token.GEQ: // >=
		return nativeBoolToBooleanObject(leftVal >= rightVal), nil
	default:
		return nil, fmt.Errorf("unknown operator for integers: %s at %d", op, pos)
	}
}

// evalStringBinaryExpr handles binary expressions specifically for strings.
func evalStringBinaryExpr(op token.Token, left, right *String) (Object, error) {
	switch op {
	case token.EQL: // ==
		return nativeBoolToBooleanObject(left.Value == right.Value), nil
	case token.NEQ: // !=
		return nativeBoolToBooleanObject(left.Value != right.Value), nil
	// TODO: Support string concatenation with '+' ?
	// case token.ADD:
	//    return &String{Value: left.Value + right.Value}, nil
	default:
		return nil, fmt.Errorf("unknown operator for strings: %s", op)
	}
}

// evalBooleanBinaryExpr handles binary expressions specifically for booleans.
func evalBooleanBinaryExpr(op token.Token, left, right *Boolean, pos token.Pos) (Object, error) {
	leftVal := left.Value
	rightVal := right.Value

	switch op {
	case token.EQL: // ==
		return nativeBoolToBooleanObject(leftVal == rightVal), nil
	case token.NEQ: // !=
		return nativeBoolToBooleanObject(leftVal != rightVal), nil
	// Go does not support <, >, <=, >= directly for booleans in the same way as numbers.
	// Logical AND (&&) and OR (||) are handled by ast.BinaryExpr with token.LAND and token.LOR.
	// These often require short-circuiting, which evalBinaryExpr doesn't do yet.
	// For now, only == and != are supported for direct boolean comparison.
	default:
		return nil, fmt.Errorf("unknown operator for booleans: %s at %d", op, pos)
	}
}

// TODO: Implement evalCallExpr, etc.
// func (i *Interpreter) evalCallExpr(node *ast.CallExpr, env *Environment) (Object, error) { ... }
// ... and other evaluation helpers

// evalAssignStmt handles assignment statements like x = 10 or x += 5.
func (i *Interpreter) evalAssignStmt(assignStmt *ast.AssignStmt, env *Environment) (Object, error) {
	// MiniGo assignment basics:
	// Lhs: list of expressions (identifiers for now)
	// Rhs: list of expressions
	// Tok: token.ASSIGN (=), token.ADD_ASSIGN (+=), etc.

	if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
		// For now, only support simple single assignment: ident = value
		// Multiple assignments like a, b = 1, 2 or tuple-like returns are not supported yet.
		return nil, fmt.Errorf("unsupported assignment: expected 1 expression on LHS and 1 on RHS, got %d and %d at %d",
			len(assignStmt.Lhs), len(assignStmt.Rhs), assignStmt.Pos())
	}

	lhs := assignStmt.Lhs[0]
	ident, ok := lhs.(*ast.Ident)
	if !ok {
		// TODO: Support assignments to array elements (e.g., arr[0] = 1) or struct fields (e.g., obj.field = 1) later.
		return nil, fmt.Errorf("unsupported assignment LHS: expected identifier, got %T at %d", lhs, lhs.Pos())
	}
	varName := ident.Name

	// Evaluate the right-hand side to get the value
	val, err := i.eval(assignStmt.Rhs[0], env)
	if err != nil {
		return nil, err
	}

	// If it's an augmented assignment (e.g., +=, -=), we need to get the current value first.
	if assignStmt.Tok != token.ASSIGN {
		existingVal, ok := env.Get(varName)
		if !ok {
			// This behavior matches Go: using += on an undeclared variable is an error.
			// "identifier not found" would be caught by Get if we tried to use it.
			// If it's a new variable, += is not allowed; must be initialized with = first.
			return nil, fmt.Errorf("cannot use %s on undeclared variable '%s' at %d", assignStmt.Tok, varName, ident.Pos())
		}

		// Perform the binary operation for augmented assignment
		// We need to construct a temporary BinaryExpr to reuse evalBinaryExpr logic,
		// or replicate the logic here. Replicating parts of it for clarity.
		// This is a simplified version. evalBinaryExpr handles type checking more robustly.
		// For example, this doesn't handle "string" + "string" for += yet.
		// It primarily assumes numeric operations for augmented assignments for now.

		// Convert token.ADD_ASSIGN to token.ADD, etc.
		var op token.Token
		switch assignStmt.Tok {
		case token.ADD_ASSIGN: // +=
			op = token.ADD
		case token.SUB_ASSIGN: // -=
			op = token.SUB
		case token.MUL_ASSIGN: // *=
			op = token.MUL
		case token.QUO_ASSIGN: // /=
			op = token.QUO
		case token.REM_ASSIGN: // %=
			op = token.REM
		// TODO: Add bitwise operators like &=, |=, ^=, <<=, >>= if the language supports them.
		default:
			return nil, fmt.Errorf("unsupported assignment operator %s at %d", assignStmt.Tok, assignStmt.Pos())
		}

		// Simulate a binary expression: existingVal op val
		// This is a bit of a hack. Ideally, evalBinaryExpr is more directly usable.
		// The following is a simplified re-implementation for Integer types primarily.
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
						return nil, fmt.Errorf("division by zero in %s at %d", assignStmt.Tok, assignStmt.Pos())
					}
					val = &Integer{Value: existingInt.Value / valInt.Value}
				case token.REM:
					if valInt.Value == 0 {
						return nil, fmt.Errorf("division by zero (modulo) in %s at %d", assignStmt.Tok, assignStmt.Pos())
					}
					val = &Integer{Value: existingInt.Value % valInt.Value}
				default:
					return nil, fmt.Errorf("unsupported operator %s for augmented integer assignment at %d", op, assignStmt.Pos())
				}
			} else {
				return nil, fmt.Errorf("type mismatch for %s: existing value is INTEGER, new value is %s at %d", assignStmt.Tok, val.Type(), assignStmt.Pos())
			}
		} else if existingString, okE := existingVal.(*String); okE && op == token.ADD { // String concatenation +=
			if valString, okV := val.(*String); okV {
				val = &String{Value: existingString.Value + valString.Value}
			} else {
				return nil, fmt.Errorf("type mismatch for string concatenation (+=): existing value is STRING, new value is %s at %d", val.Type(), assignStmt.Pos())
			}
		} else {
			// TODO: Handle other types for augmented assignment if necessary
			return nil, fmt.Errorf("unsupported type %s for augmented assignment operator %s at %d", existingVal.Type(), assignStmt.Tok, assignStmt.Pos())
		}
	}

	// Set the variable in the environment.
	// The Environment's Set method should handle whether it's a new declaration (in current scope)
	// or re-assigning an existing variable (possibly in an outer scope).
	// MiniGo's scoping for assignment (like Go): if var exists in current or outer, it's reassigned.
	// If it doesn't exist, `env.Set` would ideally declare it in the current scope.
	// The current `Environment.Set` updates if found, otherwise sets in current env. This is fine.
	// env.Set(varName, val) // Old call
	if _, ok := env.Assign(varName, val); !ok {
		return nil, fmt.Errorf("cannot assign to undeclared variable '%s' at %d", varName, ident.Pos())
	}

	// Assignment statement itself does not produce a value in many languages (e.g., Go).
	// Or it might produce the assigned value. For MiniGo, let's say it doesn't produce a value.
	return nil, nil
}

// evalIfStmt evaluates an if statement.
func (i *Interpreter) evalIfStmt(ifStmt *ast.IfStmt, env *Environment) (Object, error) {
	condition, err := i.eval(ifStmt.Cond, env)
	if err != nil {
		return nil, err
	}

	// Check if the condition is a Boolean object
	boolCond, ok := condition.(*Boolean)
	if !ok {
		return nil, fmt.Errorf("condition for if statement must be a boolean, got %s (type: %s) at %d",
			condition.Inspect(), condition.Type(), ifStmt.Cond.Pos())
	}

	if boolCond.Value { // If condition is true
		return i.evalBlockStatement(ifStmt.Body, env)
	} else if ifStmt.Else != nil { // If condition is false and there's an else block
		// The else part can be another IfStmt (for "else if") or a BlockStmt.
		// The eval function will handle these types accordingly.
		return i.eval(ifStmt.Else, env)
	}

	// If condition is false and no else block, the if statement evaluates to nothing.
	// In a language where everything is an expression, this might return a Null object.
	// For now, returning nil is consistent with how DeclStmt is handled.
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
			value := operand.(*Integer).Value
			return &Integer{Value: -value}, nil
		}
		return nil, fmt.Errorf("unsupported type for negation: %s at %d", operand.Type(), node.Pos())
	case token.NOT: // Logical not !
		// In Go, '!' is used for boolean negation.
		// Our Boolean object has TRUE and FALSE singletons.
		switch operand {
		case TRUE:
			return FALSE, nil
		case FALSE:
			return TRUE, nil
		default:
			// Following typical dynamic language behavior, often only 'false' and 'null' are falsy.
			// Everything else is truthy. Or we can be strict.
			// For now, strict: only operate on actual Boolean objects.
			return nil, fmt.Errorf("unsupported type for logical NOT: %s at %d", operand.Type(), node.Pos())
		}
	default:
		return nil, fmt.Errorf("unsupported unary operator: %s at %d", node.Op, node.Pos())
	}
}
