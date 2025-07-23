package destination

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

type DstOrder struct {
	ID          string
	TotalAmount float64
	LineItems   []DstItem // Different name
}

type DstItem struct {
	ProductCode string // Different name
	Count       int    // Different name
}
