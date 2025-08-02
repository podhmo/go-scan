package source

import "time"

// convert:import converter "github.com/podhmo/go-scan/examples/convert/sampledata/converter"
// convert:rule "time.Time" -> "string", using=converter.TimeToString
// convert:rule "*time.Time" -> "string", using=converter.PtrTimeToString

// @derivingconvert("github.com/podhmo/go-scan/examples/convert/sampledata/destination.DstUser")
type SrcUser struct {
	ID        int64
	FirstName string
	LastName  string
	SrcAddress
	ContactInfo SrcContact
	Details     []SrcInternalDetail
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

type SrcAddress struct {
	Street string
	City   string
}

type SrcContact struct {
	Email string
	Phone *string
}

type SrcInternalDetail struct {
	Code        int
	Description string
}

// @derivingconvert("github.com/podhmo/go-scan/examples/convert/sampledata/destination.DstOrder")
type SrcOrder struct {
	OrderID string
	Amount  float64
	Items   []SrcItem
}

type SrcItem struct {
	SKU      string
	Quantity int
}

// @derivingconvert("github.com/podhmo/go-scan/examples/convert/sampledata/destination.ComplexTarget")
type ComplexSource struct {
	Value       string
	Ptr         *string
	Slice       []SubSource
	SliceOfPtrs []*SubSource
}

type SubSource struct {
	Value int
}

// @derivingconvert("github.com/podhmo/go-scan/examples/convert/sampledata/destination.TargetWithMap")
type SourceWithMap struct {
	ValueMap    map[string]SubSource
	PtrMap      map[string]*SubSource
	StringToStr map[string]string
}
