package main

import (
	"github.com/podhmo/go-scan/examples/call-trace/testdata/out_of_policy/src/anotherlib"
	"github.com/podhmo/go-scan/examples/call-trace/testdata/out_of_policy/src/mylib"
)

func main() {
	mylib.InScope()
	anotherlib.OutOfScope()
}
