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
	"runtime"
	"strconv"
	"strings"
	"sync"

	goscan "github.com/podhmo/go-scan"
	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

// MaxCallStackDepth is the maximum depth of the call stack to prevent excessive recursion.
const MaxCallStackDepth = 4096

// FileScope holds the AST and file-specific import aliases for a single file.
type FileScope struct {
	AST *ast.File
}

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	scanner           *goscan.Scanner
	funcCache         map[string]object.Object
	intrinsics        *intrinsics.Registry
	logger            *slog.Logger
	tracer            object.Tracer // Tracer for debugging evaluation flow.
	callStack         []*callFrame
	interfaceBindings map[string]interfaceBinding
	resolver          *Resolver
	defaultIntrinsic  intrinsics.IntrinsicFunc
	initializedPkgs   map[string]bool // To track packages whose constants are loaded
	pkgCache          map[string]*object.Package
	files             []*FileScope
	fileMap           map[string]bool
	UniverseEnv       *object.Environment

	// accessor provides methods for finding fields and methods.
	accessor *accessor

	// evaluationInProgress tracks nodes that are currently being evaluated
	// to detect and prevent infinite recursion.
	evaluationInProgress map[ast.Node]bool

	// calledInterfaceMethods tracks all method calls on interface types.
	// The key is the fully qualified method name (e.g., "io.Writer.Write"),
	// and the value is a list of receiver objects for each call.
	calledInterfaceMethods map[string][]object.Object

	// seenPackages tracks all packages that have been successfully loaded.
	seenPackages map[string]*goscan.Package

	// syntheticMethods caches dynamically created methods for interfaces.
	// This prevents re-creating the same synthetic method multiple times.
	// The outer key is the fully qualified interface type name (e.g., "io.Writer"),
	// and the inner key is the method name.
	syntheticMethods      map[string]map[string]*scan.MethodInfo
	syntheticMethodsMutex sync.Mutex

	// step counting
	step     int
	maxSteps int

	// memoization
	memoize          bool
	memoizationCache map[*object.Function]object.Object
}

type callFrame struct {
	Function    string
	Pos         token.Pos
	Fn          *object.Function
	Args        []object.Object
	ReceiverPos token.Pos // The source position of the receiver expression for a method call.
}

// interfaceBinding stores the information needed to map an interface to a concrete type.
type interfaceBinding struct {
	ConcreteType *goscan.TypeInfo
	IsPointer    bool
}

func (f *callFrame) String() string {
	return f.Function
}

// Option configures the evaluator.
type Option func(*Evaluator)

// WithMaxSteps sets the maximum number of evaluation steps.
func WithMaxSteps(n int) Option {
	return func(e *Evaluator) {
		e.maxSteps = n
	}
}

// WithMemoization enables function analysis memoization.
func WithMemoization() Option {
	return func(e *Evaluator) {
		e.memoize = true
		e.memoizationCache = make(map[*object.Function]object.Object)
	}
}

// getAllInterfaceMethods recursively collects all methods from an interface and its embedded interfaces.
// It handles cycles by keeping track of visited interface types.
// A duplicate of this method exists in `goscan.Scanner` for historical reasons;
// the evaluator needs its own copy to resolve interface method calls during symbolic execution.
func (e *Evaluator) getAllInterfaceMethods(ctx context.Context, ifaceType *scan.TypeInfo, visited map[string]struct{}) []*scan.MethodInfo {
	if ifaceType == nil || ifaceType.Interface == nil {
		return nil
	}

	// Cycle detection
	typeName := ifaceType.PkgPath + "." + ifaceType.Name
	if _, ok := visited[typeName]; ok {
		return nil
	}
	visited[typeName] = struct{}{}

	var allMethods []*scan.MethodInfo
	allMethods = append(allMethods, ifaceType.Interface.Methods...)

	for _, embeddedField := range ifaceType.Interface.Embedded {
		// Resolve the embedded type to get its full definition.
		// Note: embeddedField.Resolve(ctx) creates a new context, so our visited map won't propagate.
		// We need to use the resolver directly or pass the context. Let's assume the resolver handles cycles.
		embeddedTypeInfo, err := embeddedField.Resolve(ctx)
		if err != nil {
			e.logc(ctx, slog.LevelWarn, "could not resolve embedded interface", "type", embeddedField.String(), "error", err)
			continue
		}

		if embeddedTypeInfo != nil && embeddedTypeInfo.Kind == scan.InterfaceKind {
			// Recursively get methods from the embedded interface.
			embeddedMethods := e.getAllInterfaceMethods(ctx, embeddedTypeInfo, visited)
			allMethods = append(allMethods, embeddedMethods...)
		}
	}

	return allMethods
}

// New creates a new Evaluator.
func New(scanner *goscan.Scanner, logger *slog.Logger, tracer object.Tracer, scanPolicy object.ScanPolicyFunc, opts ...Option) *Evaluator {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	universeEnv := object.NewEnclosedEnvironment(nil)
	universe.Walk(func(name string, obj object.Object) bool {
		universeEnv.SetLocal(name, obj)
		return true
	})

	e := &Evaluator{
		scanner:                scanner,
		funcCache:              make(map[string]object.Object),
		intrinsics:             intrinsics.New(),
		logger:                 logger,
		tracer:                 tracer,
		interfaceBindings:      make(map[string]interfaceBinding),
		resolver:               NewResolver(scanPolicy, scanner, logger),
		initializedPkgs:        make(map[string]bool),
		pkgCache:               make(map[string]*object.Package),
		files:                  make([]*FileScope, 0),
		fileMap:                make(map[string]bool),
		evaluationInProgress:   make(map[ast.Node]bool),
		calledInterfaceMethods: make(map[string][]object.Object),
		seenPackages:           make(map[string]*goscan.Package),
		UniverseEnv:            universeEnv,
		syntheticMethods:       make(map[string]map[string]*scan.MethodInfo),
		memoize:                false,
		memoizationCache:       nil,
	}
	e.accessor = newAccessor(e)

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// BindInterface registers a concrete type for an interface.
func (e *Evaluator) BindInterface(ifaceTypeName string, concreteType *goscan.TypeInfo, isPointer bool) {
	e.interfaceBindings[ifaceTypeName] = interfaceBinding{
		ConcreteType: concreteType,
		IsPointer:    isPointer,
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
func (e *Evaluator) Eval(ctx context.Context, node ast.Node, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if e.maxSteps > 0 {
		e.step++
		if e.step > e.maxSteps {
			return e.newError(ctx, node.Pos(), "max execution steps (%d) exceeded", e.maxSteps)
		}
	}

	if file, ok := node.(*ast.File); ok {
		filePath := e.scanner.Fset().File(file.Pos()).Name()
		if !e.fileMap[filePath] {
			e.fileMap[filePath] = true
			// This is a simplified way to create a file scope.
			// A more robust implementation would handle imports here.
			e.files = append(e.files, &FileScope{AST: file})
		}
	}

	if e.tracer != nil {
		e.tracer.Trace(object.TraceEvent{
			Step: e.step,
			Node: node,
			Pkg:  pkg,
			Env:  env,
		})
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
			e.logger.DebugContext(ctx, "evaluating node",
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
		return e.evalBasicLit(ctx, n)
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
	case *ast.LabeledStmt:
		return e.evalLabeledStmt(ctx, n, env, pkg)
	case *ast.SendStmt:
		// Evaluate the channel expression to trace any calls that produce the channel.
		if ch := e.Eval(ctx, n.Chan, env, pkg); isError(ch) {
			return ch
		}
		// Evaluate the value expression to trace any calls that produce the value.
		if val := e.Eval(ctx, n.Value, env, pkg); isError(val) {
			return val
		}
		return nil // Send statement does not produce a value.
	case *ast.BranchStmt:
		return e.evalBranchStmt(ctx, n)
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
	case *ast.IndexListExpr:
		return e.evalIndexListExpr(ctx, n, env, pkg)
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
	case *ast.MapType:
		// Similar to ArrayType, when a map type itself is used as an expression (e.g., in a conversion),
		// we just need to acknowledge it without producing a concrete value.
		return &object.SymbolicPlaceholder{Reason: "map type expression"}
	case *ast.ChanType:
		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, n.Pos(), "package info or fset is missing, cannot resolve types for chan type")
		}
		file := pkg.Fset.File(n.Pos())
		if file == nil {
			return e.newError(ctx, n.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(ctx, n.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		fieldType := e.scanner.TypeInfoFromExpr(ctx, n, nil, pkg, importLookup)
		placeholder := &object.SymbolicPlaceholder{Reason: "channel type expression"}
		placeholder.SetFieldType(fieldType)
		return placeholder
	case *ast.FuncType:
		// Similar to other type expressions, we don't need to evaluate it to a concrete value,
		// just prevent an "unimplemented" error.
		return &object.SymbolicPlaceholder{Reason: "function type expression"}
	case *ast.InterfaceType:
		// Similar to other type expressions, we don't need to evaluate it to a concrete value,
		// just prevent an "unimplemented" error.
		return &object.SymbolicPlaceholder{Reason: "interface type expression"}
	case *ast.StructType:
		// Similar to other type expressions, we don't need to evaluate it to a concrete value,
		// just prevent an "unimplemented" error.
		return &object.SymbolicPlaceholder{Reason: "struct type expression"}
	}
	return e.newError(ctx, node.Pos(), "evaluation not implemented for %T", node)
}

func (e *Evaluator) evalIncDecStmt(ctx context.Context, n *ast.IncDecStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Evaluate the expression to trace any calls, but we need the identifier.
	ident, ok := n.X.(*ast.Ident)
	if !ok {
		e.Eval(ctx, n.X, env, pkg)
		return nil // Cannot perform state change on complex expression.
	}

	obj, ok := env.Get(ident.Name)
	if !ok {
		return e.newError(ctx, n.Pos(), "identifier not found for IncDec: %s", ident.Name)
	}

	variable, ok := obj.(*object.Variable)
	if !ok {
		return e.newError(ctx, n.Pos(), "cannot increment/decrement non-variable: %s", ident.Name)
	}

	val := e.evalVariable(ctx, variable, pkg)
	if isError(val) {
		return val
	}

	var newInt int64
	switch v := val.(type) {
	case *object.Integer:
		newInt = v.Value
	case *object.SymbolicPlaceholder:
		// If it's a placeholder, the result of inc/dec is still a placeholder.
		// We don't change the variable's value, just acknowledge the operation.
		return nil
	default:
		// For other types, we can't meaningfully inc/dec.
		return nil
	}

	switch n.Tok {
	case token.INC:
		newInt++
	case token.DEC:
		newInt--
	}

	// Update the variable's value in place.
	variable.Value = &object.Integer{Value: newInt}
	// Also mark it as evaluated, since it now has a concrete value.
	variable.IsEvaluated = true
	// No need to call env.Set here because we have modified the object in place.
	return nil
}

func (e *Evaluator) evalIndexExpr(ctx context.Context, node *ast.IndexExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}

	// Handle generic instantiation `F[T]`
	if fn, ok := left.(*object.Function); ok {
		if fn.Def != nil && len(fn.Def.TypeParams) > 0 {
			return e.evalGenericInstantiation(ctx, fn, []ast.Expr{node.Index}, node.Pos(), pkg)
		}
	}
	if t, ok := left.(*object.Type); ok {
		if t.ResolvedType != nil && len(t.ResolvedType.TypeParams) > 0 {
			return &object.SymbolicPlaceholder{Reason: "instantiated generic type"}
		}
	}

	// Fallback to original logic for slice/map indexing at runtime.
	if index := e.Eval(ctx, node.Index, env, pkg); isError(index) {
		return index
	}

	var elemFieldType *scan.FieldType
	var resolvedElem *scan.TypeInfo

	// Determine the element type from the collection being indexed.
	var collectionFieldType *scan.FieldType
	switch l := left.(type) {
	case *object.Slice:
		collectionFieldType = l.SliceFieldType
	case *object.Map:
		collectionFieldType = l.MapFieldType
	case *object.Variable:
		// Check the variable's value first, then its static type.
		if s, ok := l.Value.(*object.Slice); ok {
			collectionFieldType = s.SliceFieldType
		} else if m, ok := l.Value.(*object.Map); ok {
			collectionFieldType = m.MapFieldType
		} else if ft := l.FieldType(); ft != nil && (ft.IsSlice || ft.IsMap) {
			collectionFieldType = ft
		} else if ti := l.TypeInfo(); ti != nil && ti.Underlying != nil && (ti.Underlying.IsSlice || ti.Underlying.IsMap) {
			collectionFieldType = ti.Underlying
		}
	case *object.SymbolicPlaceholder:
		if ft := l.FieldType(); ft != nil && (ft.IsSlice || ft.IsMap) {
			collectionFieldType = ft
		} else if ti := l.TypeInfo(); ti != nil && ti.Underlying != nil && (ti.Underlying.IsSlice || ti.Underlying.IsMap) {
			collectionFieldType = ti.Underlying
		}
	}

	// If we found a collection type, get its element type.
	if collectionFieldType != nil && collectionFieldType.Elem != nil {
		elemFieldType = collectionFieldType.Elem
		resolvedElem = e.resolver.ResolveType(ctx, elemFieldType)
	}

	return &object.SymbolicPlaceholder{
		Reason: "result of index expression",
		BaseObject: object.BaseObject{
			ResolvedTypeInfo:  resolvedElem,
			ResolvedFieldType: elemFieldType,
		},
	}
}

func (e *Evaluator) evalIndexListExpr(ctx context.Context, node *ast.IndexListExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	left := e.Eval(ctx, node.X, env, pkg)
	if isError(left) {
		return left
	}

	// Handle generic instantiation `F[T, U]`
	if fn, ok := left.(*object.Function); ok {
		if fn.Def != nil && len(fn.Def.TypeParams) > 0 {
			return e.evalGenericInstantiation(ctx, fn, node.Indices, node.Pos(), pkg)
		}
	}
	if t, ok := left.(*object.Type); ok {
		if t.ResolvedType != nil && len(t.ResolvedType.TypeParams) > 0 {
			return &object.SymbolicPlaceholder{Reason: "instantiated generic type"}
		}
	}

	// This AST node is only for generics, so if we fall through, it's an unhandled case.
	return e.newError(ctx, node.Pos(), "unhandled generic instantiation for %T", left)
}

func (e *Evaluator) evalSliceExpr(ctx context.Context, node *ast.SliceExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
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

func (e *Evaluator) evalCompositeLit(ctx context.Context, node *ast.CompositeLit, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if e.evaluationInProgress[node] {
		e.logc(ctx, slog.LevelWarn, "cyclic dependency detected in composite literal", "pos", node.Pos())
		return &object.SymbolicPlaceholder{Reason: "cyclic reference in composite literal"}
	}
	e.evaluationInProgress[node] = true
	defer delete(e.evaluationInProgress, node)

	var fieldType *scan.FieldType
	var resolvedType *scan.TypeInfo

	// First, try to resolve the type from the local environment. This handles locally defined type aliases.
	if ident, ok := node.Type.(*ast.Ident); ok {
		if obj, found := env.Get(ident.Name); found {
			if typeObj, isType := obj.(*object.Type); isType && typeObj.ResolvedType != nil {
				resolvedType = typeObj.ResolvedType
				if resolvedType.Underlying != nil {
					fieldType = resolvedType.Underlying
				} else {
					// This is likely a primitive type alias, create a field type for it.
					fieldType = &scan.FieldType{
						Name:           resolvedType.Name,
						FullImportPath: resolvedType.PkgPath,
						IsPointer:      strings.HasPrefix(resolvedType.Name, "*"),
					}
				}
			}
		}
	}

	// If the type was not found in the local env, use the scanner to resolve it from the package level.
	if resolvedType == nil {
		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, node.Pos(), "package info or fset is missing, cannot resolve types for composite literal")
		}
		file := pkg.Fset.File(node.Pos())
		if file == nil {
			return e.newError(ctx, node.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(ctx, node.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		fieldType = e.scanner.TypeInfoFromExpr(ctx, node.Type, nil, pkg, importLookup)
		if fieldType == nil {
			var typeNameBuf bytes.Buffer
			printer.Fprint(&typeNameBuf, pkg.Fset, node.Type)
			return e.newError(ctx, node.Pos(), "could not resolve type for composite literal: %s", typeNameBuf.String())
		}
		resolvedType = e.resolver.ResolveType(ctx, fieldType)
	}

	// Now that we have the type, evaluate the elements of the literal.
	elements := make([]object.Object, 0, len(node.Elts))
	for _, elt := range node.Elts {
		switch v := elt.(type) {
		case *ast.KeyValueExpr:
			value := e.Eval(ctx, v.Value, env, pkg)
			elements = append(elements, value)
			if fieldType.IsMap {
				e.Eval(ctx, v.Key, env, pkg)
			}
		default:
			element := e.Eval(ctx, v, env, pkg)
			elements = append(elements, element)
		}
	}

	// Finally, construct the appropriate object based on the type.
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

	if resolvedType != nil && resolvedType.Kind == scan.UnknownKind {
		resolvedType.Kind = scan.StructKind
	}

	if resolvedType == nil || resolvedType.Unresolved {
		placeholder := &object.SymbolicPlaceholder{
			Reason: "unresolved composite literal of type " + fieldType.String(),
		}
		placeholder.SetFieldType(fieldType)
		placeholder.SetTypeInfo(resolvedType)
		return placeholder
	}

	// Dereference the underlying type if the original was an alias.
	finalType := resolvedType
	if finalType.Underlying != nil && finalType.Underlying.Definition != nil {
		finalType = finalType.Underlying.Definition
	}

	instance := &object.Instance{
		TypeName: finalType.PkgPath + "." + finalType.Name,
		BaseObject: object.BaseObject{
			ResolvedTypeInfo: finalType,
		},
	}
	instance.SetFieldType(fieldType)
	return instance
}

func (e *Evaluator) evalBinaryExpr(ctx context.Context, node *ast.BinaryExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	leftObj := e.Eval(ctx, node.X, env, pkg)
	if isError(leftObj) {
		return leftObj
	}
	rightObj := e.Eval(ctx, node.Y, env, pkg)
	if isError(rightObj) {
		return rightObj
	}

	left := e.forceEval(ctx, leftObj, pkg)
	if isError(left) {
		return left
	}
	right := e.forceEval(ctx, rightObj, pkg)
	if isError(right) {
		return right
	}

	lType := left.Type()
	rType := right.Type()

	switch {
	case lType == object.INTEGER_OBJ && rType == object.INTEGER_OBJ:
		return e.evalIntegerInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.STRING_OBJ && rType == object.STRING_OBJ:
		return e.evalStringInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.COMPLEX_OBJ || rType == object.COMPLEX_OBJ:
		return e.evalComplexInfixExpression(ctx, node.Pos(), node.Op, left, right)
	case lType == object.FLOAT_OBJ || rType == object.FLOAT_OBJ:
		// For now, treat float operations as complex to simplify.
		// A more complete implementation would have a separate float path.
		return e.evalComplexInfixExpression(ctx, node.Pos(), node.Op, left, right)
	default:
		return &object.SymbolicPlaceholder{Reason: "binary expression"}
	}
}

func (e *Evaluator) evalComplexInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
	var lval, rval complex128

	switch l := left.(type) {
	case *object.Complex:
		lval = l.Value
	case *object.Float:
		lval = complex(l.Value, 0)
	case *object.Integer:
		lval = complex(float64(l.Value), 0)
	default:
		return e.newError(ctx, pos, "invalid left operand for complex expression: %s", left.Type())
	}

	switch r := right.(type) {
	case *object.Complex:
		rval = r.Value
	case *object.Float:
		rval = complex(r.Value, 0)
	case *object.Integer:
		rval = complex(float64(r.Value), 0)
	default:
		return e.newError(ctx, pos, "invalid right operand for complex expression: %s", right.Type())
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
		return e.newError(ctx, pos, "unknown complex operator: %s", op)
	}
}

func (e *Evaluator) evalIntegerInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
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
		if rightVal == 0 {
			return &object.SymbolicPlaceholder{Reason: "division by zero"}
		}
		return &object.Integer{Value: leftVal / rightVal}

	// Placeholders for operators that are not fully evaluated
	case token.REM: // %
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.SHL: // <<
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.SHR: // >>
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.AND: // &
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.OR: // |
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}
	case token.XOR: // ^
		return &object.SymbolicPlaceholder{Reason: "integer operation: " + op.String()}

	case token.EQL: // ==
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case token.NEQ: // !=
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case token.LSS: // <
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case token.LEQ: // <=
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case token.GTR: // >
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case token.GEQ: // >=
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	default:
		return e.newError(ctx, pos, "unknown integer operator: %s", op)
	}
}

func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return object.TRUE
	}
	return object.FALSE
}

