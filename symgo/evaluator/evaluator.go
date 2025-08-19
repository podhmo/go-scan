package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"os"
	"path"
	"strconv"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	scanner    *goscan.Scanner
	intrinsics *intrinsics.Registry
	logger     *slog.Logger
}

// New creates a new Evaluator.
func New(scanner *goscan.Scanner, logger *slog.Logger) *Evaluator {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return &Evaluator{
		scanner:    scanner,
		intrinsics: intrinsics.New(),
		logger:     logger,
	}
}

// RegisterIntrinsic registers a built-in function.
func (e *Evaluator) RegisterIntrinsic(key string, fn intrinsics.IntrinsicFunc) {
	e.intrinsics.Register(key, fn)
}

// GetIntrinsic retrieves a built-in function for testing.
func (e *Evaluator) GetIntrinsic(key string) (intrinsics.IntrinsicFunc, bool) {
	return e.intrinsics.Get(key)
}

// Eval is the main dispatch loop for the evaluator.
func (e *Evaluator) Eval(node ast.Node, env *object.Environment) object.Object {
	switch n := node.(type) {
	case *ast.File:
		return e.evalFile(n, env)
	case *ast.SelectorExpr:
		return e.evalSelectorExpr(n, env)
	case *ast.BasicLit:
		return e.evalBasicLit(n)
	case *ast.Ident:
		return e.evalIdent(n, env)
	case *ast.AssignStmt:
		return e.evalAssignStmt(n, env)
	case *ast.BlockStmt:
		return e.evalBlockStatement(n, env)
	case *ast.ReturnStmt:
		return e.evalReturnStmt(n, env)
	case *ast.IfStmt:
		return e.evalIfStmt(n, env)
	case *ast.ForStmt:
		return e.evalForStmt(n, env)
	case *ast.SwitchStmt:
		return e.evalSwitchStmt(n, env)
	case *ast.CallExpr:
		return e.evalCallExpr(n, env)
	case *ast.ExprStmt:
		return e.Eval(n.X, env)
	}
	return newError("evaluation not implemented for %T", node)
}

func (e *Evaluator) evalFile(file *ast.File, env *object.Environment) object.Object {
	// Top-level declarations
	// First, handle all imports to populate the environment with package placeholders.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				for _, spec := range d.Specs {
					e.evalImportSpec(spec, env)
				}
			}
		case *ast.FuncDecl:
			// Register the function in the environment so it can be called later.
			// The function body is not evaluated until the function is called.
			fn := &object.Function{
				Name:       d.Name,
				Parameters: d.Type.Params,
				Body:       d.Body,
				Env:        env,
				Decl:       d,
			}
			env.Set(d.Name.Name, fn)
		}
	}

	// After processing imports, we could evaluate other top-level declarations (vars, funcs, etc.)
	// For now, we'll just return nil as we are focused on setting up the environment.
	// A more complete implementation would continue evaluation.
	return nil
}

func (e *Evaluator) evalImportSpec(spec ast.Spec, env *object.Environment) object.Object {
	importSpec, ok := spec.(*ast.ImportSpec)
	if !ok {
		return nil // Should not happen
	}

	// The path is a string literal, so we need to unquote it.
	importPath, err := strconv.Unquote(importSpec.Path.Value)
	if err != nil {
		return newError("invalid import path: %s", importSpec.Path.Value)
	}

	var pkgName string
	if importSpec.Name != nil {
		// Alias is used, e.g., `str "strings"`
		pkgName = importSpec.Name.Name
	} else {
		// No alias, infer from path, e.g., `"path/filepath"` -> `filepath`
		pkgName = path.Base(importPath)
	}

	// Create a placeholder package object. The actual package will be loaded lazily.
	pkg := &object.Package{
		Name: pkgName,
		Path: importPath,
		Env:  object.NewEnvironment(), // A new, empty env for the package symbols
	}
	env.Set(pkgName, pkg)
	return nil
}

