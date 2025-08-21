//go:build minigo

package main

import "github.com/podhmo/go-scan/examples/docgen/patterns"

// Patterns defines the custom patterns for this test case.
// This version uses dynamic name inference.
var Patterns = []patterns.PatternConfig{
	{
		Key:          "full-parameters.GetQueryParam",
		Type:         patterns.QueryParameter,
		NameArgIndex: 1, // The 'key' argument
		ArgIndex:     0, // Dummy value, schema will default to string
		Description:  "A filter for the resource list.",
	},
	{
		Key:          "full-parameters.GetHeader",
		Type:         patterns.HeaderParameter,
		NameArgIndex: 1,
		ArgIndex:     0,
		Description:  "A unique ID for the request.",
	},
	{
		Key:          "full-parameters.GetPathValue",
		Type:         patterns.PathParameter,
		NameArgIndex: 1,
		ArgIndex:     0,
		Description:  "The ID of the resource.",
	},
}
