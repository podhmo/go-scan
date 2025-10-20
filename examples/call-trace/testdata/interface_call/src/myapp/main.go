package main

import (
	"fmt"

	"github.com/podhmo/go-scan/examples/call-trace/testdata/interface_call/src/mylib"
)

func main() {
	g := mylib.NewGreeter()
	// This call should be traced back to mylib.(*greeter).Greet
	message := g.Greet()
	fmt.Println(message)
}
