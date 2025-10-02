package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"os"
	"runtime"
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
	scanner          *goscan.Scanner
	funcCache        map[string]object.Object
	intrinsics       *intrinsics.Registry
	logger           *slog.Logger
	tracer           object.Tracer // Tracer for debugging evaluation flow.
	callStack        []*object.CallFrame
	resolver         *Resolver
	defaultIntrinsic intrinsics.IntrinsicFunc
	initializedPkgs  map[string]bool // To track packages whose constants are loaded
	pkgCache         map[string]*object.Package
	files            []*FileScope
	fileMap          map[string]bool
	UniverseEnv      *object.Environment

	// accessor provides methods for finding fields and methods.
	accessor *accessor

	// evaluationInProgress tracks nodes that are currently being evaluated
	// to detect and prevent infinite recursion.
	evaluationInProgress map[ast.Node]bool
	evaluatingMu         sync.Mutex
	evaluating           map[string]bool

	// scanLiteralInProgress tracks function literal bodies currently being scanned
	// to prevent infinite recursion in scanFunctionLiteral.
	scanLiteralInProgress map[*ast.BlockStmt]bool

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
	memoizationCache map[token.Pos]object.Object
}

// contextKey is a private type to avoid collisions with other packages' context keys.
type contextKey string

const (
	// callFrameKey is the context key for the current call frame.
	callFrameKey contextKey = "callFrame"
)

// FrameFromContext returns the call frame from the context, if one exists.
func FrameFromContext(ctx context.Context) (*object.CallFrame, bool) {
	frame, ok := ctx.Value(callFrameKey).(*object.CallFrame)
	return frame, ok
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
		e.memoizationCache = make(map[token.Pos]object.Object)
	}
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
		resolver:               NewResolver(scanPolicy, scanner, logger),
		initializedPkgs:        make(map[string]bool),
		pkgCache:               make(map[string]*object.Package),
		files:                  make([]*FileScope, 0),
		fileMap:                make(map[string]bool),
		evaluationInProgress:   make(map[ast.Node]bool),
		evaluating:             make(map[string]bool),
		evaluatingMu:           sync.Mutex{},
		scanLiteralInProgress:  make(map[*ast.BlockStmt]bool),
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
		// var buf bytes.Buffer
		// fset := e.scanner.Fset()
		// if fset != nil && node != nil && node.Pos().IsValid() {
		// 	printer.Fprint(&buf, fset, node)
		// } else if node != nil {
		// 	printer.Fprint(&buf, nil, node)
		// }
		// source := buf.String()

		if pkg != nil && pkg.Fset != nil && node != nil && node.Pos().IsValid() {
			e.logger.DebugContext(ctx, "evaluating node",
				"type", fmt.Sprintf("%T", node),
				"pos", pkg.Fset.Position(node.Pos()),
				// "source", source,
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
		return e.evalBlockStmt(ctx, n, env, pkg)
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
			Lit:        n, // Store the AST node for the literal
			Parameters: n.Type.Params,
			Body:       n.Body,
			Env:        env,
			Package:    pkg,
		}
	case *ast.ArrayType:
		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, n.Pos(), "package info or fset is missing, cannot resolve types for array type")
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
		resolvedType := e.resolver.ResolveType(ctx, fieldType)

		placeholder := &object.SymbolicPlaceholder{Reason: "array type expression"}
		placeholder.SetFieldType(fieldType)
		placeholder.SetTypeInfo(resolvedType)
		return placeholder
	case *ast.MapType:
		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, n.Pos(), "package info or fset is missing, cannot resolve types for map type")
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
		resolvedType := e.resolver.ResolveType(ctx, fieldType)

		placeholder := &object.SymbolicPlaceholder{Reason: "map type expression"}
		placeholder.SetFieldType(fieldType)
		placeholder.SetTypeInfo(resolvedType)
		return placeholder
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

func (e *Evaluator) getOrLoadPackage(ctx context.Context, path string) (*object.Package, error) {
	e.evaluatingMu.Lock()
	if e.evaluating[path] {
		e.evaluatingMu.Unlock()
		e.logc(ctx, slog.LevelError, "recursion detected: already evaluating package", "path", path)
		return nil, fmt.Errorf("infinite recursion detected in package loading: %s", path)
	}
	e.evaluating[path] = true
	e.evaluatingMu.Unlock()

	defer func() {
		e.evaluatingMu.Lock()
		delete(e.evaluating, path)
		e.evaluatingMu.Unlock()
	}()

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

	// Populate types
	e.logger.DebugContext(ctx, "populating package-level types", "package", pkgInfo.ImportPath)
	for _, t := range pkgInfo.Types {
		if !shouldScan && !ast.IsExported(t.Name) {
			continue
		}
		typeObj := &object.Type{
			TypeName:     t.Name,
			ResolvedType: t,
		}
		typeObj.SetTypeInfo(t)
		env.SetLocal(t.Name, typeObj)
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

// forceEval recursively evaluates an object until it is no longer a variable or ambiguous selector.
// This is crucial for handling variables whose initializers are other variables and for resolving ambiguity.
func (e *Evaluator) forceEval(ctx context.Context, obj object.Object, pkg *scan.PackageInfo) object.Object {
	for i := 0; i < 100; i++ { // Add a loop limit to prevent infinite loops in weird cases
		switch o := obj.(type) {
		case *object.Variable:
			obj = e.evalVariable(ctx, o, pkg)
			if isError(obj) {
				return obj
			}
			// Loop again in case the result is another variable.
			continue
		case *object.AmbiguousSelector:
			// If forceEval encounters an ambiguous selector, it means the expression
			// is being used in a context where a value is expected (e.g., assignment,
			// variable access). We resolve the ambiguity by assuming it's a field.
			var typeName string
			if typeInfo := o.Receiver.TypeInfo(); typeInfo != nil {
				typeName = typeInfo.Name
			}
			e.logc(ctx, slog.LevelWarn, "assuming field exists on unresolved embedded type", "field_name", o.Sel.Name, "type_name", typeName)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed field %s on type with unresolved embedded part", o.Sel.Name)}
		default:
			// Not a variable or ambiguous selector, return as is.
			return obj
		}
	}
	return e.newError(ctx, token.NoPos, "evaluation depth limit exceeded, possible variable evaluation loop")
}

// logc logs a message with the current function context from the call stack.
func (e *Evaluator) logc(ctx context.Context, level slog.Level, msg string, args ...any) {
	// usually depth is 2, because logc is called from other functions
	e.logcWithCallerDepth(ctx, level, 2, msg, args...)
}

// for user, use logc instead of this function
func (e *Evaluator) logcWithCallerDepth(ctx context.Context, level slog.Level, depth int, msg string, args ...any) {
	if !e.logger.Enabled(ctx, level) {
		return
	}

	// Get execution position (the caller of this function)
	_, file, line, ok := runtime.Caller(depth)
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
	posStr := fmt.Sprintf("%d", pos) // Default to raw number
	if e.scanner != nil && e.scanner.Fset() != nil && pos.IsValid() {
		posStr = e.scanner.Fset().Position(pos).String()
	}
	e.logcWithCallerDepth(ctx, slog.LevelError, 2, msg, "pos", posStr)

	frames := make([]*object.CallFrame, len(e.callStack))
	copy(frames, e.callStack)
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
