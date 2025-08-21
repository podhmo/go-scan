// Package openapi provides a minimal set of structs to represent an OpenAPI 3.1.0 specification.
package openapi

// OpenAPI is the root document object of the OpenAPI specification.
type OpenAPI struct {
	OpenAPI    string               `json:"openapi" yaml:"openapi"`
	Info       Info                 `json:"info" yaml:"info"`
	Paths      map[string]*PathItem `json:"paths,omitempty" yaml:"paths,omitempty"`
	Components *Components          `json:"components,omitempty" yaml:"components,omitempty"`
}

// Components holds a set of reusable objects for different aspects of the OAS.
type Components struct {
	Schemas map[string]*Schema `json:"schemas,omitempty" yaml:"schemas,omitempty"`
}

// Info provides metadata about the API.
type Info struct {
	Title   string `json:"title" yaml:"title"`
	Version string `json:"version" yaml:"version"`
}

// PathItem describes the operations available on a single path.
type PathItem struct {
	Get     *Operation `json:"get,omitempty" yaml:"get,omitempty"`
	Post    *Operation `json:"post,omitempty" yaml:"post,omitempty"`
	Put     *Operation `json:"put,omitempty" yaml:"put,omitempty"`
	Delete  *Operation `json:"delete,omitempty" yaml:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty" yaml:"patch,omitempty"`
	Head    *Operation `json:"head,omitempty" yaml:"head,omitempty"`
	Options *Operation `json:"options,omitempty" yaml:"options,omitempty"`
	Trace   *Operation `json:"trace,omitempty" yaml:"trace,omitempty"`
}

// Operation describes a single API operation on a path.
type Operation struct {
	Summary     string               `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	OperationID string               `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Parameters  []*Parameter         `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses,omitempty" yaml:"responses,omitempty"`
}

// Parameter describes a single operation parameter.
type Parameter struct {
	Name        string  `json:"name" yaml:"name"`
	In          string  `json:"in" yaml:"in"` // "query", "header", "path", "cookie"
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool    `json:"required,omitempty" yaml:"required,omitempty"`
	Schema      *Schema `json:"schema" yaml:"schema"`
}

// RequestBody describes a single request body.
type RequestBody struct {
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
	Required    bool                 `json:"required,omitempty" yaml:"required,omitempty"`
}

// Response describes a single response from an API Operation.
type Response struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

// MediaType provides schema and examples for the media type.
type MediaType struct {
	Schema *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// Schema defines the schema for a type.
type Schema struct {
	Type                 string             `json:"type,omitempty" yaml:"type,omitempty"`
	Description          string             `json:"description,omitempty" yaml:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty" yaml:"additionalProperties,omitempty"`
	Format               string             `json:"format,omitempty" yaml:"format,omitempty"` // e.g., "int32", "int64"
	Ref                  string             `json:"$ref,omitempty" yaml:"$ref,omitempty"`
}
