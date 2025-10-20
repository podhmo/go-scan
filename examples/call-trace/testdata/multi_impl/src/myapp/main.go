package main

import (
	"fmt"
	"github.com/podhmo/go-scan/examples/call-trace/testdata/multi_impl/src/mylib"
)

func main() {
	var m mylib.Messenger

	m = &mylib.English{}
	fmt.Println(m.GetMessage())

	m = &mylib.Japanese{}
	fmt.Println(m.GetMessage())
}