func (e *Evaluator) evalStringInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	switch op {
	case token.ADD:
		return &object.String{Value: leftVal + rightVal}
	case token.EQL:
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case token.NEQ:
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return e.newError(ctx, pos, "unknown string operator: %s", op)
	}
}

func (e *Evaluator) evalUnaryExpr(ctx context.Context, node *ast.UnaryExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	rightObj := e.Eval(ctx, node.X, env, pkg)
	if isError(rightObj) {
		return rightObj
	}

	// For most unary operations, we need the concrete value.
	// But for the address-of operator (&), we must NOT evaluate, because we need
	// the variable/expression itself, not its value.
	var right object.Object
	if node.Op == token.AND {
		right = rightObj
	} else {
		right = e.forceEval(ctx, rightObj, pkg)
		if isError(right) {
			return right
		}
	}

	switch node.Op {
	case token.NOT:
		return e.evalBangOperatorExpression(right)
	case token.SUB, token.ADD, token.XOR:
		return e.evalNumericUnaryExpression(ctx, node.Op, right)
	case token.AND:
		// This is the address-of operator, not a typical unary op on a value.
		// It needs to be handled specially as it operates on identifiers/expressions, not resolved objects.
		// Re-evaluating node.X might be redundant but safer.
		val := e.Eval(ctx, node.X, env, pkg)
		if isError(val) {
			return val
		}
		ptr := &object.Pointer{Value: val}
		if originalFieldType := val.FieldType(); originalFieldType != nil {
			pointerFieldType := &scan.FieldType{
				IsPointer: true,
				Elem:      originalFieldType,
			}
			ptr.SetFieldType(pointerFieldType)
		}
		ptr.SetTypeInfo(val.TypeInfo())
		return ptr
	case token.ARROW: // <-
		// Channel receive `<-ch`
		chObj := e.Eval(ctx, node.X, env, pkg)
		if isError(chObj) {
			return chObj
		}

		// Unwrap if it's a variable
		if v, ok := chObj.(*object.Variable); ok {
			chObj = v.Value
		}

		if ch, ok := chObj.(*object.Channel); ok {
			if ch.ChanFieldType != nil && ch.ChanFieldType.Elem != nil {
				elemFieldType := ch.ChanFieldType.Elem
				resolvedType := e.resolver.ResolveType(ctx, elemFieldType)
				placeholder := &object.SymbolicPlaceholder{
					Reason: fmt.Sprintf("value received from channel of type %s", ch.ChanFieldType.String()),
				}
				placeholder.SetFieldType(elemFieldType)
				placeholder.SetTypeInfo(resolvedType)
				return placeholder
			}
		}
		// Fallback for untyped or non-channel objects
		return &object.SymbolicPlaceholder{Reason: "value received from non-channel or untyped object"}
	default:
		return e.newError(ctx, node.Pos(), "unknown unary operator: %s", node.Op)
	}
}

func (e *Evaluator) evalBangOperatorExpression(right object.Object) object.Object {
	// If the operand is a symbolic placeholder, the result is also a symbolic placeholder.
	if _, ok := right.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "result of ! on symbolic value"}
	}

	switch right {
	case object.TRUE:
		return object.FALSE
	case object.FALSE:
		return object.TRUE
	case object.NIL:
		return object.TRUE
	default:
		// In Go, `!` is only for booleans. For symbolic execution,
		// we might encounter other types. We'll treat them as "truthy"
		// (so !non-boolean is false), which is a common scripty behavior,
		// but a more rigorous implementation might error here.
		return object.FALSE
	}
}

func (e *Evaluator) evalNumericUnaryExpression(ctx context.Context, op token.Token, right object.Object) object.Object {
	// If the operand is a symbolic placeholder, the result is also a symbolic placeholder.
	if _, ok := right.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of unary operator %s on symbolic value", op)}
	}

	if right.Type() != object.INTEGER_OBJ {
		// Allow unary minus on floats and complex numbers later if needed.
		return e.newError(ctx, token.NoPos, "unary operator %s not supported for type %s", op, right.Type())
	}
	value := right.(*object.Integer).Value

	switch op {
	case token.SUB:
		return &object.Integer{Value: -value}
	case token.ADD:
		return &object.Integer{Value: value} // Unary plus is a no-op.
	case token.XOR:
		return &object.Integer{Value: ^value} // Bitwise NOT.
	default:
		// This case should be unreachable due to the switch in evalUnaryExpr.
		return e.newError(ctx, token.NoPos, "unhandled numeric unary operator: %s", op)
	}
}

func (e *Evaluator) evalStarExpr(ctx context.Context, node *ast.StarExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	val := e.Eval(ctx, node.X, env, pkg)
	if isError(val) {
		return val
	}

	// First, unwrap any variable to get to the underlying value.
	if v, ok := val.(*object.Variable); ok {
		val = v.Value
	}

	if ptr, ok := val.(*object.Pointer); ok {
		// If we are dereferencing a pointer to an unresolved type, the result is
		// a symbolic placeholder representing an instance of that type.
		if ut, ok := ptr.Value.(*object.UnresolvedType); ok {
			placeholder := &object.SymbolicPlaceholder{
				Reason: fmt.Sprintf("instance of unresolved type %s.%s", ut.PkgPath, ut.TypeName),
			}
			// Attempt to resolve the type to attach its info to the placeholder
			if resolvedType, err := e.resolver.ResolvePackage(ctx, ut.PkgPath); err == nil {
				for _, t := range resolvedType.Types {
					if t.Name == ut.TypeName {
						placeholder.SetTypeInfo(t)
						break
					}
				}
			}
			return placeholder
		}

		// The value of a pointer is the object it points to.
		// By returning the pointee directly, a selector expression like `(*p).MyMethod`
		// will operate on the instance, which is the correct behavior.
		return ptr.Value
	}

	// If we have a symbolic placeholder that represents a pointer type,
	// dereferencing it should result in a new placeholder representing the element type.
	if sp, ok := val.(*object.SymbolicPlaceholder); ok {
		var elemFieldType *scan.FieldType
		var resolvedElem *scan.TypeInfo
		if ft := sp.FieldType(); ft != nil && ft.IsPointer && ft.Elem != nil {
			elemFieldType = ft.Elem
			resolvedElem = e.resolver.ResolveType(ctx, elemFieldType)
		}
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("dereferenced from %s", sp.Reason),
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  resolvedElem,
				ResolvedFieldType: elemFieldType,
			},
		}
	}

	// NEW: Handle dereferencing a type object itself.
	// This can happen in method calls on symbolic receivers.
	if t, ok := val.(*object.Type); ok {
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("instance of type %s from dereference", t.TypeName),
			BaseObject: object.BaseObject{
				ResolvedTypeInfo: t.ResolvedType,
			},
		}
	}

	// Handle dereferencing an unresolved type object itself. This is the source
	// of the "invalid indirect" errors seen in the find-orphans run.
	if ut, ok := val.(*object.UnresolvedType); ok {
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("instance of unresolved type %s.%s from dereference", ut.PkgPath, ut.TypeName),
		}
	}

	// If we are trying to dereference a symbolic placeholder that isn't a pointer,
	// we shouldn't error out, but return another placeholder. This allows analysis
	// of incorrect but plausible code paths to continue.
	if _, ok := val.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("dereference of non-pointer symbolic value %s", val.Inspect())}
	}

	return e.newError(ctx, node.Pos(), "invalid indirect of %s (type %T)", val.Inspect(), val)
}

func (e *Evaluator) evalGenDecl(ctx context.Context, node *ast.GenDecl, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	switch node.Tok {
	case token.VAR:
		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, node.Pos(), "package info or fset is missing, cannot resolve types")
		}
		file := pkg.Fset.File(node.Pos())
		if file == nil {
			return e.newError(ctx, node.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(ctx, node.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		for _, spec := range node.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			var staticFieldType *scan.FieldType
			if valSpec.Type != nil {
				staticFieldType = e.scanner.TypeInfoFromExpr(ctx, valSpec.Type, nil, pkg, importLookup)
			}

			for i, name := range valSpec.Names {
				var val object.Object
				var resolvedTypeInfo *scan.TypeInfo
				if staticFieldType != nil {
					resolvedTypeInfo = e.resolver.ResolveType(ctx, staticFieldType)
				}

				if i < len(valSpec.Values) {
					val = e.Eval(ctx, valSpec.Values[i], env, pkg)
					if isError(val) {
						return val
					}
					if ret, ok := val.(*object.ReturnValue); ok {
						val = ret.Value
					}
				} else {
					placeholder := &object.SymbolicPlaceholder{Reason: "uninitialized variable"}
					if staticFieldType != nil {
						placeholder.SetFieldType(staticFieldType)
						placeholder.SetTypeInfo(resolvedTypeInfo)
					}
					val = placeholder
				}

				v := &object.Variable{
					Name:        name.Name,
					Value:       val,
					IsEvaluated: true,
					DeclPkg:     pkg,
				}
				v.SetFieldType(val.FieldType())
				v.SetTypeInfo(val.TypeInfo())

				if staticFieldType != nil {
					if v.FieldType() == nil {
						v.SetFieldType(staticFieldType)
					}
					if v.TypeInfo() == nil {
						v.SetTypeInfo(resolvedTypeInfo)
					}
				}
				env.Set(name.Name, v)
			}
		}
	case token.TYPE:
		e.evalTypeDecl(ctx, node, env, pkg)
	}
	return nil
}

func (e *Evaluator) evalTypeDecl(ctx context.Context, d *ast.GenDecl, env *object.Environment, pkg *scan.PackageInfo) {
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		// Find the TypeInfo that the scanner created for this TypeSpec.
		var typeInfo *scan.TypeInfo
		for _, ti := range pkg.Types {
			if ti.Node == ts {
				typeInfo = ti
				break
			}
		}

		if typeInfo == nil {
			// This shouldn't happen if the scanner ran correctly.
			continue
		}

		typeObj := &object.Type{
			TypeName:     typeInfo.Name,
			ResolvedType: typeInfo,
		}
		typeObj.SetTypeInfo(typeInfo)
		env.Set(ts.Name.Name, typeObj)
	}
}

func (e *Evaluator) evalFile(ctx context.Context, file *ast.File, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Get the canonical package object for this file. This is the source of truth
	// for the package's isolated environment.
	pkgObj, err := e.getOrLoadPackage(ctx, pkg.ImportPath)
	if err != nil {
		e.logc(ctx, slog.LevelWarn, "could not load package for file evaluation", "package", pkg.ImportPath, "error", err)
		// We cannot proceed with evaluation if the package context cannot be loaded.
		return nil
	}
	targetEnv := pkgObj.Env // Always use the package's own, isolated environment.

	// Populate package-level constants and functions once per package.
	// This will correctly populate targetEnv.
	e.ensurePackageEnvPopulated(ctx, pkgObj)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			switch d.Tok {
			case token.VAR:
				e.evalGenDecl(ctx, d, targetEnv, pkg)
			case token.TYPE:
				e.evalTypeDecl(ctx, d, targetEnv, pkg)
			}
		case *ast.FuncDecl:
			var funcInfo *scan.FunctionInfo
			for _, f := range pkg.Functions {
				if f.AstDecl == d {
					funcInfo = f
					break
				}
			}
			// When creating the function object, it's critical that its definition
			// environment (`Env`) is the isolated environment of its package.
			fn := &object.Function{
				Name:       d.Name,
				Parameters: d.Type.Params,
				Body:       d.Body,
				Env:        targetEnv, // Use the package's own environment
				Decl:       d,
				Package:    pkg,
				Def:        funcInfo,
			}
			targetEnv.Set(d.Name.Name, fn) // Set in the package's own environment
		}
	}
	return nil
}

