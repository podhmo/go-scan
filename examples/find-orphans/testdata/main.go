package main

import (
	"example.com/find-orphans/testdata/english"
	"example.com/find-orphans/testdata/greeter"
	"example.com/find-orphans/testdata/utils"
)

func main() {
	g := greeter.New(&english.Greeter{})
	g.SayHello()
	utils.UsedUtil()
}
