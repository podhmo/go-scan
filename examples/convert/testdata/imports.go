package testdata

import (
	"time"
)

// convert:import ext "github.com/podhmo/go-scan/examples/convert/testdata/external"
// convert:rule "time.Time" -> "string", using=ext.TimeToString
// convert:rule "string", validator=ext.ValidateString

// @derivingconvert(DstWithImports)
type SrcWithImports struct {
	Timestamp time.Time
	Comment   string
}

type DstWithImports struct {
	Timestamp string
	Comment   string
}
