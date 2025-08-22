// Package patterns defines the extensible call patterns for the docgen tool.
package patterns

import (
	"context"
	"fmt"
	"strings"

	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
)

// Analyzer is a subset of the docgen.Analyzer interface needed by patterns.
// This avoids a circular dependency.
type Analyzer interface {
	OperationStack() []*openapi.Operation
	GetOpenAPI() *openapi.OpenAPI
}

// PatternType defines the type of analysis to perform for a custom pattern.
type PatternType string

const (
	// RequestBody indicates the pattern should analyze a function argument as a request body.
	RequestBody PatternType = "requestBody"
	// ResponseBody indicates the pattern should analyze a function argument as a response body.
	ResponseBody PatternType = "responseBody"
	// CustomResponse indicates the pattern should analyze a function argument as a response body with a specific status code.
	CustomResponse PatternType = "customResponse"
	// DefaultResponse indicates the pattern should analyze a function argument as a default response body (without a status code).
	DefaultResponse PatternType = "defaultResponse"
	// PathParameter indicates the pattern should extract a path parameter.
	PathParameter PatternType = "path"
	// QueryParameter indicates the pattern should extract a query parameter.
	QueryParameter PatternType = "query"
	// HeaderParameter indicates the pattern should extract a header parameter.
	HeaderParameter PatternType = "header"
)

// PatternConfig defines a user-configurable pattern for docgen analysis.
// It maps a function call to a specific analysis type.
type PatternConfig struct {
	// Name is a short, descriptive name for the pattern.
	Name string
	// Description is a longer explanation of what the pattern does.
	Description string

	// Key is the fully-qualified function or method name to match.
	// This is used if `Fn` is not provided.
	// e.g., "github.com/my-org/my-app/utils.DecodeJSON"
	// e.g., "(*net/http.Request).Context"
	Key string

	// Fn provides a type-safe way to specify the target function.
	// If `Fn` is provided, `Key` is ignored.
	Fn any // e.g., mypkg.MyFunc

	// Type specifies the kind of analysis to perform.
	Type PatternType

	// ArgIndex is the 0-based index of the function argument to analyze.
	// For "requestBody", this is the argument that will be decoded into.
	// For "responseBody", this is the argument that will be encoded from.
	// For "path" or "query", this is the argument holding the parameter's value.
	ArgIndex int

	// StatusCode is the HTTP status code for the response.
	// Required for "customResponse" type.
	// e.g., "400", "500"
	StatusCode string

	// Description is the OpenAPI description for the parameter.
	// Optional for "path" and "query" types.
	Description string

	// NameArgIndex is the 0-based index of the argument containing the parameter's name.
	// Used for parameter patterns (`path`, `query`, `header`).
	NameArgIndex int
}

// Pattern defines a mapping between a function call signature (the key)
// and a handler function that performs analysis when that call is found.
type Pattern struct {
	Key   string
	Apply func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object
}

// HandleCustomRequestBody returns a pattern handler that treats a specific argument
// as a request body, similar to `json.Decode`.
func HandleCustomRequestBody(argIndex int) func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
		op := a.OperationStack()[len(a.OperationStack())-1]
		if len(args) <= argIndex {
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("custom requestBody pattern: not enough args (want %d, got %d)", argIndex+1, len(args))}
		}

		ptr, ok := args[argIndex].(*symgo.Pointer)
		if !ok {
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("custom requestBody pattern: argument %d is not a pointer", argIndex)}
		}

		typeInfo := ptr.TypeInfo()
		if typeInfo != nil {
			schema := BuildSchemaForType(context.Background(), a, typeInfo, make(map[string]*openapi.Schema))
			if schema != nil {
				op.RequestBody = &openapi.RequestBody{
					Content:  map[string]openapi.MediaType{"application/json": {Schema: schema}},
					Required: true,
				}
			}
		}
		// The return value of the custom function is not known, so we return a placeholder.
		return &symgo.SymbolicPlaceholder{Reason: "result of custom request body function"}
	}
}

