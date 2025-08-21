//go:build ignore

package main

// Patterns defines the custom patterns for this test case.
// It returns a slice of maps, which is a format minigo can handle robustly.
var Patterns = []map[string]any{
	{
		"Key":      "custom-patterns.SendJSON",
		"Type":     "responseBody",
		"ArgIndex": 2, // The 3rd argument `data any` is what we want to analyze.
	},
}
