package main

import (
	"fmt"

	"github.com/podhmo/go-scan/examples/call-trace/testdata/direct/src/mylib"
)

func main() {
	// Direct function call
	mylib.Helper()
	fmt.Println("Done.")
}
