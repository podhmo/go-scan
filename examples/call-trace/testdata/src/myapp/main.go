package main

import (
	"fmt"
	"github.com/podhmo/go-scan/examples/call-trace/testdata/src/anotherlib"
	"github.com/podhmo/go-scan/examples/call-trace/testdata/src/mylib"
)

func main() {
	// Direct function call
	mylib.Helper()

	// Method call
	g := mylib.Greeter{Name: "World"}
	g.Greet()

	// Interface call
	var r anotherlib.Runner
	r = anotherlib.NewRunner()
	r.Run()

	fmt.Println("Done.")
}
