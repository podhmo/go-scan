package intermediatelib

import "github.com/podhmo/go-scan/examples/call-trace/testdata/ddd_scenario/src/mylib"

// Use takes a Thing and calls its Do method.
func Use(t mylib.Thing) {
	t.Do()
}
