package testdata

// @derivingconvert(UnmappedDst)
type UnmappedSrc struct {
	ID   int
	Name string
}

type UnmappedDst struct {
	ID          int
	Name        string
	Extra       string  // This should be in the docstring
	Another     int     // This should also be in the docstring
	Optional    *string // This should be ignored
	ExtraPtr    *bool   // This should also be ignored
}
