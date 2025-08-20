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

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	scanner           *goscan.Scanner
	intrinsics        *intrinsics.Registry
	logger            *slog.Logger
	callStack         []*callFrame
	interfaceBindings map[string]*goscan.TypeInfo
}

type callFrame struct {
	Name string
	Pos  token.Pos
}

// New creates a new Evaluator.
func New(scanner *goscan.Scanner, logger *slog.Logger) *Evaluator {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return &Evaluator{
		scanner:           scanner,
		intrinsics:        intrinsics.New(),
		logger:            logger,
		interfaceBindings: make(map[string]*goscan.TypeInfo),
	}
}

// BindInterface registers a concrete type for an interface.
func (e *Evaluator) BindInterface(ifaceTypeName string, concreteType *goscan.TypeInfo) {
	e.interfaceBindings[ifaceTypeName] = concreteType
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
func (e *Evaluator) Eval(ctx context.Context, node ast.Node, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if e.logger.Enabled(ctx, slog.LevelDebug) {
		var buf bytes.Buffer
		fset := e.scanner.Fset()
		if fset != nil && node != nil && node.Pos().IsValid() {
			printer.Fprint(&buf, fset, node)
		} else if node != nil {
			printer.Fprint(&buf, nil, node)
		}

		if pkg != nil && pkg.Fset != nil && node != nil && node.Pos().IsValid() {
			e.logger.Debug("evaluating node",
				"type", fmt.Sprintf("%T", node),
				"pos", pkg.Fset.Position(node.Pos()),
				"source", buf.String(),
			)
		}
	}

	switch n := node.(type) {
	case *ast.File:
		return e.evalFile(ctx, n, env, pkg)
	case *ast.SelectorExpr:
		return e.evalSelectorExpr(ctx, n, env, pkg)
	case *ast.BasicLit:
		return e.evalBasicLit(n)
	case *ast.Ident:
		return e.evalIdent(ctx, n, env, pkg)
	case *ast.AssignStmt:
		return e.evalAssignStmt(ctx, n, env, pkg)
	case *ast.BlockStmt:
		return e.evalBlockStatement(ctx, n, env, pkg)
	case *ast.ReturnStmt:
		return e.evalReturnStmt(ctx, n, env, pkg)
	case *ast.IfStmt:
		return e.evalIfStmt(ctx, n, env, pkg)
	case *ast.ForStmt:
		return e.evalForStmt(ctx, n, env, pkg)
	case *ast.SwitchStmt:
		return e.evalSwitchStmt(ctx, n, env, pkg)
	case *ast.CallExpr:
		return e.evalCallExpr(ctx, n, env, pkg)
	case *ast.ExprStmt:
		return e.Eval(ctx, n.X, env, pkg)
	case *ast.DeclStmt:
		return e.Eval(ctx, n.Decl, env, pkg)
	case *ast.GenDecl:
		return e.evalGenDecl(ctx, n, env, pkg)
	case *ast.UnaryExpr:
		return e.evalUnaryExpr(ctx, n, env, pkg)
	case *ast.BinaryExpr:
		return e.evalBinaryExpr(ctx, n, env, pkg)
	case *ast.CompositeLit:
		return e.evalCompositeLit(ctx, n, env, pkg)
	case *ast.IndexExpr:
		return e.evalIndexExpr(ctx, n, env, pkg)
	case *ast.FuncLit:
		return &object.Function{
			Parameters: n.Type.Params,
			Body:       n.Body,
			Env:        env,
		}
	}
	return newError(node.Pos(), "evaluation not implemented for %T", node)
}

func (e *Evaluator) evalIndexExpr(ctx context.Context, node *ast.IndexExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}

	index := e.Eval(ctx, node.Index, env, pkg)
	if isError(index) {
		return index
	}

	var sliceFieldType *scanner.FieldType

	switch l := left.(type) {
	case *object.Slice:
		sliceFieldType = l.FieldType
	case *object.Variable:
		if s, ok := l.Value.(*object.Slice); ok {
			sliceFieldType = s.FieldType
		} else if ti := l.TypeInfo(); ti != nil && ti.Underlying != nil && ti.Underlying.IsSlice {
			sliceFieldType = ti.Underlying
		}
	case *object.SymbolicPlaceholder:
		if ti := l.TypeInfo(); ti != nil && ti.Underlying != nil && ti.Underlying.IsSlice {
			sliceFieldType = ti.Underlying
		}
	}

	var elemType *scanner.TypeInfo
	if sliceFieldType != nil && sliceFieldType.IsSlice && sliceFieldType.Elem != nil {
		elemType, _ = sliceFieldType.Elem.Resolve(ctx)
	}

	return &object.SymbolicPlaceholder{
		Reason:     "result of index expression",
		BaseObject: object.BaseObject{ResolvedTypeInfo: elemType},
	}
}

