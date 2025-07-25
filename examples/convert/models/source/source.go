package source

import "time"

// @derivingconvert("example.com/convert/models/destination.DstUser")
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

// @derivingconvert("example.com/convert/models/destination.DstOrder")
type SrcOrder struct {
	OrderID string
	Amount  float64
	Items   []SrcItem
}

type SrcItem struct {
	SKU      string
	Quantity int
}

// @derivingconvert("example.com/convert/models/destination.ComplexTarget")
type ComplexSource struct {
	Value       string
	Ptr         *string
	Slice       []SubSource
	SliceOfPtrs []*SubSource
}

type SubSource struct {
	Value int
}

// @derivingconvert("example.com/convert/models/destination.TargetWithMap")
type SourceWithMap struct {
	ValueMap    map[string]SubSource
	PtrMap      map[string]*SubSource
	StringToStr map[string]string
}
