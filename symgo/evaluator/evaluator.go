package evaluator

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"log/slog"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	scanner    *scanner.Scanner
	intrinsics *intrinsics.Registry
	logger     *slog.Logger
	callStack  []*callFrame
}

type callFrame struct {
	Name string
	Pos  token.Pos
}

// New creates a new Evaluator.
func New(scanner *scanner.Scanner, logger *slog.Logger) *Evaluator {
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

// PushIntrinsics creates a new temporary scope for intrinsics.
func (e *Evaluator) PushIntrinsics() {
	e.intrinsics.Push()
}

// PopIntrinsics removes the top-most temporary scope for intrinsics.
func (e *Evaluator) PopIntrinsics() {
	e.intrinsics.Pop()
}

// Eval is the main dispatch loop for the evaluator.
func (e *Evaluator) Eval(node ast.Node, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	switch n := node.(type) {
	case *ast.File:
		return e.evalFile(n, env, pkg)
	case *ast.SelectorExpr:
		return e.evalSelectorExpr(n, env, pkg)
	case *ast.BasicLit:
		return e.evalBasicLit(n)
	case *ast.Ident:
		return e.evalIdent(n, env, pkg)
	case *ast.AssignStmt:
		return e.evalAssignStmt(n, env, pkg)
	case *ast.BlockStmt:
		return e.evalBlockStatement(n, env, pkg)
	case *ast.ReturnStmt:
		return e.evalReturnStmt(n, env, pkg)
	case *ast.IfStmt:
		return e.evalIfStmt(n, env, pkg)
	case *ast.ForStmt:
		return e.evalForStmt(n, env, pkg)
	case *ast.SwitchStmt:
		return e.evalSwitchStmt(n, env, pkg)
	case *ast.CallExpr:
		return e.evalCallExpr(n, env, pkg)
	case *ast.ExprStmt:
		return e.Eval(n.X, env, pkg)
	case *ast.DeclStmt:
		return e.Eval(n.Decl, env, pkg)
	case *ast.GenDecl:
		return e.evalGenDecl(n, env, pkg)
	case *ast.UnaryExpr:
		return e.evalUnaryExpr(n, env, pkg)
	case *ast.BinaryExpr:
		return e.evalBinaryExpr(n, env, pkg)
	case *ast.CompositeLit:
		return e.evalCompositeLit(n, env, pkg)
	}
	return newError("evaluation not implemented for %T", node)
}

func (e *Evaluator) evalCompositeLit(node *ast.CompositeLit, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if pkg == nil || pkg.Fset == nil {
		return newError("package info or fset is missing, cannot resolve types for composite literal")
	}

	// Build an import lookup specific to the file containing the literal.
	file := pkg.Fset.File(node.Pos())
	if file == nil {
		return newError("could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return newError("could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	// Resolve the type of the literal.
	typeInfo := e.scanner.TypeInfoFromExpr(context.Background(), node.Type, nil, pkg, importLookup)
	if typeInfo == nil {
		// For convenience, try to render the type expression for a better error message.
		var typeNameBuf bytes.Buffer
		printer.Fprint(&typeNameBuf, pkg.Fset, node.Type)
		return newError("could not resolve type for composite literal: %s", typeNameBuf.String())
	}

	// For types defined in the current package, FullImportPath might be empty.
	// In that case, we use the package's own import path to create a fully qualified name.
	pkgPath := typeInfo.FullImportPath
	if pkgPath == "" {
		pkgPath = pkg.ImportPath
	}
	typeName := fmt.Sprintf("%s.%s", pkgPath, typeInfo.Name)

	resolvedType, _ := typeInfo.Resolve(context.Background())

	// Create a symbolic instance of this type.
	return &object.Instance{
		TypeName:   typeName,
		BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType},
	}
}

func (e *Evaluator) evalBinaryExpr(node *ast.BinaryExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	left := e.Eval(node.X, env, pkg)
	if isError(left) {
		return left
	}
	right := e.Eval(node.Y, env, pkg)
	if isError(right) {
		return right
	}

	// Handle string concatenation
	if node.Op == token.ADD {
		if left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ {
			leftVal := left.(*object.String).Value
			rightVal := right.(*object.String).Value
			return &object.String{Value: leftVal + rightVal}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "binary expression"}
}

func (e *Evaluator) evalUnaryExpr(node *ast.UnaryExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	switch node.Op {
	case token.AND:
		val := e.Eval(node.X, env, pkg)
		if isError(val) {
			return val
		}
		// The pointer object should carry the type information of the value it points to.
		ptr := &object.Pointer{Value: val}
		typeInfo := val.TypeInfo()
		if typeInfo != nil {
			e.logger.Debug("evalUnaryExpr: attaching type to pointer", "type", typeInfo.Name)
		} else {
			e.logger.Debug("evalUnaryExpr: type info for pointer value is nil")
		}
		ptr.ResolvedTypeInfo = typeInfo
		return ptr
	}
	return newError("unknown unary operator: %s", node.Op)
}

func (e *Evaluator) evalGenDecl(node *ast.GenDecl, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if node.Tok != token.VAR {
		return nil // We only care about var declarations for now.
	}

	// Find the AST file that contains the current declaration node.
	if pkg == nil || pkg.Fset == nil {
		return newError("package info or fset is missing, cannot resolve types")
	}
	file := pkg.Fset.File(node.Pos())
	if file == nil {
		return newError("could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return newError("could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	for _, spec := range node.Specs {
		valSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		var resolvedTypeInfo *scanner.TypeInfo
		if valSpec.Type != nil {
			fieldType := e.scanner.TypeInfoFromExpr(context.Background(), valSpec.Type, nil, pkg, importLookup)
			if fieldType != nil {
				// We resolve the type to get the full definition.
				// The error is ignored for now, as it might be a type from an un-scanned package.
				resolvedTypeInfo, _ = fieldType.Resolve(context.Background())
			}
		}

		for _, name := range valSpec.Names {
			if resolvedTypeInfo != nil {
				e.logger.Debug("evalGenDecl: resolved type for var", "var", name.Name, "type", resolvedTypeInfo.Name)
			} else {
				e.logger.Debug("evalGenDecl: could not resolve type for var", "var", name.Name)
			}
			v := &object.Variable{
				Name: name.Name,
				BaseObject: object.BaseObject{
					ResolvedTypeInfo: resolvedTypeInfo,
				},
				Value: &object.SymbolicPlaceholder{Reason: "uninitialized variable"},
			}
			env.Set(name.Name, v)
		}
	}
	return nil
}

func (e *Evaluator) evalFile(file *ast.File, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// Top-level declarations
	// First, handle all imports to populate the environment with package placeholders.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				for _, spec := range d.Specs {
					e.evalImportSpec(spec, env)
				}
			} else if d.Tok == token.VAR {
				e.evalGenDecl(d, env, pkg)
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

func (e *Evaluator) evalSelectorExpr(n *ast.SelectorExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	e.logger.Debug("evalSelectorExpr", "selector", n.Sel.Name)

	// Special handling for identifiers on the LHS of a selector expression.
	// We need the raw variable object, not its unwrapped value, to access type info for field/method resolution.
	var left object.Object
	if ident, ok := n.X.(*ast.Ident); ok {
		if obj, found := env.Get(ident.Name); found {
			left = obj
		} else {
			// It might be a package name that's not in the env yet, let the normal flow handle it.
			left = e.Eval(n.X, env, pkg)
		}
	} else {
		// Evaluate the left-hand side of the selector for non-identifier expressions.
		left = e.Eval(n.X, env, pkg)
	}

	if isError(left) {
		return left
	}
	e.logger.Debug("evalSelectorExpr: evaluated left", "type", left.Type(), "value", left.Inspect())

	switch val := left.(type) {
	case *object.SymbolicPlaceholder:
		typeInfo := val.TypeInfo()
		if typeInfo == nil {
			return newError("cannot call method on symbolic placeholder with no type info")
		}

		// This logic is similar to Instance, but constructs the type name from TypeInfo
		fullTypeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)

		// Check for pointer receiver: (*T).Method
		key := fmt.Sprintf("(*%s).%s", fullTypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		// Check for value receiver: (T).Method
		key = fmt.Sprintf("(%s).%s", fullTypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		return newError("undefined method: %s on symbolic type %s", n.Sel.Name, fullTypeName)

	case *object.Package:
		// Check for a direct intrinsic on the package first.
		key := val.Path + "." + n.Sel.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return &object.Intrinsic{Fn: intrinsicFn}
		}

		// If no direct intrinsic, proceed with lazy loading.
		if val.Env.IsEmpty() {
			if e.scanner == nil {
				return newError("scanner is not available, cannot load package %q", val.Path)
			}
			pkgInfo, err := e.scanner.ScanPackageByImport(context.Background(), val.Path)
			if err != nil {
				return newError("could not scan package %q: %v", val.Path, err)
			}

			for _, f := range pkgInfo.Functions {
				if ast.IsExported(f.Name) {
					// Don't overwrite intrinsics that might have been found above.
					if _, ok := val.Env.Get(f.Name); !ok {
						val.Env.Set(f.Name, &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external func %s.%s", val.Name, f.Name)})
					}
				}
			}
			// Simplified for brevity, would populate other symbols too.
		}

		// Now that the package is loaded (or was already loaded), look up the symbol.
		symbol, ok := val.Env.Get(n.Sel.Name)
		if !ok {
			// If symbol not found, check for intrinsic again, as lazy loading might have populated it.
			key := val.Path + "." + n.Sel.Name
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				return &object.Intrinsic{Fn: intrinsicFn}
			}
			return newError("undefined symbol: %s.%s", val.Name, n.Sel.Name)
		}
		return symbol

	case *object.Instance:
		// Check for value receiver: (T).Method
		key := fmt.Sprintf("(%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			// The first argument to a method call intrinsic is the receiver itself.
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		// Check for pointer receiver: (*T).Method
		key = fmt.Sprintf("(*%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			// The first argument to a method call intrinsic is the receiver itself.
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		return newError("undefined method: %s on %s", n.Sel.Name, val.TypeName)

	case *object.Variable:
		// The selector could be a method call on the variable's type (struct or interface)
		// or a field access on a struct.

		typeInfo := val.TypeInfo()
		if typeInfo == nil {
			return newError("cannot access field or method on variable with no type info: %s", val.Name)
		}

		// First, try to resolve as a method call on the variable's type.
		// This handles both interface methods and struct methods.
		if typeInfo.Kind == scanner.InterfaceKind || typeInfo.Kind == scanner.StructKind {
			fullTypeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)

			// Check for pointer receiver method: (*T).MethodName
			key := fmt.Sprintf("(*%s).%s", fullTypeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val
				fn := func(args ...object.Object) object.Object {
					return intrinsicFn(append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}

			// Check for value receiver method: (T).MethodName
			key = fmt.Sprintf("(%s).%s", fullTypeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val
				fn := func(args ...object.Object) object.Object {
					return intrinsicFn(append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}
		}

		// If it's not a method, try to resolve as a struct field access.
		if typeInfo.Struct != nil {
			for _, field := range typeInfo.Struct.Fields {
				if field.Name == n.Sel.Name {
					fieldTypeInfo, _ := field.Type.Resolve(context.Background())
					return &object.SymbolicPlaceholder{
						BaseObject: object.BaseObject{ResolvedTypeInfo: fieldTypeInfo},
						Reason:     fmt.Sprintf("field access %s.%s", val.Name, field.Name),
					}
				}
			}
		}

		// Finally, check for methods on the underlying value if it's an instance.
		if instance, ok := val.Value.(*object.Instance); ok {
			key := fmt.Sprintf("(*%s).%s", instance.TypeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val.Value
				fn := func(args ...object.Object) object.Object {
					return intrinsicFn(append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}
		}

		return newError("undefined field or method: %s on %s", n.Sel.Name, val.Inspect())

	default:
		return newError("expected a package, instance, or variable on the left side of selector, but got %s", left.Type())
	}
}

// evalSwitchStmt evaluates a switch statement. It traverses all case clauses
// to discover patterns that could occur in any branch.
func (e *Evaluator) evalSwitchStmt(n *ast.SwitchStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
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
					result = e.Eval(stmt, caseEnv, pkg)
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
func (e *Evaluator) evalForStmt(n *ast.ForStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// The body of the for loop has its own scope.
	bodyEnv := object.NewEnclosedEnvironment(env)
	return e.Eval(n.Body, bodyEnv, pkg)
}

// evalIfStmt evaluates an if statement. Following our heuristic-based approach,
// it evaluates the body to see what *could* happen, without complex path forking.
// For simplicity, it currently ignores the condition and the else block.
func (e *Evaluator) evalIfStmt(n *ast.IfStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// The body of the if statement has its own scope.
	bodyEnv := object.NewEnclosedEnvironment(env)
	return e.Eval(n.Body, bodyEnv, pkg)
}

func (e *Evaluator) evalBlockStatement(block *ast.BlockStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	var result object.Object
	blockEnv := object.NewEnclosedEnvironment(env)

	for _, stmt := range block.List {
		result = e.Eval(stmt, blockEnv, pkg)

		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
		}
	}

	return result
}

func (e *Evaluator) evalReturnStmt(n *ast.ReturnStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if len(n.Results) > 1 {
		// For now, we only support single return values.
		return newError("unsupported return statement: expected 1 result")
	}
	val := e.Eval(n.Results[0], env, pkg)
	if isError(val) {
		return val
	}
	return &object.ReturnValue{Value: val}
}

func (e *Evaluator) evalAssignStmt(n *ast.AssignStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
		return newError("unsupported assignment: expected 1 expression on each side")
	}

	ident, ok := n.Lhs[0].(*ast.Ident)
	if !ok {
		return newError("unsupported assignment target: expected an identifier")
	}

	if ident.Name == "_" {
		return e.Eval(n.Rhs[0], env, pkg)
	}

	val := e.Eval(n.Rhs[0], env, pkg)
	if isError(val) {
		return val
	}

	if n.Tok == token.DEFINE {
		// This is `:=`, so we are declaring and assigning.
		// We wrap the value in a Variable object to be consistent with `var` declarations.
		v := &object.Variable{
			Name:  ident.Name,
			Value: val,
			BaseObject: object.BaseObject{
				ResolvedTypeInfo: val.TypeInfo(), // Carry over type info from the value
			},
		}
		e.logger.Debug("evalAssignStmt: defining var", "name", ident.Name, "type", val.Type())
		return env.Set(ident.Name, v)
	}

	// This is `=`, so the variable must already exist.
	// We need to find the variable and update its value.
	obj, ok := env.Get(ident.Name)
	if !ok {
		return newError("cannot assign to undeclared identifier: %s", ident.Name)
	}

	if v, ok := obj.(*object.Variable); ok {
		v.Value = val
		// Also update the type info if the new value provides a more specific type.
		if val.TypeInfo() != nil {
			v.ResolvedTypeInfo = val.TypeInfo()
		}
		e.logger.Debug("evalAssignStmt: updating var", "name", ident.Name, "type", val.Type())
		return v
	}

	// Fallback for non-variable assignments, though our model should primarily use variables.
	e.logger.Debug("evalAssignStmt: setting non-var", "name", ident.Name, "type", val.Type())
	return env.Set(ident.Name, val)
}

func (e *Evaluator) evalBasicLit(n *ast.BasicLit) object.Object {
	switch n.Kind {
	case token.INT:
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return newError("could not parse %q as integer", n.Value)
		}
		return &object.Integer{Value: i}
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return newError("could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	default:
		return newError("unsupported literal type: %s", n.Kind)
	}
}

func (e *Evaluator) evalIdent(n *ast.Ident, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// Check for intrinsic first to allow overriding package-local functions for testing.
	if pkg != nil {
		key := pkg.ImportPath + "." + n.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			e.logger.Debug("evalIdent: found intrinsic, overriding", "key", key)
			return &object.Intrinsic{Fn: intrinsicFn}
		}
	}

	if val, ok := env.Get(n.Name); ok {
		e.logger.Debug("evalIdent: found in env", "name", n.Name, "type", val.Type())
		if v, ok := val.(*object.Variable); ok {
			// When an identifier is evaluated as an expression, we want its value,
			// not the variable container itself.
			value := v.Value
			// However, the variable might have more precise type info than the value it contains.
			// We should propagate this type info to the value before returning it.
			if value.TypeInfo() == nil && v.TypeInfo() != nil {
				value.SetTypeInfo(v.TypeInfo())
			}
			return value
		}
		return val
	}

	if n.Name == "nil" {
		return &object.Nil{}
	}

	e.logger.Debug("evalIdent: not found in env or intrinsics", "name", n.Name)
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

func (e *Evaluator) evalCallExpr(n *ast.CallExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// Logging for call tracing
	var name string
	if pkg != nil && pkg.Fset != nil {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, pkg.Fset, n.Fun); err == nil {
			name = buf.String()
		}
	}
	if name == "" {
		name = "unknown" // fallback
	}

	frame := &callFrame{Name: name, Pos: n.Pos()}
	e.callStack = append(e.callStack, frame)
	defer func() {
		e.callStack = e.callStack[:len(e.callStack)-1]
	}()

	var stackNames []string
	for _, f := range e.callStack {
		stackNames = append(stackNames, f.Name)
	}
	e.logger.Log(context.Background(), slog.LevelDebug, "call", "stack", strings.Join(stackNames, " -> "))

	// 1. Evaluate the function itself (e.g., the identifier or selector).
	function := e.Eval(n.Fun, env, pkg)
	if isError(function) {
		return function
	}

	// 2. Evaluate the arguments.
	args := e.evalExpressions(n.Args, env, pkg)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	// 3. Apply the function.
	result := e.applyFunction(function, args, pkg)
	if isError(result) {
		return result
	}
	return result
}

func (e *Evaluator) evalExpressions(exps []ast.Expr, env *object.Environment, pkg *scanner.PackageInfo) []object.Object {
	var result []object.Object

	for _, exp := range exps {
		evaluated := e.Eval(exp, env, pkg)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

func (e *Evaluator) Apply(fn object.Object, args []object.Object, pkg *scanner.PackageInfo) object.Object {
	return e.applyFunction(fn, args, pkg)
}

func (e *Evaluator) applyFunction(fn object.Object, args []object.Object, pkg *scanner.PackageInfo) object.Object {
	e.logger.Debug("applyFunction", "type", fn.Type(), "value", fn.Inspect())
	switch fn := fn.(type) {
	case *object.Function:
		// This is a user-defined function.
		// Create a new environment for the function call, enclosed by the function's definition environment.
		extendedEnv := e.extendFunctionEnv(fn, args)
		// Evaluate the function's body in the new environment.
		evaluated := e.Eval(fn.Body, extendedEnv, pkg)
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
