package main

import (
	"context"
	"fmt"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
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
	return &symgo.Instance{TypeName: "net/http.ServeMux"}
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
		fmt.Printf("info: error during file-level symgo eval: %v\n", err)
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
		// In a real application, this would use a proper logger.
		fmt.Printf("info: error during entrypoint apply: %v\n", err)
	}

	return nil
}

// analyzeHandleFunc is the intrinsic for (*http.ServeMux).HandleFunc.
func (a *Analyzer) analyzeHandleFunc(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	// Expects 2 args for HandleFunc: pattern, handler
	if len(args) != 2 {
		return nil
	}

	// Arg 0 is the pattern string
	patternObj, ok := args[0].(*symgo.String)
	if !ok {
		return nil
	}

	// Arg 1 is the handler function
	handlerObj, ok := args[1].(*symgo.Function)
	if !ok {
		return nil
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
	intrinsics := a.buildJSONIntrinsics(op)
	a.interpreter.PushIntrinsics(intrinsics)
	defer a.interpreter.PopIntrinsics() // Ensure we clean up the scope.

	// Call the handler function with the created symbolic arguments.
	a.interpreter.Apply(handler, handlerArgs, pkg)
}

// buildJSONIntrinsics creates the map of intrinsic handlers for JSON analysis.
func (a *Analyzer) buildJSONIntrinsics(op *openapi.Operation) map[string]symgo.IntrinsicFunc {
	intrinsics := make(map[string]symgo.IntrinsicFunc)

	// Hook for json.NewDecoder(r.Body) -> *json.Decoder
	intrinsics["encoding/json.NewDecoder"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return &symgo.Instance{TypeName: "encoding/json.Decoder"}
	}

	// Hook for (*json.Decoder).Decode(&v)
	intrinsics["(*encoding/json.Decoder).Decode"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		if len(args) != 2 {
			return &symgo.SymbolicPlaceholder{Reason: "decode error"}
		}
		ptr, ok := args[1].(*symgo.Pointer)
		if !ok {
			return &symgo.SymbolicPlaceholder{Reason: "decode error"}
		}
		typeInfo := ptr.TypeInfo()
		if typeInfo != nil {
			schema := buildSchemaForType(context.Background(), typeInfo, make(map[string]*openapi.Schema))
			if schema != nil {
				op.RequestBody = &openapi.RequestBody{
					Content:  map[string]openapi.MediaType{"application/json": {Schema: schema}},
					Required: true,
				}
			}
		}
		return &symgo.SymbolicPlaceholder{Reason: "result of json.Decode"}
	}

	// Hook for json.NewEncoder(w) -> *json.Encoder
	intrinsics["encoding/json.NewEncoder"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return &symgo.Instance{TypeName: "encoding/json.Encoder"}
	}

	// Hook for (*json.Encoder).Encode(v)
	intrinsics["(*encoding/json.Encoder).Encode"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		if len(args) != 2 {
			return &symgo.SymbolicPlaceholder{Reason: "encode error"}
		}
		fmt.Printf("ENCODE: arg[1] is %T\n", args[1])
		typeInfo := args[1].TypeInfo()
		if typeInfo != nil {
			fmt.Printf("ENCODE: found type %s\n", typeInfo.Name)
			schema := buildSchemaForType(context.Background(), typeInfo, make(map[string]*openapi.Schema))
			if schema != nil {
				if op.Responses == nil {
					op.Responses = make(map[string]*openapi.Response)
				}
				op.Responses["200"] = &openapi.Response{
					Description: "OK",
					Content:     map[string]openapi.MediaType{"application/json": {Schema: schema}},
				}
			}
		} else {
			fmt.Println("ENCODE: typeInfo is nil")
		}
		return &symgo.SymbolicPlaceholder{Reason: "result of json.Encode"}
	}

	return intrinsics
}
