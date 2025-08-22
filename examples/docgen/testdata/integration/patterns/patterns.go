// Package patterns defines the extensible call patterns for the docgen tool.
package patterns

import (
	"github.com/podhmo/go-scan/examples/docgen/openapi"
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
	RequestBody     PatternType = "requestBody"
	ResponseBody    PatternType = "responseBody"
	CustomResponse  PatternType = "customResponse"
	DefaultResponse PatternType = "defaultResponse"
	PathParameter   PatternType = "path"
	QueryParameter  PatternType = "query"
	HeaderParameter PatternType = "header"
)

// PatternConfig defines a user-configurable pattern for docgen analysis.
type PatternConfig struct {
	Key          string
	Type         PatternType
	ArgIndex     int
	StatusCode   string
	Description  string
	NameArgIndex int
	// A dummy field to make this struct different from the one in the main package
	// This helps verify that the correct type is being resolved.
	IsTestPattern bool
}
