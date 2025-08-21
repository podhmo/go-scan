package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
)

// Analyzer analyzes Go code and generates an OpenAPI specification.
type Analyzer struct {
	Scanner        *goscan.Scanner
	interpreter    *symgo.Interpreter
	OpenAPI        *openapi.OpenAPI
	logger         *slog.Logger
	operationStack []*openapi.Operation
	customPatterns []Pattern
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(s *goscan.Scanner, logger *slog.Logger, customPatterns []Pattern) (*Analyzer, error) {
	interp, err := symgo.NewInterpreter(s, symgo.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("failed to create symgo interpreter: %w", err)
	}

	a := &Analyzer{
		Scanner:        s,
		interpreter:    interp,
		logger:         logger,
		customPatterns: customPatterns,
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
	interp.RegisterIntrinsic("net/http.HandlerFunc", a.handleHandlerFunc)
	interp.RegisterIntrinsic("net/http.TimeoutHandler", a.handleTimeoutHandler)
	interp.RegisterIntrinsic("(*net/http.ServeMux).Handle", a.analyzeHandle)

	return a, nil
}

func (a *Analyzer) OperationStack() []*openapi.Operation {
	return a.operationStack
}

func (a *Analyzer) handleNewServeMux(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "net/http.ServeMux")
}

func (a *Analyzer) Analyze(ctx context.Context, importPath string, entrypoint string) error {
	pkg, err := a.Scanner.ScanPackageByImport(ctx, importPath)
	if err != nil {
		return fmt.Errorf("failed to load sample API package: %w", err)
	}

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

	entrypointFile, ok := pkg.AstFiles[entrypointFunc.FilePath]
	if !ok {
		return fmt.Errorf("could not find AST file %q for entrypoint", entrypointFunc.FilePath)
	}

	if _, err := a.interpreter.Eval(ctx, entrypointFile, pkg); err != nil {
		return fmt.Errorf("error during file-level symgo eval: %w", err)
	}

	entrypointObj, ok := a.interpreter.FindObject(entrypoint)
	if !ok {
		return fmt.Errorf("entrypoint function %q not found in interpreter environment", entrypoint)
	}
	entrypointFn, ok := entrypointObj.(*symgo.Function)
	if !ok {
		return fmt.Errorf("entrypoint %q is not a function", entrypoint)
	}

	if _, err := a.interpreter.Apply(ctx, entrypointFn, []symgo.Object{}, pkg); err != nil {
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
	handler, ok := args[0].(symgo.Object)
	if !ok {
		return &symgo.Error{Message: fmt.Sprintf("TimeoutHandler expects a handler, but got %T", args[0])}
	}
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
	handlerFunc := a.unwrapHandler(args[2])
	if handlerFunc == nil {
		a.logger.DebugContext(context.Background(), "could not unwrap handler", "arg", args[2].Inspect())
		return nil
	}
	newArgs := []symgo.Object{args[0], patternObj, handlerFunc}
	return a.analyzeHandleFunc(interp, newArgs)
}

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

func (a *Analyzer) analyzeHandleFunc(interp *symgo.Interpreter, args []symgo.Object) symgo.Object {
	if len(args) != 3 {
		return &symgo.Error{Message: fmt.Sprintf("HandleFunc expects 3 arguments, but got %d", len(args))}
	}
	patternObj, ok := args[1].(*symgo.String)
	if !ok {
		return &symgo.Error{Message: fmt.Sprintf("HandleFunc pattern argument must be a string, but got %T", args[1])}
	}
	handlerObj, ok := args[2].(*symgo.Function)
	if !ok {
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

func (a *Analyzer) analyzeHandlerBody(handler *symgo.Function, op *openapi.Operation) {
	a.operationStack = append(a.operationStack, op)
	defer func() {
		a.operationStack = a.operationStack[:len(a.operationStack)-1]
	}()

	pkg, err := a.Scanner.ScanPackageByPos(context.Background(), handler.Decl.Pos())
	if err != nil {
		fmt.Printf("warn: failed to get package for handler %q: %v\n", handler.Name.Name, err)
		return
	}

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
			for _, name := range field.Names {
				arg := &symgo.Variable{
					Name:       name.Name,
					BaseObject: symgo.BaseObject{ResolvedTypeInfo: typeInfo},
					Value:      &symgo.SymbolicPlaceholder{Reason: "function parameter"},
				}
				handlerArgs = append(handlerArgs, arg)
			}
		}
	}

	if err := a.interpreter.BindInterface("net/http.ResponseWriter", "*net/http/httptest.ResponseRecorder"); err != nil {
		a.logger.Warn("failed to bind ResponseWriter interface, response analysis will be incomplete", "error", err)
	}

	intrinsics := a.buildHandlerIntrinsics()
	a.interpreter.PushIntrinsics(intrinsics)
	defer a.interpreter.PopIntrinsics()

	a.interpreter.Apply(context.Background(), handler, handlerArgs, pkg)
}

func (a *Analyzer) buildHandlerIntrinsics() map[string]symgo.IntrinsicFunc {
	intrinsics := make(map[string]symgo.IntrinsicFunc)

	intrinsics["encoding/json.NewDecoder"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return HandleNewDecoder(i, a, args)
	}
	intrinsics["encoding/json.NewEncoder"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return HandleNewEncoder(i, a, args)
	}
	intrinsics["(*net/url.URL).Query"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return HandleURLQuery(i, a, args)
	}
	intrinsics["(net/http.ResponseWriter).Header"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return HandleHeader(i, a, args)
	}
	intrinsics["(*net/http/httptest.ResponseRecorder).Header"] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return HandleHeader(i, a, args)
	}

	for _, p := range a.customPatterns {
		pattern := p
		intrinsics[pattern.Key] = func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			if len(a.operationStack) == 0 {
				a.logger.Warn("pattern called outside of handler analysis", "key", pattern.Key)
				return nil
			}
			op := a.operationStack[len(a.operationStack)-1]

			if len(args) <= pattern.ArgIndex {
				return &symgo.Error{Message: fmt.Sprintf("not enough arguments for pattern %q (wants %d, got %d)", pattern.Key, pattern.ArgIndex+1, len(args))}
			}
			arg := args[pattern.ArgIndex]

			switch pattern.Type {
			case "requestBody":
				AnalyzeRequestBody(op, arg)
			case "responseBody":
				AnalyzeResponseBody(op, arg, pattern.ContentType)
			case "responseHeader":
				AnalyzeResponseHeader(op, arg)
			case "queryParameter":
				AnalyzeQueryParameter(op, arg)
			case "placeholder":
			default:
				a.logger.Warn("unknown pattern type", "type", pattern.Type, "key", pattern.Key)
			}
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("result of %s", pattern.Key)}
		}
	}

	return intrinsics
}

