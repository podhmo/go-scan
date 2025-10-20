package main

import "github.com/podhmo/go-scan/examples/call-trace/testdata/indirect_method_call/src/intermediatelib"

func main() {
	caller := &intermediatelib.Caller{}
	caller.DoCall()
}
