package source

import "time"

// --- Source Structs ---

type SrcAddress struct {
	Street string
	City   string
}

type SrcContact struct {
	Email string
	Phone *string // Pointer to allow for nil
}

type SrcInternalDetail struct {
	Code        int
	Description string // This might need "translation"
}

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

// Another top-level type for demonstrating multiple exported converters
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
