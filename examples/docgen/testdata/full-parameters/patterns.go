//go:build minigo

package main

import "github.com/podhmo/go-scan/examples/docgen/patterns"

// Patterns defines the custom patterns for this test case.
var Patterns = []patterns.PatternConfig{
	{
		Key:         "full-parameters.GetQueryParam",
		Type:        patterns.QueryParameter,
		Name:        "filter", // This is hardcoded for this example
		Description: "A filter for the resource list.",
		ArgIndex:    1, // The 'key' argument
	},
	{
		Key:         "full-parameters.GetHeader",
		Type:        patterns.HeaderParameter,
		Name:        "X-Request-ID",
		Description: "A unique ID for the request.",
		ArgIndex:    1,
	},
	{
		Key:         "full-parameters.GetPathValue",
		Type:        patterns.PathParameter,
		Name:        "resourceId",
		Description: "The ID of the resource.",
		ArgIndex:    1,
	},
}
