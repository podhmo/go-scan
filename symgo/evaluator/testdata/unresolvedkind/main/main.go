package main

import (
	"github.com/podhmo/go-scan/symgo/evaluator/testdata/unresolvedkind/ifaceandstruct"
)

var VStruct ifaceandstruct.MyStruct

var VInterface ifaceandstruct.MyInterface

func main() {
	// This will infer MyStruct is a struct because of the composite literal.
	s := ifaceandstruct.MyStruct{Name: "test"}
	VStruct = s

	// This will infer MyInterface is an interface because of the type assertion.
	var x any
	i := x.(ifaceandstruct.MyInterface)
	VInterface = i
}
