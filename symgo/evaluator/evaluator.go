package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"log/slog"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	goscan "github.com/podhmo/go-scan"
	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/intrinsics"
	"github.com/podhmo/go-scan/symgo/object"
)

// versionSuffixRegex matches a trailing /vN path segment.
var versionSuffixRegex = regexp.MustCompile(`^v[0-9]+$`)

// guessPackageNameFromImportPath provides a heuristic to determine a package's
// potential names from its import path. It returns a slice of candidates.
func guessPackageNameFromImportPath(path string) []string {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")

	// Start with the last path segment.
	baseName := parts[len(parts)-1]

	// Handle gopkg.in/some-pkg.vN by splitting on the dot.
	if strings.HasPrefix(path, "gopkg.in/") {
		if dotIndex := strings.LastIndex(baseName, "."); dotIndex > 0 {
			baseName = baseName[:dotIndex]
		}
	}

	// If the last segment is a version suffix (e.g., "v5"), use the segment before it.
	if versionSuffixRegex.MatchString(baseName) {
		if len(parts) > 1 {
			baseName = parts[len(parts)-2]
		}
	}

	// Remove ".git" suffix if present
	baseName = strings.TrimSuffix(baseName, ".git")

	// Now generate candidates based on the cleaned baseName.
	candidates := make(map[string]struct{})

	// Candidate 1: a direct sanitization (e.g., "go-isatty" -> "goisatty")
	sanitized := strings.ReplaceAll(baseName, "-", "")
	candidates[sanitized] = struct{}{}

	// Candidate 2: strip "go-" prefix and then sanitize
	if strings.HasPrefix(baseName, "go-") {
		stripped := strings.TrimPrefix(baseName, "go-")
		strippedAndSanitized := strings.ReplaceAll(stripped, "-", "")
		candidates[strippedAndSanitized] = struct{}{}
	}

	// Candidate 3: strip "-go" suffix and then sanitize
	if strings.HasSuffix(baseName, "-go") {
		stripped := strings.TrimSuffix(baseName, "-go")
		strippedAndSanitized := strings.ReplaceAll(stripped, "-", "")
		candidates[strippedAndSanitized] = struct{}{}
	}

	// Convert map to slice to return a stable (though unordered) list of unique names.
	result := make([]string, 0, len(candidates))
	for name := range candidates {
		result = append(result, name)
	}
	return result
}

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

func (e *Evaluator) evalComplexInfixExpression(ctx context.Context, pos token.Pos, op token.Token, left, right object.Object) object.Object {
	if _, ok := left.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "complex operation with symbolic operand"}
	}
	if _, ok := right.(*object.SymbolicPlaceholder); ok {
		return &object.SymbolicPlaceholder{Reason: "complex operation with symbolic operand"}
	}

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

	switch val := right.(type) {
	case *object.Integer:
		switch op {
		case token.SUB:
			return &object.Integer{Value: -val.Value}
		case token.ADD:
			return &object.Integer{Value: val.Value} // Unary plus is a no-op.
		case token.XOR:
			return &object.Integer{Value: ^val.Value} // Bitwise NOT.
		default:
			return e.newError(ctx, token.NoPos, "unhandled numeric unary operator for INTEGER: %s", op)
		}
	case *object.Float:
		switch op {
		case token.SUB:
			return &object.Float{Value: -val.Value}
		case token.ADD:
			return &object.Float{Value: val.Value}
		default:
			return e.newError(ctx, token.NoPos, "unhandled numeric unary operator for FLOAT: %s", op)
		}
	case *object.Complex:
		switch op {
		case token.SUB:
			return &object.Complex{Value: -val.Value}
		case token.ADD:
			return &object.Complex{Value: val.Value}
		default:
			return e.newError(ctx, token.NoPos, "unhandled numeric unary operator for COMPLEX: %s", op)
		}
	default:
		return e.newError(ctx, token.NoPos, "unary operator %s not supported for type %s", op, right.Type())
	}
}