func (e *Evaluator) evalCompositeLit(ctx context.Context, node *ast.CompositeLit, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if pkg == nil || pkg.Fset == nil {
		return newError(node.Pos(), "package info or fset is missing, cannot resolve types for composite literal")
	}

	file := pkg.Fset.File(node.Pos())
	if file == nil {
		return newError(node.Pos(), "could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return newError(node.Pos(), "could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	fieldType := e.scanner.TypeInfoFromExpr(ctx, node.Type, nil, pkg, importLookup)
	if fieldType == nil {
		var typeNameBuf bytes.Buffer
		printer.Fprint(&typeNameBuf, pkg.Fset, node.Type)
		return newError(node.Pos(), "could not resolve type for composite literal: %s", typeNameBuf.String())
	}

	if fieldType.IsSlice {
		return &object.Slice{FieldType: fieldType}
	}

	resolvedType, _ := fieldType.Resolve(ctx)
	if resolvedType == nil {
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("unresolved composite literal of type %s", fieldType.String()),
		}
	}

	return &object.Instance{
		TypeName:   fmt.Sprintf("%s.%s", resolvedType.PkgPath, resolvedType.Name),
		BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType},
	}
}

func (e *Evaluator) evalBinaryExpr(ctx context.Context, node *ast.BinaryExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}
	right := e.Eval(ctx, node.Y, env, pkg)
	if isError(right) {
		return right
	}

	if left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ {
		return e.evalIntegerInfixExpression(node.Op, left, right)
	}
	if left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ {
		return e.evalStringInfixExpression(node.Op, left, right)
	}

	return &object.SymbolicPlaceholder{Reason: "binary expression"}
}

func (e *Evaluator) evalIntegerInfixExpression(op token.Token, left, right object.Object) object.Object {
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
		return &object.Integer{Value: leftVal / rightVal}
	default:
		return newError(token.NoPos, "unknown integer operator: %s", op)
	}
}

func (e *Evaluator) evalStringInfixExpression(op token.Token, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	if op != token.ADD {
		return newError(token.NoPos, "unknown string operator: %s", op)
	}
	return &object.String{Value: leftVal + rightVal}
}

