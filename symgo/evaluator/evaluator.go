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
	scanPolicy        object.ScanPolicyFunc
}

type callFrame struct {
	Function string
	Pos      token.Pos
}

// New creates a new Evaluator.
func New(scanner *goscan.Scanner, logger *slog.Logger, tracer object.Tracer, scanPolicy object.ScanPolicyFunc) *Evaluator {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return &Evaluator{
		scanner:           scanner,
		intrinsics:        intrinsics.New(),
		logger:            logger,
		tracer:            tracer,
		interfaceBindings: make(map[string]*goscan.TypeInfo),
		scanPolicy:        scanPolicy,
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
	case *ast.BranchStmt:
		return e.evalBranchStmt(n)
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
		result := e.Eval(ctx, n.X, env, pkg)
		// If the expression is a function call that returns a value, we don't want
		// that return value to be mistaken for a `return` statement from the current block.
		// So we unwrap it.
		if ret, ok := result.(*object.ReturnValue); ok {
			return ret.Value
		}
		return result
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
	case *ast.TypeAssertExpr:
		return e.evalTypeAssertExpr(ctx, n, env, pkg)
	case *ast.IncDecStmt:
		return e.evalIncDecStmt(ctx, n, env, pkg)
	case *ast.EmptyStmt:
		return nil // Empty statements do nothing.
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

func (e *Evaluator) evalIncDecStmt(ctx context.Context, n *ast.IncDecStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	ident, ok := n.X.(*ast.Ident)
	if !ok {
		// For now, we only support identifiers. If we support selectors later, we'll need more logic.
		// For anything else (like a selector on a symbolic value), we do nothing and don't error.
		return nil
	}

	// Get the variable object from the environment. Do not get its value yet.
	obj, ok := env.Get(ident.Name)
	if !ok {
		// This is a semantic error in the Go code being analyzed (undefined variable). Return an error.
		return e.newError(ident.Pos(), "identifier not found: %s", ident.Name)
	}

	v, ok := obj.(*object.Variable)
	if !ok {
		// This is also a semantic error (e.g. `myFunc++`).
		return e.newError(ident.Pos(), "cannot ++/-- a non-variable identifier: %s", ident.Name)
	}

	// Now we have the variable. Check its value.
	if intVal, ok := v.Value.(*object.Integer); ok {
		// The value is a concrete integer, so we can modify it.
		switch n.Tok {
		case token.INC:
			intVal.Value++
		case token.DEC:
			intVal.Value--
		}
	}
	// If v.Value is not an Integer (e.g., a SymbolicPlaceholder), we do nothing, as requested.

	return nil // IncDec is a statement, so it doesn't return a value.
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

	elements := make([]object.Object, 0, len(node.Elts))
	// IMPORTANT: Evaluate all field/element values within the literal.
	// This is crucial for detecting function calls inside initializers.
	for _, elt := range node.Elts {
		switch v := elt.(type) {
		case *ast.KeyValueExpr:
			// This handles struct literals: { Key: Value }
			// and map literals: { Key: Value }
			// We always need to evaluate the value part to trace calls.
			value := e.Eval(ctx, v.Value, env, pkg)
			elements = append(elements, value) // For now, just track values for structs too.
			// For maps, keys can also be expressions with function calls.
			// For structs, keys are just identifiers, so evaluating them is harmless
			// if we check the type first.
			if fieldType.IsMap {
				e.Eval(ctx, v.Key, env, pkg)
			}
		default:
			// This handles slice/array literals: { Value1, Value2 }
			element := e.Eval(ctx, v, env, pkg)
			elements = append(elements, element)
		}
	}

	if fieldType.IsMap {
		mapObj := &object.Map{MapFieldType: fieldType}
		mapObj.SetFieldType(fieldType)
		return mapObj
	}

	if fieldType.IsSlice {
		sliceObj := &object.Slice{
			SliceFieldType: fieldType,
			Elements:       elements,
		}
		sliceObj.SetFieldType(fieldType)
		return sliceObj
	}

	// Policy check before resolving.
	if fieldType.FullImportPath != "" && e.scanPolicy != nil && !e.scanPolicy(fieldType.FullImportPath) {
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("unresolved composite literal of type %s", fieldType.String()),
		}
		placeholder.SetFieldType(fieldType)
		return placeholder
	}

	resolvedType, _ := fieldType.Resolve(ctx)
	if resolvedType == nil {
		// This can happen for built-in types or if resolution fails for other reasons.
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("unresolved composite literal of type %s", fieldType.String()),
		}
		placeholder.SetFieldType(fieldType)
		return placeholder
	}

	// The original logic checked for unresolved types *after* resolution, which was too late.
	// The policy check is now done above. This part handles the case where resolution
	// succeeds but the type is still marked as unresolved (which shouldn't happen with
	// the new logic, but is safe to keep).
	if resolvedType.Unresolved {
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

	lType := left.Type()
	rType := right.Type()

	switch {
	case lType == object.INTEGER_OBJ && rType == object.INTEGER_OBJ:
		return e.evalIntegerInfixExpression(node.Pos(), node.Op, left, right)
	case lType == object.STRING_OBJ && rType == object.STRING_OBJ:
		return e.evalStringInfixExpression(node.Pos(), node.Op, left, right)
	case lType == object.COMPLEX_OBJ || rType == object.COMPLEX_OBJ:
		return e.evalComplexInfixExpression(node.Pos(), node.Op, left, right)
	case lType == object.FLOAT_OBJ || rType == object.FLOAT_OBJ:
		// For now, treat float operations as complex to simplify.
		// A more complete implementation would have a separate float path.
		return e.evalComplexInfixExpression(node.Pos(), node.Op, left, right)
	default:
		return &object.SymbolicPlaceholder{Reason: "binary expression"}
	}
}

func (e *Evaluator) evalComplexInfixExpression(pos token.Pos, op token.Token, left, right object.Object) object.Object {
	var lval, rval complex128

	switch l := left.(type) {
	case *object.Complex:
		lval = l.Value
	case *object.Float:
		lval = complex(l.Value, 0)
	case *object.Integer:
		lval = complex(float64(l.Value), 0)
	default:
		return e.newError(pos, "invalid left operand for complex expression: %s", left.Type())
	}

	switch r := right.(type) {
	case *object.Complex:
		rval = r.Value
	case *object.Float:
		rval = complex(r.Value, 0)
	case *object.Integer:
		rval = complex(float64(r.Value), 0)
	default:
		return e.newError(pos, "invalid right operand for complex expression: %s", right.Type())
	}

	switch op {
	case token.ADD:
		return &object.Complex{Value: lval + rval}
	case token.SUB:
		return &object.Complex{Value: lval - rval}
	case token.MUL:
		return &object.Complex{Value: lval * rval}
	case token.QUO:
		return &object.Complex{Value: lval / rval}
	default:
		return e.newError(pos, "unknown complex operator: %s", op)
	}
}

func (e *Evaluator) evalIntegerInfixExpression(pos token.Pos, op token.Token, left, right object.Object) object.Object {
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
		return e.newError(pos, "unknown integer operator: %s", op)
	}
}

