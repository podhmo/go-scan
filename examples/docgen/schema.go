package main

import (
	"context"
	"fmt"

	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/scanner"
)

// buildSchemaForType generates an OpenAPI schema for a given Go type.
// It uses a map to cache results and avoid infinite recursion.
func buildSchemaForType(ctx context.Context, typeInfo *scanner.TypeInfo, cache map[string]*openapi.Schema) *openapi.Schema {
	if typeInfo == nil {
		return &openapi.Schema{Type: "object", Description: "unknown type"}
	}

	// Use a canonical name for caching to handle pointers vs. values.
	canonicalName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if cached, ok := cache[canonicalName]; ok {
		// For recursive types, return a reference to the already-defined schema.
		// We don't have a good way to name them right now, so this is a simplification.
		return cached
	}

	// We only handle structs for now.
	if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
		return &openapi.Schema{Type: "object", Description: "unsupported type kind"}
	}

	// Create a placeholder in the cache *before* recursing to handle cycles.
	schema := &openapi.Schema{
		Type:       "object",
		Properties: make(map[string]*openapi.Schema),
	}
	cache[canonicalName] = schema

	// Iterate over struct fields to build properties.
	for _, field := range typeInfo.Struct.Fields {
		if field.Name == "" || field.Name[0] < 'A' || field.Name[0] > 'Z' {
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

// buildSchemaFromFieldType is the recursive helper that works with scanner.FieldType.
func buildSchemaFromFieldType(ctx context.Context, ft *scanner.FieldType, cache map[string]*openapi.Schema) *openapi.Schema {
	if ft == nil {
		return nil
	}

	if ft.IsSlice {
		return &openapi.Schema{
			Type:  "array",
			Items: buildSchemaFromFieldType(ctx, ft.Elem, cache),
		}
	}

	if ft.IsPointer {
		return buildSchemaFromFieldType(ctx, ft.Elem, cache)
	}

	if ft.IsBuiltin {
		return buildSchemaFromBasic(ft.Name)
	}

	// For named types (structs), resolve the full type definition.
	typeInfo, err := ft.Resolve(ctx)
	if err != nil {
		fmt.Printf("warn: could not resolve type %q: %v\n", ft.Name, err)
		return &openapi.Schema{Type: "object", Description: "unresolved type"}
	}

	return buildSchemaForType(ctx, typeInfo, cache)
}

// buildSchemaFromBasic converts a basic Go type string to an OpenAPI schema.
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