func (e *Evaluator) evalUnaryExpr(ctx context.Context, node *ast.UnaryExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	switch node.Op {
	case token.AND:
		val := e.Eval(ctx, node.X, env, pkg)
		if isError(val) {
			return val
		}
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
	return newError(node.Pos(), "unknown unary operator: %s", node.Op)
}

func (e *Evaluator) evalGenDecl(ctx context.Context, node *ast.GenDecl, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if node.Tok != token.VAR {
		return nil
	}

	if pkg == nil || pkg.Fset == nil {
		return newError(node.Pos(), "package info or fset is missing, cannot resolve types")
	}
	file := pkg.Fset.File(node.Pos())
	if file == nil {
		return newError(node.Pos(), "could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return newError(node.Pos(), "could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	for _, spec := range node.Specs {
		valSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		var resolvedTypeInfo *scanner.TypeInfo
		if valSpec.Type != nil {
			fieldType := e.scanner.TypeInfoFromExpr(ctx, valSpec.Type, nil, pkg, importLookup)
			if fieldType != nil {
				resolvedTypeInfo, _ = fieldType.Resolve(ctx)
			}
		}

		for i, name := range valSpec.Names {
			var val object.Object = &object.SymbolicPlaceholder{Reason: "uninitialized variable"}
			if i < len(valSpec.Values) {
				val = e.Eval(ctx, valSpec.Values[i], env, pkg)
				if isError(val) {
					return val
				}
			}

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
				Value: val,
			}
			env.Set(name.Name, v)
		}
	}
	return nil
}

func (e *Evaluator) evalFile(ctx context.Context, file *ast.File, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				for _, spec := range d.Specs {
					e.evalImportSpec(spec, env)
				}
			} else if d.Tok == token.VAR {
				e.evalGenDecl(ctx, d, env, pkg)
			}
		case *ast.FuncDecl:
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
	return nil
}

func (e *Evaluator) evalImportSpec(spec ast.Spec, env *object.Environment) object.Object {
	importSpec, ok := spec.(*ast.ImportSpec)
	if !ok {
		return nil
	}

	importPath, err := strconv.Unquote(importSpec.Path.Value)
	if err != nil {
		return newError(importSpec.Pos(), "invalid import path: %s", importSpec.Path.Value)
	}

	var pkgName string
	if importSpec.Name != nil {
		pkgName = importSpec.Name.Name
	} else {
		pkgName = path.Base(importPath)
	}

	pkg := &object.Package{
		Name: pkgName,
		Path: importPath,
		Env:  object.NewEnvironment(),
	}
	env.Set(pkgName, pkg)
	return nil
}

func (e *Evaluator) findPackageByPath(env *object.Environment, pkgPath string) (*object.Package, bool) {
	var foundPkg *object.Package
	env.Walk(func(name string, obj object.Object) bool {
		if p, ok := obj.(*object.Package); ok {
			if p.Path == pkgPath {
				foundPkg = p
				return false
			}
		}
		return true
	})
	if foundPkg != nil {
		return foundPkg, true
	}
	return nil, false
}

func (e *Evaluator) evalSelectorExpr(ctx context.Context, n *ast.SelectorExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	e.logger.Debug("evalSelectorExpr", "selector", n.Sel.Name)

	var left object.Object
	if ident, ok := n.X.(*ast.Ident); ok {
		if obj, found := env.Get(ident.Name); found {
			left = obj
		} else {
			left = e.Eval(ctx, n.X, env, pkg)
		}
	} else {
		left = e.Eval(ctx, n.X, env, pkg)
	}

	if isError(left) {
		return left
	}
	e.logger.Debug("evalSelectorExpr: evaluated left", "type", left.Type(), "value", left.Inspect())

	switch val := left.(type) {
	case *object.SymbolicPlaceholder:
		typeInfo := val.TypeInfo()
		if typeInfo == nil {
			return newError(n.Pos(), "cannot call method on symbolic placeholder with no type info")
		}
		fullTypeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
		key := fmt.Sprintf("(*%s).%s", fullTypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object { return intrinsicFn(append([]object.Object{self}, args...)...) }
			return &object.Intrinsic{Fn: fn}
		}
		key = fmt.Sprintf("(%s).%s", fullTypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object { return intrinsicFn(append([]object.Object{self}, args...)...) }
			return &object.Intrinsic{Fn: fn}
		}
		return newError(n.Pos(), "undefined method: %s on symbolic type %s", n.Sel.Name, fullTypeName)

	case *object.Package:
		key := val.Path + "." + n.Sel.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return &object.Intrinsic{Fn: intrinsicFn}
		}
		if val.ScannedInfo == nil {
			if e.scanner == nil {
				return newError(n.Pos(), "scanner is not available, cannot load package %q", val.Path)
			}
			pkgInfo, err := e.scanner.ScanPackageByImport(ctx, val.Path)
			if err != nil {
				return newError(n.Pos(), "could not scan package %q: %v", val.Path, err)
			}
			val.ScannedInfo = pkgInfo
		}
		if val.Env.IsEmpty() {
			for _, f := range val.ScannedInfo.Functions {
				if ast.IsExported(f.Name) {
					if _, ok := val.Env.Get(f.Name); !ok {
						val.Env.Set(f.Name, &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external func %s.%s", val.Name, f.Name)})
					}
				}
			}
			for _, c := range val.ScannedInfo.Constants {
				if ast.IsExported(c.Name) {
					if _, ok := val.Env.Get(c.Name); !ok {
						val.Env.Set(c.Name, &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external const %s.%s", val.Name, c.Name)})
					}
				}
			}
		}
		symbol, ok := val.Env.Get(n.Sel.Name)
		if !ok {
			key := val.Path + "." + n.Sel.Name
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				return &object.Intrinsic{Fn: intrinsicFn}
			}
			return newError(n.Pos(), "undefined symbol: %s.%s", val.Name, n.Sel.Name)
		}
		return symbol

	case *object.Instance:
		key := fmt.Sprintf("(%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object { return intrinsicFn(append([]object.Object{self}, args...)...) }
			return &object.Intrinsic{Fn: fn}
		}
		key = fmt.Sprintf("(*%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object { return intrinsicFn(append([]object.Object{self}, args...)...) }
			return &object.Intrinsic{Fn: fn}
		}
		return newError(n.Pos(), "undefined method: %s on %s", n.Sel.Name, val.TypeName)

	case *object.Variable:
		typeInfo := val.TypeInfo()
		if typeInfo == nil {
			return newError(n.Pos(), "cannot access field or method on variable with no type info: %s", val.Name)
		}

		if typeInfo.Kind == scanner.InterfaceKind {
			// Check for interface binding override
			qualifiedIfaceName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
			if boundType, ok := e.interfaceBindings[qualifiedIfaceName]; ok {
				e.logger.Debug("evalSelectorExpr: found interface binding", "interface", qualifiedIfaceName, "concrete", boundType.Name)
				typeInfo = boundType
			}

			resolutionPkg := pkg
			if typeInfo.PkgPath != "" && typeInfo.PkgPath != pkg.ImportPath {
				if foreignPkgObj, ok := e.findPackageByPath(env, typeInfo.PkgPath); ok {
					if foreignPkgObj.ScannedInfo == nil {
						scanned, err := e.scanner.ScanPackageByImport(ctx, foreignPkgObj.Path)
						if err != nil {
							return newError(n.Pos(), "failed to scan dependent package %s: %v", foreignPkgObj.Path, err)
						}
						foreignPkgObj.ScannedInfo = scanned
					}
					resolutionPkg = foreignPkgObj.ScannedInfo
				} else {
					scanned, err := e.scanner.ScanPackageByImport(ctx, typeInfo.PkgPath)
					if err != nil {
						return newError(n.Pos(), "failed to scan transitive dependency package %s: %v", typeInfo.PkgPath, err)
					}
					resolutionPkg = scanned
				}
			}
			var definitiveType *scanner.TypeInfo
			if resolutionPkg != nil {
				for _, t := range resolutionPkg.Types {
					if t.Name == typeInfo.Name {
						definitiveType = t
						break
					}
				}
			}
			if definitiveType != nil {
				typeInfo = definitiveType
			}
		}

		if typeInfo.Kind == scanner.InterfaceKind || typeInfo.Kind == scanner.StructKind {
			fullTypeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
			key := fmt.Sprintf("(*%s).%s", fullTypeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val
				fn := func(args ...object.Object) object.Object { return intrinsicFn(append([]object.Object{self}, args...)...) }
				return &object.Intrinsic{Fn: fn}
			}
			key = fmt.Sprintf("(%s).%s", fullTypeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val
				fn := func(args ...object.Object) object.Object { return intrinsicFn(append([]object.Object{self}, args...)...) }
				return &object.Intrinsic{Fn: fn}
			}
		}

		if typeInfo.Struct != nil {
			for _, field := range typeInfo.Struct.Fields {
				if field.Name == n.Sel.Name {
					fieldTypeInfo, _ := field.Type.Resolve(ctx)
					return &object.SymbolicPlaceholder{
						BaseObject: object.BaseObject{ResolvedTypeInfo: fieldTypeInfo},
						Reason:     fmt.Sprintf("field access %s.%s", val.Name, field.Name),
					}
				}
			}
		}

		if instance, ok := val.Value.(*object.Instance); ok {
			key := fmt.Sprintf("(*%s).%s", instance.TypeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val.Value
				fn := func(args ...object.Object) object.Object { return intrinsicFn(append([]object.Object{self}, args...)...) }
				return &object.Intrinsic{Fn: fn}
			}
		}

		return newError(n.Pos(), "undefined field or method: %s on %s", n.Sel.Name, val.Inspect())

	default:
		return newError(n.Pos(), "expected a package, instance, or variable on the left side of selector, but got %s", left.Type())
	}
}

func (e *Evaluator) evalSwitchStmt(ctx context.Context, n *ast.SwitchStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	var result object.Object
	if n.Body != nil {
		for _, c := range n.Body.List {
			if caseClause, ok := c.(*ast.CaseClause); ok {
				caseEnv := object.NewEnclosedEnvironment(env)
				for _, stmt := range caseClause.Body {
					result = e.Eval(ctx, stmt, caseEnv, pkg)
				}
			}
		}
	}
	return result
}

func (e *Evaluator) evalForStmt(ctx context.Context, n *ast.ForStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	bodyEnv := object.NewEnclosedEnvironment(env)
	return e.Eval(ctx, n.Body, bodyEnv, pkg)
}

func (e *Evaluator) evalIfStmt(ctx context.Context, n *ast.IfStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	bodyEnv := object.NewEnclosedEnvironment(env)
	result := e.Eval(ctx, n.Body, bodyEnv, pkg)

	if n.Else != nil {
		elseEnv := object.NewEnclosedEnvironment(env)
		result = e.Eval(ctx, n.Else, elseEnv, pkg)
	}

	return result
}

func (e *Evaluator) evalBlockStatement(ctx context.Context, block *ast.BlockStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	var result object.Object
	blockEnv := object.NewEnclosedEnvironment(env)

	for _, stmt := range block.List {
		result = e.Eval(ctx, stmt, blockEnv, pkg)

		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
		}
	}

	return result
}

