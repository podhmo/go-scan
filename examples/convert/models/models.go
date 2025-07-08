package models

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

// --- Destination Structs ---

type DstAddress struct {
	FullStreet string // Different name
	CityName   string // Different name
}

type DstContact struct {
	EmailAddress string // Different name
	PhoneNumber  string // Pointer to value
}

// DstInternalDetail is an example of a type that might have an unexported converter
// if it's only used within the DstUser conversion.
type DstInternalDetail struct {
	ItemCode      int    // Different name
	LocalizedDesc string // Different name, implies processing
}

// DstUser is a "top-level type" for which an exported converter will be generated.
type DstUser struct {
	UserID    string // Different type (int64 to string)
	FullName  string // Combination of FirstName and LastName
	Address   DstAddress
	Contact   DstContact
	Details   []DstInternalDetail
	CreatedAt string // Different type (time.Time to string)
	UpdatedAt string // Pointer to value, different type
}

// Another top-level type for demonstrating multiple exported converters
type SrcOrder struct {
	OrderID string
	Amount  float64
	Items   []SrcItem
}

type SrcItem struct {
	SKU      string
	Quantity int
}

type DstOrder struct {
	ID          string
	TotalAmount float64
	LineItems   []DstItem // Different name
}

type DstItem struct {
	ProductCode string // Different name
	Count       int    // Different name
}