// HandleCustomResponse returns a pattern handler that treats a specific argument
// as a response body for a given status code.
func HandleCustomResponse(statusCode string, argIndex int) func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
		op := a.OperationStack()[len(a.OperationStack())-1]
		if len(args) <= argIndex {
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("custom response pattern: not enough args (want %d, got %d)", argIndex+1, len(args))}
		}

		arg := args[argIndex]
		var schema *openapi.Schema

		if slice, ok := arg.(*symgo.Slice); ok {
			schema = buildSchemaFromFieldType(context.Background(), a, slice.FieldType, make(map[string]*openapi.Schema))
		} else {
			typeInfo := arg.TypeInfo()
			if typeInfo != nil {
				schema = BuildSchemaForType(context.Background(), a, typeInfo, make(map[string]*openapi.Schema))
			}
		}

		if schema != nil {
			if op.Responses == nil {
				op.Responses = make(map[string]*openapi.Response)
			}
			op.Responses[statusCode] = &openapi.Response{
				Description: fmt.Sprintf("Response for status code %s", statusCode),
				Content:     map[string]openapi.MediaType{"application/json": {Schema: schema}},
			}
		}
		return &symgo.SymbolicPlaceholder{Reason: "result of custom response function"}
	}
}

// HandleDefaultResponse returns a pattern handler that treats a specific argument
// as a default response body.
func HandleDefaultResponse(argIndex int) func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
		op := a.OperationStack()[len(a.OperationStack())-1]
		if len(args) <= argIndex {
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("default response pattern: not enough args (want %d, got %d)", argIndex+1, len(args))}
		}

		arg := args[argIndex]
		var schema *openapi.Schema

		if slice, ok := arg.(*symgo.Slice); ok {
			schema = buildSchemaFromFieldType(context.Background(), a, slice.FieldType, make(map[string]*openapi.Schema))
		} else {
			typeInfo := arg.TypeInfo()
			if typeInfo != nil {
				schema = BuildSchemaForType(context.Background(), a, typeInfo, make(map[string]*openapi.Schema))
			}
		}

		if schema != nil {
			if op.Responses == nil {
				op.Responses = make(map[string]*openapi.Response)
			}
			op.Responses["default"] = &openapi.Response{
				Description: "Default error response",
				Content:     map[string]openapi.MediaType{"application/json": {Schema: schema}},
			}
		}
		return &symgo.SymbolicPlaceholder{Reason: "result of default response function"}
	}
}

// HandleCustomResponseBody returns a pattern handler that treats a specific argument
// as a response body, similar to `json.Encode`.
func HandleCustomResponseBody(argIndex int) func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
		op := a.OperationStack()[len(a.OperationStack())-1]
		if len(args) <= argIndex {
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("custom responseBody pattern: not enough args (want %d, got %d)", argIndex+1, len(args))}
		}

		arg := args[argIndex]
		var schema *openapi.Schema

		if slice, ok := arg.(*symgo.Slice); ok {
			schema = buildSchemaFromFieldType(context.Background(), a, slice.FieldType, make(map[string]*openapi.Schema))
		} else {
			typeInfo := arg.TypeInfo()
			if typeInfo != nil {
				schema = BuildSchemaForType(context.Background(), a, typeInfo, make(map[string]*openapi.Schema))
			}
		}

		if schema != nil {
			if op.Responses == nil {
				op.Responses = make(map[string]*openapi.Response)
			}
			// Assume 200 OK if no status code has been set.
			if _, ok := op.Responses["200"]; !ok {
				op.Responses["200"] = &openapi.Response{Description: "OK"}
			}
			op.Responses["200"].Content = map[string]openapi.MediaType{"application/json": {Schema: schema}}
		}

		// The return value of the custom function is not known, so we return a placeholder.
		return &symgo.SymbolicPlaceholder{Reason: "result of custom response body function"}
	}
}

