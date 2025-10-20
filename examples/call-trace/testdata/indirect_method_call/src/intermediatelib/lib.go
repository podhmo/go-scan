package intermediatelib

import "github.com/podhmo/go-scan/examples/call-trace/testdata/indirect_method_call/src/mylib"

type Caller struct{}

func (c *Caller) DoCall() {
	mylib.TargetFunc()
}