func (e *Evaluator) evalSelectorExpr(n *ast.SelectorExpr, env *object.Environment) object.Object {
	e.logger.Debug("evalSelectorExpr", "selector", n.Sel.Name)
	// Evaluate the left-hand side of the selector (e.g., `http` in `http.HandleFunc`).
	left := e.Eval(n.X, env)
	if isError(left) {
		return left
	}
	e.logger.Debug("evalSelectorExpr: evaluated left", "type", left.Type(), "value", left.Inspect())

	// Handle method calls on symbolic instances.
	if inst, ok := left.(*object.Instance); ok {
		// e.g., TypeName="net/http.ServeMux", Sel.Name="HandleFunc"
		// constructs key "(*net/http.ServeMux).HandleFunc"
		key := fmt.Sprintf("(*%s).%s", inst.TypeName, n.Sel.Name)
		e.logger.Debug("evalSelectorExpr: looking for instance method intrinsic", "key", key)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			e.logger.Debug("evalSelectorExpr: found instance method intrinsic", "key", key)
			return &object.Intrinsic{Fn: intrinsicFn}
		}
	}

	// Check if the left-hand side is a package.
	pkg, ok := left.(*object.Package)
	if !ok {
		return newError("expected a package or an instance on the left side of selector, but got %s", left.Type())
	}

	// LAZY LOADING: If the package's environment is empty, it's a placeholder.
	// We need to load its symbols using the scanner.
	if pkg.Env.IsEmpty() {
		if e.scanner == nil {
			return newError("scanner is not available, cannot load package %q", pkg.Path)
		}
		pkgInfo, err := e.scanner.ScanPackageByImport(context.Background(), pkg.Path)
		if err != nil {
			return newError("could not scan package %q: %v", pkg.Path, err)
		}

		// Populate the package's environment with its exported symbols.
		for _, f := range pkgInfo.Functions {
			if ast.IsExported(f.Name) {
				// Check if there is a registered intrinsic for this function.
				key := pkgInfo.ImportPath + "." + f.Name
				if intrinsicFn, ok := e.intrinsics.Get(key); ok {
					pkg.Env.Set(f.Name, &object.Intrinsic{Fn: intrinsicFn})
				} else {
					pkg.Env.Set(f.Name, &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external func %s.%s", pkg.Name, f.Name)})
				}
			}
		}
		for _, v := range pkgInfo.Variables {
			if v.IsExported {
				pkg.Env.Set(v.Name, &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external var %s.%s", pkg.Name, v.Name)})
			}
		}
		for _, c := range pkgInfo.Constants {
			if c.IsExported {
				pkg.Env.Set(c.Name, &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external const %s.%s", pkg.Name, c.Name)})
			}
		}
		for _, t := range pkgInfo.Types {
			if ast.IsExported(t.Name) {
				pkg.Env.Set(t.Name, &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external type %s.%s", pkg.Name, t.Name)})
			}
		}
	}

	// Now that the package is loaded, look up the symbol.
	symbol, ok := pkg.Env.Get(n.Sel.Name)
	if !ok {
		return newError("undefined symbol: %s.%s", pkg.Name, n.Sel.Name)
	}

	return symbol
}

// evalSwitchStmt evaluates a switch statement. It traverses all case clauses
// to discover patterns that could occur in any branch.
func (e *Evaluator) evalSwitchStmt(n *ast.SwitchStmt, env *object.Environment) object.Object {
	// The result of a switch statement is the result of the last evaluated statement
	// in the taken branch. Since we evaluate all branches, we'll just return the
	// result of the last statement in the last case block for now.
	var result object.Object
	if n.Body != nil {
		for _, c := range n.Body.List {
			if caseClause, ok := c.(*ast.CaseClause); ok {
				// Each case block gets its own scope
				caseEnv := object.NewEnclosedEnvironment(env)
				for _, stmt := range caseClause.Body {
					result = e.Eval(stmt, caseEnv)
					// We don't break early on return/error here because we want
					// to analyze all branches. A more sophisticated implementation
					// might collect results from all branches.
				}
			}
		}
	}
	return result
}

// evalForStmt evaluates a for statement. Following the "bounded analysis" principle,
// it evaluates the loop body exactly once to find patterns within it.
// It ignores the loop's condition, initializer, and post-iteration statement.
func (e *Evaluator) evalForStmt(n *ast.ForStmt, env *object.Environment) object.Object {
	// The body of the for loop has its own scope.
	bodyEnv := object.NewEnclosedEnvironment(env)
	return e.Eval(n.Body, bodyEnv)
}

// evalIfStmt evaluates an if statement. Following our heuristic-based approach,
// it evaluates the body to see what *could* happen, without complex path forking.
// For simplicity, it currently ignores the condition and the else block.
func (e *Evaluator) evalIfStmt(n *ast.IfStmt, env *object.Environment) object.Object {
	// The body of the if statement has its own scope.
	bodyEnv := object.NewEnclosedEnvironment(env)
	return e.Eval(n.Body, bodyEnv)
}

func (e *Evaluator) evalBlockStatement(block *ast.BlockStmt, env *object.Environment) object.Object {
	var result object.Object
	blockEnv := object.NewEnclosedEnvironment(env)

	for _, stmt := range block.List {
		result = e.Eval(stmt, blockEnv)

		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
		}
	}

	return result
}

func (e *Evaluator) evalReturnStmt(n *ast.ReturnStmt, env *object.Environment) object.Object {
	if len(n.Results) != 1 {
		// For now, we only support single return values.
		return newError("unsupported return statement: expected 1 result")
	}
	val := e.Eval(n.Results[0], env)
	if isError(val) {
		return val
	}
	return &object.ReturnValue{Value: val}
}

func (e *Evaluator) evalAssignStmt(n *ast.AssignStmt, env *object.Environment) object.Object {
	// For now, we only support simple assignment like `x = ...`
	if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
		return newError("unsupported assignment: expected 1 expression on each side")
	}

	// We only support assigning to an identifier.
	ident, ok := n.Lhs[0].(*ast.Ident)
	if !ok {
		return newError("unsupported assignment target: expected an identifier")
	}

	// Evaluate the right-hand side.
	val := e.Eval(n.Rhs[0], env)
	if isError(val) {
		return val
	}

	// We only support `=` for now, not `:=`.
	// This means the variable must already exist in the scope.
	if _, ok := env.Get(ident.Name); !ok {
		// TODO: This check might be too restrictive. For now, we allow setting new variables.
		// return newError("cannot assign to undeclared identifier: %s", ident.Name)
	}

	// Set the value in the scope.
	e.logger.Debug("evalAssignStmt: setting var", "name", ident.Name, "type", val.Type())
	return env.Set(ident.Name, val)
}

func (e *Evaluator) evalBasicLit(n *ast.BasicLit) object.Object {
	switch n.Kind {
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return newError("could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	// TODO: Add support for INT, FLOAT, etc. later.
	default:
		return newError("unsupported literal type: %s", n.Kind)
	}
}

func (e *Evaluator) evalIdent(n *ast.Ident, env *object.Environment) object.Object {
	if val, ok := env.Get(n.Name); ok {
		e.logger.Debug("evalIdent: found in env", "name", n.Name, "type", val.Type())
		return val
	}
	e.logger.Debug("evalIdent: not found in env", "name", n.Name)
	return newError("identifier not found: %s", n.Name)
}

// newError is a helper to create a new Error object.
func newError(format string, args ...interface{}) *object.Error {
	return &object.Error{Message: fmt.Sprintf(format, args...)}
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

func (e *Evaluator) evalCallExpr(n *ast.CallExpr, env *object.Environment) object.Object {
	// 1. Evaluate the function itself (e.g., the identifier or selector).
	function := e.Eval(n.Fun, env)
	if isError(function) {
		return function
	}

	// 2. Evaluate the arguments.
	args := e.evalExpressions(n.Args, env)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	// 3. Apply the function.
	return e.applyFunction(function, args)
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

func (e *Evaluator) applyFunction(fn object.Object, args []object.Object) object.Object {
	e.logger.Debug("applyFunction", "type", fn.Type(), "value", fn.Inspect())
	switch fn := fn.(type) {
	case *object.Function:
		// This is a user-defined function.
		// Create a new environment for the function call, enclosed by the function's definition environment.
		extendedEnv := e.extendFunctionEnv(fn, args)
		// Evaluate the function's body in the new environment.
		evaluated := e.Eval(fn.Body, extendedEnv)
		// Unwrap the return value.
		if returnValue, ok := evaluated.(*object.ReturnValue); ok {
			return returnValue.Value
		}
		return evaluated

	case *object.Intrinsic:
		// This is a built-in function.
		return fn.Fn(args...)

	case *object.SymbolicPlaceholder:
		// Calling an external or unknown function results in a symbolic value.
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling %s", fn.Inspect())}

	default:
		return newError("not a function: %s", fn.Type())
	}
}

func (e *Evaluator) extendFunctionEnv(fn *object.Function, args []object.Object) *object.Environment {
	// Create a new scope enclosed by the function's *definition* scope, not the *call site* scope.
	// This ensures lexical scoping.
	env := object.NewEnclosedEnvironment(fn.Env)

	// Bind the arguments to the parameter names.
	for i, param := range fn.Parameters.List {
		if i < len(args) {
			// Each parameter can have multiple names (e.g., `a, b int`).
			for _, name := range param.Names {
				env.Set(name.Name, args[i])
			}
		}
	}

	return env
}
