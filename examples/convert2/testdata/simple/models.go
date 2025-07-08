package simple

import (
	"fmt" // Add fmt import
	"time"
)

type SrcSimple struct {
	ID                 int
	Name               string
	Description        string `convert:"-"` // Skip this field
	Value              float64
	Timestamp          time.Time `convert:"CreationTime"` // Rename
	NoMatchDst         string    // This field has no corresponding field in DstSimple by default
	PtrString          *string
	StringPtr          string   // For T -> *T
	PtrToValue         *float32 // For *T -> T (default)
	RequiredPtrToValue *int     `convert:",required"`                // For *T -> T (required)
	CustomIntToString  int      `convert:"CustomStr,using=IntToStr"` // Test field "using"
}

type DstSimple struct {
	ID   int
	Name string
	// Description string // Skipped from source
	Value              float64
	CreationTime       time.Time // Renamed from Timestamp
	NoMatchSrc         string    // This field has no corresponding field in SrcSimple by default
	PtrString          *string
	StringPtr          *string // For T -> *T
	PtrToValue         float32 // For *T -> T (default)
	RequiredPtrToValue int     // For *T -> T (required)
	CustomStr          string  // For field "using"
}

// For type alias test
type MyTime time.Time

type SrcWithAlias struct {
	EventTime MyTime `convert:"EventTimestamp"`
}

type DstWithAlias struct {
	EventTimestamp time.Time
}

// IntToStr is a helper function that might be used by 'using' directive.
// It's placed here to be available during testing of generated code.
// The 'ec *errorCollector' parameter is based on how the generator currently
// calls 'using' functions. The actual errorCollector type is defined in the
// generated _gen.go file.
func IntToStr(ec *errorCollector, val int) string {
	// To distinguish from conversions.go version if both were somehow present
	return fmt.Sprintf("converted_%d_from_models", val)
}

// --- Nested Structs Test ---

type InnerSrc struct {
	InnerID   int
	InnerName string
}

type InnerDst struct {
	InnerID   int
	InnerName string
}

type OuterSrc struct {
	OuterID   int
	Nested    InnerSrc
	NestedPtr *InnerSrc
	Name      string `convert:"OuterName"`
}

type OuterDst struct {
	OuterID   int
	Nested    InnerDst
	NestedPtr *InnerDst
	OuterName string
}

// For testing nested with different field names
type InnerSrcDiff struct {
	SrcInnerVal int `convert:"DstInnerVal"`
}
type InnerDstDiff struct {
	DstInnerVal int
}
type OuterSrcDiff struct {
	ID         int
	DiffNested InnerSrcDiff `convert:"DestNested"`
}
type OuterDstDiff struct {
	ID         int
	DestNested InnerDstDiff
}

// --- Underlying Type Match Test ---

type SrcUnderlying struct {
	ID         int
	MyAge      MyInt // MyInt is 'type MyInt int'
	MyName     MyString
	MaybeValue MyFloatPtr // type MyFloatPtr *float64
}

type DstUnderlying struct {
	ID         int
	YourAge    YourInt // YourInt is 'type YourInt int'
	YourName   YourString
	MaybeValue *float64 // Direct *float64
}

type MyInt int
type YourInt int

type MyString string
type YourString string

type MyFloat float64
type MyFloatPtr *MyFloat // Pointer to a named type whose underlying is float64
