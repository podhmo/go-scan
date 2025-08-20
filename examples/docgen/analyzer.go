package main

import (
	"context"
	"fmt"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/symgo"
)

// Analyzer analyzes Go code and generates an OpenAPI specification.
type Analyzer struct {
	Scanner     *goscan.Scanner
	interpreter *symgo.Interpreter
	OpenAPI     *openapi.OpenAPI
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(s *goscan.Scanner) (*Analyzer, error) {
	internalScanner, err := s.ScannerForSymgo()
	if err != nil {
		return nil, fmt.Errorf("failed to get internal scanner: %w", err)
	}
	interp, err := symgo.NewInterpreter(internalScanner, s.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create symgo interpreter: %w", err)
	}

	a := &Analyzer{
		Scanner:     s,
		interpreter: interp,
		OpenAPI: &openapi.OpenAPI{
			OpenAPI: "3.1.0",
			Info: openapi.Info{
				Title:   "Sample API",
				Version: "0.0.1",
			},
			Paths: make(map[string]*openapi.PathItem),
		},
	}

	// Register intrinsics.
	interp.RegisterIntrinsic("net/http.NewServeMux", a.handleNewServeMux)
	interp.RegisterIntrinsic("(*net/http.ServeMux).HandleFunc", a.analyzeHandleFunc)

	return a, nil
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
	if _, err := a.interpreter.Eval(entrypointFile, pkg); err != nil {
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
	if _, err := a.interpreter.Apply(entrypointFn, []symgo.Object{}, pkg); err != nil {
		// This is the change: we now return the error instead of just printing it.
		return fmt.Errorf("error during entrypoint apply: %w", err)
	}

	return nil
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
		a.analyzeHandlerBody(handlerObj, op)
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
func (a *Analyzer) analyzeHandlerBody(handler *symgo.Function, op *openapi.Operation) {
	pkg, err := a.Scanner.ScanPackageByPos(context.Background(), handler.Decl.Pos())
	if err != nil {
		fmt.Printf("warn: failed to get package for handler %q: %v\n", handler.Name.Name, err)
		return
	}

	// Create symbolic arguments for the handler function (w, r).
	var handlerArgs []symgo.Object
	if handler.Decl.Type.Params != nil {
		file := pkg.Fset.File(handler.Decl.Pos())
		if file == nil {
			fmt.Printf("warn: could not find file for handler %q\n", handler.Name.Name)
			return
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			fmt.Printf("warn: could not find AST file for handler %q\n", handler.Name.Name)
			return
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

	// Push a new scope for temporary intrinsics for this handler.
	intrinsics := a.buildHandlerIntrinsics(op)
	a.interpreter.PushIntrinsics(intrinsics)
	defer a.interpreter.PopIntrinsics() // Ensure we clean up the scope.

	// Call the handler function with the created symbolic arguments.
	a.interpreter.Apply(handler, handlerArgs, pkg)
}

// buildHandlerIntrinsics creates the map of intrinsic handlers for analyzing
// a handler's body by using the extensible pattern registry.
func (a *Analyzer) buildHandlerIntrinsics(op *openapi.Operation) map[string]symgo.IntrinsicFunc {
	intrinsics := make(map[string]symgo.IntrinsicFunc)

	for _, p := range patterns.GetDefaultPatterns() {
		// Capture the pattern for the closure.
		pattern := p
		intrinsics[pattern.Key] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			// The `op` is captured from the outer scope here.
			return pattern.Apply(i, args, op)
		}
	}

	return intrinsics
}
