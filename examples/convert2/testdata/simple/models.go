package simple

import "time"

type SrcSimple struct {
	ID          int
	Name        string
	Description string `convert:"-"` // Skip this field
	Value       float64
	Timestamp   time.Time `convert:"CreationTime"` // Rename
	NoMatchDst  string    // This field has no corresponding field in DstSimple by default
	PtrString   *string
	StringPtr   string
}

type DstSimple struct {
	ID           int
	Name         string
	// Description string // Skipped from source
	Value        float64
	CreationTime time.Time // Renamed from Timestamp
	NoMatchSrc   string    // This field has no corresponding field in SrcSimple by default
	PtrString    *string
	StringPtr    *string // For T -> *T or *T -> *T
}

// For type alias test
type MyTime time.Time

type SrcWithAlias struct {
	EventTime MyTime `convert:"EventTimestamp"`
}

type DstWithAlias struct {
	EventTimestamp time.Time
}