// convertGoConstant converts a go/constant.Value to a symgo/object.Object.
func (e *Evaluator) convertGoConstant(ctx context.Context, val constant.Value, pos token.Pos) object.Object {
	switch val.Kind() {
	case constant.String:
		return &object.String{Value: constant.StringVal(val)}
	case constant.Int:
		i, ok := constant.Int64Val(val)
		if !ok {
			// This might be a large integer that doesn't fit in int64.
			// For symbolic execution, this is an acceptable limitation for now.
			return e.newError(ctx, pos, "could not convert constant to int64: %s", val.String())
		}
		return &object.Integer{Value: i}
	case constant.Bool:
		return nativeBoolToBooleanObject(constant.BoolVal(val))
	case constant.Float:
		f, _ := constant.Float64Val(val)
		return &object.Float{Value: f}
	case constant.Complex:
		r, _ := constant.Float64Val(constant.Real(val))
		i, _ := constant.Float64Val(constant.Imag(val))
		return &object.Complex{Value: complex(r, i)}
	default:
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("unsupported constant kind: %s", val.Kind())}
	}
}

func (e *Evaluator) getOrLoadPackage(ctx context.Context, path string) (*object.Package, error) {
	e.logc(ctx, slog.LevelDebug, "getOrLoadPackage: requesting package", "path", path)
	if pkg, ok := e.pkgCache[path]; ok {
		e.logc(ctx, slog.LevelDebug, "getOrLoadPackage: found in cache", "path", path, "scanned", pkg.ScannedInfo != nil)
		// Ensure even cached packages are populated if they were created as placeholders first.
		e.ensurePackageEnvPopulated(ctx, pkg)
		return pkg, nil
	}

	// Use the policy-enforcing ResolvePackage method.
	scannedPkg, err := e.resolver.ResolvePackage(ctx, path)
	if err != nil {
		// This error now occurs if the package is excluded by policy OR if scanning fails.
		// In either case, we create a placeholder package object to cache the result
		// and avoid re-scanning. The ScannedInfo will be nil.
		e.logc(ctx, slog.LevelDebug, "package resolution failed or denied by policy", "package", path, "error", err)
		pkgObj := &object.Package{
			Name:        "", // We don't know the name yet.
			Path:        path,
			Env:         object.NewEnclosedEnvironment(e.UniverseEnv),
			ScannedInfo: nil, // Mark as not scanned.
		}
		e.pkgCache[path] = pkgObj
		// We return the placeholder object itself, not an error, because failing to load
		// a package due to policy is not an evaluation-halting error.
		return pkgObj, nil
	}

	pkgObj := &object.Package{
		Name:        scannedPkg.Name,
		Path:        scannedPkg.ImportPath,
		Env:         object.NewEnclosedEnvironment(e.UniverseEnv),
		ScannedInfo: scannedPkg,
	}

	e.ensurePackageEnvPopulated(ctx, pkgObj)
	e.pkgCache[path] = pkgObj
	return pkgObj, nil
}

func (e *Evaluator) ensurePackageEnvPopulated(ctx context.Context, pkgObj *object.Package) {
	e.logc(ctx, slog.LevelDebug, "ensurePackageEnvPopulated: checking package", "path", pkgObj.Path, "scanned", pkgObj.ScannedInfo != nil)
	if pkgObj.ScannedInfo == nil {
		return // Not scanned yet, nothing to populate.
	}

	// If we have already populated this package's environment, do nothing.
	if e.initializedPkgs[pkgObj.Path] {
		return
	}

	pkgInfo := pkgObj.ScannedInfo
	env := pkgObj.Env
	shouldScan := e.resolver.ScanPolicy(pkgInfo.ImportPath)

	e.logger.DebugContext(ctx, "populating package-level constants", "package", pkgInfo.ImportPath)

	// Populate constants
	for _, c := range pkgInfo.Constants {
		if !shouldScan && !c.IsExported {
			continue
		}
		constObj := e.convertGoConstant(ctx, c.ConstVal, token.NoPos)
		if isError(constObj) {
			e.logc(ctx, slog.LevelWarn, "could not convert constant to object", "const", c.Name, "error", constObj)
			continue
		}
		env.SetLocal(c.Name, constObj)
	}

	// Populate variables (lazily)
	for _, v := range pkgInfo.Variables {
		if !shouldScan && !v.IsExported {
			continue
		}
		if v.GenDecl == nil {
			continue
		}

		// A single var declaration can have multiple specs (e.g., var ( a=1; b=2 )).
		// We need to find the right spec for the current variable `v`.
		for _, spec := range v.GenDecl.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				// Check if this spec contains our variable `v.Name`.
				var valueIndex = -1
				for i, nameIdent := range vs.Names {
					if nameIdent.Name == v.Name {
						valueIndex = i
						break
					}
				}

				// If we found our variable in this spec, determine its initializer.
				if valueIndex != -1 {
					var initializer ast.Expr
					// Case 1: var a, b = 1, 2 (1-to-1 mapping)
					if len(vs.Values) == len(vs.Names) {
						initializer = vs.Values[valueIndex]
					}
					// Case 2: var a, b = f() (multi-value return from a single call)
					if len(vs.Values) == 1 {
						initializer = vs.Values[0]
					}
					// Case 3: var a, b string (no initializer) -> initializer remains nil.

					lazyVar := &object.Variable{
						Name:        v.Name,
						IsEvaluated: false,
						Initializer: initializer,
						DeclEnv:     env,
						DeclPkg:     pkgInfo,
					}
					lazyVar.SetFieldType(v.Type) // Set the static type from the declaration
					env.SetLocal(v.Name, lazyVar)
					break // Found the right spec, move to the next variable in pkgInfo.Variables
				}
			}
		}
	}

	// Populate functions
	for _, f := range pkgInfo.Functions {
		if !shouldScan && !ast.IsExported(f.Name) {
			continue
		}
		fnObject := e.getOrResolveFunction(ctx, pkgObj, f)
		env.SetLocal(f.Name, fnObject)
	}

	// Mark this package as fully populated.
	e.initializedPkgs[pkgObj.Path] = true
}

// evalSymbolicSelection centralizes the logic for handling a selector expression (e.g., `x.Field` or `x.Method()`)
// where `x` is a symbolic placeholder. This is a common case when dealing with values of unresolved types.
func (e *Evaluator) evalSymbolicSelection(ctx context.Context, val *object.SymbolicPlaceholder, sel *ast.Ident, env *object.Environment, receiver object.Object, receiverPos token.Pos) object.Object {
	typeInfo := val.TypeInfo()
	if typeInfo == nil {
		// If we are calling a method on a placeholder that has no type info (e.g., from an
		// undefined identifier in an out-of-policy package), we can't resolve the method.
		// Instead of erroring, we return another placeholder representing the result of the call.
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of call to method %q on typeless placeholder", sel.Name)}
	}
	fullTypeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	key := fmt.Sprintf("(*%s).%s", fullTypeName, sel.Name)
	if intrinsicFn, ok := e.intrinsics.Get(key); ok {
		self := val
		fn := func(ctx context.Context, args ...object.Object) object.Object {
			return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
		}
		return &object.Intrinsic{Fn: fn}
	}
	key = fmt.Sprintf("(%s).%s", fullTypeName, sel.Name)
	if intrinsicFn, ok := e.intrinsics.Get(key); ok {
		self := val
		fn := func(ctx context.Context, args ...object.Object) object.Object {
			return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
		}
		return &object.Intrinsic{Fn: fn}
	}

	// Fallback to searching for the method on the instance's type.
	if typeInfo := val.TypeInfo(); typeInfo != nil {
		if method, err := e.accessor.findMethodOnType(ctx, typeInfo, sel.Name, env, receiver, receiverPos); err == nil && method != nil {
			return method
		}

		// If it's not a method, check if it's a field on the struct (including embedded).
		// This must be done *before* the unresolved check, as an unresolved type can still have field info.
		if typeInfo.Struct != nil {
			if field, err := e.accessor.findFieldOnType(ctx, typeInfo, sel.Name); err == nil && field != nil {
				return e.resolver.ResolveSymbolicField(ctx, field, val)
			}
		}

		if typeInfo.Unresolved {
			placeholder := &object.SymbolicPlaceholder{
				Reason:   fmt.Sprintf("symbolic method call %s on unresolved symbolic type %s", sel.Name, typeInfo.Name),
				Receiver: val,
			}
			// Try to find method in interface definition if available
			if typeInfo.Interface != nil {
				for _, method := range typeInfo.Interface.Methods {
					if method.Name == sel.Name {
						// Convert MethodInfo to a temporary FunctionInfo
						placeholder.UnderlyingFunc = &scan.FunctionInfo{
							Name:       method.Name,
							Parameters: method.Parameters,
							Results:    method.Results,
						}
						break
					}
				}
			}
			return placeholder
		}
	}

	// For symbolic placeholders, don't error - return another placeholder
	// This allows analysis to continue even when types are unresolved
	return &object.SymbolicPlaceholder{
		Reason:   "method or field " + sel.Name + " on symbolic type " + val.Inspect(),
		Receiver: val,
	}
}

