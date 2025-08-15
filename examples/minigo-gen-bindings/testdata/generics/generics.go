package generics

const ExportedConstant = "hello"

func ExportedFunction() {}

func ExportedGenericFunction[T any](arg T) {
	// do nothing
}
