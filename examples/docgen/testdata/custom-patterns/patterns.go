//go:build minigo

package main

import "github.com/podhmo/go-scan/examples/docgen/patterns"

// Patterns defines the custom patterns for this test case.
// It now returns a slice of `patterns.PatternConfig` structs directly,
// which is possible due to the fix in go-scan's module resolution.
var Patterns = []patterns.PatternConfig{
	{
		Key:      "custom-patterns.SendJSON",
		Type:     patterns.ResponseBody,
		ArgIndex: 2, // The 3rd argument `data any` is what we want to analyze.
	},
	{
		Key:         "main.GetPetID",
		Type:        patterns.PathParameter,
		Name:        "petID",
		Description: "ID of the pet to fetch",
		ArgIndex:    0, // The *http.Request argument
	},
}