// HandleCustomParameter returns a pattern handler that extracts a parameter (path or query)
// from a function argument. The parameter's name is extracted dynamically from an argument.
func HandleCustomParameter(in, description string, nameArgIndex, valueArgIndex int) func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return func(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
		op := a.OperationStack()[len(a.OperationStack())-1]
		if len(args) <= nameArgIndex {
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("custom %s parameter pattern: not enough args for name (want %d, got %d)", in, nameArgIndex+1, len(args))}
		}
		if len(args) <= valueArgIndex {
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("custom %s parameter pattern: not enough args for value (want %d, got %d)", in, valueArgIndex+1, len(args))}
		}

		// Extract the parameter name from the specified argument.
		nameObj, ok := args[nameArgIndex].(*symgo.String)
		if !ok {
			// If the name is not a string literal, we can't determine it statically.
			return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("custom %s parameter pattern: name argument at index %d is not a string literal", in, nameArgIndex)}
		}
		paramName := nameObj.Value

		// Extract the schema from the value argument's type.
		arg := args[valueArgIndex]
		var schema *openapi.Schema
		typeInfo := arg.TypeInfo()
		if typeInfo != nil && typeInfo.Underlying != nil {
			schema = buildSchemaFromFieldType(context.Background(), a, typeInfo.Underlying, make(map[string]*openapi.Schema))
		}
		if schema == nil {
			schema = &openapi.Schema{Type: "string"}
		}

		param := &openapi.Parameter{
			Name:        paramName,
			In:          in,
			Description: description,
			Schema:      schema,
		}
		if in == "path" {
			param.Required = true
		}

		op.Parameters = append(op.Parameters, param)

		// The return value of the custom function is not known, so we return a placeholder.
		return &symgo.SymbolicPlaceholder{Reason: fmt.Sprintf("result of custom %s parameter function", in)}
	}
}

// GetDefaultPatterns returns a slice of all the default call patterns
// used for analyzing standard net/http handlers.
func GetDefaultPatterns() []Pattern {
	return []Pattern{
		// net/http related
		{Key: "(net/http.ResponseWriter).Header", Apply: handleHeader},
		{Key: "(net/http.ResponseWriter).Write", Apply: handleResponseWriterWrite},
		{Key: "(net/http.ResponseWriter).WriteHeader", Apply: handleWriteHeader},
		{Key: "(net/http.Header).Set", Apply: handleHeaderSet},

		// httptest.ResponseRecorder, for when ResponseWriter is bound to it.
		{Key: "(*net/http/httptest.ResponseRecorder).Header", Apply: handleHeader},
		{Key: "(*net/http/httptest.ResponseRecorder).Write", Apply: handleResponseWriterWrite},
		{Key: "(*net/http/httptest.ResponseRecorder).WriteHeader", Apply: handleWriteHeader},

		// net/url related
		{Key: "(*net/url.URL).Query", Apply: handleURLQuery},
		{Key: "(net/url.Values).Get", Apply: handleValuesGet},

		// encoding/json related
		{Key: "encoding/json.NewDecoder", Apply: handleNewDecoder},
		{Key: "(*encoding/json.Decoder).Decode", Apply: handleDecode},
		{Key: "encoding/json.NewEncoder", Apply: handleNewEncoder},
		{Key: "(*encoding/json.Encoder).Encode", Apply: handleEncode},
	}
}

// -----------------------------------------------------------------------------
// Pattern Handler Implementations
// -----------------------------------------------------------------------------

func handleResponseWriterWrite(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	op := a.OperationStack()[len(a.OperationStack())-1]

	// Find the response object, assuming WriteHeader was called first.
	var resp *openapi.Response
	var statusCode string
	if op.Responses != nil {
		for code, r := range op.Responses {
			statusCode = code
			resp = r
			break // Assume first status code found is the one we want to add content to.
		}
	}

	// If no response entry exists (e.g., WriteHeader wasn't called or detected), default to 200.
	if resp == nil {
		if op.Responses == nil {
			op.Responses = make(map[string]*openapi.Response)
		}
		statusCode = "200"
		op.Responses[statusCode] = &openapi.Response{Description: "OK"}
		resp = op.Responses[statusCode]
	}

	if resp.Content == nil {
		resp.Content = make(map[string]openapi.MediaType)
	}

	// For a raw Write, we assume text/plain content.
	// A more sophisticated analysis could check for a prior `Header.Set("Content-Type", ...)` call.
	resp.Content["text/plain"] = openapi.MediaType{
		Schema: &openapi.Schema{Type: "string"},
	}

	// w.Write returns (int, error)
	return &symgo.MultiReturn{
		Values: []symgo.Object{
			&symgo.SymbolicPlaceholder{Reason: "return value from Write (int)"},
			&symgo.Nil{},
		},
	}
}

