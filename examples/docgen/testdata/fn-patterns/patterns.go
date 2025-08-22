//go:build minigo

package main

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api"
)

// The user must define a typed nil variable for the struct whose methods they want to reference.
var (
	_api *api.API
)

// Patterns defines the custom patterns for this test case using the new `Fn` field.
var Patterns = []patterns.PatternConfig{
	// Pattern for a standalone function
	{
		Fn:       api.SendJSON,
		Type:     patterns.ResponseBody,
		ArgIndex: 2, // The 3rd argument `data any`
	},

	// Pattern for a method, referenced via a typed nil pointer.
	{
		Fn:          (_api).GetUser,
		Type:        patterns.PathParameter,
		ArgIndex:    1, // The *http.Request argument
		Name:        "id",
		Description: "ID of user to get",
	},
}
