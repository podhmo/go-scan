package main

import (
	"github.com/podhmo/go-scan/examples/call-trace/testdata/method_call/src/mylib"
)

func main() {
	g := &mylib.Greeter{Name: "World"}
	g.Greet()
}
