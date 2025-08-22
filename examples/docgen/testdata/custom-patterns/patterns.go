//go:build minigo

package main

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
)

var Patterns = []patterns.PatternConfig{
	{
		Fn:       SendJSON,
		Type:     patterns.ResponseBody,
		ArgIndex: 2,
	},
	{
		Fn:          GetPetID,
		Type:        patterns.PathParameter,
		Name:        "petID",
		Description: "ID of the pet to fetch",
		ArgIndex:    0,
	},
}
