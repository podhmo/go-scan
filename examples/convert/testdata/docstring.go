package testdata

// @derivingconvert(DocstringDst)
type DocstringSrc struct {
	ID          int
	Name        string
	Optional    *string
	Required    *string `convert:"Required,required"`
	Skipped     string  `convert:"-"`
	AnotherSkip *string `convert:"-"`
}

type DocstringDst struct {
	ID       int
	Name     string
	Optional *string
	Required *string
}
