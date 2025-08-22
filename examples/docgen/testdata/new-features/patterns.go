//go:build minigo
// +build minigo

package main

import (
	"new-features/helpers"

	"github.com/podhmo/go-scan/examples/docgen/patterns"
)

// Patterns defines a list of custom patterns for the docgen tool.
var Patterns = []patterns.PatternConfig{
	{
		Fn:       helpers.RenderError,
		Type:     patterns.DefaultResponse,
		ArgIndex: 3, // err error
	},
	{
		Fn:         helpers.RenderCustomError,
		Type:       patterns.CustomResponse,
		StatusCode: "400",
		ArgIndex:   2, // err helpers.ErrorResponse
	},
	{
		Fn:       helpers.RenderJSON,
		Type:     patterns.ResponseBody,
		ArgIndex: 2, // The `v any` argument
	},
}
