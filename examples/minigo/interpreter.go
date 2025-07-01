package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings" // Added for strings.Join
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
	builtins  map[string]*Builtin
}

// NewInterpreter creates a new Interpreter with a global environment.
func NewInterpreter() *Interpreter {
	i := &Interpreter{
		globalEnv: NewEnvironment(nil),
		builtins:  make(map[string]*Builtin),
	}
	// Initialize built-in functions
	i.builtins["fmt.Sprintf"] = &Builtin{Fn: builtinFmtSprintf}
	i.builtins["strings.Join"] = &Builtin{Fn: builtinStringsJoin}
	i.builtins["strings.ToUpper"] = &Builtin{Fn: builtinStringsToUpper}
	i.builtins["strings.TrimSpace"] = &Builtin{Fn: builtinStringsTrimSpace}
	// TODO: Add more built-ins as needed

	return i
}

// LoadAndRun loads a Go source file, parses it, and runs the specified entry point function.
func (i *Interpreter) LoadAndRun(filename string, entryPoint string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("error parsing file %s: %w", filename, err)
	}

	// Evaluate top-level declarations first (e.g., global variables)
	for _, decl := range node.Decls {
		// We are interested in GenDecl for var, const, type, import.
		// For now, let's focus on var declarations at the top level.
		// FuncDecls will be handled by finding the entry point or for future function calls.
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			if genDecl.Tok == token.VAR { // Ensure it's a var declaration
				// Wrap GenDecl in a DeclStmt to use existing eval logic
				declStmt := &ast.DeclStmt{Decl: genDecl}
				// Evaluate declaration in the global environment
				_, err := i.eval(declStmt, i.globalEnv) // Use globalEnv for top-level var declarations
				if err != nil {
					return fmt.Errorf("error evaluating top-level var declaration: %w", err)
				}
			}
			// TODO: Handle top-level const declarations (genDecl.Tok == token.CONST)
		}
		// TODO: Handle other top-level declarations if needed.
	}

	entryFunc := findFunction(node, entryPoint)
	if entryFunc == nil {
		// If no entry point is specified (e.g. empty string), and we only wanted to eval globals,
		// this might not be an error. For now, assume entryPoint is always required if LoadAndRun is called.
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
	// fmt.Printf("eval: Received node type %T at %d\n", node, node.Pos()) // DEBUG: Entry log
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
			// The Value field includes the quotes. We need to unquote it to handle escape sequences.
			s, err := strconv.Unquote(n.Value)
			if err != nil {
				return nil, fmt.Errorf("could not unquote string literal %s: %w", n.Value, err)
			}
			return &String{Value: s}, nil
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

	case *ast.CallExpr:
		return i.evalCallExpr(n, env)

	case *ast.CompositeLit:
		// fmt.Printf("eval: Entering *ast.CompositeLit case for node type %T at %d\n", n, n.Pos()) // DEBUG
		// Currently, only handling array literals like []string{"a", "b"}
		// Type checking (e.g., ensuring it's an array of strings if specified) is rudimentary.
		// For `[]string{"a", "b"}`, n.Type would be an *ast.ArrayType.
		// For `[]MyType{...}`, n.Type would be an *ast.Ident if MyType is defined.
		// We are simplifying here and not deeply inspecting n.Type for now.
		// We assume it's intended to be a generic array of objects.

		elements := make([]Object, len(n.Elts))
		for j, eltExpr := range n.Elts {
			evaluatedElt, err := i.eval(eltExpr, env) // ここでネストした要素を評価
			if err != nil {
				return nil, fmt.Errorf("error evaluating element %d in composite literal: %w", j, err)
			}
			elements[j] = evaluatedElt
		}
		return &Array{Elements: elements}, nil

	case *ast.AssignStmt:
		// Simplified assignment: assumes Lhs is a single Ident.
		// e.g., x = 10. Does not handle a, b = 1, 2 or obj.field = val yet.
		// Also, this currently always sets in the current 'env'.
		// For global/local distinction, env structure and lookup rules are key.
		// fmt.Printf("eval: Entering *ast.AssignStmt case for node type %T at %d\n", n, n.Pos()) // DEBUG
		if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
			return nil, fmt.Errorf("unsupported assignment: expected 1 Lhs and 1 Rhs, got %d and %d at %d", len(n.Lhs), len(n.Rhs), n.Pos())
		}
		ident, ok := n.Lhs[0].(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("unsupported assignment: Lhs is not an identifier (%T) at %d", n.Lhs[0], n.Lhs[0].Pos())
		}
		varName := ident.Name

		val, err := i.eval(n.Rhs[0], env)
		if err != nil {
			return nil, fmt.Errorf("error evaluating Rhs of assignment to %s: %w", varName, err)
		}
		// For now, Set also updates outer environment if var exists there and not locally.
		// Or, if we want strict shadowing, env.Set should only set in the current env.
		// The current env.Set logic (from environment.go, not shown) will determine this behavior.
		// For typical lexical scoping, assignment should update the narrowest scope where var is defined,
		// or create in local if not defined anywhere.
		// If `env.Set` implements "set if exists in current or outer, else create in current", this might work for globals too.
		env.Set(varName, val)
		return nil, nil // Assignment statement itself doesn't yield a value in Go expressions.

	// TODO: Add more cases for other AST node types:
	// *ast.IfStmt, *ast.ForStmt, *ast.ReturnStmt etc.

	default:
		// fmt.Printf("eval: Entering default case for node type %T at %d\n", n, n.Pos()) // DEBUG
		return nil, fmt.Errorf("unsupported AST node type: %T at %d", n, n.Pos())
	}
}

