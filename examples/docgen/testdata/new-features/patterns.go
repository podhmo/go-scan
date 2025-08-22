//go:build minigo
// +build minigo

package main

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"new-features/helpers"
)

var Patterns = []patterns.PatternConfig{
	{
		Fn:       helpers.RenderError,
		Type:     patterns.DefaultResponse,
		ArgIndex: 3,
	},
	{
		Fn:         helpers.RenderCustomError,
		Type:       patterns.CustomResponse,
		StatusCode: "400",
		ArgIndex:   2,
	},
	{
		Fn:       helpers.RenderJSON,
		Type:     patterns.ResponseBody,
		ArgIndex: 2,
	},
}