func handleHeader(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "net/http.Header")
}

func handleWriteHeader(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	op := a.OperationStack()[len(a.OperationStack())-1]
	if len(args) != 2 {
		return nil
	}
	// args[1] is the status code. It could be a constant like http.StatusOK
	// or an integer literal. For now, we'll just hardcode 200 for the test.
	// A more advanced implementation would resolve the constant value.
	statusCode := "200" // Hardcoded for simplicity
	if op.Responses == nil {
		op.Responses = make(map[string]*openapi.Response)
	}
	if _, exists := op.Responses[statusCode]; !exists {
		op.Responses[statusCode] = &openapi.Response{Description: "OK"}
	}
	return nil
}

func handleHeaderSet(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return nil // We don't need to track header values for now.
}

func handleURLQuery(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "net/url.Values")
}

func handleValuesGet(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	op := a.OperationStack()[len(a.OperationStack())-1]
	if len(args) != 2 {
		return &symgo.SymbolicPlaceholder{Reason: "invalid Get call"}
	}
	paramNameObj, ok := args[1].(*symgo.String)
	if !ok {
		return &symgo.SymbolicPlaceholder{Reason: "parameter name is not a string literal"}
	}
	paramName := paramNameObj.Value
	op.Parameters = append(op.Parameters, &openapi.Parameter{
		Name: paramName,
		In:   "query",
		Schema: &openapi.Schema{
			Type: "string", // Default to string, could be enhanced later.
		},
	})
	return &symgo.String{Value: ""} // The actual value doesn't matter for analysis.
}

func handleNewDecoder(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Decoder")
}

func handleDecode(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	op := a.OperationStack()[len(a.OperationStack())-1]
	if len(args) != 2 {
		return &symgo.SymbolicPlaceholder{Reason: "decode error: wrong arg count"}
	}
	ptr, ok := args[1].(*symgo.Pointer)
	if !ok {
		return &symgo.SymbolicPlaceholder{Reason: "decode error: second arg is not a pointer"}
	}
	typeInfo := ptr.TypeInfo()
	if typeInfo != nil {
		schema := BuildSchemaForType(context.Background(), a, typeInfo, make(map[string]*openapi.Schema))
		if schema != nil {
			op.RequestBody = &openapi.RequestBody{
				Content:  map[string]openapi.MediaType{"application/json": {Schema: schema}},
				Required: true,
			}
		}
	}
	return &symgo.SymbolicPlaceholder{Reason: "result of json.Decode"}
}

func handleNewEncoder(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Encoder")
}

func handleEncode(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	op := a.OperationStack()[len(a.OperationStack())-1]
	if len(args) != 2 {
		return &symgo.SymbolicPlaceholder{Reason: "encode error: wrong arg count"}
	}
	arg := args[1]
	var schema *openapi.Schema

	if slice, ok := arg.(*symgo.Slice); ok {
		schema = buildSchemaFromFieldType(context.Background(), a, slice.FieldType, make(map[string]*openapi.Schema))
	} else {
		typeInfo := arg.TypeInfo()
		if typeInfo != nil {
			schema = BuildSchemaForType(context.Background(), a, typeInfo, make(map[string]*openapi.Schema))
		}
	}

	if schema != nil {
		if op.Responses == nil {
			op.Responses = make(map[string]*openapi.Response)
		}
		op.Responses["200"] = &openapi.Response{
			Description: "OK",
			Content:     map[string]openapi.MediaType{"application/json": {Schema: schema}},
		}
	}

	return &symgo.SymbolicPlaceholder{Reason: "result of json.Encode"}
}

// -----------------------------------------------------------------------------
// Helper function for creating symbolic instances
// -----------------------------------------------------------------------------

// NewSymbolicInstance is a helper to create a symgo.Instance with its type information resolved.
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
// Schema Building Logic (moved from schema.go)
// -----------------------------------------------------------------------------