func (e *Evaluator) evalReturnStmt(ctx context.Context, n *ast.ReturnStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if len(n.Results) == 0 {
		return nil // naked return
	}
	if len(n.Results) > 1 {
		return newError(n.Pos(), "unsupported return statement: expected 1 result")
	}
	val := e.Eval(ctx, n.Results[0], env, pkg)
	if isError(val) {
		return val
	}
	return &object.ReturnValue{Value: val}
}

func (e *Evaluator) evalAssignStmt(ctx context.Context, n *ast.AssignStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if len(n.Rhs) == 1 && len(n.Lhs) > 1 {
		rhsValue := e.Eval(ctx, n.Rhs[0], env, pkg)
		if isError(rhsValue) {
			return rhsValue
		}

		multiRet, ok := rhsValue.(*object.MultiReturn)
		if !ok {
			// If the RHS is not a multi-return object, we can't perform the assignment.
			// Fallback to assigning placeholders.
			for _, lhsExpr := range n.Lhs {
				if ident, ok := lhsExpr.(*ast.Ident); ok {
					if ident.Name == "_" {
						continue
					}
					v := &object.Variable{
						Name:  ident.Name,
						Value: &object.SymbolicPlaceholder{Reason: "unhandled multi-value assignment"},
					}
					env.Set(ident.Name, v)
				}
			}
			return nil
		}

		if len(multiRet.Values) != len(n.Lhs) {
			return newError(n.Pos(), "assignment mismatch: %d variables but %d values", len(n.Lhs), len(multiRet.Values))
		}

		for i, lhsExpr := range n.Lhs {
			if ident, ok := lhsExpr.(*ast.Ident); ok {
				if ident.Name == "_" {
					continue
				}
				val := multiRet.Values[i]
				e.assignIdentifier(ident, val, n.Tok, env)
			}
		}
		return nil
	}

	if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
		return newError(n.Pos(), "unsupported assignment: expected 1 expression on each side, or multi-value assignment")
	}

	switch lhs := n.Lhs[0].(type) {
	case *ast.Ident:
		if lhs.Name == "_" {
			return e.Eval(ctx, n.Rhs[0], env, pkg)
		}
		return e.evalIdentAssignment(ctx, lhs, n.Rhs[0], n.Tok, env, pkg)
	case *ast.SelectorExpr:
		return nil
	default:
		return newError(n.Pos(), "unsupported assignment target: expected an identifier or selector, but got %T", lhs)
	}
}

