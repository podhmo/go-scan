package main

import (
	"my-test-module/helpers"

	"github.com/podhmo/go-scan/examples/docgen/patterns"
)

// Patterns defines the custom patterns for the docgen tool.
var Patterns = []patterns.PatternConfig{
	{
		Name: "CustomJSONResponse",
		Description: "Handles custom JSON responses.",
		Fn: helpers.RespondJSON,
		Type: patterns.ResponseBody,
		ArgIndex: 1,
	},
}