func (e *Evaluator) evalSelectorExpr(ctx context.Context, n *ast.SelectorExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	e.logger.Debug("evalSelectorExpr", "selector", n.Sel.Name)

	// New, more robust check for interface method calls.
	// Instead of relying on the scanner's static analysis of the expression, we look up
	// the variable in our own environment and check the static type info we have stored on it.
	if ident, ok := n.X.(*ast.Ident); ok {
		if obj, found := env.Get(ident.Name); found {
			var staticType *scan.TypeInfo
			if v, isVar := obj.(*object.Variable); isVar {
				if ft := v.FieldType(); ft != nil {
					resolved, err := ft.Resolve(ctx)
					if err == nil && resolved != nil {
						staticType = resolved
					}
				} else if ti := v.TypeInfo(); ti != nil {
					staticType = ti
				}
			} else if sp, isSym := obj.(*object.SymbolicPlaceholder); isSym {
				if ti := sp.TypeInfo(); ti != nil {
					staticType = ti
				}
			}

			if staticType != nil && staticType.Kind == scan.InterfaceKind {
				// Check for a registered intrinsic for this interface method call.
				key := fmt.Sprintf("(%s.%s).%s", staticType.PkgPath, staticType.Name, n.Sel.Name)
				if intrinsicFn, ok := e.intrinsics.Get(key); ok {
					// Create a closure that prepends the receiver to the arguments.
					boundIntrinsic := func(ctx context.Context, args ...object.Object) object.Object {
						return intrinsicFn(ctx, append([]object.Object{obj}, args...)...)
					}
					return &object.Intrinsic{Fn: boundIntrinsic}
				}

				// Check for a manual interface binding.
				bindingKey := fmt.Sprintf("%s.%s", staticType.PkgPath, staticType.Name)
				if binding, ok := e.interfaceBindings[bindingKey]; ok {
					concreteType := binding.ConcreteType
					var fullReceiverName string
					if binding.IsPointer {
						fullReceiverName = fmt.Sprintf("*%s.%s", concreteType.PkgPath, concreteType.Name)
					} else {
						fullReceiverName = fmt.Sprintf("%s.%s", concreteType.PkgPath, concreteType.Name)
					}

					// Check for an intrinsic on the concrete type's method.
					// The key format is "(*pkg.path.Name).MethodName"
					intrinsicKey := fmt.Sprintf("(%s).%s", fullReceiverName, n.Sel.Name)
					if intrinsicFn, ok := e.intrinsics.Get(intrinsicKey); ok {
						boundIntrinsic := func(ctx context.Context, args ...object.Object) object.Object {
							// The intrinsic expects the receiver as the first argument.
							// The original object `obj` (the interface variable) is the logical receiver.
							return intrinsicFn(ctx, append([]object.Object{obj}, args...)...)
						}
						return &object.Intrinsic{Fn: boundIntrinsic}
					}

					// Fallback: find the method on the concrete type.
					// We need to create a synthetic pointer type if the binding is for a pointer.
					typeToSearch := concreteType
					if binding.IsPointer {
						ptrType := *concreteType // copy
						ptrType.Name = "*" + concreteType.Name
						typeToSearch = &ptrType
					}
					if method, err := e.accessor.findMethodOnType(ctx, typeToSearch, n.Sel.Name, env, obj, n.X.Pos()); err == nil && method != nil {
						return method
					}
				}

				// Correct approach: Return a callable placeholder.
				// a. Record the call if the interface is named.
				if staticType.Name != "" {
					key := fmt.Sprintf("%s.%s.%s", staticType.PkgPath, staticType.Name, n.Sel.Name)
					e.logger.DebugContext(ctx, "evalSelectorExpr: recording interface method call", "key", key)
					receiverObj := e.Eval(ctx, n.X, env, pkg)
					e.calledInterfaceMethods[key] = append(e.calledInterfaceMethods[key], receiverObj)
				}

				// b. Find the method definition, checking static, then synthetic, then creating a new one.
				var methodInfo *scan.MethodInfo
				ifaceKey := staticType.PkgPath + "." + staticType.Name

				// Check static methods first.
				if staticType.Interface != nil {
					allMethods := e.getAllInterfaceMethods(ctx, staticType, make(map[string]struct{}))
					for _, method := range allMethods {
						if method.Name == n.Sel.Name {
							methodInfo = method
							break
						}
					}
				}

				// If not found, check the synthetic cache.
				if methodInfo == nil {
					e.syntheticMethodsMutex.Lock()
					if methods, ok := e.syntheticMethods[ifaceKey]; ok {
						methodInfo = methods[n.Sel.Name]
					}
					e.syntheticMethodsMutex.Unlock()
				}

				// If still not found, create a new synthetic method and cache it.
				if methodInfo == nil {
					e.logc(ctx, slog.LevelInfo, "undefined method on interface, creating synthetic method", "interface", staticType.Name, "method", n.Sel.Name)
					methodInfo = &scan.MethodInfo{
						Name:       n.Sel.Name,
						Parameters: []*scan.FieldInfo{}, // Parameters are unknown
						Results:    []*scan.FieldInfo{}, // Results are unknown
					}

					e.syntheticMethodsMutex.Lock()
					if _, ok := e.syntheticMethods[ifaceKey]; !ok {
						e.syntheticMethods[ifaceKey] = make(map[string]*scan.MethodInfo)
					}
					e.syntheticMethods[ifaceKey][n.Sel.Name] = methodInfo
					e.syntheticMethodsMutex.Unlock()
				}

				// Convert the found/created MethodInfo to a FunctionInfo for the placeholder.
				methodFuncInfo := &scan.FunctionInfo{
					Name:       methodInfo.Name,
					Parameters: methodInfo.Parameters,
					Results:    methodInfo.Results,
				}

				// c. Return a callable SymbolicPlaceholder.
				return &object.SymbolicPlaceholder{
					Reason:         fmt.Sprintf("interface method %s.%s", staticType.Name, n.Sel.Name),
					Receiver:       obj, // Pass the variable object itself as the receiver
					UnderlyingFunc: methodFuncInfo,
					Package:        pkg,
				}
			}

			// NEW: Handle struct field access on variables directly
			if staticType != nil && staticType.Kind == scan.StructKind {
				if field, err := e.accessor.findFieldOnType(ctx, staticType, n.Sel.Name); err == nil && field != nil {
					var fieldValue object.Object
					if v, isVar := obj.(*object.Variable); isVar {
						fieldValue = e.evalVariable(ctx, v, pkg)
					} else {
						fieldValue = obj // Should be a placeholder or instance
					}
					return e.resolver.ResolveSymbolicField(ctx, field, fieldValue)
				}
			}
		}
	}

	leftObj := e.Eval(ctx, n.X, env, pkg)
	if isError(leftObj) {
		return leftObj
	}

	// Unwrap the result if it's a return value from a previous call in a chain.
	if ret, ok := leftObj.(*object.ReturnValue); ok {
		leftObj = ret.Value
	}

	// We must fully evaluate the left-hand side before trying to select a field or method from it.
	left := e.forceEval(ctx, leftObj, pkg)
	if isError(left) {
		return left
	}

	e.logger.Debug("evalSelectorExpr: evaluated left", "type", left.Type(), "value", inspectValuer{left})

	switch val := left.(type) {
	case *object.SymbolicPlaceholder:
		return e.evalSymbolicSelection(ctx, val, n.Sel, env, val, n.X.Pos())

	case *object.Package:
		e.logc(ctx, slog.LevelDebug, "evalSelectorExpr: left is a package", "package", val.Path, "selector", n.Sel.Name)

		// If the package object is just a shell, try to fully load it now.
		if val.ScannedInfo == nil {
			e.logc(ctx, slog.LevelDebug, "evalSelectorExpr: package not scanned, attempting to load", "package", val.Path)
			loadedPkg, err := e.getOrLoadPackage(ctx, val.Path)
			if err != nil {
				// if loading fails, it's a real error
				return e.newError(ctx, n.Pos(), "failed to load package %s: %v", val.Path, err)
			}
			// Replace the shell package object with the fully loaded one for the rest of the logic.
			val = loadedPkg
		}

		// If ScannedInfo is still nil after trying to load, it means it's out of policy.
		if val.ScannedInfo == nil {
			e.logc(ctx, slog.LevelDebug, "package not scanned (out of policy), creating placeholder for symbol", "package", val.Path, "symbol", n.Sel.Name)
			// When a symbol is from an unscanned package, we don't know if it's a type or a function.
			// We now correctly represent it as a generic UnresolvedType.
			// The consumer of this object (e.g., `new()` or a function call) will determine how to interpret it.
			unresolvedType := &object.UnresolvedType{
				PkgPath:  val.Path,
				TypeName: n.Sel.Name,
			}
			val.Env.Set(n.Sel.Name, unresolvedType)
			return unresolvedType
		}

		// When we encounter a package selector, we must ensure its environment
		// is populated with all its top-level declarations. This is crucial
		// for closures to capture their environment correctly.
		e.ensurePackageEnvPopulated(ctx, val)

		key := val.Path + "." + n.Sel.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return &object.Intrinsic{Fn: intrinsicFn}
		}

		// If the symbol is already in the package's environment, return it.
		if symbol, ok := val.Env.Get(n.Sel.Name); ok {
			// If the cached symbol is not callable, but a function with the same name exists,
			// it's a sign of cache pollution. We prioritize the function.
			if !isCallable(symbol) {
				for _, f := range val.ScannedInfo.Functions {
					if f.Name == n.Sel.Name {
						e.logc(ctx, slog.LevelWarn, "correcting polluted cache: found function for non-callable symbol", "package", val.Path, "symbol", n.Sel.Name)
						fnObject := e.getOrResolveFunction(ctx, val, f)
						val.Env.Set(n.Sel.Name, fnObject) // Correct the cache
						return fnObject
					}
				}
			}
			return symbol
		}

		// Try to find the symbol as a function.
		for _, f := range val.ScannedInfo.Functions {
			if f.Name == n.Sel.Name {
				if !ast.IsExported(f.Name) {
					continue
				}

				// Delegate function object creation to the resolver.
				fnObject := e.getOrResolveFunction(ctx, val, f)
				val.Env.Set(n.Sel.Name, fnObject)
				return fnObject
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
						return e.newError(ctx, n.Pos(), "could not convert constant %s to int64", c.Name)
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

		// Check for types.
		for _, t := range val.ScannedInfo.Types {
			if t.Name == n.Sel.Name {
				if !ast.IsExported(t.Name) {
					continue
				}
				typeObj := &object.Type{
					TypeName:     t.Name,
					ResolvedType: t,
				}
				typeObj.SetTypeInfo(t)
				val.Env.Set(n.Sel.Name, typeObj) // Cache it
				return typeObj
			}
		}

		// Check for variables.
		for _, v := range val.ScannedInfo.Variables {
			if v.Name == n.Sel.Name {
				if !ast.IsExported(v.Name) {
					continue
				}
				resolvedType := e.resolver.ResolveType(ctx, v.Type)
				placeholder := &object.SymbolicPlaceholder{
					Reason: fmt.Sprintf("external variable %s.%s", val.Path, v.Name),
				}
				placeholder.SetFieldType(v.Type)
				placeholder.SetTypeInfo(resolvedType)

				val.Env.Set(n.Sel.Name, placeholder)
				return placeholder
			}
		}

		// If the symbol is not found, assume it's a function we can't see
		// due to the scan policy. Create an UnresolvedFunction object.
		// This allows `applyFunction` to handle it gracefully.
		unresolvedFn := &object.UnresolvedFunction{
			PkgPath:  val.Path,
			FuncName: n.Sel.Name,
		}
		val.Env.Set(n.Sel.Name, unresolvedFn)
		return unresolvedFn

	case *object.Instance:
		key := fmt.Sprintf("(%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(ctx context.Context, args ...object.Object) object.Object {
				return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		key = fmt.Sprintf("(*%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(ctx context.Context, args ...object.Object) object.Object {
				return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}

		// Fallback to searching for the method on the instance's type.
		if typeInfo := val.TypeInfo(); typeInfo != nil {
			if method, err := e.accessor.findMethodOnType(ctx, typeInfo, n.Sel.Name, env, val, n.X.Pos()); err == nil && method != nil {
				return method
			}
			// If not a method, check if it's a field on the struct (including embedded).
			if typeInfo.Struct != nil {
				if field, err := e.accessor.findFieldOnType(ctx, typeInfo, n.Sel.Name); err == nil && field != nil {
					return e.resolver.ResolveSymbolicField(ctx, field, val)
				}
			}
		}

		return e.newError(ctx, n.Pos(), "undefined method or field: %s on %s", n.Sel.Name, val.TypeName)
	case *object.Pointer:
		// When we have a selector on a pointer, we look for the method on the
		// type of the object the pointer points to.
		pointee := val.Value
		if instance, ok := pointee.(*object.Instance); ok {
			if typeInfo := instance.TypeInfo(); typeInfo != nil {
				// The receiver for the method call is the pointer itself, not the instance.
				if method, err := e.accessor.findMethodOnType(ctx, typeInfo, n.Sel.Name, env, val, n.X.Pos()); err == nil && method != nil {
					return method
				}
				// If not a method, check for a field on the underlying struct.
				if typeInfo.Struct != nil {
					if field, err := e.accessor.findFieldOnType(ctx, typeInfo, n.Sel.Name); err == nil && field != nil {
						return e.resolver.ResolveSymbolicField(ctx, field, instance)
					}
				}
			}
		}

		// Handle pointers to symbolic placeholders, which can occur with pointers to unresolved types.
		if sp, ok := pointee.(*object.SymbolicPlaceholder); ok {
			// The logic for selecting from a symbolic placeholder is already well-defined.
			// We can simulate calling that logic with the pointee. The receiver for any
			// method call is the pointer `val`, not the placeholder `sp`.
			// This is effectively doing `(*p).N` where `*p` is a symbolic value.
			return e.evalSymbolicSelection(ctx, sp, n.Sel, env, val, n.X.Pos())
		}

		// If the pointee is not an instance or nothing is found, fall through to the error.
		return e.newError(ctx, n.Pos(), "undefined method or field: %s for pointer type %s", n.Sel.Name, pointee.Type())

	case *object.Nil:
		// Nil can have methods in Go (e.g., interface with nil value).
		// Check if we have type information for this nil (it might be a typed nil interface)
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("method %s on nil", n.Sel.Name),
		}

		// If the NIL has type information (e.g., it's a typed interface nil),
		// try to find the method in the interface definition
		if left.TypeInfo() != nil && left.TypeInfo().Interface != nil {
			for _, method := range left.TypeInfo().Interface.Methods {
				if method.Name == n.Sel.Name {
					placeholder.UnderlyingFunc = &scan.FunctionInfo{
						Name:       method.Name,
						Parameters: method.Parameters,
						Results:    method.Results,
					}
					placeholder.Receiver = left
					break
				}
			}
		}

		return placeholder

	case *object.UnresolvedType:
		// If we are selecting from an unresolved type, we can't know what the field or method is.
		// We return a placeholder to allow analysis to continue.
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("selection from unresolved type %s.%s", val.PkgPath, val.TypeName),
		}
	default:
		return e.newError(ctx, n.Pos(), "expected a package, instance, or pointer on the left side of selector, but got %s", left.Type())
	}
}

func (e *Evaluator) evalSwitchStmt(ctx context.Context, n *ast.SwitchStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	switchEnv := env
	if n.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, switchEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	if n.Body == nil {
		return &object.SymbolicPlaceholder{Reason: "switch statement"}
	}

	// Iterate through each case clause as a potential starting point for a new execution path.
	for i := 0; i < len(n.Body.List); i++ {
		pathEnv := object.NewEnclosedEnvironment(switchEnv) // Each path gets its own environment to track state.

		// From this starting point `i`, trace the path until a break or the end of a case without fallthrough.
	pathLoop:
		for j := i; j < len(n.Body.List); j++ {
			caseClause, ok := n.Body.List[j].(*ast.CaseClause)
			if !ok {
				continue
			}

			// Evaluate case expressions to trace calls for their side-effects.
			for _, expr := range caseClause.List {
				if res := e.Eval(ctx, expr, pathEnv, pkg); isError(res) {
					return res // Propagate errors from case expressions.
				}
			}

			hasFallthrough := false
			for _, stmt := range caseClause.Body {
				result := e.Eval(ctx, stmt, pathEnv, pkg)

				if result != nil {
					switch result.Type() {
					case object.FALLTHROUGH_OBJ:
						hasFallthrough = true
						break // Exit statement loop, continue to next case
					case object.BREAK_OBJ:
						break pathLoop // This path is terminated by break.
					case object.RETURN_VALUE_OBJ, object.ERROR_OBJ, object.CONTINUE_OBJ:
						// Propagate these control flow changes immediately, terminating the whole switch evaluation.
						// This is a simplification but consistent with if-stmt handling.
						return result
					}
				}
			}

			if !hasFallthrough {
				break // End of this path. Start a new path from the next case.
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "switch statement"}
}

func (e *Evaluator) evalSelectStmt(ctx context.Context, n *ast.SelectStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
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
					e.logc(ctx, slog.LevelWarn, "error evaluating select case communication", "error", res)
				}
			}

			// Evaluate the body of the case.
			for _, stmt := range caseClause.Body {
				if res := e.Eval(ctx, stmt, caseEnv, pkg); isError(res) {
					e.logc(ctx, slog.LevelWarn, "error evaluating statement in select case", "error", res)
					if isInfiniteRecursionError(res) {
						return res // Stop processing on infinite recursion
					}
				}
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "select statement"}
}