func (e *Evaluator) evalIdentAssignment(ctx context.Context, ident *ast.Ident, rhs ast.Expr, tok token.Token, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	val := e.Eval(ctx, rhs, env, pkg)
	if isError(val) {
		return val
	}
	return e.assignIdentifier(ident, val, tok, env)
}

func (e *Evaluator) assignIdentifier(ident *ast.Ident, val object.Object, tok token.Token, env *object.Environment) object.Object {
	if tok == token.DEFINE || tok == token.ASSIGN {
		// For :=, we define a new variable.
		// For =, we update an existing one.
		// If the variable doesn't exist with =, we'll handle it below.
		if tok == token.DEFINE {
			v := &object.Variable{
				Name:  ident.Name,
				Value: val,
				BaseObject: object.BaseObject{
					ResolvedTypeInfo: val.TypeInfo(),
				},
			}
			e.logger.Debug("evalAssignStmt: defining var", "name", ident.Name, "type", val.Type())
			return env.Set(ident.Name, v)
		}
	}

	// Handle plain assignment (=)
	obj, ok := env.Get(ident.Name)
	if !ok {
		// This case can be hit in `x, _ = myFunc()` where `x` is a package-level var.
		// `Get` won't find it if it's not in the current function's env stack.
		// We'll define it here for simplicity. A more robust solution might search outer scopes more explicitly.
		v := &object.Variable{
			Name:  ident.Name,
			Value: val,
			BaseObject: object.BaseObject{
				ResolvedTypeInfo: val.TypeInfo(),
			},
		}
		e.logger.Debug("evalAssignStmt: defining global-like var", "name", ident.Name, "type", val.Type())
		return env.Set(ident.Name, v)
	}

	if v, ok := obj.(*object.Variable); ok {
		v.Value = val
		if val.TypeInfo() != nil {
			v.ResolvedTypeInfo = val.TypeInfo()
		}
		e.logger.Debug("evalAssignStmt: updating var", "name", ident.Name, "type", val.Type())
		return v
	}

	// This case handles assignment to non-variable objects, like map keys, if they were supported.
	e.logger.Debug("evalAssignStmt: setting non-var", "name", ident.Name, "type", val.Type())
	return env.Set(ident.Name, val)
}