// -----------------------------------------------------------------------------
// Analysis Pattern Helpers
// -----------------------------------------------------------------------------

func AnalyzeRequestBody(op *openapi.Operation, arg symgo.Object) {
	ptr, ok := arg.(*symgo.Pointer)
	if !ok {
		return
	}
	typeInfo := ptr.TypeInfo()
	if typeInfo != nil {
		schema := BuildSchemaForType(context.Background(), typeInfo, make(map[string]*openapi.Schema))
		if schema != nil {
			op.RequestBody = &openapi.RequestBody{
				Content:  map[string]openapi.MediaType{"application/json": {Schema: schema}},
				Required: true,
			}
		}
	}
}

func AnalyzeResponseBody(op *openapi.Operation, arg symgo.Object, contentType string) {
	if contentType == "" {
		contentType = "application/json"
	}

	var schema *openapi.Schema
	if contentType == "text/plain" {
		schema = &openapi.Schema{Type: "string"}
	} else {
		typeInfo := arg.TypeInfo()
		if typeInfo == nil {
			return
		}
		schema = BuildSchemaForType(context.Background(), typeInfo, make(map[string]*openapi.Schema))
	}

	if schema != nil {
		if op.Responses == nil {
			op.Responses = make(map[string]*openapi.Response)
		}
		statusCode := "200"
		if len(op.Responses) > 0 {
			for code := range op.Responses {
				statusCode = code
				break
			}
		}
		if _, exists := op.Responses[statusCode]; !exists {
			op.Responses[statusCode] = &openapi.Response{Description: "OK"}
		}
		if op.Responses[statusCode].Content == nil {
			op.Responses[statusCode].Content = make(map[string]openapi.MediaType)
		}
		op.Responses[statusCode].Content[contentType] = openapi.MediaType{Schema: schema}
	}
}

func AnalyzeResponseHeader(op *openapi.Operation, arg symgo.Object) {
	statusCode := "200"
	if op.Responses == nil {
		op.Responses = make(map[string]*openapi.Response)
	}
	if _, exists := op.Responses[statusCode]; !exists {
		op.Responses[statusCode] = &openapi.Response{Description: "OK"}
	}
}

