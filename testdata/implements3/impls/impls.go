package impls

type SimpleInterface interface {
	Do()
}

type MyStruct struct{}

func (s *MyStruct) Do() {}

type StructAlias = MyStruct
type PointerAlias = *MyStruct
type NonStructAlias = string
