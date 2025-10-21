package main

import (
	"fmt"
	"os"

	"##WORKDIR##/examples/call-trace/testdata/multi_impl/mylib"
)

func main() {
	var g mylib.Greeter
	name := os.Args[1]
	switch name {
	case "person":
		g = &mylib.Person{Name: "gopher"}
	case "dog":
		g = &mylib.Dog{Name: "pochi"}
	default:
		return
	}
	fmt.Println("before call")
	Say(g)
	fmt.Println("after call")
}

func Say(g mylib.Greeter) {
	g.Hello()
}
