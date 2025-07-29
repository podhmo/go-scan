package embedded

type Base struct {
	ID int
}

// @derivingconvert(Destination)
type Source struct {
	Base
	Name string
}
