// Package openapi provides a minimal set of structs to represent an OpenAPI 3.1.0 specification.
package openapi

// OpenAPI is the root document object of the OpenAPI specification.
type OpenAPI struct {
	OpenAPI string               `json:"openapi"`
	Info    Info                 `json:"info"`
	Paths   map[string]*PathItem `json:"paths,omitempty"`
}

// Info provides metadata about the API.
type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

// PathItem describes the operations available on a single path.
type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Head    *Operation `json:"head,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Trace   *Operation `json:"trace,omitempty"`
}

// Operation describes a single API operation on a path.
type Operation struct {
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}
