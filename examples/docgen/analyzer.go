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
	interp, err := symgo.NewInterpreter(s, s.Logger)
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
	if _, err := a.interpreter.Eval(ctx, entrypointFile); err != nil {
		fmt.Printf("info: error during file-level symgo eval: %v\n", err)
	}

	// Then, start evaluation from the body of the entrypoint function.
	if _, err := a.interpreter.Eval(ctx, entrypointFunc.AstDecl.Body); err != nil {
		// In a real application, this would use a proper logger.
		fmt.Printf("info: error during body-level symgo eval: %v\n", err)
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
