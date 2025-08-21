//go:build ignore
// +build ignore

package main

import "github.com/podhmo/go-scan/examples/docgen/patterns"

// Patterns defines a list of custom patterns for the docgen tool.
var Patterns = []patterns.PatternConfig{
	{
		// Key is the fully qualified function name: <module-path>/<package-name>.<function-name>
		Key: "new-features/helpers.RenderError",
		// Type tells the analyzer to treat this as a default response.
		Type: patterns.DefaultResponse,
		// StatusCode is the HTTP status code to use for this response.
		StatusCode: "404",
		// ArgIndex points to the argument containing the response body's type.
		// The analyzer will see the `error` type and create a generic schema for it.
		ArgIndex: 3,
	},
	{
		Key:      "new-features/helpers.RenderJSON",
		Type:     patterns.ResponseBody,
		ArgIndex: 2, // The `v any` argument
	},
}
