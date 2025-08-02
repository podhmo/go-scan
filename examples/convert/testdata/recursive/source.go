package source

// @derivingconvert(example.com/m/destination.DstParent)
type SrcParent struct {
	ID    string
	Child SrcChild
}

type SrcChild struct {
	Value string
}
