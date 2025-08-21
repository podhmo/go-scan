package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/symgo"
)

// Analyzer analyzes Go code and generates an OpenAPI specification.
type Analyzer struct {
	Scanner        *goscan.Scanner
	interpreter    *symgo.Interpreter
	OpenAPI        *openapi.OpenAPI
	logger         *slog.Logger
	tracer         symgo.Tracer // Optional tracer
	operationStack []*openapi.Operation
	customPatterns []patterns.Pattern
}

// Option is a functional option for configuring the Analyzer.
type Option func(*Analyzer)

// WithTracer sets a tracer on the analyzer to instrument the symbolic execution.
func WithTracer(tracer symgo.Tracer) Option {
	return func(a *Analyzer) {
		a.tracer = tracer
	}
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(s *goscan.Scanner, logger *slog.Logger, options ...any) (*Analyzer, error) {
	a := &Analyzer{
		Scanner: s,
		logger:  logger,
		OpenAPI: &openapi.OpenAPI{
			OpenAPI: "3.1.0",
			Info: openapi.Info{
				Title:   "Sample API",
				Version: "0.0.1",
			},
			Paths: make(map[string]*openapi.PathItem),
		},
	}

	// Process options
	for _, opt := range options {
		switch v := opt.(type) {
		case Option:
			v(a)
		case patterns.Pattern:
			a.customPatterns = append(a.customPatterns, v)
		default:
			// For backward compatibility, assume it's a pattern.
			if p, ok := opt.(patterns.Pattern); ok {
				a.customPatterns = append(a.customPatterns, p)
			}
		}
	}

	interpOpts := []symgo.Option{symgo.WithLogger(logger)}
	if a.tracer != nil {
		interpOpts = append(interpOpts, symgo.WithTracer(a.tracer))
	}

	interp, err := symgo.NewInterpreter(s, interpOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create symgo interpreter: %w", err)
	}
	a.interpreter = interp

	// Register intrinsics.
	interp.RegisterIntrinsic("net/http.NewServeMux", a.handleNewServeMux)
	interp.RegisterIntrinsic("(*net/http.ServeMux).HandleFunc", a.analyzeHandleFunc)

	// Intrinsics for handling http.Handler interface wrappers
	interp.RegisterIntrinsic("net/http.HandlerFunc", a.handleHandlerFunc)
	interp.RegisterIntrinsic("net/http.TimeoutHandler", a.handleTimeoutHandler)
	interp.RegisterIntrinsic("(*net/http.ServeMux).Handle", a.analyzeHandle)

	return a, nil
}

func (a *Analyzer) OperationStack() []*openapi.Operation {
	return a.operationStack
}

func (a *Analyzer) handleNewServeMux(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	return patterns.NewSymbolicInstance(interp, "net/http.ServeMux")
}

// Analyze analyzes the package starting from a specific entrypoint function.
func (a *Analyzer) Analyze(ctx context.Context, importPath string, entrypoint string) error {
	pkg, err := a.Scanner.ScanPackageByImport(ctx, importPath)
	if err != nil {
		return fmt.Errorf("failed to load sample API package: %w", err)
	}

	// Find the entrypoint function declaration in the scanned package.
	var entrypointFunc *goscan.FunctionInfo
	for _, f := range pkg.Functions {
		if f.Name == entrypoint {
			entrypointFunc = f
			break
		}
	}
	if entrypointFunc == nil || entrypointFunc.AstDecl.Body == nil {
		return fmt.Errorf("entrypoint function %q not found or has no body", entrypoint)
	}

	// Find the AST file that contains the entrypoint function.
	entrypointFile, ok := pkg.AstFiles[entrypointFunc.FilePath]
	if !ok {
		return fmt.Errorf("could not find AST file %q for entrypoint", entrypointFunc.FilePath)
	}

	// The core analysis is now driven by the symgo interpreter.
	// First, evaluate the entire file of the entrypoint. This will populate the
	// interpreter's environment with imports and top-level declarations.
	if _, err := a.interpreter.Eval(ctx, entrypointFile, pkg); err != nil {
		// This is the change: we now return the error instead of just printing it.
		return fmt.Errorf("error during file-level symgo eval: %w", err)
	}

	// Get the function object from the environment.
	entrypointObj, ok := a.interpreter.FindObject(entrypoint)
	if !ok {
		return fmt.Errorf("entrypoint function %q not found in interpreter environment", entrypoint)
	}
	entrypointFn, ok := entrypointObj.(*symgo.Function)
	if !ok {
		return fmt.Errorf("entrypoint %q is not a function", entrypoint)
	}

	// Then, call the entrypoint function.
	if _, err := a.interpreter.Apply(ctx, entrypointFn, []symgo.Object{}, pkg); err != nil {
		// This is the change: we now return the error instead of just printing it.
		return fmt.Errorf("error during entrypoint apply: %w", err)
	}

	return nil
}

func (a *Analyzer) handleHandlerFunc(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	if len(args) != 1 {
		return &symgo.Error{Message: fmt.Sprintf("HandlerFunc expects 1 argument, but got %d", len(args))}
	}
	fn, ok := args[0].(*symgo.Function)
	if !ok {
		// It might be an instance wrapping a function, let's try to unwrap it.
		unwrapped := a.unwrapHandler(args[0])
		if unwrapped == nil {
			return &symgo.Error{Message: fmt.Sprintf("HandlerFunc expects a function, but got %T", args[0])}
		}
		fn = unwrapped
	}
	return &symgo.Instance{
		TypeName:   "net/http.Handler",
		Underlying: fn,
		BaseObject: symgo.BaseObject{ResolvedTypeInfo: fn.TypeInfo()},
	}
}

func (a *Analyzer) handleTimeoutHandler(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	if len(args) != 3 {
		return &symgo.Error{Message: fmt.Sprintf("TimeoutHandler expects 3 arguments, but got %d", len(args))}
	}
	// The first argument is the handler, which we care about.
	// The other two are timeout and message, which we can ignore for doc generation.
	handler, ok := args[0].(symgo.Object) // Should be an Instance, but we just pass it through
	if !ok {
		return &symgo.Error{Message: fmt.Sprintf("TimeoutHandler expects a handler, but got %T", args[0])}
	}

	// Wrap it in another instance to represent the handler returned by TimeoutHandler.
	return &symgo.Instance{
		TypeName:   "net/http.Handler",
		Underlying: handler,
		BaseObject: symgo.BaseObject{ResolvedTypeInfo: handler.TypeInfo()},
	}
}

func (a *Analyzer) analyzeHandle(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	if len(args) != 3 {
		return &symgo.Error{Message: fmt.Sprintf("Handle expects 3 arguments, but got %d", len(args))}
	}

	patternObj, ok := args[1].(*symgo.String)
	if !ok {
		return &symgo.Error{Message: fmt.Sprintf("Handle pattern argument must be a string, but got %T", args[1])}
	}

	// Unwrap the handler to find the root function.
	handlerFunc := a.unwrapHandler(args[2])
	if handlerFunc == nil {
		// Return nil instead of error, as some handlers might be intentionally opaque.
		a.logger.DebugContext(context.Background(), "could not unwrap handler", "arg", args[2].Inspect())
		return nil
	}

	// Create a new argument slice for analyzeHandleFunc.
	// The first arg (receiver) and second (pattern) are the same.
	// The third is the unwrapped function.
	newArgs := []symgo.Object{args[0], patternObj, handlerFunc}

	return a.analyzeHandleFunc(interp, newArgs)
}

// unwrapHandler recursively unwraps http.Handler instances to find the underlying function.
func (a *Analyzer) unwrapHandler(obj symgo.Object) *symgo.Function {
	switch v := obj.(type) {
	case *symgo.Function:
		return v
	case *symgo.Instance:
		if v.Underlying != nil {
			return a.unwrapHandler(v.Underlying)
		}
		return nil
	default:
		return nil
	}
}

// analyzeHandleFunc is the intrinsic for (*http.ServeMux).HandleFunc.
func (a *Analyzer) analyzeHandleFunc(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	// Expects 3 args for HandleFunc: receiver, pattern, handler
	if len(args) != 3 {
		return &symgo.Error{Message: fmt.Sprintf("HandleFunc expects 3 arguments, but got %d", len(args))}
	}

	// Arg 0 is the receiver, which we can ignore.
	// Arg 1 is the pattern string.
	patternObj, ok := args[1].(*symgo.String)
	if !ok {
		return &symgo.Error{Message: fmt.Sprintf("HandleFunc pattern argument must be a string, but got %T", args[1])}
	}

	// Arg 2 is the handler function.
	handlerObj, ok := args[2].(*symgo.Function)
	if !ok {
		// It's possible the handler is not yet resolved, this is a limitation for now.
		return &symgo.Error{Message: fmt.Sprintf("HandleFunc handler argument must be a function, but got %T", args[2])}
	}

	pattern := patternObj.Value
	method, path, _ := strings.Cut(pattern, " ")
	if path == "" {
		path = method
		method = "GET"
	}
	method = strings.ToUpper(method)

	handlerDecl := handlerObj.Decl
	if handlerDecl == nil {
		return nil
	}

	op := &openapi.Operation{
		OperationID: handlerDecl.Name.Name,
	}
	if handlerDecl.Doc != nil {
		op.Description = strings.TrimSpace(handlerDecl.Doc.Text())
	}

	// Analyze the handler body for request/response schemas
	if handlerDecl.Body != nil {
		op = a.analyzeHandlerBody(handlerObj, op)
	}

	if a.OpenAPI.Paths[path] == nil {
		a.OpenAPI.Paths[path] = &openapi.PathItem{}
	}
	pathItem := a.OpenAPI.Paths[path]

	switch method {
	case "GET":
		pathItem.Get = op
	case "POST":
		pathItem.Post = op
	case "PUT":
		pathItem.Put = op
	case "DELETE":
		pathItem.Delete = op
	case "PATCH":
		pathItem.Patch = op
	case "HEAD":
		pathItem.Head = op
	case "OPTIONS":
		pathItem.Options = op
	case "TRACE":
		pathItem.Trace = op
	}

	return nil
}

// analyzeHandlerBody analyzes the body of an HTTP handler function to find
// request and response schemas.
func (a *Analyzer) analyzeHandlerBody(handler *symgo.Function, op *openapi.Operation) *openapi.Operation {
	// Capture stack size before we modify it.
	originalStackSize := len(a.operationStack)
	defer func() {
		// Always restore the stack to its original size.
		a.operationStack = a.operationStack[:originalStackSize]
	}()

	// Push the current operation onto the stack for the duration of this analysis.
	a.operationStack = append(a.operationStack, op)

	pkg, err := a.Scanner.ScanPackageByPos(context.Background(), handler.Decl.Pos())
	if err != nil {
		fmt.Printf("warn: failed to get package for handler %q: %v\n", handler.Name.Name, err)
		return op // Return original op on error
	}

	// Create symbolic arguments for the handler function (w, r).
	var handlerArgs []symgo.Object
	if handler.Decl.Type.Params != nil {
		file := pkg.Fset.File(handler.Decl.Pos())
		if file == nil {
			fmt.Printf("warn: could not find file for handler %q\n", handler.Name.Name)
			return op // Return original op on error
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			fmt.Printf("warn: could not find AST file for handler %q\n", handler.Name.Name)
			return op // Return original op on error
		}
		importLookup := a.Scanner.BuildImportLookup(astFile)

		for _, field := range handler.Decl.Type.Params.List {
			fieldType := a.Scanner.TypeInfoFromExpr(context.Background(), field.Type, nil, pkg, importLookup)
			typeInfo, _ := fieldType.Resolve(context.Background())

			// For each parameter name (can be multiple like w1, w2 http.ResponseWriter), create a variable.
			for _, name := range field.Names {
				arg := &symgo.Variable{
					Name: name.Name,
					BaseObject: symgo.BaseObject{
						ResolvedTypeInfo: typeInfo,
					},
					Value: &symgo.SymbolicPlaceholder{Reason: "function parameter"},
				}
				handlerArgs = append(handlerArgs, arg)
			}
		}
	}

	// Bind the http.ResponseWriter interface to a concrete type for analysis.
	// This allows us to track calls to methods like WriteHeader and Write.
	if err := a.interpreter.BindInterface("net/http.ResponseWriter", "*net/http/httptest.ResponseRecorder"); err != nil {
		// This binding is critical, so we log a warning if it fails.
		a.logger.Warn("failed to bind ResponseWriter interface, response analysis will be incomplete", "error", err)
	}

	// Push a new scope for temporary intrinsics for this handler.
	intrinsics := a.buildHandlerIntrinsics(a)
	a.interpreter.PushIntrinsics(intrinsics)
	defer a.interpreter.PopIntrinsics() // Ensure we clean up the scope.

	// Call the handler function with the created symbolic arguments.
	a.interpreter.Apply(context.Background(), handler, handlerArgs, pkg)

	// After Apply, the operation on the top of the stack is the one that has been modified.
	// We retrieve it before the defer pops it.
	finalOp := a.operationStack[len(a.operationStack)-1]

	return finalOp
}

// buildHandlerIntrinsics creates the map of intrinsic handlers for analyzing
// a handler's body by using the extensible pattern registry.
func (a *Analyzer) buildHandlerIntrinsics(analyzer *Analyzer) map[string]symgo.IntrinsicFunc {
	intrinsics := make(map[string]symgo.IntrinsicFunc)

	allPatterns := append(patterns.GetDefaultPatterns(), a.customPatterns...)

	for _, p := range allPatterns {
		// Capture the pattern for the closure.
		pattern := p
		intrinsics[pattern.Key] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			// The analyzer instance `a` is captured from the outer scope.
			// The pattern's Apply function will use it to get the current operation.
			return pattern.Apply(i, analyzer, args)
		}
	}

	return intrinsics
}