func (e *Evaluator) evalTypeDecl(ctx context.Context, d *ast.GenDecl, env *object.Environment, pkg *scan.PackageInfo) {
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		// Find the TypeInfo that the scanner created for this TypeSpec.
		var typeInfo *scan.TypeInfo
		if pkg != nil { // pkg can be nil in some tests
			for _, ti := range pkg.Types {
				if ti.Node == ts {
					typeInfo = ti
					break
				}
			}
		}

		if typeInfo == nil {
			// This could be a local type definition inside a function.
			// The scanner does not create TypeInfo for these, so we create one on the fly.
			if pkg == nil || pkg.Fset == nil {
				e.logc(ctx, slog.LevelWarn, "cannot create local type info without package context", "type", ts.Name.Name)
				continue
			}
			file := pkg.Fset.File(ts.Pos())
			if file == nil {
				e.logc(ctx, slog.LevelWarn, "could not find file for local type node position", "type", ts.Name.Name)
				continue
			}
			astFile, fileOK := pkg.AstFiles[file.Name()]
			if !fileOK {
				e.logc(ctx, slog.LevelWarn, "could not find ast.File for local type", "type", ts.Name.Name, "path", file.Name())
				continue
			}
			importLookup := e.scanner.BuildImportLookup(astFile)

			// Determine the underlying type information.
			underlyingFieldType := e.scanner.TypeInfoFromExpr(ctx, ts.Type, nil, pkg, importLookup)
			// Note: We don't resolve the underlying type here. The important part is to
			// capture the AST (`ts`) and the textual representation of the underlying type (`underlyingFieldType`).
			// The resolution will happen later when this type is actually used.

			// Create a new TypeInfo for the local alias.
			typeInfo = &scan.TypeInfo{
				Name:       ts.Name.Name,
				PkgPath:    pkg.ImportPath, // Local types belong to the current package.
				Node:       ts,             // IMPORTANT: Store the AST node.
				Underlying: underlyingFieldType,
				Kind:       scan.AliasKind, // Mark it as an alias.
			}
		}

		typeObj := &object.Type{
			TypeName:     typeInfo.Name,
			ResolvedType: typeInfo,
		}
		typeObj.SetTypeInfo(typeInfo)
		env.Set(ts.Name.Name, typeObj)
	}
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
						// The break was redundant here. The switch statement exits, and the
						// inner for-loop continues to the next statement in the case body.
						// The `hasFallthrough` flag is handled after the loop.
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

