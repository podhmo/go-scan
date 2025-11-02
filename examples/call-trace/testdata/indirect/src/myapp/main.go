package main

import (
	"fmt"

	"github.com/podhmo/go-scan/examples/call-trace/testdata/indirect/src/intermediatelib"
)

func main() {
	// Indirect function call
	intermediatelib.CallHelper()
	fmt.Println("Done.")
}