func (e *Evaluator) evalStringInfixExpression(pos token.Pos, op token.Token, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	if op != token.ADD {
		return e.newError(pos, "unknown string operator: %s", op)
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
				// Shallow-scan policy check
				if staticFieldType.FullImportPath != "" && e.scanPolicy != nil && !e.scanPolicy(staticFieldType.FullImportPath) {
					// Policy says NO. Create a placeholder unresolved type.
					resolvedTypeInfo = scanner.NewUnresolvedTypeInfo(staticFieldType.FullImportPath, staticFieldType.TypeName)
				} else {
					// Policy says YES (or no policy exists). Resolve normally.
					resolvedTypeInfo, _ = staticFieldType.Resolve(ctx)
				}
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

		// If the symbol is already in the package's environment, return it.
		// This happens if the package was already scanned and the symbol resolved.
		if symbol, ok := val.Env.Get(n.Sel.Name); ok {
			return symbol
		}

		// Policy check: decide whether to scan this package from source.
		if !e.scanPolicy(val.Path) {
			// Policy says NO. Don't scan, just return a placeholder.
			placeholder := &object.SymbolicPlaceholder{Reason: fmt.Sprintf("external symbol %s.%s", val.Path, n.Sel.Name)}
			val.Env.Set(n.Sel.Name, placeholder) // Cache for subsequent lookups.
			return placeholder
		}

		// Policy says YES. Proceed with scanning.
		if val.ScannedInfo == nil {
			if e.scanner == nil {
				return e.newError(n.Pos(), "scanner is not available, cannot load package %q", val.Path)
			}
			pkgInfo, err := e.scanner.ScanPackageByImport(ctx, val.Path)
			if err != nil {
				// If scanning fails, we can still treat it as an external symbol.
				e.logWithContext(ctx, slog.LevelWarn, "could not scan package, treating as external", "package", val.Path, "error", err)
				placeholder := &object.SymbolicPlaceholder{Reason: fmt.Sprintf("unscannable symbol %s.%s", val.Path, n.Sel.Name)}
				val.Env.Set(n.Sel.Name, placeholder)
				return placeholder
			}
			val.ScannedInfo = pkgInfo
		}

		// This is a call to a package within the workspace.
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

		// If it's not a function, check for constants.
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

		// If it's not a function or a known constant, it's probably a variable.
		// Create a generic placeholder for it.
		placeholder := &object.SymbolicPlaceholder{Reason: fmt.Sprintf("workspace symbol %s.%s", val.Path, n.Sel.Name)}
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
			// This can happen if the variable's type comes from a module that
			// the scan policy disallowed from being scanned. Instead of erroring,
			// we treat the method call as symbolic and continue.
			e.logger.DebugContext(ctx, "variable has no type info, treating method call as symbolic", "variable", val.Name, "method", n.Sel.Name)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("method call on variable %q with untyped symbolic value", val.Name)}
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

func (e *Evaluator) evalTypeAssertExpr(ctx context.Context, n *ast.TypeAssertExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// This function handles the single-value form: v := x.(T)
	// The multi-value form (v, ok := x.(T)) is handled specially in evalAssignStmt.

	// First, evaluate the expression whose type is being asserted (x).
	// This is important to trace any function calls that produce the value.
	val := e.Eval(ctx, n.X, env, pkg)
	if isError(val) {
		return val
	}

	// Next, resolve the asserted type (T).
	if pkg == nil || pkg.Fset == nil {
		return e.newError(n.Pos(), "package info or fset is missing, cannot resolve types for type assertion")
	}
	file := pkg.Fset.File(n.Pos())
	if file == nil {
		return e.newError(n.Pos(), "could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return e.newError(n.Pos(), "could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	fieldType := e.scanner.TypeInfoFromExpr(ctx, n.Type, nil, pkg, importLookup)
	if fieldType == nil {
		var typeNameBuf bytes.Buffer
		printer.Fprint(&typeNameBuf, pkg.Fset, n.Type)
		return e.newError(n.Pos(), "could not resolve type for type assertion: %s", typeNameBuf.String())
	}
	resolvedType, _ := fieldType.Resolve(ctx)

	// In the single-value form, the result is just a value of the asserted type.
	// We create a symbolic placeholder for it.
	return &object.SymbolicPlaceholder{
		Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
		BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
	}
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
	result := e.Eval(ctx, n.Body, object.NewEnclosedEnvironment(forEnv), pkg)
	if result != nil {
		switch result.Type() {
		case object.BREAK_OBJ, object.CONTINUE_OBJ:
			// Absorb the break/continue signal, loop terminates symbolically.
			return &object.SymbolicPlaceholder{Reason: "for loop"}
		case object.ERROR_OBJ:
			return result // Propagate errors.
		}
	}

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

	result := e.Eval(ctx, n.Body, rangeEnv, pkg)
	if result != nil {
		switch result.Type() {
		case object.BREAK_OBJ, object.CONTINUE_OBJ:
			// Absorb the break/continue signal, loop terminates symbolically.
			return &object.SymbolicPlaceholder{Reason: "for-range loop"}
		case object.ERROR_OBJ:
			return result // Propagate errors.
		}
	}

	return &object.SymbolicPlaceholder{Reason: "for-range loop"}
}

func (e *Evaluator) evalBranchStmt(n *ast.BranchStmt) object.Object {
	switch n.Tok {
	case token.BREAK:
		return object.BREAK
	case token.CONTINUE:
		return object.CONTINUE
	default:
		return e.newError(n.Pos(), "unsupported branch statement: %s", n.Tok)
	}
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
	thenResult := e.Eval(ctx, n.Body, thenEnv, pkg)

	var elseResult object.Object
	if n.Else != nil {
		elseEnv := object.NewEnclosedEnvironment(ifStmtEnv)
		elseResult = e.Eval(ctx, n.Else, elseEnv, pkg)
	}

	// If both branches terminate with the same type of control flow, propagate it.
	// This is a simplification. A more robust implementation might track multiple
	// possible return states.
	if thenResult != nil && elseResult != nil && thenResult.Type() == elseResult.Type() {
		switch thenResult.Type() {
		case object.BREAK_OBJ, object.CONTINUE_OBJ, object.RETURN_VALUE_OBJ, object.ERROR_OBJ:
			return thenResult
		}
	}

	// If only one branch terminates, or they terminate differently, the overall
	// execution path can continue, so we return a placeholder.
	return &object.SymbolicPlaceholder{Reason: "if/else statement"}
}

func (e *Evaluator) evalBlockStatement(ctx context.Context, block *ast.BlockStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	var result object.Object
	// The caller is responsible for creating a new scope if one is needed.
	// We evaluate the statements in the provided environment.
	for _, stmt := range block.List {
		result = e.Eval(ctx, stmt, env, pkg)

		// It's possible for a statement (like a declaration) to evaluate to a nil object.
		// We must check for this before calling .Type() to avoid a panic.
		if result == nil {
			continue
		}

		rt := result.Type()
		if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ || rt == object.BREAK_OBJ || rt == object.CONTINUE_OBJ {
			return result
		}
	}

	return result
}

func (e *Evaluator) evalReturnStmt(ctx context.Context, n *ast.ReturnStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if len(n.Results) == 0 {
		return &object.ReturnValue{Value: object.NIL} // naked return
	}

	if len(n.Results) == 1 {
		val := e.Eval(ctx, n.Results[0], env, pkg)
		if isError(val) {
			return val
		}
		if _, ok := val.(*object.ReturnValue); ok {
			return val
		}
		return &object.ReturnValue{Value: val}
	}

	// Handle multiple return values
	vals := e.evalExpressions(ctx, n.Results, env, pkg)
	if len(vals) == 1 && isError(vals[0]) {
		return vals[0] // Error occurred during expression evaluation
	}

	return &object.ReturnValue{Value: &object.MultiReturn{Values: vals}}
}

func (e *Evaluator) evalAssignStmt(ctx context.Context, n *ast.AssignStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	// Handle multi-value assignment, e.g., x, y := f() or x, y = f()
	if len(n.Rhs) == 1 && len(n.Lhs) > 1 {
		// Special case for two-value type assertions: v, ok := x.(T)
		if typeAssert, ok := n.Rhs[0].(*ast.TypeAssertExpr); ok {
			if len(n.Lhs) != 2 {
				return e.newError(n.Pos(), "type assertion with 2 values on RHS must have 2 variables on LHS, got %d", len(n.Lhs))
			}

			// Evaluate the source expression to trace calls
			e.Eval(ctx, typeAssert.X, env, pkg)

			// Resolve the asserted type (T).
			if pkg == nil || pkg.Fset == nil {
				return e.newError(n.Pos(), "package info or fset is missing, cannot resolve types for type assertion")
			}
			file := pkg.Fset.File(n.Pos())
			if file == nil {
				return e.newError(n.Pos(), "could not find file for node position")
			}
			astFile, ok := pkg.AstFiles[file.Name()]
			if !ok {
				return e.newError(n.Pos(), "could not find ast.File for path: %s", file.Name())
			}
			importLookup := e.scanner.BuildImportLookup(astFile)

			fieldType := e.scanner.TypeInfoFromExpr(ctx, typeAssert.Type, nil, pkg, importLookup)
			if fieldType == nil {
				var typeNameBuf bytes.Buffer
				printer.Fprint(&typeNameBuf, pkg.Fset, typeAssert.Type)
				return e.newError(typeAssert.Pos(), "could not resolve type for type assertion: %s", typeNameBuf.String())
			}
			resolvedType, _ := fieldType.Resolve(ctx)

			// Create placeholders for the two return values.
			valuePlaceholder := &object.SymbolicPlaceholder{
				Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
			}

			okPlaceholder := &object.SymbolicPlaceholder{
				Reason: "ok from type assertion",
				BaseObject: object.BaseObject{
					ResolvedTypeInfo: nil, // Built-in types do not have a TypeInfo struct.
					ResolvedFieldType: &scanner.FieldType{
						Name:      "bool",
						IsBuiltin: true,
					},
				},
			}

			// Assign the placeholders to the LHS variables.
			if ident, ok := n.Lhs[0].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ident, valuePlaceholder, n.Tok, env)
				}
			}
			if ident, ok := n.Lhs[1].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ident, okPlaceholder, n.Tok, env)
				}
			}
			return nil
		}

		rhsValue := e.Eval(ctx, n.Rhs[0], env, pkg)
		if isError(rhsValue) {
			return rhsValue
		}

		// The result of a function call might be wrapped in a ReturnValue.
		if ret, ok := rhsValue.(*object.ReturnValue); ok {
			rhsValue = ret.Value
		}

		multiRet, ok := rhsValue.(*object.MultiReturn)
		if !ok {
			// This can happen if a function that is supposed to return multiple values
			// is not correctly modeled. We fall back to assigning placeholders.
			e.logWithContext(ctx, slog.LevelWarn, "expected multi-return value on RHS of assignment", "got_type", rhsValue.Type())
			for _, lhsExpr := range n.Lhs {
				if ident, ok := lhsExpr.(*ast.Ident); ok && ident.Name != "_" {
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
				e.assignIdentifier(ident, val, n.Tok, env) // Use the statement's token (:= or =)
			}
		}
		return nil
	}

	// Handle single assignment: x = y or x := y
	if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
		switch lhs := n.Lhs[0].(type) {
		case *ast.Ident:
			if lhs.Name == "_" {
				// Evaluate RHS for side-effects even if assigned to blank identifier.
				return e.Eval(ctx, n.Rhs[0], env, pkg)
			}
			return e.evalIdentAssignment(ctx, lhs, n.Rhs[0], n.Tok, env, pkg)
		case *ast.SelectorExpr:
			// For now, we don't model state changes on fields, but we evaluate the RHS
			// to trace any function calls.
			e.Eval(ctx, n.Rhs[0], env, pkg)
			return nil
		default:
			return e.newError(n.Pos(), "unsupported assignment target: expected an identifier or selector, but got %T", lhs)
		}
	}

	// Handle parallel assignment: x, y = y, x
	if len(n.Lhs) == len(n.Rhs) {
		// First, evaluate all RHS expressions before any assignments are made.
		// This is crucial for correctness in cases like `x, y = y, x`.
		rhsValues := make([]object.Object, len(n.Rhs))
		for i, rhsExpr := range n.Rhs {
			val := e.Eval(ctx, rhsExpr, env, pkg)
			if isError(val) {
				return val
			}
			rhsValues[i] = val
		}

		// Now, perform the assignments.
		for i, lhsExpr := range n.Lhs {
			if ident, ok := lhsExpr.(*ast.Ident); ok {
				if ident.Name == "_" {
					continue
				}
				e.assignIdentifier(ident, rhsValues[i], n.Tok, env)
			} else {
				// Handle other LHS types like selectors if needed in the future.
				e.logWithContext(ctx, slog.LevelWarn, "unsupported LHS in parallel assignment", "type", fmt.Sprintf("%T", lhsExpr))
			}
		}
		return nil
	}

	return e.newError(n.Pos(), "unsupported assignment statement")
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
		// In Go, `:=` can redeclare a variable if it's in a different scope,
		// but in our symbolic engine, we'll simplify and just overwrite in the local scope.
		// A more complex implementation would handle shadowing more precisely.
		v := &object.Variable{
			Name:  ident.Name,
			Value: val,
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  val.TypeInfo(),
				ResolvedFieldType: val.FieldType(),
			},
		}
		if val.FieldType() != nil {
			if resolved, _ := val.FieldType().Resolve(context.Background()); resolved != nil && resolved.Kind == scanner.InterfaceKind {
				v.PossibleConcreteTypes = make(map[*scanner.FieldType]struct{})
				if ft := val.FieldType(); ft != nil {
					v.PossibleConcreteTypes[ft] = struct{}{}
				}
			}
		}
		e.logger.Debug("evalAssignStmt: defining var", "name", ident.Name)
		return env.SetLocal(ident.Name, v) // Use SetLocal for :=
	}

	// For `=`, find the variable and update it in-place.
	obj, ok := env.Get(ident.Name)
	if !ok {
		// This can happen for package-level variables not yet evaluated,
		// or if the code is invalid Go. We define it in the current scope as a fallback.
		return e.assignIdentifier(ident, val, token.DEFINE, env)
	}

	v, ok := obj.(*object.Variable)
	if !ok {
		// Not a variable, just overwrite it in the environment.
		e.logger.Debug("evalAssignStmt: overwriting non-variable in env", "name", ident.Name)
		return env.Set(ident.Name, val)
	}

	v.Value = val
	newFieldType := val.FieldType()

	// Check if the variable was originally typed as an interface.
	var isInterface bool
	if v.FieldType() != nil {
		if staticType, _ := v.FieldType().Resolve(context.Background()); staticType != nil {
			isInterface = staticType.Kind == scanner.InterfaceKind
		}
	}

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
		// For concrete types, we can still track the type for robustness,
		// but we don't need to accumulate them.
		v.PossibleConcreteTypes = make(map[*scanner.FieldType]struct{})
		if newFieldType != nil {
			v.PossibleConcreteTypes[newFieldType] = struct{}{}
		}
		e.logger.Debug("evalAssignStmt: setting concrete type for var", "name", ident.Name)
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
	case token.FLOAT:
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return e.newError(n.Pos(), "could not parse %q as float", n.Value)
		}
		return &object.Float{Value: f}
	case token.IMAG:
		// The value is like "123i", "0.5i", etc.
		// We need to parse the numeric part.
		imagStr := strings.TrimSuffix(n.Value, "i")
		f, err := strconv.ParseFloat(imagStr, 64)
		if err != nil {
			return e.newError(n.Pos(), "could not parse %q as imaginary", n.Value)
		}
		return &object.Complex{Value: complex(0, f)}
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

	// Fallback to universe scope for built-in values and functions.
	if val, ok := universe.GetValue(n.Name); ok {
		return val
	}
	if fn, ok := universe.GetFunction(n.Name); ok {
		return &object.Intrinsic{Fn: fn}
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
				posStr := ""
				if e.scanner != nil && e.scanner.Fset() != nil {
					posStr = e.scanner.Fset().Position(frame.Pos).String()
				}
				contextArgs := []any{
					slog.String("in_func", frame.Function),
					slog.String("in_func_pos", posStr),
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
	frames := make([]*object.CallFrame, len(e.callStack))
	for i, frame := range e.callStack {
		frames[i] = &object.CallFrame{
			Function: frame.Function,
			Pos:      frame.Pos,
		}
	}
	err := &object.Error{
		Message:   fmt.Sprintf(format, args...),
		Pos:       pos,
		CallStack: frames,
	}
	if e.scanner != nil {
		err.AttachFileSet(e.scanner.Fset())
	}
	return err
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

	frame := &callFrame{Function: name, Pos: n.Pos()}
	e.callStack = append(e.callStack, frame)
	defer func() {
		e.callStack = e.callStack[:len(e.callStack)-1]
	}()

	var stackNames []string
	for _, f := range e.callStack {
		stackNames = append(stackNames, f.Function)
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

	// If the call includes `...`, the last argument is a slice to be expanded.
	// We wrap it in a special Variadic object to signal this to `applyFunction`.
	if n.Ellipsis.IsValid() {
		if len(args) == 0 {
			return e.newError(n.Ellipsis, "invalid use of ... with no arguments")
		}
		lastArg := args[len(args)-1]
		// The argument should be a slice, but we don't check it here.
		// `extendFunctionEnv` will handle the type logic.
		args[len(args)-1] = &object.Variadic{Value: lastArg}
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

		evaluatedValue := evaluated
		if ret, ok := evaluated.(*object.ReturnValue); ok {
			evaluatedValue = ret.Value
		}

		// If the evaluated result is a Go nil (from a naked return), wrap it.
		if evaluatedValue == nil {
			return &object.ReturnValue{Value: object.NIL}
		}

		return &object.ReturnValue{Value: evaluatedValue}

	case *object.Intrinsic:
		return fn.Fn(args...)

	case *object.SymbolicPlaceholder:
		// Case 1: The placeholder represents an unresolved interface method call.
		if fn.UnderlyingMethod != nil {
			method := fn.UnderlyingMethod
			if len(method.Results) <= 1 {
				var resultTypeInfo *scanner.TypeInfo
				if len(method.Results) == 1 {
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
			} else {
				// Multiple return values from interface method
				results := make([]object.Object, len(method.Results))
				for i, res := range method.Results {
					resultType, _ := res.Type.Resolve(context.Background())
					results[i] = &object.SymbolicPlaceholder{
						Reason:     fmt.Sprintf("result %d of interface method call %s", i, method.Name),
						BaseObject: object.BaseObject{ResolvedTypeInfo: resultType},
					}
				}
				return &object.MultiReturn{Values: results}
			}
		}

		// Case 2: The placeholder represents an external function call.
		if fn.UnderlyingFunc != nil && fn.Package != nil {
			results := fn.UnderlyingFunc.AstDecl.Type.Results
			if results == nil || len(results.List) == 0 {
				return &object.SymbolicPlaceholder{Reason: "result of external call with no return value"}
			}

			var importLookup map[string]string
			if file := fn.Package.Fset.File(fn.UnderlyingFunc.AstDecl.Pos()); file != nil {
				if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
					importLookup = e.scanner.BuildImportLookup(astFile)
				}
			}

			if len(results.List) == 1 {
				// Single return value
				resultASTExpr := results.List[0].Type
				fieldType := e.scanner.TypeInfoFromExpr(context.Background(), resultASTExpr, nil, fn.Package, importLookup)
				resolvedType, _ := fieldType.Resolve(context.Background())

				if resolvedType == nil && fieldType.IsBuiltin && fieldType.Name == "error" {
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
				}

				return &object.SymbolicPlaceholder{
					Reason:     fmt.Sprintf("result of external call to %s", fn.UnderlyingFunc.Name),
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
				}
			}

			// Multiple return values
			returnValues := make([]object.Object, 0, len(results.List))
			for i, field := range results.List {
				fieldType := e.scanner.TypeInfoFromExpr(context.Background(), field.Type, nil, fn.Package, importLookup)
				resolvedType, _ := fieldType.Resolve(context.Background())

				if resolvedType == nil && fieldType.IsBuiltin && fieldType.Name == "error" {
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
				}

				placeholder := &object.SymbolicPlaceholder{
					Reason: fmt.Sprintf("result %d of external call to %s", i, fn.UnderlyingFunc.Name),
					BaseObject: object.BaseObject{
						ResolvedTypeInfo:  resolvedType,
						ResolvedFieldType: fieldType,
					},
				}
				returnValues = append(returnValues, placeholder)
			}
			return &object.MultiReturn{Values: returnValues}
		}

		// Case 3: Generic placeholder.
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling %s", fn.Inspect())}

	default:
		return e.newError(callPos, "not a function: %s", fn.Type())
	}
}

func (e *Evaluator) extendFunctionEnv(ctx context.Context, fn *object.Function, args []object.Object) (*object.Environment, error) {
	// The new environment should be enclosed by the function's own package environment,
	// not the caller's environment.
	env := object.NewEnclosedEnvironment(fn.Env)

	// If this is a method call, bind the receiver to its name in the new env.
	if fn.Decl != nil && fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
		recvField := fn.Decl.Recv.List[0]
		if len(recvField.Names) > 0 {
			receiverName := recvField.Names[0].Name
			if receiverName != "" && receiverName != "_" {
				receiverToBind := fn.Receiver
				if receiverToBind == nil {
					// This happens when analysis starts from a method entry point
					// without a concrete receiver instance. We create a symbolic one.
					var importLookup map[string]string
					if file := fn.Package.Fset.File(fn.Decl.Pos()); file != nil {
						if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
							importLookup = e.scanner.BuildImportLookup(astFile)
						}
					}
					fieldType := e.scanner.TypeInfoFromExpr(ctx, recvField.Type, nil, fn.Package, importLookup)
					resolvedType, _ := fieldType.Resolve(ctx)
					receiverToBind = &object.SymbolicPlaceholder{
						Reason:     "symbolic receiver for entry point method",
						BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
					}
				}
				env.Set(receiverName, receiverToBind)
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

	params := fn.Parameters.List
	argIndex := 0

	for _, field := range params {
		paramType := field.Type
		isVariadic := false
		if ellipsis, ok := paramType.(*ast.Ellipsis); ok {
			isVariadic = true
			paramType = ellipsis.Elt
		}

		if isVariadic {
			var variadicSlice object.Object

			// Case 1: The call is of the form `myFunc(slice...)`.
			// The argument will be a single `*object.Variadic` wrapper.
			if argIndex < len(args) && args[argIndex] != nil {
				if v, ok := args[argIndex].(*object.Variadic); ok {
					variadicSlice = v.Value
					argIndex++ // Consume the single variadic argument.
				}
			}

			// Case 2: The call is `myFunc(1, 2, 3)`.
			// Collect all remaining arguments into a slice.
			if variadicSlice == nil {
				variadicArgs := args[argIndex:]
				argIndex = len(args) // Consume all remaining arguments.

				sliceElemFieldType := e.scanner.TypeInfoFromExpr(ctx, paramType, nil, fn.Package, importLookup)
				sliceFieldType := &scanner.FieldType{
					IsSlice: true,
					Elem:    sliceElemFieldType,
				}
				sliceObj := &object.Slice{
					Elements:       variadicArgs,
					SliceFieldType: sliceFieldType,
				}
				sliceObj.SetFieldType(sliceFieldType)
				variadicSlice = sliceObj
			}

			// Bind the final slice to the variadic parameter name.
			if len(field.Names) > 0 {
				name := field.Names[0] // Variadic param is always the last, single identifier.
				if name.Name != "_" {
					fieldType := e.scanner.TypeInfoFromExpr(ctx, paramType, nil, fn.Package, importLookup)
					resolvedType, _ := fieldType.Resolve(ctx)
					v := &object.Variable{
						Name:       name.Name,
						Value:      variadicSlice,
						BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType},
					}
					env.Set(name.Name, v)
				}
			}
			break // A variadic parameter must be the final parameter.
		}

		// This is a regular, non-variadic parameter.
		for _, name := range field.Names {
			if argIndex >= len(args) {
				break
			}
			arg := args[argIndex]

			fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, fn.Package, importLookup)
			if fieldType == nil {
				continue
			}
			resolvedType, _ := fieldType.Resolve(ctx)

			if name.Name != "_" {
				v := &object.Variable{
					Name:       name.Name,
					Value:      arg,
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType},
				}
				env.Set(name.Name, v)
			}
			argIndex++
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