// evalCallExpr evaluates a function call expression.
func (i *Interpreter) evalCallExpr(node *ast.CallExpr, env *Environment) (Object, error) {
	// The Fun part of a CallExpr can be an Identifier (e.g., myFunc())
	// or a SelectorExpr (e.g., fmt.Sprintf()).
	var funcName string
	switch fun := node.Fun.(type) {
	case *ast.Ident:
		funcName = fun.Name
	case *ast.SelectorExpr:
		// For simplicity, assuming selector is of form "package.Function"
		// e.g. fmt.Sprintf. X would be "fmt", Sel would be "Sprintf".
		xIdent, okX := fun.X.(*ast.Ident)
		if !okX {
			return nil, fmt.Errorf("unsupported selector expression type for function call: %T at %d", fun.X, fun.Pos())
		}
		funcName = xIdent.Name + "." + fun.Sel.Name
	default:
		return nil, fmt.Errorf("unsupported function call type: %T at %d", node.Fun, node.Fun.Pos())
	}

	// Evaluate arguments
	args := []Object{}
	for _, argExpr := range node.Args {
		argVal, err := i.eval(argExpr, env)
		if err != nil {
			return nil, fmt.Errorf("error evaluating argument for %s: %w", funcName, err)
		}
		args = append(args, argVal)
	}

	// Check if it's a built-in function
	if builtin, ok := i.builtins[funcName]; ok {
		return builtin.Fn(args...)
	}

	// TODO: Handle user-defined functions.
	// For now, if it's not a built-in, it's an error.
	return nil, fmt.Errorf("undefined function: %s at %d", funcName, node.Fun.Pos())
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
				env.Define(varName, val) // Use Define for var declarations
			} else {
				// No initializer, e.g., var x int. Set to zero value for its type.
				// For now, we only have strings, so what's the zero value? An empty string? Null?
				// This part needs type information (valueSpec.Type).
				// For now, let's assume uninitialized vars are an error or a special "undefined" object.
				// Or, for simplicity in this early stage, require initialization.
				if valueSpec.Type != nil {
					// TODO: Handle type-specific zero values when type system is more developed.
					// For now, if type is specified but no value, it's an unhandled case.
					// Let's default to a NullObject or similar if we had one.
					// For strings, perhaps an empty string.
					// This is a simplification. In a typed language, this would be type-dependent.
					// For now, treat as an error or make it a Null object if available.
					// For initial simplicity, we will require explicit initialization.
					return nil, fmt.Errorf("variable '%s' declared without explicit initializer is not yet supported (type: %s)", varName, valueSpec.Type)
				}
				// If no type and no value, also an issue.
				return nil, fmt.Errorf("variable '%s' declared without explicit initializer or type", varName)
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
	case token.ADD: // +
		return left.Add(right)
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

// --- Built-in Function Implementations ---

func builtinFmtSprintf(args ...Object) (Object, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fmt.Sprintf: expected at least 1 argument, got %d", len(args))
	}
	formatStr, ok := args[0].(*String)
	if !ok {
		return nil, fmt.Errorf("fmt.Sprintf: first argument must be a string, got %s", args[0].Type())
	}

	// Convert minigo String arguments to Go interface{} for fmt.Sprintf
	sArgs := make([]interface{}, len(args)-1)
	for i, arg := range args[1:] {
		switch a := arg.(type) {
		case *String:
			sArgs[i] = a.Value
		case *Integer:
			sArgs[i] = a.Value
		// case *Boolean: // Boolean is not directly supported by %s or %d in user-provided format string usually
		//	sArgs[i] = a.Value
		// Add other types as needed
		default:
			// Check against format string verb? For now, be strict.
			// %s expects string-like, %d expects integer-like.
			// If formatStr.Value contains %s at the corresponding position, arg must be String.
			// If formatStr.Value contains %d, arg must be Integer.
			// This is a simplification; real Sprintf is more complex.
			// For now, if it's not a known type that Sprintf can handle universally (like string, int), error out.
			return nil, fmt.Errorf("fmt.Sprintf: unsupported argument type %s for format string", arg.Type())
		}
	}

	result := fmt.Sprintf(formatStr.Value, sArgs...)
	return &String{Value: result}, nil
}

