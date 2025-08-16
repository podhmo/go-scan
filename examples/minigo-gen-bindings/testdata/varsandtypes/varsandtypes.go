package varsandtypes

import "fmt"

type ExportedType struct {
	Name string
}

var ExportedVar = ExportedType{Name: "test"}

const ExportedConstant = "hello"

func ExportedFunction() {
	fmt.Println("hello from function")
}
