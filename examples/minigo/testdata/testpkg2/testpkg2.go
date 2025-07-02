package testpkg2

type Foo struct {
	Name string
	ID   int
}

func NewFoo(name string, id int) *Foo {
	return &Foo{Name: name, ID: id}
}

func GetFooName(f *Foo) string {
	if f == nil {
		return "nil foo"
	}
	return f.Name
}

func GetFooID(f *Foo) int {
	if f == nil {
		return -1
	}
	return f.ID
}

type Bar struct {
	Value string
}

type Baz struct {
	F *Foo
	B Bar
}

func NewBaz(fooName string, fooID int, barValue string) *Baz {
	return &Baz{
		F: NewFoo(fooName, fooID),
		B: Bar{Value: barValue},
	}
}
