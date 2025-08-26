package evaluator

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/constant"
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
	tracer            object.Tracer // Tracer for debugging evaluation flow.
	callStack         []*callFrame
	interfaceBindings map[string]*goscan.TypeInfo
	defaultIntrinsic  intrinsics.IntrinsicFunc
	extraPackages     []string
}

type callFrame struct {
	Name string
	Pos  token.Pos
}

// New creates a new Evaluator.
func New(scanner *goscan.Scanner, logger *slog.Logger, tracer object.Tracer, extraPackages []string) *Evaluator {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return &Evaluator{
		scanner:           scanner,
		intrinsics:        intrinsics.New(),
		logger:            logger,
		tracer:            tracer,
		interfaceBindings: make(map[string]*goscan.TypeInfo),
		extraPackages:     extraPackages,
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

// RegisterDefaultIntrinsic registers a default function to be called for any function call.
func (e *Evaluator) RegisterDefaultIntrinsic(fn intrinsics.IntrinsicFunc) {
	e.defaultIntrinsic = fn
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
	if e.tracer != nil {
		e.tracer.Visit(node)
	}
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
	case *ast.RangeStmt:
		return e.evalRangeStmt(ctx, n, env, pkg)
	case *ast.SwitchStmt:
		return e.evalSwitchStmt(ctx, n, env, pkg)
	case *ast.TypeSwitchStmt:
		return e.evalTypeSwitchStmt(ctx, n, env, pkg)
	case *ast.SelectStmt:
		return e.evalSelectStmt(ctx, n, env, pkg)
	case *ast.CallExpr:
		return e.evalCallExpr(ctx, n, env, pkg)
	case *ast.ExprStmt:
		return e.Eval(ctx, n.X, env, pkg)
	case *ast.DeferStmt:
		return e.Eval(ctx, n.Call, env, pkg)
	case *ast.GoStmt:
		return e.Eval(ctx, n.Call, env, pkg)
	case *ast.DeclStmt:
		return e.Eval(ctx, n.Decl, env, pkg)
	case *ast.GenDecl:
		return e.evalGenDecl(ctx, n, env, pkg)
	case *ast.StarExpr:
		return e.evalStarExpr(ctx, n, env, pkg)
	case *ast.UnaryExpr:
		return e.evalUnaryExpr(ctx, n, env, pkg)
	case *ast.BinaryExpr:
		return e.evalBinaryExpr(ctx, n, env, pkg)
	case *ast.CompositeLit:
		return e.evalCompositeLit(ctx, n, env, pkg)
	case *ast.IndexExpr:
		return e.evalIndexExpr(ctx, n, env, pkg)
	case *ast.SliceExpr:
		return e.evalSliceExpr(ctx, n, env, pkg)
	case *ast.ParenExpr:
		return e.Eval(ctx, n.X, env, pkg)
	case *ast.FuncLit:
		return &object.Function{
			Parameters: n.Type.Params,
			Body:       n.Body,
			Env:        env,
			Package:    pkg,
		}
	case *ast.ArrayType:
		// For expressions like `[]byte("foo")`, the `[]byte` part is an ArrayType.
		// We don't need to evaluate it to a concrete value, just prevent an "unimplemented" error.
		return &object.SymbolicPlaceholder{Reason: "array type expression"}
	}
	return e.newError(node.Pos(), "evaluation not implemented for %T", node)
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
		sliceFieldType = l.SliceFieldType
	case *object.Variable:
		if s, ok := l.Value.(*object.Slice); ok {
			sliceFieldType = s.SliceFieldType
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

func (e *Evaluator) evalSliceExpr(ctx context.Context, node *ast.SliceExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// Evaluate the expression being sliced to trace any calls within it.
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}

	// Evaluate index expressions to trace calls.
	if node.Low != nil {
		if low := e.Eval(ctx, node.Low, env, pkg); isError(low) {
			return low
		}
	}
	if node.High != nil {
		if high := e.Eval(ctx, node.High, env, pkg); isError(high) {
			return high
		}
	}
	if node.Max != nil {
		if max := e.Eval(ctx, node.Max, env, pkg); isError(max) {
			return max
		}
	}

	// The result of a slice expression is another slice (or array), which we represent
	// with a placeholder that carries the original type information.
	placeholder := &object.SymbolicPlaceholder{
		Reason: "result of slice expression",
	}
	if left.TypeInfo() != nil {
		placeholder.SetTypeInfo(left.TypeInfo())
	}
	if left.FieldType() != nil {
		placeholder.SetFieldType(left.FieldType())
	}
	return placeholder
}

func (e *Evaluator) evalCompositeLit(ctx context.Context, node *ast.CompositeLit, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if pkg == nil || pkg.Fset == nil {
		return e.newError(node.Pos(), "package info or fset is missing, cannot resolve types for composite literal")
	}

	file := pkg.Fset.File(node.Pos())
	if file == nil {
		return e.newError(node.Pos(), "could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return e.newError(node.Pos(), "could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	fieldType := e.scanner.TypeInfoFromExpr(ctx, node.Type, nil, pkg, importLookup)
	if fieldType == nil {
		var typeNameBuf bytes.Buffer
		printer.Fprint(&typeNameBuf, pkg.Fset, node.Type)
		return e.newError(node.Pos(), "could not resolve type for composite literal: %s", typeNameBuf.String())
	}

	// IMPORTANT: Evaluate all field/element values within the literal.
	// This is crucial for detecting function calls inside initializers.
	for _, elt := range node.Elts {
		switch v := elt.(type) {
		case *ast.KeyValueExpr:
			// This handles struct literals: { Key: Value }
			// We only need to evaluate the value part to trace calls.
			e.Eval(ctx, v.Value, env, pkg)
		default:
			// This handles slice/array literals: { Value1, Value2 }
			e.Eval(ctx, v, env, pkg)
		}
	}

	if fieldType.IsSlice {
		sliceObj := &object.Slice{SliceFieldType: fieldType}
		sliceObj.SetFieldType(fieldType)
		return sliceObj
	}

	resolvedType, _ := fieldType.Resolve(ctx)
	if resolvedType == nil {
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("unresolved composite literal of type %s", fieldType.String()),
		}
		placeholder.SetFieldType(fieldType)
		return placeholder
	}

	instance := &object.Instance{
		TypeName: fmt.Sprintf("%s.%s", resolvedType.PkgPath, resolvedType.Name),
		BaseObject: object.BaseObject{
			ResolvedTypeInfo: resolvedType,
		},
	}
	instance.SetFieldType(fieldType)
	return instance
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
		return e.newError(token.NoPos, "unknown integer operator: %s", op)
	}
}

func (e *Evaluator) evalStringInfixExpression(op token.Token, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	if op != token.ADD {
		return e.newError(token.NoPos, "unknown string operator: %s", op)
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

		// Create a new FieldType for the pointer.
		if originalFieldType := val.FieldType(); originalFieldType != nil {
			pointerFieldType := &scanner.FieldType{
				IsPointer: true,
				Elem:      originalFieldType,
			}
			ptr.SetFieldType(pointerFieldType)
		}
		ptr.SetTypeInfo(val.TypeInfo())
		return ptr
	case token.ARROW: // <-
		// For a channel receive `<-ch`, we just need to evaluate `ch` itself
		// to trace any function calls that produce the channel.
		return e.Eval(ctx, node.X, env, pkg)
	}
	return e.newError(node.Pos(), "unknown unary operator: %s", node.Op)
}

func (e *Evaluator) evalStarExpr(ctx context.Context, node *ast.StarExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	val := e.Eval(ctx, node.X, env, pkg)
	if isError(val) {
		return val
	}

	// First, unwrap any variable to get to the underlying value.
	if v, ok := val.(*object.Variable); ok {
		val = v.Value
	}

	if ptr, ok := val.(*object.Pointer); ok {
		// The value of a pointer is the object it points to.
		return ptr.Value
	}

	// If we have a symbolic placeholder that represents a pointer type,
	// dereferencing it should result in a new placeholder representing the element type.
	if sp, ok := val.(*object.SymbolicPlaceholder); ok {
		if ft := sp.FieldType(); ft != nil && ft.IsPointer {
			newPlaceholder := &object.SymbolicPlaceholder{
				Reason: fmt.Sprintf("dereferenced from %s", sp.Reason),
			}
			if ft.Elem != nil {
				resolvedElem, _ := ft.Elem.Resolve(ctx)
				newPlaceholder.SetFieldType(ft.Elem)
				newPlaceholder.SetTypeInfo(resolvedElem)
			}
			return newPlaceholder
		}
	}

	return e.newError(node.Pos(), "invalid indirect of %s (type %T)", val.Inspect(), val)
}

func (e *Evaluator) evalGenDecl(ctx context.Context, node *ast.GenDecl, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if node.Tok != token.VAR {
		return nil
	}

	if pkg == nil || pkg.Fset == nil {
		return e.newError(node.Pos(), "package info or fset is missing, cannot resolve types")
	}
	file := pkg.Fset.File(node.Pos())
	if file == nil {
		return e.newError(node.Pos(), "could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return e.newError(node.Pos(), "could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	for _, spec := range node.Specs {
		valSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		var staticFieldType *scanner.FieldType
		if valSpec.Type != nil {
			staticFieldType = e.scanner.TypeInfoFromExpr(ctx, valSpec.Type, nil, pkg, importLookup)
		}

		for i, name := range valSpec.Names {
			var val object.Object = &object.SymbolicPlaceholder{Reason: "uninitialized variable"}
			if i < len(valSpec.Values) {
				val = e.Eval(ctx, valSpec.Values[i], env, pkg)
				if isError(val) {
					return val
				}
			}

			var resolvedTypeInfo *scanner.TypeInfo
			if staticFieldType != nil {
				resolvedTypeInfo, _ = staticFieldType.Resolve(ctx)
			}

			v := &object.Variable{
				Name:  name.Name,
				Value: val,
				BaseObject: object.BaseObject{
					ResolvedTypeInfo:  resolvedTypeInfo,
					ResolvedFieldType: staticFieldType,
				},
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
			var funcInfo *scanner.FunctionInfo
			for _, f := range pkg.Functions {
				if f.AstDecl == d {
					funcInfo = f
					break
				}
			}
			fn := &object.Function{
				Name:       d.Name,
				Parameters: d.Type.Params,
				Body:       d.Body,
				Env:        env,
				Decl:       d,
				Package:    pkg,
				Def:        funcInfo,
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
		return e.newError(importSpec.Pos(), "invalid import path: %s", importSpec.Path.Value)
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

	// Unwrap the result if it's a return value from a previous call in a chain.
	if ret, ok := left.(*object.ReturnValue); ok {
		left = ret.Value
	}

	if isError(left) {
		return left
	}
	e.logger.Debug("evalSelectorExpr: evaluated left", "type", left.Type(), "value", left.Inspect())

	switch val := left.(type) {
	case *object.SymbolicPlaceholder:
		typeInfo := val.TypeInfo()
		if typeInfo == nil {
			return e.newError(n.Pos(), "cannot call method on symbolic placeholder with no type info")
		}
		fullTypeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
		key := fmt.Sprintf("(*%s).%s", fullTypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		key = fmt.Sprintf("(%s).%s", fullTypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}

		// Fallback to searching for the method on the instance's type.
		if typeInfo := val.TypeInfo(); typeInfo != nil {
			if method, err := e.findMethodOnType(ctx, typeInfo, n.Sel.Name, env, val); err == nil && method != nil {
				return method
			}
		}

		return e.newError(n.Pos(), "undefined method: %s on symbolic type %s", n.Sel.Name, fullTypeName)

	case *object.Package:
		key := val.Path + "." + n.Sel.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return &object.Intrinsic{Fn: intrinsicFn}
		}
		if val.ScannedInfo == nil {
			if e.scanner == nil {
				return e.newError(n.Pos(), "scanner is not available, cannot load package %q", val.Path)
			}
			pkgInfo, err := e.scanner.ScanPackageByImport(ctx, val.Path)
			if err != nil {
				return e.newError(n.Pos(), "could not scan package %q: %v", val.Path, err)
			}
			val.ScannedInfo = pkgInfo
		}
		// If the symbol is already in the package's environment, return it.
		if symbol, ok := val.Env.Get(n.Sel.Name); ok {
			return symbol
		}

		// Resolve the symbol on-demand. Check if it's a package we should scan from source.
		isScannable := e.isScannablePackage(val.ScannedInfo, pkg)

		if isScannable {
			// This is a call to a package within the same module or an included extra package.
			// Attempt to resolve it to a real, evaluatable function.
			for _, f := range val.ScannedInfo.Functions {
				if f.Name == n.Sel.Name {
					if !ast.IsExported(f.Name) {
						continue // Cannot access unexported functions.
					}
					// Create a real Function object that can be recursively evaluated.
					fn := &object.Function{
						Name:       f.AstDecl.Name,
						Parameters: f.AstDecl.Type.Params,
						Body:       f.AstDecl.Body,
						Env:        val.Env, // Use the target package's environment.
						Decl:       f.AstDecl,
						Package:    val.ScannedInfo,
						Def:        f,
					}
					val.Env.Set(n.Sel.Name, fn) // Cache in the package's env for subsequent calls.
					return fn
				}
			}
		}

		// If it's an extra-module call, or not a resolvable function, treat it as a placeholder.
		// This is the default behavior for external dependencies.
		var placeholder object.Object

		// First, check if the symbol is a known constant.
		for _, c := range val.ScannedInfo.Constants {
			if c.Name == n.Sel.Name {
				if !c.IsExported {
					continue // Cannot access unexported constants.
				}

				var constObj object.Object
				switch c.ConstVal.Kind() {
				case constant.String:
					constObj = &object.String{Value: constant.StringVal(c.ConstVal)}
				case constant.Int:
					val, ok := constant.Int64Val(c.ConstVal)
					if !ok {
						return e.newError(n.Pos(), "could not convert constant %s to int64", c.Name)
					}
					constObj = &object.Integer{Value: val}
				case constant.Bool:
					if constant.BoolVal(c.ConstVal) {
						constObj = object.TRUE
					} else {
						constObj = object.FALSE
					}
				default:
					// Other constant types (float, complex, etc.) are not yet supported.
					// Fall through to create a placeholder.
				}

				if constObj != nil {
					val.Env.Set(n.Sel.Name, constObj) // Cache the resolved constant.
					return constObj
				}
			}
		}

		// Check if the symbol is a function to enrich the placeholder.
		var funcInfo *scanner.FunctionInfo
		for _, f := range val.ScannedInfo.Functions {
			if f.Name == n.Sel.Name {
				funcInfo = f
				break
			}
		}

		if funcInfo != nil {
			placeholder = &object.SymbolicPlaceholder{
				Reason:         fmt.Sprintf("external func %s.%s", val.Name, n.Sel.Name),
				UnderlyingFunc: funcInfo,
				Package:        val.ScannedInfo,
			}
		} else {
			// Not a function or a resolvable constant, so it's likely a var or unsupported const.
			// Create a generic placeholder.
			placeholder = &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external symbol %s.%s", val.Name, n.Sel.Name)}
		}

		val.Env.Set(n.Sel.Name, placeholder)
		return placeholder

	case *object.Instance:
		key := fmt.Sprintf("(%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		key = fmt.Sprintf("(*%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(args ...object.Object) object.Object {
				return intrinsicFn(append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}

		// Fallback to searching for the method on the instance's type.
		if typeInfo := val.TypeInfo(); typeInfo != nil {
			if method, err := e.findMethodOnType(ctx, typeInfo, n.Sel.Name, env, val); err == nil && method != nil {
				return method
			}
		}

		return e.newError(n.Pos(), "undefined method: %s on %s", n.Sel.Name, val.TypeName)

	case *object.Variable:
		typeInfo := val.TypeInfo()
		if typeInfo == nil {
			return e.newError(n.Pos(), "cannot access field or method on variable with no type info: %s", val.Name)
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
							return e.newError(n.Pos(), "failed to scan dependent package %s: %v", foreignPkgObj.Path, err)
						}
						foreignPkgObj.ScannedInfo = scanned
					}
					resolutionPkg = foreignPkgObj.ScannedInfo
				} else {
					scanned, err := e.scanner.ScanPackageByImport(ctx, typeInfo.PkgPath)
					if err != nil {
						return e.newError(n.Pos(), "failed to scan transitive dependency package %s: %v", typeInfo.PkgPath, err)
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
				fn := func(args ...object.Object) object.Object {
					return intrinsicFn(append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}
			key = fmt.Sprintf("(%s).%s", fullTypeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val
				fn := func(args ...object.Object) object.Object {
					return intrinsicFn(append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}
		}

		// Check for a method call on the type, including embedded structs.
		if method, err := e.findMethodOnType(ctx, typeInfo, n.Sel.Name, env, val); err == nil && method != nil {
			return method
		} else if err != nil {
			// Log the error for debugging, but don't fail the evaluation.
			e.logWithContext(ctx, slog.LevelWarn, "error trying to find method", "method", n.Sel.Name, "type", typeInfo.Name, "error", err)
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
				fn := func(args ...object.Object) object.Object {
					return intrinsicFn(append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}
		}

		// Handle method calls on symbolic interfaces
		if typeInfo.Interface != nil {
			for _, method := range typeInfo.Interface.Methods {
				if method.Name == n.Sel.Name {
					// This is an unresolved interface method call.
					// We return a placeholder representing the *function itself*, not its result.
					// This allows the default intrinsic to inspect it.
					placeholder := &object.SymbolicPlaceholder{
						Reason:           fmt.Sprintf("interface method call %s.%s", typeInfo.Name, method.Name),
						Receiver:         val,
						UnderlyingMethod: method,
						BaseObject:       object.BaseObject{ResolvedTypeInfo: typeInfo},
					}

					// If we have tracked concrete types for the variable, add them to the placeholder.
					if len(val.PossibleConcreteTypes) > 0 {
						types := make([]*scanner.FieldType, 0, len(val.PossibleConcreteTypes))
						for t := range val.PossibleConcreteTypes {
							types = append(types, t)
						}
						placeholder.PossibleConcreteTypes = types
					}

					return placeholder
				}
			}
		}

		return e.newError(n.Pos(), "undefined field or method: %s on %s", n.Sel.Name, val.Inspect())

	default:
		return e.newError(n.Pos(), "expected a package, instance, or variable on the left side of selector, but got %s", left.Type())
	}
}

func (e *Evaluator) evalSwitchStmt(ctx context.Context, n *ast.SwitchStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	switchEnv := env
	if n.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, switchEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	if n.Body != nil {
		// In this new model, we don't need to merge environments.
		// We just evaluate all branches in their own scope. The assignment logic
		// will handle updating the parent interface variable.
		for _, c := range n.Body.List {
			if caseClause, ok := c.(*ast.CaseClause); ok {
				caseEnv := object.NewEnclosedEnvironment(switchEnv)
				for _, stmt := range caseClause.Body {
					e.Eval(ctx, stmt, caseEnv, pkg)
				}
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "switch statement"}
}

func (e *Evaluator) evalSelectStmt(ctx context.Context, n *ast.SelectStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if n.Body == nil {
		return &object.SymbolicPlaceholder{Reason: "empty select statement"}
	}
	// Symbolically execute all cases.
	for _, c := range n.Body.List {
		if caseClause, ok := c.(*ast.CommClause); ok {
			caseEnv := object.NewEnclosedEnvironment(env)

			// Evaluate the communication expression (e.g., the channel operation).
			if caseClause.Comm != nil {
				if res := e.Eval(ctx, caseClause.Comm, caseEnv, pkg); isError(res) {
					e.logWithContext(ctx, slog.LevelWarn, "error evaluating select case communication", "error", res)
				}
			}

			// Evaluate the body of the case.
			for _, stmt := range caseClause.Body {
				if res := e.Eval(ctx, stmt, caseEnv, pkg); isError(res) {
					e.logWithContext(ctx, slog.LevelWarn, "error evaluating statement in select case", "error", res)
				}
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "select statement"}
}

func (e *Evaluator) evalTypeSwitchStmt(ctx context.Context, n *ast.TypeSwitchStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	switchEnv := env
	if n.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, switchEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	assignStmt, ok := n.Assign.(*ast.AssignStmt)
	if !ok {
		return e.newError(n.Pos(), "expected AssignStmt in TypeSwitchStmt, got %T", n.Assign)
	}
	if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
		return e.newError(n.Pos(), "expected one variable and one value in type switch assignment")
	}
	ident, ok := assignStmt.Lhs[0].(*ast.Ident)
	if !ok {
		return e.newError(n.Pos(), "expected identifier on LHS of type switch assignment")
	}
	varName := ident.Name

	typeAssert, ok := assignStmt.Rhs[0].(*ast.TypeAssertExpr)
	if !ok {
		return e.newError(n.Pos(), "expected TypeAssertExpr on RHS of type switch assignment")
	}
	originalObj := e.Eval(ctx, typeAssert.X, switchEnv, pkg)
	if isError(originalObj) {
		return originalObj
	}

	if n.Body != nil {
		file := pkg.Fset.File(n.Pos())
		if file == nil {
			return e.newError(n.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(n.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		for _, c := range n.Body.List {
			caseClause, ok := c.(*ast.CaseClause)
			if !ok {
				continue
			}
			caseEnv := object.NewEnclosedEnvironment(switchEnv)

			if caseClause.List == nil { // default case
				v := &object.Variable{
					Name:  varName,
					Value: originalObj,
					BaseObject: object.BaseObject{
						ResolvedTypeInfo:  originalObj.TypeInfo(),
						ResolvedFieldType: originalObj.FieldType(),
					},
				}
				caseEnv.Set(varName, v)
			} else {
				typeExpr := caseClause.List[0]
				fieldType := e.scanner.TypeInfoFromExpr(ctx, typeExpr, nil, pkg, importLookup)
				if fieldType == nil {
					if id, ok := typeExpr.(*ast.Ident); ok {
						fieldType = &scanner.FieldType{Name: id.Name, IsBuiltin: true}
					} else {
						return e.newError(typeExpr.Pos(), "could not resolve type for case clause")
					}
				}

				resolvedType, _ := fieldType.Resolve(ctx)
				val := &object.SymbolicPlaceholder{
					Reason:     fmt.Sprintf("type switch case variable %s", fieldType.String()),
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
				}
				v := &object.Variable{
					Name:  varName,
					Value: val,
					BaseObject: object.BaseObject{
						ResolvedTypeInfo:  resolvedType,
						ResolvedFieldType: fieldType,
					},
				}
				caseEnv.Set(varName, v)
			}

			for _, stmt := range caseClause.Body {
				if res := e.Eval(ctx, stmt, caseEnv, pkg); isError(res) {
					e.logWithContext(ctx, slog.LevelWarn, "error evaluating statement in type switch case", "error", res)
				}
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "type switch statement"}
}

func (e *Evaluator) evalForStmt(ctx context.Context, n *ast.ForStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// For symbolic execution, we unroll the loop once.
	// A more sophisticated engine might unroll N times or use summaries.
	forEnv := object.NewEnclosedEnvironment(env)

	if n.Init != nil {
		if initResult := e.Eval(ctx, n.Init, forEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	// We don't check the condition, just execute the body once.
	e.Eval(ctx, n.Body, object.NewEnclosedEnvironment(forEnv), pkg)

	// The result of a for statement is not a value.
	return &object.SymbolicPlaceholder{Reason: "for loop"}
}

func (e *Evaluator) evalRangeStmt(ctx context.Context, n *ast.RangeStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// For symbolic execution, the most important part is to evaluate the expression
	// being ranged over, as it might contain function calls we need to trace.
	e.Eval(ctx, n.X, env, pkg)

	// We symbolically execute the body once.
	rangeEnv := object.NewEnclosedEnvironment(env)

	// Create placeholder variables for the key and value in the loop's scope.
	if n.Key != nil {
		if ident, ok := n.Key.(*ast.Ident); ok && ident.Name != "_" {
			keyVar := &object.Variable{
				Name:  ident.Name,
				Value: &object.SymbolicPlaceholder{Reason: "range loop key"},
			}
			rangeEnv.Set(ident.Name, keyVar)
		}
	}
	if n.Value != nil {
		if ident, ok := n.Value.(*ast.Ident); ok && ident.Name != "_" {
			valueVar := &object.Variable{
				Name:  ident.Name,
				Value: &object.SymbolicPlaceholder{Reason: "range loop value"},
			}
			rangeEnv.Set(ident.Name, valueVar)
		}
	}

	e.Eval(ctx, n.Body, rangeEnv, pkg)

	return &object.SymbolicPlaceholder{Reason: "for-range loop"}
}

func (e *Evaluator) evalIfStmt(ctx context.Context, n *ast.IfStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	ifStmtEnv := env
	if n.Init != nil {
		ifStmtEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, ifStmtEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	// Evaluate both branches. Each gets its own enclosed environment.
	// The new assignment logic handles updating parent scopes correctly.
	thenEnv := object.NewEnclosedEnvironment(ifStmtEnv)
	e.Eval(ctx, n.Body, thenEnv, pkg)

	if n.Else != nil {
		elseEnv := object.NewEnclosedEnvironment(ifStmtEnv)
		e.Eval(ctx, n.Else, elseEnv, pkg)
	}

	return &object.SymbolicPlaceholder{Reason: "if/else statement"}
}

func (e *Evaluator) evalBlockStatement(ctx context.Context, block *ast.BlockStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	var result object.Object
	// The caller is responsible for creating a new scope if one is needed.
	// We evaluate the statements in the provided environment.
	for _, stmt := range block.List {
		result = e.Eval(ctx, stmt, env, pkg)

		if result != nil {
			// Only terminate the block if we hit an actual return statement.
			// A ReturnValue from a regular function call used as a statement (in an ExprStmt)
			// should not stop the evaluation of the rest of the block.
			if _, isReturnStmt := stmt.(*ast.ReturnStmt); isReturnStmt {
				rt := result.Type()
				if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
					return result
				}
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
		return e.newError(n.Pos(), "unsupported return statement: expected 1 result")
	}
	val := e.Eval(ctx, n.Results[0], env, pkg)
	if isError(val) {
		return val
	}
	// If the evaluated result is already a ReturnValue (from a nested function call),
	// just pass it up. Otherwise, wrap the result in a new ReturnValue.
	if _, ok := val.(*object.ReturnValue); ok {
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
			return e.newError(n.Pos(), "assignment mismatch: %d variables but %d values", len(n.Lhs), len(multiRet.Values))
		}

		for i, lhsExpr := range n.Lhs {
			if ident, ok := lhsExpr.(*ast.Ident); ok {
				if ident.Name == "_" {
					continue
				}
				val := multiRet.Values[i]
				// When assigning from a multi-value return, the token is always ASSIGN, not DEFINE.
				e.assignIdentifier(ident, val, token.ASSIGN, env)
			}
		}
		return nil
	}

	if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
		return e.newError(n.Pos(), "unsupported assignment: expected 1 expression on each side, or multi-value assignment")
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
		return e.newError(n.Pos(), "unsupported assignment target: expected an identifier or selector, but got %T", lhs)
	}
}

func (e *Evaluator) evalIdentAssignment(ctx context.Context, ident *ast.Ident, rhs ast.Expr, tok token.Token, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	val := e.Eval(ctx, rhs, env, pkg)
	if isError(val) {
		return val
	}

	// If the value is a return value from a function call, unwrap it.
	if ret, ok := val.(*object.ReturnValue); ok {
		val = ret.Value
	}

	// Log the type info of the value being assigned.
	typeInfo := val.TypeInfo()
	typeName := "<nil>"
	if typeInfo != nil {
		typeName = typeInfo.Name
	}
	e.logger.Debug("evalIdentAssignment: assigning value", "var", ident.Name, "value_type", val.Type(), "value_typeinfo", typeName)

	return e.assignIdentifier(ident, val, tok, env)
}

func (e *Evaluator) assignIdentifier(ident *ast.Ident, val object.Object, tok token.Token, env *object.Environment) object.Object {
	// For `:=`, we always define a new variable in the current scope.
	if tok == token.DEFINE {
		possibleTypes := make(map[*scanner.FieldType]struct{})
		if ft := val.FieldType(); ft != nil {
			possibleTypes[ft] = struct{}{}
		}
		v := &object.Variable{
			Name:                  ident.Name,
			Value:                 val,
			PossibleConcreteTypes: possibleTypes,
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  val.TypeInfo(),
				ResolvedFieldType: val.FieldType(),
			},
		}
		e.logger.Debug("evalAssignStmt: defining var", "name", ident.Name)
		return env.Set(ident.Name, v)
	}

	// For `=`, find the variable and update it in-place.
	obj, ok := env.Get(ident.Name)
	if !ok {
		// This can happen for package-level variables not yet evaluated.
		// We define it in the current scope as a fallback.
		return e.assignIdentifier(ident, val, token.DEFINE, env)
	}

	v, ok := obj.(*object.Variable)
	if !ok {
		// Not a variable, just overwrite it in the environment.
		return env.Set(ident.Name, val)
	}

	v.Value = val
	newFieldType := val.FieldType()

	// Check if the variable is an interface.
	staticType := v.FieldType()
	isInterface := staticType != nil && staticType.Definition != nil && staticType.Definition.Kind == scanner.InterfaceKind

	if isInterface {
		// For interfaces, we ADD the new concrete type to the set.
		if v.PossibleConcreteTypes == nil {
			v.PossibleConcreteTypes = make(map[*scanner.FieldType]struct{})
		}
		if newFieldType != nil {
			v.PossibleConcreteTypes[newFieldType] = struct{}{}
			e.logger.Debug("evalAssignStmt: adding concrete type to interface var", "name", ident.Name, "new_type", newFieldType.String())
		}
	} else {
		// For concrete types, we REPLACE the set.
		v.PossibleConcreteTypes = make(map[*scanner.FieldType]struct{})
		if newFieldType != nil {
			v.PossibleConcreteTypes[newFieldType] = struct{}{}
		}
		e.logger.Debug("evalAssignStmt: replacing concrete type for var", "name", ident.Name)
	}

	return v
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
	return e.newError(n.Pos(), "identifier not found: %s", n.Name)
}

// logWithContext logs a message, adding call stack information if an error object is provided.
func (e *Evaluator) logWithContext(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !e.logger.Enabled(ctx, level) {
		return
	}

	// Look for an error object in the arguments to extract call stack info.
	for _, arg := range args {
		if err, ok := arg.(*object.Error); ok {
			if len(err.CallStack) > 0 {
				// The most recent frame is at the end of the slice.
				frame := err.CallStack[len(err.CallStack)-1]
				contextArgs := []any{
					slog.String("in_func", frame.Name),
					slog.String("in_func_pos", frame.Pos),
				}
				// Prepend context args so they appear first in the log.
				args = append(contextArgs, args...)
				break // Found an error, don't need to look for more.
			}
		}
	}

	e.logger.Log(ctx, level, msg, args...)
}

func (e *Evaluator) newError(pos token.Pos, format string, args ...interface{}) *object.Error {
	frames := make([]object.FrameInfo, len(e.callStack))
	for i, frame := range e.callStack {
		var posStr string
		if frame.Pos.IsValid() {
			posStr = e.scanner.Fset().Position(frame.Pos).String()
		}
		frames[i] = object.FrameInfo{
			Name: frame.Name,
			Pos:  posStr,
		}
	}
	return &object.Error{
		Message:   fmt.Sprintf(format, args...),
		Pos:       pos,
		CallStack: frames,
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

	if e.defaultIntrinsic != nil {
		// The default intrinsic is a "catch-all" handler that can be used for logging,
		// dependency tracking, etc. It receives the function object itself as the first
		// argument, followed by the regular arguments.
		e.defaultIntrinsic(append([]object.Object{function}, args...)...)
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
		// When applying a function, the evaluation context switches to that function's
		// package. We must pass fn.Package to both extendFunctionEnv and Eval.
		extendedEnv, err := e.extendFunctionEnv(ctx, fn, args)
		if err != nil {
			return e.newError(fn.Decl.Pos(), "failed to extend function env: %v", err)
		}

		// Populate the new environment with the imports from the function's source file.
		if fn.Package != nil && fn.Decl != nil {
			file := fn.Package.Fset.File(fn.Decl.Pos())
			if file != nil {
				if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
					for _, imp := range astFile.Imports {
						var name string
						if imp.Name != nil {
							name = imp.Name.Name
						} else {
							parts := strings.Split(strings.Trim(imp.Path.Value, `"`), "/")
							name = parts[len(parts)-1]
						}
						path := strings.Trim(imp.Path.Value, `"`)
						// Set ScannedInfo to nil to force on-demand loading.
						extendedEnv.Set(name, &object.Package{Path: path, ScannedInfo: nil, Env: object.NewEnvironment()})
					}
				}
			}
		}

		evaluated := e.Eval(ctx, fn.Body, extendedEnv, fn.Package)
		if isError(evaluated) {
			return evaluated
		}

		// If 'evaluated' is a Go nil, it's from a naked return.
		// We must wrap it in an object.Object type to prevent panics.
		// For now, we'll use object.Nil, as the most common case for naked
		// returns in Go is for pointer/interface/slice types where nil is the zero value.
		if evaluated == nil {
			evaluated = &object.Nil{}
		}

		if ret, ok := evaluated.(*object.ReturnValue); ok {
			return ret
		}
		return &object.ReturnValue{Value: evaluated}

	case *object.Intrinsic:
		return fn.Fn(args...)

	case *object.SymbolicPlaceholder:
		// Case 1: The placeholder represents an unresolved interface method call.
		if fn.UnderlyingMethod != nil {
			method := fn.UnderlyingMethod
			var resultTypeInfo *scanner.TypeInfo
			if len(method.Results) > 0 {
				// Simplified: use the first return value.
				resultType, _ := method.Results[0].Type.Resolve(context.Background())
				if resultType == nil && method.Results[0].Type.IsBuiltin {
					resultType = &scanner.TypeInfo{Name: method.Results[0].Type.Name}
				}
				resultTypeInfo = resultType
			}
			return &object.SymbolicPlaceholder{
				Reason:     fmt.Sprintf("result of interface method call %s", method.Name),
				BaseObject: object.BaseObject{ResolvedTypeInfo: resultTypeInfo},
			}
		}

		// Case 2: The placeholder represents an external function call.
		if fn.UnderlyingFunc != nil && fn.Package != nil {
			if fn.UnderlyingFunc.AstDecl.Type.Results != nil && len(fn.UnderlyingFunc.AstDecl.Type.Results.List) > 0 {
				var importLookup map[string]string
				if fn.UnderlyingFunc.AstDecl != nil {
					file := fn.Package.Fset.File(fn.UnderlyingFunc.AstDecl.Pos())
					if file != nil {
						if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
							importLookup = e.scanner.BuildImportLookup(astFile)
						}
					}
				}

				resultASTExpr := fn.UnderlyingFunc.AstDecl.Type.Results.List[0].Type
				fieldType := e.scanner.TypeInfoFromExpr(context.Background(), resultASTExpr, nil, fn.Package, importLookup)
				resolvedType, _ := fieldType.Resolve(context.Background())

				if resolvedType == nil && fieldType.IsBuiltin {
					if fieldType.Name == "error" {
						stringFieldType := &scanner.FieldType{Name: "string", IsBuiltin: true}
						errorMethod := &scanner.MethodInfo{
							Name:    "Error",
							Results: []*scanner.FieldInfo{{Type: stringFieldType}},
						}
						resolvedType = &scanner.TypeInfo{
							Name:      "error",
							Kind:      scanner.InterfaceKind,
							Interface: &scanner.InterfaceInfo{Methods: []*scanner.MethodInfo{errorMethod}},
						}
					} else {
						resolvedType = &scanner.TypeInfo{
							Name:    fieldType.Name,
							PkgPath: "", // Built-in types have no package path.
							Kind:    scanner.InterfaceKind,
						}
					}
				}

				return &object.SymbolicPlaceholder{
					Reason:     fmt.Sprintf("result of external call to %s", fn.UnderlyingFunc.Name),
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType},
				}
			}
		}

		// Case 3: Generic placeholder.
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling %s", fn.Inspect())}

	default:
		return e.newError(callPos, "not a function: %s", fn.Type())
	}
}

// isScannablePackage determines if a package should be deeply analyzed (scanned from source).
// This is true if the package is part of the main module being analyzed, or if it has been
// explicitly included via the `extraPackages` configuration.
func (e *Evaluator) isScannablePackage(targetPkg, currentPkg *scanner.PackageInfo) bool {
	if targetPkg == nil {
		return false
	}

	// Check if it's part of the same module as the current package.
	if currentPkg != nil && currentPkg.ModulePath != "" && strings.HasPrefix(targetPkg.ImportPath, currentPkg.ModulePath) {
		return true
	}

	// Check if the package path matches any of the extra packages to be scanned.
	for _, extraPkg := range e.extraPackages {
		if strings.HasPrefix(targetPkg.ImportPath, extraPkg) {
			return true
		}
	}

	return false
}

func (e *Evaluator) extendFunctionEnv(ctx context.Context, fn *object.Function, args []object.Object) (*object.Environment, error) {
	// The new environment should be enclosed by the function's own package environment,
	// not the caller's environment.
	env := object.NewEnclosedEnvironment(fn.Env)

	// If this is a method call, bind the receiver to its name in the new env.
	if fn.Receiver != nil && fn.Decl != nil && fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
		// The receiver field list should have exactly one entry for a method.
		// That entry might have multiple names, but we'll use the first.
		if len(fn.Decl.Recv.List[0].Names) > 0 {
			receiverName := fn.Decl.Recv.List[0].Names[0].Name
			if receiverName != "" && receiverName != "_" {
				env.Set(receiverName, fn.Receiver)
			}
		}
	}

	if fn.Parameters == nil {
		return env, nil
	}

	if fn.Package == nil || fn.Package.Fset == nil {
		// Cannot resolve parameter types without package info.
		// This can happen for func literals or in some test setups.
		// We'll proceed but types will be less precise.
		e.logWithContext(ctx, slog.LevelWarn, "extendFunctionEnv: function has no package info; cannot resolve param types")
		return env, nil
	}

	var importLookup map[string]string
	if fn.Decl != nil {
		file := fn.Package.Fset.File(fn.Decl.Pos())
		if file != nil {
			if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
				importLookup = e.scanner.BuildImportLookup(astFile)
			}
		}
	} else if len(fn.Package.AstFiles) > 0 {
		// HACK: For FuncLit, just grab the first file's imports.
		for _, astFile := range fn.Package.AstFiles {
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

		// Resolve the parameter's type using the function's own package context.
		fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, fn.Package, importLookup)
		if fieldType == nil {
			continue
		}
		resolvedType, _ := fieldType.Resolve(ctx)

		for _, name := range field.Names {
			if name.Name == "_" {
				continue
			}
			v := &object.Variable{
				Name:       name.Name,
				Value:      arg,
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType},
			}
			env.Set(name.Name, v)
		}
	}

	return env, nil
}

// findMethodOnType recursively finds a method on a type or its embedded types.
// It returns a callable Function object if found.
func (e *Evaluator) findMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object) (*object.Function, error) {
	if typeInfo == nil {
		return nil, nil // Cannot find method without type info
	}

	// Use a map to track visited types and prevent infinite recursion.
	visited := make(map[string]bool)
	return e.findMethodRecursive(ctx, typeInfo, methodName, env, receiver, visited)
}

func (e *Evaluator) findMethodRecursive(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object, visited map[string]bool) (*object.Function, error) {
	if typeInfo == nil {
		return nil, nil
	}

	// Create a unique key for the type to track visited nodes.
	typeKey := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if visited[typeKey] {
		return nil, nil // Cycle detected
	}
	visited[typeKey] = true

	// 1. Search for a direct method on the current type.
	if method, err := e.findDirectMethodOnType(ctx, typeInfo, methodName, env, receiver); err != nil || method != nil {
		return method, err
	}

	// 2. If not found, search in embedded structs.
	if typeInfo.Struct != nil {
		for _, field := range typeInfo.Struct.Fields {
			if field.Embedded {
				embeddedTypeInfo, _ := field.Type.Resolve(ctx)
				if embeddedTypeInfo != nil {
					// Recursive call, passing the original receiver.
					if foundFn, err := e.findMethodRecursive(ctx, embeddedTypeInfo, methodName, env, receiver, visited); err != nil || foundFn != nil {
						return foundFn, err
					}
				}
			}
		}
	}

	return nil, nil // Not found
}

func (e *Evaluator) findDirectMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object) (*object.Function, error) {
	if typeInfo == nil || typeInfo.PkgPath == "" {
		return nil, nil
	}

	methodPkg, err := e.scanner.ScanPackageByImport(ctx, typeInfo.PkgPath)
	if err != nil {
		// This can happen for built-in types like 'error', which is fine.
		if strings.Contains(err.Error(), "cannot find package") {
			return nil, nil
		}
		return nil, fmt.Errorf("could not scan package %q: %w", typeInfo.PkgPath, err)
	}

	for _, fn := range methodPkg.Functions {
		if fn.Receiver == nil || fn.Name != methodName {
			continue
		}

		// fn.Receiver.Type is a FieldType. We need to get its base name.
		recvTypeName := fn.Receiver.Type.TypeName
		if recvTypeName == "" {
			recvTypeName = fn.Receiver.Type.Name
		}

		// The type name from the scanner might be `T` or `*T`.
		// The receiver type name from the function decl will be `T` or `*T`.
		// Let's compare base names.
		baseRecvTypeName := strings.TrimPrefix(recvTypeName, "*")
		baseTypeName := strings.TrimPrefix(typeInfo.Name, "*")

		if baseRecvTypeName == baseTypeName {
			// This is a potential match. Now check pointer compatibility.
			// If method has pointer receiver, variable must be a pointer.
			// If method has value receiver, variable can be value or pointer.
			isMethodPtrRecv := strings.HasPrefix(recvTypeName, "*")

			var isVarPointer bool
			if v, ok := receiver.(*object.Variable); ok {
				// Check the variable's type, not the value's, as value might be nil.
				if v.FieldType() != nil {
					isVarPointer = v.FieldType().IsPointer
				} else if v.TypeInfo() != nil {
					// Fallback for less precise type info
					isVarPointer = strings.HasPrefix(v.TypeInfo().Name, "*")
				}
			} else if _, ok := receiver.(*object.Pointer); ok {
				isVarPointer = true
			} else if i, ok := receiver.(*object.Instance); ok {
				// An instance from a constructor like `NewFoo()` is usually a pointer.
				isVarPointer = strings.HasPrefix(i.TypeName, "*")
			}

			if isMethodPtrRecv && !isVarPointer {
				continue // Cannot call pointer method on non-pointer
			}

			return &object.Function{
				Name:       fn.AstDecl.Name,
				Parameters: fn.AstDecl.Type.Params,
				Body:       fn.AstDecl.Body,
				Env:        env,
				Decl:       fn.AstDecl,
				Package:    methodPkg,
				Receiver:   receiver,
				Def:        fn,
			}, nil
		}
	}

	return nil, nil // Not found
}