func AnalyzeQueryParameter(op *openapi.Operation, arg symgo.Object) {
	paramNameObj, ok := arg.(*symgo.String)
	if !ok {
		return
	}
	paramName := paramNameObj.Value
	op.Parameters = append(op.Parameters, &openapi.Parameter{
		Name:   paramName,
		In:     "query",
		Schema: &openapi.Schema{Type: "string"},
	})
}

func HandleHeader(interp *symgo.Interpreter, a *Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "net/http.Header")
}

func HandleURLQuery(interp *symgo.Interpreter, a *Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "net/url.Values")
}

func HandleNewDecoder(interp *symgo.Interpreter, a *Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Decoder")
}

func HandleNewEncoder(interp *symgo.Interpreter, a *Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Encoder")
}

func NewSymbolicInstance(interp *symgo.Interpreter, fqtn string) symgo.Object {
	lastDot := strings.LastIndex(fqtn, ".")
	if lastDot == -1 {
		return &symgo.Error{Message: fmt.Sprintf("invalid fully-qualified type name: %s", fqtn)}
	}
	pkgPath := fqtn[:lastDot]
	typeName := fqtn[lastDot+1:]

	pkg, err := interp.Scanner().ScanPackageByImport(context.Background(), pkgPath)
	if err != nil {
		return &symgo.Error{Message: fmt.Sprintf("could not load package %s: %v", pkgPath, err)}
	}

	var resolvedType *scanner.TypeInfo
	for _, t := range pkg.Types {
		if t.Name == typeName {
			resolvedType = t
			break
		}
	}
	if resolvedType == nil {
		return &symgo.Error{Message: fmt.Sprintf("could not find type %s in package %s", typeName, pkgPath)}
	}

	return &symgo.Instance{
		TypeName:   fqtn,
		BaseObject: symgo.BaseObject{ResolvedTypeInfo: resolvedType},
	}
}

// -----------------------------------------------------------------------------
// Schema Building Logic
// -----------------------------------------------------------------------------

func BuildSchemaForType(ctx context.Context, typeInfo *scanner.TypeInfo, cache map[string]*openapi.Schema) *openapi.Schema {
	if typeInfo == nil {
		return &openapi.Schema{Type: "object", Description: "unknown type"}
	}
	if typeInfo.Underlying != nil {
		return buildSchemaFromFieldType(ctx, typeInfo.Underlying, cache)
	}
	canonicalName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if cached, ok := cache[canonicalName]; ok {
		return cached
	}
	if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
		return &openapi.Schema{Type: "object", Description: "unsupported type kind"}
	}

	schema := &openapi.Schema{
		Type:       "object",
		Properties: make(map[string]*openapi.Schema),
	}
	cache[canonicalName] = schema

	for _, field := range typeInfo.Struct.Fields {
		if !field.IsExported {
			continue
		}
		jsonName := field.TagValue("json")
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = field.Name
		}
		schema.Properties[jsonName] = buildSchemaFromFieldType(ctx, field.Type, cache)
	}
	return schema
}

func buildSchemaFromFieldType(ctx context.Context, ft *scanner.FieldType, cache map[string]*openapi.Schema) *openapi.Schema {
	if ft == nil {
		return nil
	}
	if ft.IsSlice {
		return &openapi.Schema{Type: "array", Items: buildSchemaFromFieldType(ctx, ft.Elem, cache)}
	}
	if ft.IsPointer {
		return buildSchemaFromFieldType(ctx, ft.Elem, cache)
	}
	if ft.IsBuiltin {
		return buildSchemaFromBasic(ft.Name)
	}
	typeInfo, err := ft.Resolve(ctx)
	if err != nil {
		fmt.Printf("warn: could not resolve type %q: %v\n", ft.Name, err)
		return &openapi.Schema{Type: "object", Description: "unresolved type"}
	}
	return BuildSchemaForType(ctx, typeInfo, cache)
}

func buildSchemaFromBasic(typeName string) *openapi.Schema {
	switch typeName {
	case "string":
		return &openapi.Schema{Type: "string"}
	case "int", "int8", "int16", "int32":
		return &openapi.Schema{Type: "integer", Format: "int32"}
	case "int64":
		return &openapi.Schema{Type: "integer", Format: "int64"}
	case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return &openapi.Schema{Type: "integer", Format: "int64"}
	case "bool":
		return &openapi.Schema{Type: "boolean"}
	case "float32":
		return &openapi.Schema{Type: "number", Format: "float"}
	case "float64":
		return &openapi.Schema{Type: "number", Format: "double"}
	default:
		return &openapi.Schema{Type: "string", Description: fmt.Sprintf("unsupported basic type: %s", typeName)}
	}
}
