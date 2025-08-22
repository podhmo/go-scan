package patterns

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
)

var Patterns = []patterns.PatternConfig{
	{
		Key:      "github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api.GetFoo",
		Type:     patterns.RequestBody,
		ArgIndex: 1,
	},
}