func builtinStringsJoin(args ...Object) (Object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("strings.Join: expected 2 arguments, got %d", len(args))
	}

	// First argument: expecting a slice of strings.
	// This is tricky as minigo doesn't have explicit array/slice objects yet.
	// For now, we'll assume the AST for this argument was parsed and evaluated
	// into some representation we can work with.
	// Let's assume, for this limited implementation, that the first argument *must* be
	// an *Array object (which we haven't fully defined yet).
	// For the test case `strings.Join([]string{"a", "b"}, ",")`, the first arg
	// would need to be evaluated to an internal representation of `[]string{"a", "b"}`.
	//
	// TEMPORARY SIMPLIFICATION: We will expect the first argument to be an *object.String
	// that itself contains a comma-separated list of strings, representing the elements
	// that would have been in a slice. This is a workaround until proper array/slice
	// types are implemented in minigo.
	// e.g. strings.Join("a,b,c", ",")
	// This is not how strings.Join works in Go, but a temporary measure.
	//
	// A better simplification for now might be to expect individual string arguments
	// and join them, or require a specific format that's easier to parse without full array support.
	//
	// Let's try to handle a list of String objects directly if passed.
	// No, the AST `[]string{"a", "b"}` is a CompositeLit. We need to evaluate that first.
	// For now, this function will be very limited.
	//
	// Plan B: The arguments to `strings.Join` will be evaluated by `evalCallExpr`.
	// If `strings.Join` is called like `strings.Join(myStringArray, ",")`, then `myStringArray`
	// must evaluate to an `Array` object in our interpreter. Since we don't have `Array` yet,
	// this is hard.
	//
	// Simplest path for the requested test `strings.Join([]string{"a", "b", "c"}, ",")`:
	// The `[]string{"a", "b", "c"}` is an `ast.CompositeLit`. We need to make `eval` handle `ast.CompositeLit`
	// and produce an `Array` object. Then `builtinStringsJoin` can consume this `Array` object.
	// This is a prerequisite.
	//
	// Given the current plan, we'll make a strong assumption:
	// The test will pass a *String object as the first argument, and this string will be
	// a placeholder for what should be an array. This is not ideal but fits the constraint
	// of not implementing full array support *yet* in this step.
	//
	// Let's refine: The plan mentions "strings.Join([]string{\"a\", \"b\", \"c\"}, \",\")".
	// This implies `eval` needs to handle `ast.CompositeLit` to create some form of list/array object.
	// We'll need a basic `Array` object in `object.go`.
	//
	// For now, let's assume the arguments passed to `builtinStringsJoin` are already evaluated.
	// The first argument *should* be an Array object.
	// Since Array object is not implemented, this will fail or require a placeholder.
	//
	// Let's adjust the expectation for `strings.Join` for this step to be simpler,
	// avoiding the immediate need for full Array implementation.
	// Assume `strings.Join` will take a varidic number of string arguments followed by a separator.
	// e.g. `strings.Join("a", "b", "c", ",")` -> "a,b,c"
	// This is not standard Go `strings.Join` but is achievable now.
	// Or, stick to the plan and defer proper handling to the test phase, which will force Array implementation.
	//
	// The plan says: "strings.Join([]string{\"a\", \"b\", \"c\"}, \",\") -> \"a,b,c\" (ただし、minigo で配列をどう表現するかを先に検討する必要があります...)"
	// This means we should anticipate needing a basic Array.
	//
	// Let's proceed with a temporary, limited `builtinStringsJoin` that expects specific argument types
	// and structure, acknowledging it will need to be improved when Array types are added.
	// For now, to make *any* progress, we'll assume the first argument is a *String
	// where the Value is a comma-separated list of elements, and the second arg is the separator.
	// This is a significant simplification.
	//
	// Acknowledging the above, let's try to implement based on what the test will likely provide.
	// The test will likely try to evaluate `[]string{"a", "b"}`. This needs `ast.CompositeLit` handling in `eval`.
	//
	// Simplification for this step: `strings.Join` will take exactly two *String arguments.
	// The first string's `Value` is assumed to be a pre-joined string of elements using a default separator (e.g., space),
	// and the second string is the *new* separator. This is not `strings.Join`'s behavior.
	//
	// Let's try to make it slightly more robust for the test structure:
	// We need to handle `ast.CompositeLit` in `eval` to produce an `Array` object.
	// So, first, add a basic `Array` object to `object.go`.

	// This implementation of builtinStringsJoin assumes the first argument has been evaluated to an ArrayObject.
	// This part of the plan ("CallExpr の実装と fmt, strings パッケージ関数の限定的サポート")
	// might need to be deferred or simplified if ArrayObject implementation is too complex for this single step.

	// Given the prompt's focus on `fmt` and `strings` *limitedly*,
	// we'll make `strings.Join` expect its arguments already be minigo String objects.
	// The test for `strings.Join([]string{"a", "b"}, ",")` will require `eval` to handle CompositeLit.
	// This is a dependency.

	// For now, let's assume args[0] is an Array object (to be implemented).
	// If not, this function will error. This makes the need for Array explicit.
	arr, ok := args[0].(*Array) // Assuming Array type exists
	if !ok {
		// Temporary fallback for testing if Array is not ready:
		// If it's a string, assume it's a placeholder for elements "e1 e2 e3"
		if strElements, okStr := args[0].(*String); okStr {
			sepObj, okSep := args[1].(*String)
			if !okSep {
				return nil, fmt.Errorf("strings.Join: second argument must be a string separator, got %s", args[1].Type())
			}
			// This is a placeholder behavior: split the string by space and join with new separator
			elements := strings.Split(strElements.Value, " ")
			return &String{Value: strings.Join(elements, sepObj.Value)}, nil
		}
		return nil, fmt.Errorf("strings.Join: first argument must be an array (or a placeholder string for now), got %s", args[0].Type())
	}

	sep, ok := args[1].(*String)
	if !ok {
		return nil, fmt.Errorf("strings.Join: second argument must be a string separator, got %s", args[1].Type())
	}

	if len(arr.Elements) == 0 {
		return &String{Value: ""}, nil
	}

	strElements := make([]string, len(arr.Elements))
	for i, el := range arr.Elements {
		sEl, ok := el.(*String)
		if !ok {
			return nil, fmt.Errorf("strings.Join: all elements in the array must be strings, got %s", el.Type())
		}
		strElements[i] = sEl.Value
	}

	return &String{Value: strings.Join(strElements, sep.Value)}, nil
}

func builtinStringsToUpper(args ...Object) (Object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("strings.ToUpper: expected 1 argument, got %d", len(args))
	}
	s, ok := args[0].(*String)
	if !ok {
		return nil, fmt.Errorf("strings.ToUpper: argument must be a string, got %s", args[0].Type())
	}
	return &String{Value: strings.ToUpper(s.Value)}, nil
}

func builtinStringsTrimSpace(args ...Object) (Object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("strings.TrimSpace: expected 1 argument, got %d", len(args))
	}
	s, ok := args[0].(*String)
	if !ok {
		return nil, fmt.Errorf("strings.TrimSpace: argument must be a string, got %s", args[0].Type())
	}
	return &String{Value: strings.TrimSpace(s.Value)}, nil
}
