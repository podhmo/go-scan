package foo

// Foo is a sample struct.
type Foo struct{}

// Bar is a sample method with a pointer receiver.
func (f *Foo) Bar() {}

// Qux is a sample method with a value receiver.
func (f Foo) Qux() {}

// Baz is a standalone function.
func Baz() {}
