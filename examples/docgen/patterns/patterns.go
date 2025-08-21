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
}

// -----------------------------------------------------------------------------
// Generic Analysis Functions (for declarative patterns)
// -----------------------------------------------------------------------------

// AnalyzeRequestBody analyzes the argument as a request body.
func AnalyzeRequestBody(op *openapi.Operation, arg symgo.Object) {
	ptr, ok := arg.(*symgo.Pointer)
	if !ok {
		return // Not a pointer, cannot decode into it.
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

// AnalyzeResponseBody analyzes the argument as a response body.
func AnalyzeResponseBody(op *openapi.Operation, arg symgo.Object, contentType string) {
	if contentType == "" {
		contentType = "application/json" // Default to JSON
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

// AnalyzeResponseHeader analyzes the argument as a response header (status code).
func AnalyzeResponseHeader(op *openapi.Operation, arg symgo.Object) {
	// For now, we don't inspect the value of the status code.
	// A more advanced implementation would resolve the constant value.
	statusCode := "200" // Hardcoded for simplicity
	if op.Responses == nil {
		op.Responses = make(map[string]*openapi.Response)
	}
	if _, exists := op.Responses[statusCode]; !exists {
		op.Responses[statusCode] = &openapi.Response{Description: "OK"}
	}
}

// AnalyzeQueryParameter analyzes the argument as a query parameter name.
func AnalyzeQueryParameter(op *openapi.Operation, arg symgo.Object) {
	paramNameObj, ok := arg.(*symgo.String)
	if !ok {
		return // Parameter name is not a string literal
	}
	paramName := paramNameObj.Value
	op.Parameters = append(op.Parameters, &openapi.Parameter{
		Name: paramName,
		In:   "query",
		Schema: &openapi.Schema{
			Type: "string", // Default to string, could be enhanced later.
		},
	})
}

// -----------------------------------------------------------------------------
// Legacy Pattern Handler Implementations (for intrinsics that return values)
// -----------------------------------------------------------------------------

func HandleHeader(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "net/http.Header")
}

func HandleURLQuery(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "net/url.Values")
}

func HandleNewDecoder(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Decoder")
}

func HandleNewEncoder(interp *symgo.Interpreter, a Analyzer, args []symgo.Object) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Encoder")
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
// Schema Building Logic
// -----------------------------------------------------------------------------

// BuildSchemaForType generates an OpenAPI schema for a given Go type.
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
			continue // Skip unexported fields
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