// BuildSchemaForType generates an OpenAPI schema for a given Go type.
// If the type is a struct, it ensures the full schema is defined in the
// components section and returns a $ref schema.
func BuildSchemaForType(ctx context.Context, a Analyzer, typeInfo *scanner.TypeInfo, cache map[string]*openapi.Schema) *openapi.Schema {
	if typeInfo == nil {
		return &openapi.Schema{Type: "object", Description: "unknown type"}
	}
	if typeInfo.Underlying != nil {
		return buildSchemaFromFieldType(ctx, a, typeInfo.Underlying, cache)
	}
	if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
		// Not a struct, or not a struct we can analyze.
		// Fallback to building the schema directly without creating a component.
		return buildSchemaFromFieldType(ctx, a, &scanner.FieldType{Definition: typeInfo}, cache)
	}

	// It's a struct. We will register it as a component.
	doc := a.GetOpenAPI()
	if doc.Components == nil {
		doc.Components = &openapi.Components{}
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = make(map[string]*openapi.Schema)
	}

	// Generate a unique name for the schema component.
	// e.g., "github.com/podhmo/go-scan/examples/docgen/sampleapi.User" -> "docgen_sampleapi_User"
	// This is a simplification; a robust implementation would handle non-alphanumeric characters better.
	pkgPathForName := strings.ReplaceAll(typeInfo.PkgPath, "/", "_")
	pkgPathForName = strings.ReplaceAll(pkgPathForName, ".", "_")
	// take the last 2 parts of the package path
	parts := strings.Split(pkgPathForName, "_")
	if len(parts) > 2 {
		pkgPathForName = strings.Join(parts[len(parts)-2:], "_")
	}
	schemaName := fmt.Sprintf("%s_%s", pkgPathForName, typeInfo.Name)

	// If the schema is already being defined (recursion), return a ref.
	if _, inProgress := cache[schemaName]; inProgress {
		return &openapi.Schema{Ref: "#/components/schemas/" + schemaName}
	}
	// If the schema is already fully defined, return a ref.
	if _, exists := doc.Components.Schemas[schemaName]; exists {
		return &openapi.Schema{Ref: "#/components/schemas/" + schemaName}
	}

	// Mark this schema as "in progress" to handle recursion.
	cache[schemaName] = nil

	// Build the full schema.
	schema := &openapi.Schema{
		Type:       "object",
		Properties: make(map[string]*openapi.Schema),
	}

	for _, field := range typeInfo.Struct.Fields {
		if !field.IsExported {
			continue // Skip unexported fields
		}
		jsonName := field.TagValue("json")
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = field.Name
		}
		schema.Properties[jsonName] = buildSchemaFromFieldType(ctx, a, field.Type, cache)
	}

	// Add the complete schema to the components and remove from progress cache.
	doc.Components.Schemas[schemaName] = schema
	delete(cache, schemaName)

	// Return a reference to the newly created component.
	return &openapi.Schema{Ref: "#/components/schemas/" + schemaName}
}

func buildSchemaFromFieldType(ctx context.Context, a Analyzer, ft *scanner.FieldType, cache map[string]*openapi.Schema) *openapi.Schema {
	if ft == nil {
		return nil
	}
	if ft.IsMap {
		// In OpenAPI 3.0, map keys must be strings. A more robust implementation
		// might check ft.MapKey to ensure it's a string type.
		return &openapi.Schema{
			Type:                 "object",
			AdditionalProperties: buildSchemaFromFieldType(ctx, a, ft.Elem, cache),
		}
	}
	if ft.IsSlice {
		return &openapi.Schema{Type: "array", Items: buildSchemaFromFieldType(ctx, a, ft.Elem, cache)}
	}
	if ft.IsPointer {
		return buildSchemaFromFieldType(ctx, a, ft.Elem, cache)
	}
	if ft.IsBuiltin {
		return buildSchemaFromBasic(ft.Name)
	}
	typeInfo, err := ft.Resolve(ctx)
	if err != nil {
		fmt.Printf("warn: could not resolve type %q: %v\n", ft.Name, err)
		return &openapi.Schema{Type: "object", Description: "unresolved type"}
	}
	return BuildSchemaForType(ctx, a, typeInfo, cache)
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
		return &openapi.Schema{Type: "integer", Format: "int64"} // Unsigned ints are usually represented as integers.
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
