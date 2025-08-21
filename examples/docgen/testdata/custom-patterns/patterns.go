//go:build minigo

package main

// Define the pattern type constants directly in the minigo script
// to avoid the complex cross-module import issues during testing.
var (
	RequestBody  = "requestBody"
	ResponseBody = "responseBody"
)

// Patterns defines the custom patterns for this test case.
// It returns a slice of maps, which is a format minigo can handle robustly.
var Patterns = []map[string]any{
	{
		"Key": "custom-patterns.SendJSON",
		// Use the locally defined constant.
		"Type":     ResponseBody,
		"ArgIndex": 2, // The 3rd argument `data any` is what we want to analyze.
	},
}
