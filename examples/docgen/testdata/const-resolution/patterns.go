package main

import "github.com/podhmo/go-scan/examples/docgen/patterns"

// Patterns defines the custom analysis patterns for this test.
var Patterns = []patterns.PatternConfig{
	{
		Name: "get-query-from-const",
		Type: patterns.QueryParameter,
		Key:  "example.com/const-resolution/query.Get",

		// Tell the pattern handler that the NAME of the query parameter
		// can be found at argument index 1.
		NameArgIndex: 1,

		// We don't need to specify ArgIndex (for the value) because
		// our dummy Get() function doesn't have a value argument.
		// The schema will default to string, which is fine for this test.
	},
}