func (e *Evaluator) evalTypeSwitchStmt(ctx context.Context, n *ast.TypeSwitchStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	switchEnv := env
	if n.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, switchEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	var varName string
	var originalObj object.Object

	switch assign := n.Assign.(type) {
	case *ast.AssignStmt:
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return e.newError(ctx, n.Pos(), "expected one variable and one value in type switch assignment")
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return e.newError(ctx, n.Pos(), "expected identifier on LHS of type switch assignment")
		}
		varName = ident.Name

		typeAssert, ok := assign.Rhs[0].(*ast.TypeAssertExpr)
		if !ok {
			return e.newError(ctx, n.Pos(), "expected TypeAssertExpr on RHS of type switch assignment")
		}
		originalObj = e.Eval(ctx, typeAssert.X, switchEnv, pkg)
		if isError(originalObj) {
			return originalObj
		}

	case *ast.ExprStmt:
		typeAssert, ok := assign.X.(*ast.TypeAssertExpr)
		if !ok {
			return e.newError(ctx, n.Pos(), "expected TypeAssertExpr in ExprStmt of type switch")
		}
		// In `switch x.(type)`, there is no new variable, so varName remains empty.
		// We still need to evaluate the expression being switched on.
		originalObj = e.Eval(ctx, typeAssert.X, switchEnv, pkg)
		if isError(originalObj) {
			return originalObj
		}

	default:
		return e.newError(ctx, n.Pos(), "expected AssignStmt or ExprStmt in TypeSwitchStmt, got %T", n.Assign)
	}

	if n.Body != nil {
		file := pkg.Fset.File(n.Pos())
		if file == nil {
			return e.newError(ctx, n.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(ctx, n.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		for _, c := range n.Body.List {
			caseClause, ok := c.(*ast.CaseClause)
			if !ok {
				continue
			}
			caseEnv := object.NewEnclosedEnvironment(switchEnv)

			// If varName is set, we are in the `v := x.(type)` form.
			// We need to create a new variable `v` in the case's scope.
			if varName != "" {
				if caseClause.List == nil { // default case
					v := &object.Variable{
						Name:        varName,
						Value:       originalObj,
						IsEvaluated: true, // Mark as evaluated since originalObj is already set
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
							fieldType = &scan.FieldType{Name: id.Name, IsBuiltin: true}
						} else {
							return e.newError(ctx, typeExpr.Pos(), "could not resolve type for case clause")
						}
					}

					var resolvedType *scan.TypeInfo
					if !fieldType.IsBuiltin {
						resolvedType = e.resolver.ResolveType(ctx, fieldType)
						if resolvedType != nil && resolvedType.Kind == scan.UnknownKind {
							resolvedType.Kind = scan.InterfaceKind
						}
					}

					val := &object.SymbolicPlaceholder{
						Reason:     fmt.Sprintf("type switch case variable %s", fieldType.String()),
						BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
					}
					v := &object.Variable{
						Name:        varName,
						Value:       val,
						IsEvaluated: true,
						BaseObject: object.BaseObject{
							ResolvedTypeInfo:  resolvedType,
							ResolvedFieldType: fieldType,
						},
					}
					caseEnv.Set(varName, v)
				}
			}
			// If varName is empty, we are in the `x.(type)` form. No new variable is created.
			// The environment for the case body is just a new scope above the switch environment.

			for _, stmt := range caseClause.Body {
				if res := e.Eval(ctx, stmt, caseEnv, pkg); isError(res) {
					e.logc(ctx, slog.LevelWarn, "error evaluating statement in type switch case", "error", res)
					if isInfiniteRecursionError(res) {
						return res // Stop processing on infinite recursion
					}
				}
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "type switch statement"}
}

func (e *Evaluator) evalTypeAssertExpr(ctx context.Context, n *ast.TypeAssertExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
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
		return e.newError(ctx, n.Pos(), "package info or fset is missing, cannot resolve types for type assertion")
	}
	file := pkg.Fset.File(n.Pos())
	if file == nil {
		return e.newError(ctx, n.Pos(), "could not find file for node position")
	}
	astFile, ok := pkg.AstFiles[file.Name()]
	if !ok {
		return e.newError(ctx, n.Pos(), "could not find ast.File for path: %s", file.Name())
	}
	importLookup := e.scanner.BuildImportLookup(astFile)

	fieldType := e.scanner.TypeInfoFromExpr(ctx, n.Type, nil, pkg, importLookup)
	if fieldType == nil {
		var typeNameBuf bytes.Buffer
		printer.Fprint(&typeNameBuf, pkg.Fset, n.Type)
		return e.newError(ctx, n.Pos(), "could not resolve type for type assertion: %s", typeNameBuf.String())
	}
	resolvedType := e.resolver.ResolveType(ctx, fieldType)

	// If the type was unresolved, we can now infer its kind to be an interface.
	if resolvedType != nil && resolvedType.Kind == scan.UnknownKind {
		resolvedType.Kind = scan.InterfaceKind
	}

	// In the single-value form, the result is just a value of the asserted type.
	// We create a symbolic placeholder for it.
	return &object.SymbolicPlaceholder{
		Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
		BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
	}
}

func (e *Evaluator) evalForStmt(ctx context.Context, n *ast.ForStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// For symbolic execution, we unroll the loop once.
	// A more sophisticated engine might unroll N times or use summaries.
	forEnv := object.NewEnclosedEnvironment(env)

	if n.Init != nil {
		if initResult := e.Eval(ctx, n.Init, forEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	// Also evaluate the condition to trace any function calls within it.
	if n.Cond != nil {
		if condResult := e.Eval(ctx, n.Cond, forEnv, pkg); isError(condResult) {
			// If the condition errors, we can't proceed with analysis of this loop.
			return condResult
		}
	}

	// We don't check the condition's result, just execute the body once symbolically.
	if n.Body != nil {
		result := e.Eval(ctx, n.Body, object.NewEnclosedEnvironment(forEnv), pkg)
		if result != nil {
			switch obj := result.(type) {
			case *object.Break:
				// If the break has a label, it's for an outer loop. Propagate it.
				if obj.Label != "" {
					return obj
				}
				// Otherwise, it's for this loop, so we absorb it.
				return &object.SymbolicPlaceholder{Reason: "for loop"}
			case *object.Continue:
				// If the continue has a label, it's for an outer loop. Propagate it.
				if obj.Label != "" {
					return obj
				}
				// Otherwise, it's for this loop, so we absorb it.
				return &object.SymbolicPlaceholder{Reason: "for loop"}
			case *object.Error:
				return result // Propagate errors.
			}
		}
	}

	// The result of a for statement is not a value.
	return &object.SymbolicPlaceholder{Reason: "for loop"}
}

func (e *Evaluator) evalRangeStmt(ctx context.Context, n *ast.RangeStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
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
		switch obj := result.(type) {
		case *object.Break:
			if obj.Label != "" {
				return obj
			}
			return &object.SymbolicPlaceholder{Reason: "for-range loop"}
		case *object.Continue:
			if obj.Label != "" {
				return obj
			}
			return &object.SymbolicPlaceholder{Reason: "for-range loop"}
		case *object.Error:
			return result // Propagate errors.
		}
	}

	return &object.SymbolicPlaceholder{Reason: "for-range loop"}
}

func (e *Evaluator) evalBranchStmt(ctx context.Context, n *ast.BranchStmt) object.Object {
	var label string
	if n.Label != nil {
		label = n.Label.Name
	}

	switch n.Tok {
	case token.BREAK:
		return &object.Break{Label: label}
	case token.CONTINUE:
		return &object.Continue{Label: label}
	case token.FALLTHROUGH:
		return object.FALLTHROUGH
	default:
		return e.newError(ctx, n.Pos(), "unsupported branch statement: %s", n.Tok)
	}
}

func (e *Evaluator) evalLabeledStmt(ctx context.Context, n *ast.LabeledStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	result := e.Eval(ctx, n.Stmt, env, pkg)

	switch obj := result.(type) {
	case *object.Break:
		if obj.Label == n.Label.Name {
			// This break was for this label. Absorb it.
			return &object.SymbolicPlaceholder{Reason: "labeled statement"}
		}
	case *object.Continue:
		if obj.Label == n.Label.Name {
			// This continue was for this label. Absorb it and continue symbolic execution.
			return &object.SymbolicPlaceholder{Reason: "labeled statement"}
		}
	}

	// If it's a break/continue for another label, or any other kind of object,
	// just propagate it up.
	return result
}

func (e *Evaluator) evalIfStmt(ctx context.Context, n *ast.IfStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	ifStmtEnv := env
	if n.Init != nil {
		ifStmtEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, ifStmtEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	// Also evaluate the condition to trace any function calls within it.
	if n.Cond != nil {
		if condResult := e.Eval(ctx, n.Cond, ifStmtEnv, pkg); isError(condResult) {
			// If the condition errors, we can't proceed.
			return condResult
		}
	}

	// Evaluate both branches. Each gets its own enclosed environment.
	thenEnv := object.NewEnclosedEnvironment(ifStmtEnv)
	thenResult := e.Eval(ctx, n.Body, thenEnv, pkg)

	var elseResult object.Object
	if n.Else != nil {
		elseEnv := object.NewEnclosedEnvironment(ifStmtEnv)
		elseResult = e.Eval(ctx, n.Else, elseEnv, pkg)
	}

	// If the 'then' branch returned a control flow object, propagate it.
	// This is a heuristic; a more complex analysis might merge states.
	// We prioritize the 'then' branch's signal.
	// We do NOT propagate ReturnValue, as that would prematurely terminate
	// the analysis of the current function just because one symbolic path returned.
	switch thenResult.(type) {
	case *object.Error, *object.Break, *object.Continue:
		return thenResult
	}
	// Otherwise, check the 'else' branch.
	switch elseResult.(type) {
	case *object.Error, *object.Break, *object.Continue:
		return elseResult
	}

	// A more sophisticated, path-sensitive analysis would require a different
	// approach. For now, if no control flow signal was returned, we continue.
	return nil
}

func (e *Evaluator) evalBlockStatement(ctx context.Context, block *ast.BlockStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if block == nil {
		return nil // Function has no body, which is valid for declarations-only scanning.
	}
	var result object.Object
	// The caller is responsible for creating a new scope if one is needed.
	// We evaluate the statements in the provided environment.
	for _, stmt := range block.List {
		// If a statement is itself a block, it introduces a new lexical scope.
		if innerBlock, ok := stmt.(*ast.BlockStmt); ok {
			blockEnv := object.NewEnclosedEnvironment(env)
			result = e.evalBlockStatement(ctx, innerBlock, blockEnv, pkg)
		} else {
			result = e.Eval(ctx, stmt, env, pkg)
		}

		// It's possible for a statement (like a declaration) to evaluate to a nil object.
		// We must check for this before calling .Type() to avoid a panic.
		if result == nil {
			continue
		}

		switch result.(type) {
		case *object.ReturnValue, *object.Error, *object.Break, *object.Continue:
			return result
		}
	}

	return result
}

func (e *Evaluator) evalReturnStmt(ctx context.Context, n *ast.ReturnStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if len(n.Results) == 0 {
		return &object.ReturnValue{Value: object.NIL} // naked return
	}

	if len(n.Results) == 1 {
		val := e.Eval(ctx, n.Results[0], env, pkg)
		if isError(val) {
			return val
		}
		// The result of an expression must be fully evaluated before being returned.
		val = e.forceEval(ctx, val, pkg)
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

func (e *Evaluator) evalAssignStmt(ctx context.Context, n *ast.AssignStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Handle multi-value assignment, e.g., x, y := f() or x, y = f()
	if len(n.Rhs) == 1 && len(n.Lhs) > 1 {
		// Special case for two-value type assertions: v, ok := x.(T)
		if typeAssert, ok := n.Rhs[0].(*ast.TypeAssertExpr); ok {
			if len(n.Lhs) != 2 {
				return e.newError(ctx, n.Pos(), "type assertion with 2 values on RHS must have 2 variables on LHS, got %d", len(n.Lhs))
			}

			// Evaluate the source expression to trace calls
			e.Eval(ctx, typeAssert.X, env, pkg)

			// Resolve the asserted type (T).
			if pkg == nil || pkg.Fset == nil {
				return e.newError(ctx, n.Pos(), "package info or fset is missing, cannot resolve types for type assertion")
			}
			file := pkg.Fset.File(n.Pos())
			if file == nil {
				return e.newError(ctx, n.Pos(), "could not find file for node position")
			}
			astFile, ok := pkg.AstFiles[file.Name()]
			if !ok {
				return e.newError(ctx, n.Pos(), "could not find ast.File for path: %s", file.Name())
			}
			importLookup := e.scanner.BuildImportLookup(astFile)

			fieldType := e.scanner.TypeInfoFromExpr(ctx, typeAssert.Type, nil, pkg, importLookup)
			if fieldType == nil {
				var typeNameBuf bytes.Buffer
				printer.Fprint(&typeNameBuf, pkg.Fset, typeAssert.Type)
				return e.newError(ctx, typeAssert.Pos(), "could not resolve type for type assertion: %s", typeNameBuf.String())
			}
			resolvedType := e.resolver.ResolveType(ctx, fieldType)

			// If the type was unresolved, we can now infer its kind to be an interface.
			if resolvedType != nil && resolvedType.Kind == scan.UnknownKind {
				resolvedType.Kind = scan.InterfaceKind
			}

			// Create placeholders for the two return values.
			valuePlaceholder := &object.SymbolicPlaceholder{
				Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
			}

			okPlaceholder := &object.SymbolicPlaceholder{
				Reason: "ok from type assertion",
				BaseObject: object.BaseObject{
					ResolvedTypeInfo: nil, // Built-in types do not have a TypeInfo struct.
					ResolvedFieldType: &scan.FieldType{
						Name:      "bool",
						IsBuiltin: true,
					},
				},
			}

			// Assign the placeholders to the LHS variables.
			if ident, ok := n.Lhs[0].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ctx, ident, valuePlaceholder, n.Tok, env)
				}
			}
			if ident, ok := n.Lhs[1].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ctx, ident, okPlaceholder, n.Tok, env)
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

		// If the result is a single symbolic placeholder, but we expect multiple return values,
		// we can expand it into a MultiReturn object with the correct number of placeholders.
		// This handles calls to unscannable functions that are expected to return multiple values.
		if sp, isPlaceholder := rhsValue.(*object.SymbolicPlaceholder); isPlaceholder {
			if len(n.Lhs) > 1 {
				placeholders := make([]object.Object, len(n.Lhs))
				for i := 0; i < len(n.Lhs); i++ {
					// The first placeholder inherits the reason from the original.
					if i == 0 {
						placeholders[i] = sp
					} else {
						placeholders[i] = &object.SymbolicPlaceholder{
							Reason: fmt.Sprintf("inferred result %d from multi-value assignment to %s", i, sp.Reason),
						}
					}
				}
				rhsValue = &object.MultiReturn{Values: placeholders}
			}
		}

		multiRet, ok := rhsValue.(*object.MultiReturn)
		if !ok {
			// This can happen if a function that is supposed to return multiple values
			// is not correctly modeled. We fall back to assigning placeholders.
			e.logc(ctx, slog.LevelWarn, "expected multi-return value on RHS of assignment", "got_type", rhsValue.Type(), "value", inspectValuer{rhsValue})
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
			return e.newError(ctx, n.Pos(), "assignment mismatch: %d variables but %d values", len(n.Lhs), len(multiRet.Values))
		}

		for i, lhsExpr := range n.Lhs {
			if ident, ok := lhsExpr.(*ast.Ident); ok {
				if ident.Name == "_" {
					continue
				}
				val := multiRet.Values[i]
				e.assignIdentifier(ctx, ident, val, n.Tok, env) // Use the statement's token (:= or =)
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
			// This is an assignment to a field, like `foo.Bar = 1`.
			// We need to evaluate the `foo` part (lhs.X) to trace any calls within it.
			e.Eval(ctx, lhs.X, env, pkg)
			// Then evaluate the RHS.
			e.Eval(ctx, n.Rhs[0], env, pkg)
			return nil
		case *ast.IndexExpr:
			// This is an assignment to a map or slice index, like `m[k] = v`.
			// We need to evaluate all parts to trace calls.
			// 1. Evaluate the map/slice expression (e.g., `m`).
			e.Eval(ctx, lhs.X, env, pkg)
			// 2. Evaluate the index expression (e.g., `k`).
			e.Eval(ctx, lhs.Index, env, pkg)
			// 3. Evaluate the RHS value (e.g., `v`).
			e.Eval(ctx, n.Rhs[0], env, pkg)
			return nil
		case *ast.StarExpr:
			// This is an assignment to a pointer dereference, like `*p = v`.
			// Evaluate the pointer expression (e.g., `p`).
			e.Eval(ctx, lhs.X, env, pkg)
			// Evaluate the RHS value (e.g., `v`).
			e.Eval(ctx, n.Rhs[0], env, pkg)
			return nil
		default:
			return e.newError(ctx, n.Pos(), "unsupported assignment target: expected an identifier, selector or index expression, but got %T", lhs)
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
				e.assignIdentifier(ctx, ident, rhsValues[i], n.Tok, env)
			} else {
				// Handle other LHS types like selectors if needed in the future.
				e.logc(ctx, slog.LevelWarn, "unsupported LHS in parallel assignment", "type", fmt.Sprintf("%T", lhsExpr))
			}
		}
		return nil
	}

	return e.newError(ctx, n.Pos(), "unsupported assignment statement")
}

func (e *Evaluator) evalIdentAssignment(ctx context.Context, ident *ast.Ident, rhs ast.Expr, tok token.Token, env *object.Environment, pkg *scan.PackageInfo) object.Object {
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

	return e.assignIdentifier(ctx, ident, val, tok, env)
}

func (e *Evaluator) assignIdentifier(ctx context.Context, ident *ast.Ident, val object.Object, tok token.Token, env *object.Environment) object.Object {
	// Before assigning, the RHS must be fully evaluated.
	val = e.forceEval(ctx, val, nil) // pkg is not strictly needed here as DeclPkg is used.
	if isError(val) {
		return val
	}

	// For `:=`, we always define a new variable in the current scope.
	if tok == token.DEFINE {
		// In Go, `:=` can redeclare a variable if it's in a different scope,
		// but in our symbolic engine, we'll simplify and just overwrite in the local scope.
		// A more complex implementation would handle shadowing more precisely.
		v := &object.Variable{
			Name:        ident.Name,
			Value:       val,
			IsEvaluated: true, // A variable defined with `:=` has its value evaluated immediately.
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  val.TypeInfo(),
				ResolvedFieldType: val.FieldType(),
			},
		}
		if val.FieldType() != nil {
			if resolved := e.resolver.ResolveType(ctx, val.FieldType()); resolved != nil && resolved.Kind == scan.InterfaceKind {
				v.PossibleTypes = make(map[string]struct{})
				if ft := val.FieldType(); ft != nil {
					v.PossibleTypes[ft.String()] = struct{}{}
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
		return e.assignIdentifier(ctx, ident, val, token.DEFINE, env)
	}

	v, ok := obj.(*object.Variable)
	if !ok {
		// Not a variable, just overwrite it in the environment.
		e.logger.Debug("evalAssignStmt: overwriting non-variable in env", "name", ident.Name)
		return env.Set(ident.Name, val)
	}

	// If the variable's declared type is an interface, we should preserve that
	// static type information on the variable itself. The concrete type of the
	// assigned value is still available on `val` (which becomes `v.Value`).
	var isLHSInterface bool
	if ft := v.FieldType(); ft != nil {
		if ti := e.resolver.ResolveType(ctx, ft); ti != nil {
			isLHSInterface = ti.Kind == scan.InterfaceKind
		}
	}

	v.Value = val
	if !isLHSInterface {
		v.SetTypeInfo(val.TypeInfo())
		v.SetFieldType(val.FieldType())
	}
	newFieldType := val.FieldType()

	// Always accumulate possible types. Resetting the map can lead to lost
	// information, especially when dealing with interface assignments where the
	// static type of the variable might be unresolved.
	if v.PossibleTypes == nil {
		v.PossibleTypes = make(map[string]struct{})
	}
	if newFieldType != nil {
		key := newFieldType.String()

		// Workaround: If the default string representation of a pointer type is just "*",
		// it's likely because the underlying element's FieldType has an empty name.
		// In this case, we construct a more robust key using the TypeInfo from the
		// object the pointer points to. This makes the analysis resilient to
		// incomplete FieldType information from the scanner.
		if key == "*" {
			if ptr, ok := val.(*object.Pointer); ok {
				if inst, ok := ptr.Value.(*object.Instance); ok {
					if ti := inst.TypeInfo(); ti != nil && ti.PkgPath != "" && ti.Name != "" {
						key = fmt.Sprintf("%s.*%s", ti.PkgPath, ti.Name)
					}
				}
			}
		}

		v.PossibleTypes[key] = struct{}{}
		e.logger.Debug("evalAssignStmt: adding possible type to var", "name", ident.Name, "new_type", key)
	}

	return v
}

func (e *Evaluator) evalBasicLit(ctx context.Context, n *ast.BasicLit) object.Object {
	switch n.Kind {
	case token.INT:
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not parse %q as integer", n.Value)
		}
		return &object.Integer{Value: i}
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	case token.CHAR:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not unquote char %q", n.Value)
		}
		// A char literal unquotes to a string containing the single character.
		// We take the first (and only) rune from that string.
		if len(s) == 0 {
			return e.newError(ctx, n.Pos(), "invalid empty char literal %q", n.Value)
		}
		runes := []rune(s)
		return &object.Integer{Value: int64(runes[0])}
	case token.FLOAT:
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not parse %q as float", n.Value)
		}
		return &object.Float{Value: f}
	case token.IMAG:
		// The value is like "123i", "0.5i", etc.
		// We need to parse the numeric part.
		imagStr := strings.TrimSuffix(n.Value, "i")
		f, err := strconv.ParseFloat(imagStr, 64)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not parse %q as imaginary", n.Value)
		}
		return &object.Complex{Value: complex(0, f)}
	default:
		return e.newError(ctx, n.Pos(), "unsupported literal type: %s", n.Kind)
	}
}

// forceEval recursively evaluates an object until it is no longer a variable.
// This is crucial for handling variables whose initializers are other variables.
func (e *Evaluator) forceEval(ctx context.Context, obj object.Object, pkg *scan.PackageInfo) object.Object {
	for i := 0; i < 100; i++ { // Add a loop limit to prevent infinite loops in weird cases
		v, ok := obj.(*object.Variable)
		if !ok {
			return obj // Not a variable, return as is.
		}
		obj = e.evalVariable(ctx, v, pkg)
		if isError(obj) {
			return obj
		}
		// Loop again in case the result of evaluating a variable is another variable.
	}
	return e.newError(ctx, token.NoPos, "evaluation depth limit exceeded, possible variable evaluation loop")
}

// evalVariable evaluates a variable, triggering its initializer if it's lazy.
func (e *Evaluator) evalVariable(ctx context.Context, v *object.Variable, pkg *scan.PackageInfo) object.Object {
	e.logger.DebugContext(ctx, "evalVariable: start", "var", v.Name, "is_evaluated", v.IsEvaluated)
	if v.IsEvaluated {
		e.logger.DebugContext(ctx, "evalVariable: already evaluated, returning cached value", "var", v.Name, "value_type", v.Value.Type(), "value", inspectValuer{v.Value})
		return v.Value
	}

	// Prevent infinite recursion for variable initializers.
	if v.Initializer == nil {
		// This is a variable declared without a value, like `var x int`.
		// Its value is the zero value for its type. For symbolic execution,
		// we represent this with a specific placeholder that carries the type info.
		placeholder := &object.SymbolicPlaceholder{Reason: "zero value for uninitialized variable"}
		if ft := v.FieldType(); ft != nil {
			placeholder.SetFieldType(ft)
			placeholder.SetTypeInfo(e.resolver.ResolveType(ctx, ft))
		}
		v.Value = placeholder
		v.IsEvaluated = true
		return v.Value
	}

	if e.evaluationInProgress[v.Initializer] {
		e.logc(ctx, slog.LevelWarn, "cyclic dependency detected in variable initializer", "var", v.Name, "pos", v.Initializer.Pos())
		return e.newError(ctx, v.Initializer.Pos(), "cyclic dependency for variable %q", v.Name)
	}
	e.evaluationInProgress[v.Initializer] = true
	defer delete(e.evaluationInProgress, v.Initializer)

	e.logger.DebugContext(ctx, "evalVariable: evaluating initializer", "var", v.Name)
	// Evaluate the initializer in the environment where the variable was declared.
	val := e.Eval(ctx, v.Initializer, v.DeclEnv, v.DeclPkg)
	if isError(val) {
		return val
	}
	v.Value = val
	v.IsEvaluated = true
	e.logger.DebugContext(ctx, "evalVariable: finished evaluation", "var", v.Name, "value_type", val.Type(), "value", inspectValuer{val})
	return val
}

func (e *Evaluator) evalIdent(ctx context.Context, n *ast.Ident, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if pkg != nil {
		key := pkg.ImportPath + "." + n.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			e.logger.Debug("evalIdent: found intrinsic, overriding", "key", key)
			return &object.Intrinsic{Fn: intrinsicFn}
		}
	}

	if val, ok := env.Get(n.Name); ok {
		e.logger.Debug("evalIdent: found in env", "name", n.Name, "type", val.Type(), "val", inspectValuer{val})
		if _, ok := val.(*object.Variable); ok {
			e.logger.Debug("evalIdent: identifier is a variable, evaluating it", "name", n.Name)
			// When an identifier is accessed, we must force its full evaluation.
			evaluatedValue := e.forceEval(ctx, val, pkg)
			e.logger.Debug("evalIdent: evaluated variable", "name", n.Name, "type", evaluatedValue.Type(), "value", inspectValuer{evaluatedValue})
			return evaluatedValue
		}
		return val
	}

	// If the identifier is not in the environment, it might be a package name.
	if pkg != nil && pkg.Fset != nil {
		file := pkg.Fset.File(n.Pos())
		if file != nil {
			if astFile, ok := pkg.AstFiles[file.Name()]; ok {
				for _, imp := range astFile.Imports {
					importPath, _ := strconv.Unquote(imp.Path.Value)

					// Case 1: The import has an alias.
					if imp.Name != nil {
						if n.Name == imp.Name.Name {
							pkgObj, _ := e.getOrLoadPackage(ctx, importPath)
							return pkgObj
						}
						continue
					}

					// Case 2: No alias. The identifier might be the package's actual name.
					pkgObj, _ := e.getOrLoadPackage(ctx, importPath) // Error is not fatal here.
					if pkgObj == nil {
						e.logc(ctx, slog.LevelDebug, "could not get package for ident", "ident", n.Name, "path", importPath)
						continue
					}

					// If the package was scanned, we can definitively match its name.
					if pkgObj.ScannedInfo != nil {
						if n.Name == pkgObj.ScannedInfo.Name {
							return pkgObj
						}
					} else {
						// If the package is just a placeholder (not scanned due to policy),
						// we can't know its real name for sure. As a strong heuristic for
						// packages without an alias, we assume the identifier name matches
						// the base of the import path. This works for `fmt`, `os`, etc.
						parts := strings.Split(importPath, "/")
						assumedName := parts[len(parts)-1]
						if n.Name == assumedName {
							return pkgObj
						}
					}
				}
			}
		}
	}

	// Fallback to universe scope for built-in values, types, and functions.
	if obj, ok := universe.Get(n.Name); ok {
		return obj
	}
	if pkg != nil {
		for _, c := range pkg.Constants {
			if c.Name == n.Name {
				e.logger.Debug("evalIdent: found in package-level constants as fallback", "name", n.Name)
				return e.convertGoConstant(ctx, c.ConstVal, n.Pos())
			}
		}
	}

	e.logger.Debug("evalIdent: not found in env or intrinsics", "name", n.Name)

	if pkg != nil && !e.resolver.ScanPolicy(pkg.ImportPath) {
		e.logger.DebugContext(ctx, "treating undefined identifier as symbolic in out-of-policy package", "ident", n.Name, "package", pkg.ImportPath)
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("undefined identifier %s in out-of-policy package", n.Name)}
	}

	if val, ok := universe.Get(n.Name); ok {
		return val
	}
	return e.newError(ctx, n.Pos(), "identifier not found: %s", n.Name)
}

