package generics

// ExportedFunction is a generic function.
func ExportedFunction[T any](t T) T {
	return t
}

// ExportedConstant is a constant.
const ExportedConstant = "hello"
