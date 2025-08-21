//go:build ignore

package main

// Patterns defines a list of analysis patterns for docgen.
// The minigo interpreter evaluates this file and the docgen tool
// uses the `Patterns` variable to extend its analysis capabilities.
var Patterns = []any{
	map[string]any{
		"key":         "(*encoding/json.Encoder).Encode",
		"type":        "responseBody",
		"argIndex":    1,
		"contentType": "application/json",
	},
	map[string]any{
		"key":         "(*net/http/httptest.ResponseRecorder).Write",
		"type":        "responseBody",
		"argIndex":    1,
		"contentType": "text/plain",
	},
	map[string]any{
		"key":      "(*encoding/json.Decoder).Decode",
		"type":     "requestBody",
		"argIndex": 1,
	},
	map[string]any{
		"key":      "(net/url.Values).Get",
		"type":     "queryParameter",
		"argIndex": 1,
	},
	map[string]any{
		"key":      "(*net/http/httptest.ResponseRecorder).WriteHeader",
		"type":     "responseHeader",
		"argIndex": 1,
	},
}