// logc logs a message with the current function context from the call stack.
func (e *Evaluator) logc(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !e.logger.Enabled(ctx, level) {
		return
	}

	// Get execution position (the caller of this function)
	_, file, line, ok := runtime.Caller(1)
	if ok {
		// Prepend exec_pos so it appears early in the log output.
		args = append([]any{slog.String("exec_pos", fmt.Sprintf("%s:%d", file, line))}, args...)
	}

	// Add context from the current call stack frame.
	if len(e.callStack) > 0 {
		frame := e.callStack[len(e.callStack)-1]
		posStr := ""
		if e.scanner != nil && e.scanner.Fset() != nil && frame.Pos.IsValid() {
			posStr = e.scanner.Fset().Position(frame.Pos).String()
		}
		contextArgs := []any{
			slog.String("in_func", frame.Function),
			slog.String("in_func_pos", posStr),
		}
		// Prepend context args so they appear first in the log.
		args = append(contextArgs, args...)
	}

	// Prevent recursion: if an argument is an *object.Error, don't inspect it deeply.
	for i, arg := range args {
		if err, ok := arg.(*object.Error); ok {
			args[i] = slog.String("error", err.Message)
		}
	}

	e.logger.Log(ctx, level, msg, args...)
}

func (e *Evaluator) newError(ctx context.Context, pos token.Pos, format string, args ...interface{}) *object.Error {
	msg := fmt.Sprintf(format, args...)
	e.logc(ctx, slog.LevelError, msg, "pos", pos)

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

// areArgsEqual is a helper to compare function arguments for recursion detection.
// It performs a direct object comparison.
func areArgsEqual(a, b []object.Object) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isInfiniteRecursionError(obj object.Object) bool {
	if err, ok := obj.(*object.Error); ok {
		return strings.Contains(err.Message, "infinite recursion detected")
	}
	return false
}

// isCallable checks if an object is of a type that can be invoked.
func isCallable(obj object.Object) bool {
	if obj == nil {
		return false
	}
	switch obj.(type) {
	case *object.Function, *object.InstantiatedFunction, *object.Intrinsic, *object.UnresolvedFunction, *object.SymbolicPlaceholder:
		return true
	case *object.Variable:
		// A variable can be callable if it holds a function object.
		return true
	default:
		return false
	}
}

func (e *Evaluator) evalCallExpr(ctx context.Context, n *ast.CallExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if e.logger.Enabled(ctx, slog.LevelDebug) {
		stackAttrs := make([]any, 0, len(e.callStack))
		for i, frame := range e.callStack {
			posStr := ""
			if e.scanner != nil && e.scanner.Fset() != nil && frame.Pos.IsValid() {
				posStr = e.scanner.Fset().Position(frame.Pos).String()
			}
			stackAttrs = append(stackAttrs, slog.Group(fmt.Sprintf("%d", i),
				slog.String("func", frame.Function),
				slog.String("pos", posStr),
			))
		}
		e.logger.Log(ctx, slog.LevelDebug, "call", slog.Group("stack", stackAttrs...))
	}

	function := e.Eval(ctx, n.Fun, env, pkg)
	if isError(function) {
		return function
	}

	// If the function expression itself resolves to a return value (e.g., from an interface method call
	// that we intercept), we need to unwrap it before applying it.
	if ret, ok := function.(*object.ReturnValue); ok {
		function = ret.Value
	}

	args := e.evalExpressions(ctx, n.Args, env, pkg)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	// If the call includes `...`, the last argument is a slice to be expanded.
	// We wrap it in a special Variadic object to signal this to `applyFunction`.
	if n.Ellipsis.IsValid() {
		if len(args) == 0 {
			return e.newError(ctx, n.Ellipsis, "invalid use of ... with no arguments")
		}
		lastArg := args[len(args)-1]
		// The argument should be a slice, but we don't check it here.
		// `extendFunctionEnv` will handle the type logic.
		args[len(args)-1] = &object.Variadic{Value: lastArg}
	}

	// After evaluating arguments, check if any of them are function literals.
	// If so, we need to "scan" inside them to find usages. This must be done
	// before the default intrinsic is called, so the usage map is populated
	// before the parent function call is even registered.
	for _, arg := range args {
		if fn, ok := arg.(*object.Function); ok {
			e.scanFunctionLiteral(ctx, fn)
		}
	}

	if e.defaultIntrinsic != nil {
		// The default intrinsic is a "catch-all" handler that can be used for logging,
		// dependency tracking, etc. It receives the function object itself as the first
		// argument, followed by the regular arguments.
		e.defaultIntrinsic(ctx, append([]object.Object{function}, args...)...)
	}

	result := e.applyFunction(ctx, function, args, pkg, n.Pos())
	if isError(result) {
		return result
	}
	return result
}

// scanFunctionLiteral evaluates the body of a function literal in a new, symbolic
// environment. This is used to find function calls inside anonymous functions that are
// passed as arguments, without needing to fully execute the function they are passed to.
func (e *Evaluator) scanFunctionLiteral(ctx context.Context, fn *object.Function) {
	if fn.Body == nil || fn.Package == nil {
		return // Nothing to scan.
	}

	e.logger.DebugContext(ctx, "scanning function literal to find usages", "pos", fn.Package.Fset.Position(fn.Body.Pos()))

	// Create a new environment for the function literal's execution.
	// It's enclosed by the environment where the literal was defined.
	fnEnv := object.NewEnclosedEnvironment(fn.Env)

	// Populate the environment with symbolic placeholders for the function's parameters.
	if fn.Parameters != nil {
		var importLookup map[string]string
		// A FuncLit doesn't have a specific *ast.File, but its body has a position.
		// We can use this position to find the containing file and its imports.
		file := fn.Package.Fset.File(fn.Body.Pos())
		if file != nil {
			if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
				importLookup = e.scanner.BuildImportLookup(astFile)
			}
		}
		// Fallback if we couldn't get a specific import lookup.
		if importLookup == nil && len(fn.Package.AstFiles) > 0 {
			for _, astFile := range fn.Package.AstFiles {
				importLookup = e.scanner.BuildImportLookup(astFile)
				break
			}
		}

		for _, field := range fn.Parameters.List {
			fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, fn.Package, importLookup)
			var resolvedType *scan.TypeInfo
			if fieldType != nil {
				resolvedType = e.resolver.ResolveType(ctx, fieldType)
			}

			placeholder := &object.SymbolicPlaceholder{
				Reason: "symbolic parameter for function literal scan",
				BaseObject: object.BaseObject{
					ResolvedTypeInfo:  resolvedType,
					ResolvedFieldType: fieldType,
				},
			}

			for _, name := range field.Names {
				if name.Name != "_" {
					// Bind the placeholder to a new variable in the function's environment.
					v := &object.Variable{Name: name.Name, Value: placeholder}
					v.SetFieldType(fieldType)
					v.SetTypeInfo(resolvedType)
					fnEnv.Set(name.Name, v)
				}
			}
		}
	}

	// Now evaluate the body. The result is ignored; we only care about the side effects
	// (i.e., triggering the defaultIntrinsic on calls within the body).
	e.Eval(ctx, fn.Body, fnEnv, fn.Package)
}