// scanFunctionLiteral evaluates the body of a function literal or method value in a new,
// symbolic environment. This is used to find function calls inside anonymous functions or
// method values that are passed as arguments, without needing to fully execute the function
// they are passed to.
func (e *Evaluator) scanFunctionLiteral(ctx context.Context, fn *object.Function) {
	if fn.Body == nil || fn.Package == nil {
		return // Nothing to scan.
	}

	// Prevent infinite recursion.
	if e.scanLiteralInProgress[fn.Body] {
		return
	}
	e.scanLiteralInProgress[fn.Body] = true
	defer delete(e.scanLiteralInProgress, fn.Body)

	e.logger.DebugContext(ctx, "scanning function literal/method value to find usages", "pos", fn.Package.Fset.Position(fn.Body.Pos()))

	// Create a new environment for the function's execution.
	// It's enclosed by the environment where the function was defined.
	fnEnv := object.NewEnclosedEnvironment(fn.Env)

	// Bind receiver if it's a method value.
	if fn.Receiver != nil && fn.Decl != nil && fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
		recvField := fn.Decl.Recv.List[0]
		if len(recvField.Names) > 0 && recvField.Names[0].Name != "" {
			receiverName := recvField.Names[0].Name
			fnEnv.SetLocal(receiverName, fn.Receiver)
			e.logger.DebugContext(ctx, "scanFunctionLiteral: bound receiver", "name", receiverName, "type", fn.Receiver.Type())
		}
	}

	// Populate the environment with symbolic placeholders for the parameters.
	if fn.Parameters != nil {
		var importLookup map[string]string
		file := fn.Package.Fset.File(fn.Body.Pos())
		if file != nil {
			if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
				importLookup = e.scanner.BuildImportLookup(astFile)
			}
		}
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
				Reason: "symbolic parameter for function scan",
				BaseObject: object.BaseObject{
					ResolvedTypeInfo:  resolvedType,
					ResolvedFieldType: fieldType,
				},
			}

			for _, name := range field.Names {
				if name.Name != "_" {
					v := &object.Variable{Name: name.Name, Value: placeholder}
					v.SetFieldType(fieldType)
					v.SetTypeInfo(resolvedType)
					fnEnv.Set(name.Name, v)
				}
			}
		}
	}

	// Now evaluate the body. The result is ignored; we only care about the side effects.
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
		if e.memoize && f.Decl != nil {
			if cachedResult, found := e.memoizationCache[f.Decl.Pos()]; found {
				e.logc(ctx, slog.LevelDebug, "returning memoized result for function", "function", f.Name)
				return cachedResult
			}
		}
	}

	result := e.applyFunctionImpl(ctx, fn, args, pkg, callPos)

	if f, ok := fn.(*object.Function); ok {
		if e.memoize && !isError(result) && f.Decl != nil {
			e.logc(ctx, slog.LevelDebug, "caching result for function", "function", f.Name)
			e.memoizationCache[f.Decl.Pos()] = result
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

	// New recursion check based on function definition (for named functions)
	// or function literal position (for anonymous functions).
	if f, ok := fn.(*object.Function); ok {
		// Determine which call stack to use for recursion detection.
		// If the function has a BoundCallStack, it means it was passed as an argument,
		// and that stack represents the true logical path leading to this call.
		stackToScan := e.callStack
		if f.BoundCallStack != nil {
			stackToScan = f.BoundCallStack
		}

		recursionCount := 0
		for _, frame := range stackToScan {
			if frame.Fn == nil {
				continue
			}

			// Case 1: Named function with a definition. Compare declaration positions.
			if f.Def != nil && f.Def.AstDecl != nil && frame.Fn.Def != nil && frame.Fn.Def.AstDecl != nil {
				if f.Def.AstDecl.Pos() == frame.Fn.Def.AstDecl.Pos() {
					recursionCount++
				}
				continue
			}

			// Case 2: Anonymous function (function literal). Compare literal positions.
			if f.Lit != nil && frame.Fn.Lit != nil {
				if f.Lit.Pos() == frame.Fn.Lit.Pos() {
					recursionCount++
				}
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

	frame := &object.CallFrame{Function: name, Pos: callPos, Args: args}
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
	case *object.AmbiguousSelector:
		// If applyFunction encounters an ambiguous selector, it means the expression
		// is being used in a call context `expr()`. We resolve the ambiguity
		// by assuming it's a method call.
		var typeName string
		if typeInfo := fn.Receiver.TypeInfo(); typeInfo != nil {
			typeName = typeInfo.Name
		}
		e.logc(ctx, slog.LevelWarn, "assuming method exists on unresolved embedded type", "method_name", fn.Sel.Name, "type_name", typeName)
		placeholder := &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed method %s on type with unresolved embedded part", fn.Sel.Name)}
		// The placeholder is now the function, so we recursively call applyFunction with it.
		return e.applyFunction(ctx, placeholder, args, pkg, callPos)
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
		if evaluated != nil {
			if isError(evaluated) || evaluated.Type() == object.PANIC_OBJ {
				return evaluated
			}
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
			e.logc(ctx, slog.LevelInfo, "could not scan package for unresolved function (or denied by policy)", "package", fn.PkgPath, "function", fn.FuncName, "error", err)
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
				// If the argument is a function, we need to "tag" it with the current call stack
				// to enable recursion detection through higher-order functions.
				if funcArg, ok := arg.(*object.Function); ok {
					clonedFunc := funcArg.Clone()
					// Create a copy of the call stack to avoid shared state issues.
					stackCopy := make([]*object.CallFrame, len(e.callStack))
					copy(stackCopy, e.callStack)
					clonedFunc.BoundCallStack = stackCopy
					arg = clonedFunc // Use the tagged clone for the binding
				}

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
