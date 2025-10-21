package main

import (
	"fmt"

	"##WORKDIR##/examples/call-trace/testdata/ddd_scenario/intermediatelib"
	"##WORKDIR##/examples/call-trace/testdata/ddd_scenario/mylib"
)

func main() {
	repo := &mylib.ConcreteRepository{}
	uc := &intermediatelib.Usecase{Repo: repo}
	fmt.Println("before call")
	result := Run(uc, "my-id")
	fmt.Println("after call")
	fmt.Println(result)
}

func Run(uc *intermediatelib.Usecase, id string) string {
	return uc.Run(id)
}