func (e *Evaluator) evalExpressions(ctx context.Context, exps []ast.Expr, env *object.Environment, pkg *scan.PackageInfo) []object.Object {
	var result []object.Object

	for _, exp := range exps {
		evaluated := e.Eval(ctx, exp, env, pkg)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		// Force full evaluation of the argument.
		evaluated = e.forceEval(ctx, evaluated, pkg)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

func (e *Evaluator) Apply(ctx context.Context, fn object.Object, args []object.Object, pkg *scan.PackageInfo) object.Object {
	return e.applyFunction(ctx, fn, args, pkg, token.NoPos)
}

type inspectValuer struct {
	obj object.Object
}

func (v inspectValuer) LogValue() slog.Value {
	if v.obj == nil {
		return slog.StringValue("<nil>")
	}
	return slog.StringValue(v.obj.Inspect())
}

func (e *Evaluator) applyFunction(ctx context.Context, fn object.Object, args []object.Object, pkg *scan.PackageInfo, callPos token.Pos) object.Object {
	if f, ok := fn.(*object.Function); ok {
		if e.memoize {
			if cachedResult, found := e.memoizationCache[f]; found {
				e.logc(ctx, slog.LevelDebug, "returning memoized result for function", "function", f.Name)
				return cachedResult
			}
		}
	}

	result := e.applyFunctionImpl(ctx, fn, args, pkg, callPos)

	if f, ok := fn.(*object.Function); ok {
		if e.memoize && !isError(result) {
			e.logc(ctx, slog.LevelDebug, "caching result for function", "function", f.Name)
			e.memoizationCache[f] = result
		}
	}

	return result
}

func (e *Evaluator) applyFunctionImpl(ctx context.Context, fn object.Object, args []object.Object, pkg *scan.PackageInfo, callPos token.Pos) object.Object {
	var name string

	if f, ok := fn.(*object.Function); ok {
		if f.Name != nil {
			name = f.Name.Name
		} else {
			name = "<closure>"
		}
	} else if v, ok := fn.(*object.Variable); ok {
		name = v.Name
	} else {
		name = fn.Inspect()
	}

	if len(e.callStack) >= MaxCallStackDepth {
		e.logc(ctx, slog.LevelWarn, "call stack depth exceeded, aborting recursion", "function", name)
		return &object.SymbolicPlaceholder{Reason: "max call stack depth exceeded"}
	}

	// New recursion check based on function definition and receiver position.
	if f, ok := fn.(*object.Function); ok && f.Def != nil {
		recursionCount := 0
		for _, frame := range e.callStack {
			// The most robust way to detect recursion on a definition is to compare the
			// source position of the function's declaration AST node. This correctly
			// identifies recursion on a specific function/method definition, which is
			// the goal of symgo's analysis, rather than tracking object instances.
			if frame.Fn != nil && frame.Fn.Def != nil && frame.Fn.Def.AstDecl != nil &&
				f.Def.AstDecl != nil && frame.Fn.Def.AstDecl.Pos() == f.Def.AstDecl.Pos() {
				recursionCount++
			}
		}

		// Allow one level of recursion, but stop at the second call.
		if recursionCount > 0 { // Changed from > 1 to > 0 to be more strict.
			e.logc(ctx, slog.LevelDebug, "bounded recursion depth exceeded, halting analysis for this path", "function", name)
			// Return a symbolic placeholder that matches the function's return signature.
			if f.Def != nil && f.Def.AstDecl.Type.Results != nil {
				numResults := len(f.Def.AstDecl.Type.Results.List)
				if numResults > 1 {
					results := make([]object.Object, numResults)
					for i := 0; i < numResults; i++ {
						results[i] = &object.SymbolicPlaceholder{Reason: "bounded recursion halt"}
					}
					return &object.MultiReturn{Values: results}
				}
			}
			// Default to a single placeholder if signature is not available or has <= 1 return values.
			return &object.SymbolicPlaceholder{Reason: "bounded recursion halt"}
		}
	}

	frame := &callFrame{Function: name, Pos: callPos, Args: args}
	if f, ok := fn.(*object.Function); ok {
		frame.Fn = f
		if f.Receiver != nil {
			frame.ReceiverPos = f.ReceiverPos
		}
	}
	e.callStack = append(e.callStack, frame)
	defer func() {
		e.callStack = e.callStack[:len(e.callStack)-1]
	}()

	if e.logger.Enabled(ctx, slog.LevelDebug) {
		argStrs := make([]string, len(args))
		for i, arg := range args {
			argStrs[i] = arg.Inspect()
		}
		e.logc(ctx, slog.LevelDebug, "applyFunction", "in_func", name, "in_func_pos", e.scanner.Fset().Position(callPos), "exec_pos", callPos, "type", fn.Type(), "value", inspectValuer{fn}, "args", strings.Join(argStrs, ", "))
	}

	// If `fn` is a variable, we need to evaluate it to get the underlying function.
	if v, ok := fn.(*object.Variable); ok {
		underlyingFn := e.forceEval(ctx, v, pkg) // Use forceEval to handle chained variables
		if isError(underlyingFn) {
			return underlyingFn
		}
		// Recursively call applyFunction with the resolved function object.
		return e.applyFunction(ctx, underlyingFn, args, pkg, callPos)
	}

	switch fn := fn.(type) {
	case *object.InstantiatedFunction:
		// This is the new logic for handling calls to generic functions.
		extendedEnv := object.NewEnclosedEnvironment(fn.Function.Env)

		// Bind type parameters to their concrete types in the new environment.
		if fn.Function.Def != nil && len(fn.Function.Def.TypeParams) == len(fn.TypeArgs) {
			for i, typeParam := range fn.Function.Def.TypeParams {
				typeArgInfo := fn.TypeArgs[i]
				if typeArgInfo != nil {
					typeObj := &object.Type{
						TypeName:     typeArgInfo.Name,
						ResolvedType: typeArgInfo,
					}
					typeObj.SetTypeInfo(typeArgInfo)
					extendedEnv.SetLocal(typeParam.Name, typeObj)
				}
			}
		}

		// Now, extend the environment with the regular function arguments, using the
		// new environment that contains the type parameter bindings.
		finalEnv, err := e.extendFunctionEnv(ctx, fn.Function, args, extendedEnv)
		if err != nil {
			return e.newError(ctx, fn.Decl.Pos(), "failed to extend generic function env: %v", err)
		}

		evaluated := e.Eval(ctx, fn.Function.Body, finalEnv, fn.Function.Package)
		if ret, ok := evaluated.(*object.ReturnValue); ok {
			return ret
		}
		// if the function has a naked return, evaluated will be nil.
		if evaluated == nil {
			evaluated = object.NIL
		}
		return &object.ReturnValue{Value: evaluated}

	case *object.Function:
		// If the function has no body, it's a declaration (e.g., in an interface, or an external function).
		// Treat it as an external call and create a symbolic result based on its signature.
		if fn.Body == nil {
			return e.createSymbolicResultForFunc(ctx, fn)
		}

		// When calling a function, ensure its defining package's environment is fully populated.
		if fn.Package != nil {
			pkgObj, err := e.getOrLoadPackage(ctx, fn.Package.ImportPath)
			if err == nil {
				e.ensurePackageEnvPopulated(ctx, pkgObj)
			}
		}

		// Check the scan policy before executing the body.
		if fn.Package != nil && !e.resolver.ScanPolicy(fn.Package.ImportPath) {
			// If the package is not in the primary analysis scope, treat the call
			// as symbolic, just like an external function call.
			return e.createSymbolicResultForFunc(ctx, fn)
		}

		// When applying a function, the evaluation context switches to that function's
		// package. We must pass fn.Package to both extendFunctionEnv and Eval.
		extendedEnv, err := e.extendFunctionEnv(ctx, fn, args, nil) // Pass nil for non-generic calls
		if err != nil {
			return e.newError(ctx, fn.Decl.Pos(), "failed to extend function env: %v", err)
		}

		// Populate the new environment with the imports from the function's source file.
		if fn.Package != nil && fn.Package.Fset != nil && fn.Decl != nil {
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
						extendedEnv.Set(name, &object.Package{Path: path, ScannedInfo: nil, Env: object.NewEnclosedEnvironment(e.UniverseEnv)})
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
		return fn.Fn(ctx, args...)

	case *object.SymbolicPlaceholder:
		// This now handles both external function calls and interface method calls.
		if fn.UnderlyingFunc != nil {
			// If it has an AST declaration, it's a real function from source.
			if fn.UnderlyingFunc.AstDecl != nil {
				return e.createSymbolicResultForFuncInfo(ctx, fn.UnderlyingFunc, fn.Package, "result of external call to %s", fn.UnderlyingFunc.Name)
			}

			// Otherwise, it's a constructed FunctionInfo for an interface method.
			// We create the result based on the Parameters/Results fields directly.
			method := fn.UnderlyingFunc
			var result object.Object
			if len(method.Results) <= 1 {
				var resultTypeInfo *scan.TypeInfo
				var resultFieldType *scan.FieldType
				if len(method.Results) == 1 {
					resultFieldType = method.Results[0].Type
					if resultFieldType != nil {
						resultType := e.resolver.ResolveType(ctx, resultFieldType)
						if resultType == nil && resultFieldType.IsBuiltin {
							resultType = &scan.TypeInfo{Name: resultFieldType.Name}
						}
						resultTypeInfo = resultType
					}
				}
				result = &object.SymbolicPlaceholder{
					Reason:     fmt.Sprintf("result of interface method call %s", method.Name),
					BaseObject: object.BaseObject{ResolvedTypeInfo: resultTypeInfo, ResolvedFieldType: resultFieldType},
				}
			} else {
				// Multiple return values from interface method
				results := make([]object.Object, len(method.Results))
				for i, resFieldInfo := range method.Results {
					resultFieldType := resFieldInfo.Type
					var resultType *scan.TypeInfo
					if resultFieldType != nil {
						resultType = e.resolver.ResolveType(ctx, resultFieldType)
						if resultType == nil && resultFieldType.IsBuiltin && resultFieldType.Name == "error" {
							resultType = ErrorInterfaceTypeInfo
						}
					}
					results[i] = &object.SymbolicPlaceholder{
						Reason:     fmt.Sprintf("result %d of interface method call %s", i, method.Name),
						BaseObject: object.BaseObject{ResolvedTypeInfo: resultType, ResolvedFieldType: resultFieldType},
					}
				}
				result = &object.MultiReturn{Values: results}
			}
			return &object.ReturnValue{Value: result}
		}

		// Case 3: A placeholder representing a callable variable (like flag.Usage)
		if typeInfo := fn.TypeInfo(); typeInfo != nil && typeInfo.Kind == scan.FuncKind && typeInfo.Func != nil {
			funcInfo := typeInfo.Func
			var pkgInfo *scan.PackageInfo
			var err error
			if fn.FieldType() != nil && fn.FieldType().FullImportPath != "" {
				pkg, loadErr := e.getOrLoadPackage(ctx, fn.FieldType().FullImportPath)
				if loadErr == nil && pkg != nil {
					pkgInfo = pkg.ScannedInfo
				}
				err = loadErr
			}
			if pkgInfo == nil {
				e.logc(ctx, slog.LevelWarn, "could not load package for function variable type", "path", typeInfo.PkgPath, "error", err)
				return &object.ReturnValue{Value: &object.SymbolicPlaceholder{Reason: "result of calling function variable with unloadable type"}}
			}
			return e.createSymbolicResultForFuncInfo(ctx, funcInfo, pkgInfo, "result of call to var %s", fn.Reason)
		}

		// Case 4: A placeholder representing a built-in type, used in a conversion.
		if strings.HasPrefix(fn.Reason, "built-in type") {
			result := &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of conversion to %s", fn.Reason)}
			return &object.ReturnValue{Value: result}
		}
		// Fallback for any other kind of placeholder is to treat it as a symbolic call.
		result := &object.SymbolicPlaceholder{Reason: "result of calling " + fn.Inspect()}
		return &object.ReturnValue{Value: result}

	case *object.UnresolvedType:
		// This is a symbol from an out-of-policy package, which we are now attempting to call.
		// We treat it as a function call, mirroring the logic for UnresolvedFunction.
		e.logc(ctx, slog.LevelDebug, "applying unresolved type as function", "package", fn.PkgPath, "function", fn.TypeName)

		key := fn.PkgPath + "." + fn.TypeName
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return intrinsicFn(ctx, args...)
		}

		scannedPkg, err := e.resolver.ResolvePackage(ctx, fn.PkgPath)
		if err != nil {
			e.logc(ctx, slog.LevelDebug, "could not scan package for unresolved symbol (or denied by policy)", "package", fn.PkgPath, "symbol", fn.TypeName, "error", err)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved symbol %s.%s", fn.PkgPath, fn.TypeName)}
		}

		var funcInfo *scan.FunctionInfo
		for _, f := range scannedPkg.Functions {
			if f.Name == fn.TypeName {
				funcInfo = f
				break
			}
		}

		if funcInfo == nil {
			e.logc(ctx, slog.LevelWarn, "could not find function signature in package for symbol", "package", fn.PkgPath, "symbol", fn.TypeName)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved symbol %s.%s", fn.PkgPath, fn.TypeName)}
		}
		return e.createSymbolicResultForFuncInfo(ctx, funcInfo, scannedPkg, "result of call to %s.%s", fn.PkgPath, fn.TypeName)

	case *object.UnresolvedFunction:
		// This is a function that could not be resolved during symbol lookup.
		// We make a best effort to find its signature now.
		e.logc(ctx, slog.LevelDebug, "attempting to resolve and apply unresolved function", "package", fn.PkgPath, "function", fn.FuncName)

		// Before trying to scan the package, check if there's a registered intrinsic for it.
		key := fn.PkgPath + "." + fn.FuncName
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return intrinsicFn(ctx, args...)
		}

		// Use the policy-enforcing method to resolve the package.
		scannedPkg, err := e.resolver.ResolvePackage(ctx, fn.PkgPath)
		if err != nil {
			e.logc(ctx, slog.LevelWarn, "could not scan package for unresolved function (or denied by policy)", "package", fn.PkgPath, "function", fn.FuncName, "error", err)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved function %s.%s", fn.PkgPath, fn.FuncName)}
		}

		var funcInfo *scan.FunctionInfo
		for _, f := range scannedPkg.Functions {
			if f.Name == fn.FuncName {
				funcInfo = f
				break
			}
		}

		if funcInfo == nil {
			e.logc(ctx, slog.LevelWarn, "could not find function signature in package", "package", fn.PkgPath, "function", fn.FuncName)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved function %s.%s", fn.PkgPath, fn.FuncName)}
		}

		// We found the function info. Now create a symbolic result based on its signature.
		return e.createSymbolicResultForFuncInfo(ctx, funcInfo, scannedPkg, "result of call to %s.%s", fn.PkgPath, fn.FuncName)

	case *object.Type:
		// This handles type conversions like string(b) or int(x).
		if len(args) != 1 {
			return e.newError(ctx, callPos, "wrong number of arguments for type conversion: got=%d, want=1", len(args))
		}
		// The result is a symbolic value of the target type.
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("result of conversion to %s", fn.TypeName),
		}
		placeholder.SetTypeInfo(fn.ResolvedType)
		return &object.ReturnValue{Value: placeholder}

	default:
		return e.newError(ctx, callPos, "not a function: %s", fn.Type())
	}
}

