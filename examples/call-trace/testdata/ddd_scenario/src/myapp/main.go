package main

import (
	"github.com/podhmo/go-scan/examples/call-trace/testdata/ddd_scenario/src/intermediatelib"
	"github.com/podhmo/go-scan/examples/call-trace/testdata/ddd_scenario/src/mylib"
)

func main() {
	thing := mylib.NewThing()
	intermediatelib.Use(thing)
}