func (e *Evaluator) evalBasicLit(n *ast.BasicLit) object.Object {
	switch n.Kind {
	case token.INT:
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return newError(n.Pos(), "could not parse %q as integer", n.Value)
		}
		return &object.Integer{Value: i}
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return newError(n.Pos(), "could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	default:
		return newError(n.Pos(), "unsupported literal type: %s", n.Kind)
	}
}

func (e *Evaluator) evalIdent(ctx context.Context, n *ast.Ident, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
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
			value := v.Value
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
	return newError(n.Pos(), "identifier not found: %s", n.Name)
}

func newError(pos token.Pos, format string, args ...interface{}) *object.Error {
	return &object.Error{
		Message: fmt.Sprintf(format, args...),
		Pos:     pos,
	}
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

func (e *Evaluator) evalCallExpr(ctx context.Context, n *ast.CallExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	var name string
	if pkg != nil && pkg.Fset != nil {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, pkg.Fset, n.Fun); err == nil {
			name = buf.String()
		}
	}
	if name == "" {
		name = "unknown"
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
	e.logger.Log(ctx, slog.LevelDebug, "call", "stack", strings.Join(stackNames, " -> "))

	function := e.Eval(ctx, n.Fun, env, pkg)
	if isError(function) {
		return function
	}

	args := e.evalExpressions(ctx, n.Args, env, pkg)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	result := e.applyFunction(ctx, function, args, pkg, n.Pos())
	if isError(result) {
		return result
	}
	return result
}

func (e *Evaluator) evalExpressions(ctx context.Context, exps []ast.Expr, env *object.Environment, pkg *scanner.PackageInfo) []object.Object {
	var result []object.Object

	for _, exp := range exps {
		evaluated := e.Eval(ctx, exp, env, pkg)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

func (e *Evaluator) Apply(ctx context.Context, fn object.Object, args []object.Object, pkg *scanner.PackageInfo) object.Object {
	return e.applyFunction(ctx, fn, args, pkg, token.NoPos)
}

func (e *Evaluator) applyFunction(ctx context.Context, fn object.Object, args []object.Object, pkg *scanner.PackageInfo, callPos token.Pos) object.Object {
	e.logger.Debug("applyFunction", "type", fn.Type(), "value", fn.Inspect())
	switch fn := fn.(type) {
	case *object.Function:
		extendedEnv, err := e.extendFunctionEnv(ctx, fn, args, pkg)
		if err != nil {
			return newError(fn.Decl.Pos(), "failed to extend function env: %v", err)
		}
		evaluated := e.Eval(ctx, fn.Body, extendedEnv, pkg)
		if isError(evaluated) {
			return evaluated
		}
		if ret, ok := evaluated.(*object.ReturnValue); ok {
			return ret
		}
		return &object.ReturnValue{Value: evaluated}

	case *object.Intrinsic:
		return fn.Fn(args...)

	case *object.SymbolicPlaceholder:
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling %s", fn.Inspect())}

	default:
		return newError(callPos, "not a function: %s", fn.Type())
	}
}

func (e *Evaluator) extendFunctionEnv(ctx context.Context, fn *object.Function, args []object.Object, pkg *scanner.PackageInfo) (*object.Environment, error) {
	env := object.NewEnclosedEnvironment(fn.Env)

	if fn.Parameters == nil {
		return env, nil
	}

	// FuncLit doesn't have a declaration, so it doesn't have a position to look up the file.
	// We assume it's in the same file as the current package context.
	// This is a simplification.
	var importLookup map[string]string
	if fn.Decl != nil {
		file := pkg.Fset.File(fn.Decl.Pos())
		if file == nil {
			funcName := "anonymous"
			if fn.Name != nil {
				funcName = fn.Name.Name
			}
			return nil, fmt.Errorf("could not find file for function %q", funcName)
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return nil, fmt.Errorf("could not find AST file for path: %s", file.Name())
		}
		importLookup = e.scanner.BuildImportLookup(astFile)
	} else if len(pkg.AstFiles) > 0 {
		// HACK: For FuncLit, just grab the first file's imports.
		for _, astFile := range pkg.AstFiles {
			importLookup = e.scanner.BuildImportLookup(astFile)
			break
		}
	}

	paramIndex := 0
	for _, field := range fn.Parameters.List {
		if paramIndex >= len(args) {
			break
		}
		arg := args[paramIndex]
		paramIndex++

		fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, pkg, importLookup)
		if fieldType == nil {
			continue
		}
		resolvedType, _ := fieldType.Resolve(ctx)

		for _, name := range field.Names {
			if name.Name == "_" {
				continue
			}
			v := &object.Variable{
				Name:             name.Name,
				Value:            arg,
				BaseObject:       object.BaseObject{ResolvedTypeInfo: resolvedType},
			}
			env.Set(name.Name, v)
		}
	}

	return env, nil
}