func (e *Evaluator) extendFunctionEnv(ctx context.Context, fn *object.Function, args []object.Object, baseEnv *object.Environment) (*object.Environment, error) {
	var env *object.Environment
	if baseEnv != nil {
		env = baseEnv
	} else {
		env = object.NewEnclosedEnvironment(fn.Env)
	}

	// 1. Bind receiver
	if fn.Decl != nil && fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
		recvField := fn.Decl.Recv.List[0]
		if len(recvField.Names) > 0 && recvField.Names[0].Name != "" && recvField.Names[0].Name != "_" {
			receiverName := recvField.Names[0].Name
			receiverToBind := fn.Receiver
			if receiverToBind == nil {
				var importLookup map[string]string
				if file := fn.Package.Fset.File(fn.Decl.Pos()); file != nil {
					if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
						importLookup = e.scanner.BuildImportLookup(astFile)
					}
				}
				fieldType := e.scanner.TypeInfoFromExpr(ctx, recvField.Type, nil, fn.Package, importLookup)
				resolvedType := e.resolver.ResolveType(ctx, fieldType)
				receiverToBind = &object.SymbolicPlaceholder{
					Reason:     "symbolic receiver for entry point method",
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
				}
			}
			env.SetLocal(receiverName, receiverToBind)
		}
	}

	// 2. Bind named return values (if any)
	// This must be done before binding parameters, in case a parameter has the same name.
	if fn.Decl != nil && fn.Decl.Type.Results != nil {
		for _, field := range fn.Decl.Type.Results.List {
			if len(field.Names) == 0 {
				continue // Unnamed return value
			}
			var importLookup map[string]string
			if file := fn.Package.Fset.File(field.Pos()); file != nil {
				if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
					importLookup = e.scanner.BuildImportLookup(astFile)
				}
			}

			fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, fn.Package, importLookup)
			resolvedType := e.resolver.ResolveType(ctx, fieldType)

			for _, name := range field.Names {
				if name.Name == "_" {
					continue
				}
				// The zero value for any type in symbolic execution is a placeholder.
				// This placeholder carries the type information of the variable.
				zeroValue := &object.SymbolicPlaceholder{
					Reason:     "zero value for named return",
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
				}
				v := &object.Variable{
					Name:        name.Name,
					Value:       zeroValue,
					IsEvaluated: true, // It has its zero value.
				}
				v.SetFieldType(fieldType)
				v.SetTypeInfo(resolvedType)
				env.SetLocal(name.Name, v)
			}
		}
	}

	// 3. Bind parameters
	if fn.Def != nil {
		// Bind parameters using the reliable FunctionInfo definition
		argIndex := 0
		for i, paramDef := range fn.Def.Parameters {
			var arg object.Object
			if argIndex < len(args) {
				arg = args[argIndex]
				argIndex++
			} else {
				arg = &object.SymbolicPlaceholder{Reason: "symbolic parameter for entry point"}
			}

			if paramDef.Name != "" && paramDef.Name != "_" {
				v := &object.Variable{
					Name:        paramDef.Name,
					Value:       arg,
					IsEvaluated: true,
				}

				// The static type from the function signature is the most reliable source.
				staticFieldType := paramDef.Type
				if staticFieldType != nil {
					staticTypeInfo := e.resolver.ResolveType(ctx, staticFieldType)
					v.SetFieldType(staticFieldType)
					v.SetTypeInfo(staticTypeInfo)
				} else {
					// Fallback to the dynamic type from the argument.
					v.SetFieldType(arg.FieldType())
					v.SetTypeInfo(arg.TypeInfo())
				}

				// If the argument is NIL and we have static type info, preserve it on the object.
				if nilObj, ok := arg.(*object.Nil); ok && staticFieldType != nil {
					nilObj.SetFieldType(v.FieldType())
					nilObj.SetTypeInfo(v.TypeInfo())
				}
				env.SetLocal(paramDef.Name, v)
			}

			// Handle variadic parameters using the flag on the FunctionInfo.
			if fn.Def.IsVariadic && i == len(fn.Def.Parameters)-1 {
				// This is the variadic parameter. The logic here would need to collect remaining args into a slice.
				// For now, we assume the single variadic argument is handled correctly by the caller providing a slice.
				// This part of the refactoring is left as a TODO if complex variadic cases fail.
				break
			}
		}
	} else if fn.Parameters != nil {
		// Fallback for function literals which don't have a FunctionInfo
		e.logc(ctx, slog.LevelDebug, "function definition not available in extendFunctionEnv, falling back to AST", "function", fn.Name)
		argIndex := 0
		for _, field := range fn.Parameters.List {
			// Handle variadic parameters indicated by Ellipsis in the AST
			isVariadic := false
			if _, ok := field.Type.(*ast.Ellipsis); ok {
				isVariadic = true
			}

			for _, name := range field.Names {
				if argIndex >= len(args) {
					break
				}
				if name.Name != "_" {
					var valToBind object.Object
					if isVariadic {
						// Collect remaining args into a slice for the variadic parameter
						sliceElements := args[argIndex:]
						valToBind = &object.Slice{Elements: sliceElements}
						// We could try to infer a field type here if needed
					} else {
						valToBind = args[argIndex]
					}

					v := &object.Variable{
						Name:        name.Name,
						Value:       valToBind,
						IsEvaluated: true,
					}
					v.SetTypeInfo(valToBind.TypeInfo())
					v.SetFieldType(valToBind.FieldType())
					env.SetLocal(name.Name, v)
				}
				if !isVariadic {
					argIndex++
				}
			}
			if isVariadic {
				break // Variadic parameter is always the last one
			}
		}
	}

	return env, nil
}

// evalGenericInstantiation handles the creation of an InstantiatedFunction object
// from a generic function and its type arguments.
func (e *Evaluator) evalGenericInstantiation(ctx context.Context, fn *object.Function, typeArgs []ast.Expr, pos token.Pos, pkg *scan.PackageInfo) object.Object {
	// Resolve type arguments into TypeInfo objects
	var resolvedArgs []*scan.TypeInfo
	if pkg != nil && pkg.Fset != nil {
		file := pkg.Fset.File(pos)
		if file != nil {
			if astFile, ok := pkg.AstFiles[file.Name()]; ok {
				importLookup := e.scanner.BuildImportLookup(astFile)
				for _, argExpr := range typeArgs {
					fieldType := e.scanner.TypeInfoFromExpr(ctx, argExpr, nil, pkg, importLookup)
					resolvedType := e.resolver.ResolveType(ctx, fieldType)
					resolvedArgs = append(resolvedArgs, resolvedType)
				}
			}
		}
	}

	return &object.InstantiatedFunction{
		Function:      fn,
		TypeArguments: typeArgs,
		TypeArgs:      resolvedArgs,
	}
}

// createSymbolicResultForFuncInfo creates a symbolic result for a function call based on its FunctionInfo.
// This is used for functions that are not deeply executed (e.g., due to scan policy or being unresolved).
func (e *Evaluator) createSymbolicResultForFuncInfo(ctx context.Context, funcInfo *scan.FunctionInfo, pkgInfo *scan.PackageInfo, reasonFormat string, reasonArgs ...any) object.Object {
	if funcInfo.AstDecl == nil || funcInfo.AstDecl.Type == nil || pkgInfo == nil {
		return &object.SymbolicPlaceholder{Reason: "result of call with incomplete info"}
	}
	reason := fmt.Sprintf(reasonFormat, reasonArgs...)

	results := funcInfo.AstDecl.Type.Results
	if results == nil || len(results.List) == 0 {
		return &object.SymbolicPlaceholder{Reason: reason + " (no return value)"}
	}

	var importLookup map[string]string
	if file := pkgInfo.Fset.File(funcInfo.AstDecl.Pos()); file != nil {
		if astFile, ok := pkgInfo.AstFiles[file.Name()]; ok {
			importLookup = e.scanner.BuildImportLookup(astFile)
		}
	}

	if len(results.List) == 1 {
		resultASTExpr := results.List[0].Type
		fieldType := e.scanner.TypeInfoFromExpr(ctx, resultASTExpr, nil, pkgInfo, importLookup)
		resolvedType := e.resolver.ResolveType(ctx, fieldType)

		// Special handling for the built-in error interface.
		if resolvedType == nil && fieldType.IsBuiltin && fieldType.Name == "error" {
			resolvedType = ErrorInterfaceTypeInfo
		}

		return &object.ReturnValue{
			Value: &object.SymbolicPlaceholder{
				Reason:     reason,
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
			},
		}
	}

	// Multiple return values
	returnValues := make([]object.Object, 0, len(results.List))
	for i, field := range results.List {
		fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, pkgInfo, importLookup)
		resolvedType := e.resolver.ResolveType(ctx, fieldType)

		// Special handling for the built-in error interface.
		if resolvedType == nil && fieldType.IsBuiltin && fieldType.Name == "error" {
			resolvedType = ErrorInterfaceTypeInfo
		}

		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("%s (result %d)", reason, i),
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  resolvedType,
				ResolvedFieldType: fieldType,
			},
		}
		returnValues = append(returnValues, placeholder)
	}
	return &object.ReturnValue{Value: &object.MultiReturn{Values: returnValues}}
}

// CalledInterfaceMethodsForTest returns the map of called interface methods for testing.
func (e *Evaluator) CalledInterfaceMethodsForTest() map[string][]object.Object {
	return e.calledInterfaceMethods
}

// SeenPackagesForTest returns the map of seen packages for testing.
func (e *Evaluator) SeenPackagesForTest() map[string]*goscan.Package {
	return e.seenPackages
}

// GetOrLoadPackageForTest is a test helper to expose the internal getOrLoadPackage method.
func (e *Evaluator) GetOrLoadPackageForTest(ctx context.Context, path string) (*object.Package, error) {
	return e.getOrLoadPackage(ctx, path)
}

// PackageEnvForTest is a test helper to get a package's environment.
func (e *Evaluator) PackageEnvForTest(pkgPath string) (*object.Environment, bool) {
	if pkg, ok := e.pkgCache[pkgPath]; ok {
		return pkg.Env, true
	}
	return nil, false
}

// GetOrResolveFunctionForTest is a test helper to expose the internal getOrResolveFunction method.
func (e *Evaluator) GetOrResolveFunctionForTest(ctx context.Context, pkg *object.Package, funcInfo *scan.FunctionInfo) object.Object {
	return e.getOrResolveFunction(ctx, pkg, funcInfo)
}

// createSymbolicResultForFunc creates a symbolic result for a function call
// that is not being deeply executed due to scan policy.
func (e *Evaluator) createSymbolicResultForFunc(ctx context.Context, fn *object.Function) object.Object {
	if fn.Def == nil {
		return &object.SymbolicPlaceholder{Reason: "result of external call with incomplete info"}
	}
	return e.createSymbolicResultForFuncInfo(ctx, fn.Def, fn.Package, "result of out-of-policy call to %s", fn.Name.Name)
}

// Files returns the file scopes that have been loaded into the evaluator.
func (e *Evaluator) Files() []*FileScope {
	return e.files
}

// ApplyFunction is a public wrapper for the internal applyFunction, allowing it to be called from other packages.
func (e *Evaluator) ApplyFunction(ctx context.Context, call *ast.CallExpr, fn object.Object, args []object.Object, fscope *FileScope) object.Object {
	// This is a simplification. A real implementation would need to determine the correct environment.
	// For now, we'll use a new top-level environment, which will work for pure functions
	// but not for closures that capture variables.
	// The pkg argument is nil here, which might limit some functionality.
	return e.applyFunction(ctx, fn, args, nil, call.Pos())
}

func (e *Evaluator) getOrResolveFunction(ctx context.Context, pkg *object.Package, funcInfo *scan.FunctionInfo) object.Object {
	// Generate a unique key for the function. For methods, the receiver type is crucial.
	key := ""
	if funcInfo.Receiver != nil && funcInfo.Receiver.Type != nil {
		// e.g., "example.com/me/impl.(*Dog).Speak"
		key = fmt.Sprintf("%s.(%s).%s", pkg.Path, funcInfo.Receiver.Type.String(), funcInfo.Name)
	} else {
		// e.g., "example.com/me.MyFunction"
		key = fmt.Sprintf("%s.%s", pkg.Path, funcInfo.Name)
	}

	// Check cache first.
	if fn, ok := e.funcCache[key]; ok {
		return fn
	}

	// Not in cache, resolve it.
	fn := e.resolver.ResolveFunction(ctx, pkg, funcInfo)

	// Store in cache for next time.
	e.funcCache[key] = fn
	return fn
}

// Finalize performs the final analysis step, connecting interface method calls
// to their concrete implementations. This should be called after all initial
// symbolic execution is complete.
func (e *Evaluator) Finalize(ctx context.Context) {
	if e.defaultIntrinsic == nil {
		e.logger.DebugContext(ctx, "skipping finalize: no default intrinsic registered")
		return // Nothing to do if no intrinsic is registered to receive the results.
	}

	allStructs := make(map[string]*scan.TypeInfo)
	allInterfaces := make(map[string]*scan.TypeInfo)

	// 1. Collect all packages from the scanner's cache, respecting the scan policy.
	// This replaces the old `e.seenPackages` mechanism.
	allPackagesFromScanner := e.scanner.AllSeenPackages()
	e.seenPackages = make(map[string]*goscan.Package) // Reset seenPackages
	for importPath, pkg := range allPackagesFromScanner {
		if e.resolver.ScanPolicy(importPath) {
			e.seenPackages[importPath] = pkg
		}
	}

	if len(e.seenPackages) == 0 {
		e.logger.DebugContext(ctx, "finalize: no packages seen after applying policy, skipping")
		return
	}

	// 2. Collect all struct and interface types from the policy-filtered packages.
	e.logger.DebugContext(ctx, "finalize: starting type collection", "package_count", len(e.seenPackages))
	for _, pkg := range e.seenPackages {
		if pkg == nil {
			continue
		}
		for _, t := range pkg.Types {
			fullName := fmt.Sprintf("%s.%s", t.PkgPath, t.Name)
			if t.Struct != nil {
				allStructs[fullName] = t
			} else if t.Interface != nil {
				allInterfaces[fullName] = t
			}
		}
	}
	e.logger.DebugContext(ctx, "finalize: finished type collection", "struct_count", len(allStructs), "interface_count", len(allInterfaces))

	// 3. Build the implementation map.
	interfaceImplementers := make(map[string]map[string]struct{}) // key: interface name, value: set of implementer names
	for ifaceName, ifaceType := range allInterfaces {
		for structName, structType := range allStructs {
			if e.scanner.Implements(ctx, structType, ifaceType) {
				if _, ok := interfaceImplementers[ifaceName]; !ok {
					interfaceImplementers[ifaceName] = make(map[string]struct{})
				}
				interfaceImplementers[ifaceName][structName] = struct{}{}
			}
		}
	}
	e.logger.DebugContext(ctx, "finalize: implementation map", "map", fmt.Sprintf("%#v", interfaceImplementers))

	// 4. Process called interface methods.
	e.logger.DebugContext(ctx, "finalize: processing called interface methods", "count", len(e.calledInterfaceMethods))
	for calledMethodKey := range e.calledInterfaceMethods {
		parts := strings.Split(calledMethodKey, ".")
		if len(parts) < 2 {
			continue
		}
		methodName := parts[len(parts)-1]
		ifaceName := strings.Join(parts[:len(parts)-1], ".")
		e.logger.DebugContext(ctx, "finalize: processing key", "key", calledMethodKey, "iface", ifaceName, "method", methodName)

		implementers := interfaceImplementers[ifaceName]
		if len(implementers) == 0 {
			e.logger.DebugContext(ctx, "finalize: no implementers found for interface", "interface", ifaceName)
			continue
		}

		for structName := range implementers {
			structType := allStructs[structName]
			if structType == nil {
				continue
			}

			// Find the concrete method on the struct.
			concreteMethodInfo := e.accessor.findMethodInfoOnType(ctx, structType, methodName)
			if concreteMethodInfo == nil {
				e.logger.DebugContext(ctx, "finalize: concrete method not found on struct", "struct", structName, "method", methodName)
				continue
			}

			// Get the package object for the method's definition.
			pkg, err := e.getOrLoadPackage(ctx, structType.PkgPath)
			if err != nil || pkg == nil {
				e.logc(ctx, slog.LevelWarn, "could not load package for concrete method", "pkg", structType.PkgPath, "err", err)
				continue
			}

			// Create a callable object.Function for the concrete method.
			fnObject := e.getOrResolveFunction(ctx, pkg, concreteMethodInfo)
			if fnObject == nil {
				continue
			}

			// Mark the concrete method as "used" by calling the default intrinsic.
			e.logger.DebugContext(ctx, "finalize: marking concrete method as used", "method", fmt.Sprintf("%s.%s", structName, methodName))
			e.defaultIntrinsic(ctx, fnObject)
		}
	}
}
