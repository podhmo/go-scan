package testdata

import "strings"

// // convert:variable builder strings.Builder
// @derivingconvert(DstVariable)
type SrcVariable struct {
	FirstName string
	LastName  string
}

type DstVariable struct {
	FullName string `convert:",using=buildFullName(&builder, src.FirstName, src.LastName)"`
}

func buildFullName(builder *strings.Builder, firstName, lastName string) string {
	builder.Reset()
	builder.WriteString(firstName)
	builder.WriteString(" ")
	builder.WriteString(lastName)
	return builder.String()
}
