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

// Pattern defines a mapping between a function call signature (the key)
// and a handler function that performs analysis when that call is found.
type Pattern struct {
	Key   string
	Apply func(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object
}

// GetDefaultPatterns returns a slice of all the default call patterns
// used for analyzing standard net/http handlers.
func GetDefaultPatterns() []Pattern {
	return []Pattern{
		// net/http related
		{Key: "(net/http.ResponseWriter).Header", Apply: handleHeader},
		{Key: "(net/http.ResponseWriter).WriteHeader", Apply: handleWriteHeader},
		{Key: "(net/http.Header).Set", Apply: handleHeaderSet},

		// net/url related
		{Key: "(*net/url.URL).Query", Apply: handleURLQuery},
		{Key: "(net/url.Values).Get", Apply: handleValuesGet},

		// encoding/json related
		{Key: "encoding/json.NewDecoder", Apply: handleNewDecoder},
		{Key: "(*encoding/json.Decoder).Decode", Apply: handleDecode},
		{Key: "encoding/json.NewEncoder", Apply: handleNewEncoder},
		{Key: "(*encoding/json.Encoder).Encode", Apply: handleEncode},

		// strconv related
		{Key: "strconv.Atoi", Apply: handleAtoi},
	}
}

func handleAtoi(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	// We don't need the actual integer value, just to know that it's an integer.
	// Returning a placeholder allows analysis to continue. We could potentially
	// return a typed placeholder for an integer if more advanced analysis was needed.
	return &symgo.SymbolicPlaceholder{Reason: "result of strconv.Atoi"}
}

// -----------------------------------------------------------------------------
// Pattern Handler Implementations
// -----------------------------------------------------------------------------

func handleHeader(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	return NewSymbolicInstance(interp, "net/http.Header")
}

func handleWriteHeader(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	return nil // We don't need to do anything with the status code for now.
}

func handleHeaderSet(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	return nil // We don't need to track header values for now.
}

func handleURLQuery(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	return NewSymbolicInstance(interp, "net/url.Values")
}

func handleValuesGet(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
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

func handleNewDecoder(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Decoder")
}

func handleDecode(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	if len(args) != 2 {
		return &symgo.SymbolicPlaceholder{Reason: "decode error: wrong arg count"}
	}
	ptr, ok := args[1].(*symgo.Pointer)
	if !ok {
		return &symgo.SymbolicPlaceholder{Reason: "decode error: second arg is not a pointer"}
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
	return &symgo.SymbolicPlaceholder{Reason: "result of json.Decode"}
}

func handleNewEncoder(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	return NewSymbolicInstance(interp, "encoding/json.Encoder")
}

func handleEncode(interp *symgo.Interpreter, args []symgo.Object, op *openapi.Operation) symgo.Object {
	if len(args) != 2 {
		return &symgo.SymbolicPlaceholder{Reason: "encode error: wrong arg count"}
	}
	arg := args[1]
	var schema *openapi.Schema

	if slice, ok := arg.(*symgo.Slice); ok {
		schema = buildSchemaFromFieldType(context.Background(), slice.FieldType, make(map[string]*openapi.Schema))
	} else {
		typeInfo := arg.TypeInfo()
		if typeInfo != nil {
			schema = BuildSchemaForType(context.Background(), typeInfo, make(map[string]*openapi.Schema))
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
