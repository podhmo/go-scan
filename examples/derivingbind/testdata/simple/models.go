package simple

// @deriving:binding in:"body"
// This struct will have its fields bound from various parts of an HTTP request.
// If the struct doc comment also contains `in:"body"`, then fields without specific 'in' tags
// are assumed to be from the JSON body.
type ComprehensiveBind struct {
	PathString    string `in:"path" path:"id"`
	QueryName     string `in:"query" query:"name"`
	QueryAge      int    `in:"query" query:"age"`
	QueryActive   bool   `in:"query" query:"active"`
	HeaderToken   string `in:"header" header:"X-Auth-Token"`
	CookieSession string `in:"cookie" cookie:"session_id"`

	// These fields will be part of the JSON body if the struct is marked as `in:"body"` in its doc,
	// or if this struct is a field in another struct with `in:"body"` on that field.
	// If not, they are ignored by default unless `ComprehensiveBind` itself is unmarshaled directly.
	Description string `json:"description"`
	Value       int    `json:"value"`
}

// @deriving:binding
// This struct demonstrates a specific field being the target for `in:"body"`.
type SpecificBodyFieldBind struct {
	RequestID       string      `in:"header" header:"X-Request-ID"`
	Payload         RequestBody `in:"body"` // The entire request body will be unmarshalled into this field
	OtherQueryParam string      `in:"query" query:"other"`
}

type RequestBody struct {
	ItemName string `json:"itemName"`
	Quantity int    `json:"quantity"`
	IsMember bool   `json:"isMember"`
}

// @deriving:binding in:"body"
// This struct itself is the target for the request body because of `in:"body"` in the doc comment.
// Fields without other `in:` tags are expected from JSON.
type FullBodyBind struct {
	Title        string `json:"title"`                  // From JSON body
	Count        int    `json:"count"`                  // From JSON body
	IsPublished  bool   `json:"is_published"`           // From JSON body
	SourceHeader string `in:"header" header:"X-Source"` // This field explicitly comes from a header
}

// @deriving:binding
// Test case for a struct that is NOT a body target itself, but has query/path params.
// Used to ensure that non-body structs are handled correctly.
type QueryAndPathOnlyBind struct {
	UserID   string `in:"path" path:"userID"`
	ItemCode string `in:"query" query:"itemCode"`
	Limit    int    `in:"query" query:"limit"`
}

// TestPointerFields is a struct for testing pointer fields.
// @deriving:binding
type TestPointerFields struct {
	QueryStrOptional  *string `in:"query" query:"qStrOpt"`
	QueryStrRequired  *string `in:"query" query:"qStrReq" required:"true"`
	QueryIntOptional  *int    `in:"query" query:"qIntOpt"`
	QueryIntRequired  *int    `in:"query" query:"qIntReq" required:"true"`
	QueryBoolOptional *bool   `in:"query" query:"qBoolOpt"`
	QueryBoolRequired *bool   `in:"query" query:"qBoolReq" required:"true"`
	HeaderStrOptional *string `in:"header" header:"hStrOpt"`
	HeaderStrRequired *string `in:"header" header:"hStrReq" required:"true"`
	PathStrOptional   *string `in:"path" path:"pStrOpt"`
	PathStrRequired   *string `in:"path" path:"pStrReq" required:"true"`
	CookieStrOptional *string `in:"cookie" cookie:"cStrOpt"`
	CookieStrRequired *string `in:"cookie" cookie:"cStrReq" required:"true"`
}

// TestRequiredNonPointerFields is a struct for testing required non-pointer fields.
// @deriving:binding
type TestRequiredNonPointerFields struct {
	QueryStrRequired  string `in:"query" query:"qStrReq" required:"true"`
	QueryIntRequired  int    `in:"query" query:"qIntReq" required:"true"`
	HeaderStrRequired string `in:"header" header:"hStrReq" required:"true"`
	PathStrRequired   string `in:"path" path:"pStrReq" required:"true"`
	CookieStrRequired string `in:"cookie" cookie:"cStrReq" required:"true"`
}

// @deriving:binding
// TestExtendedTypesBind is a struct for testing newly supported types including slices and various numerics.
type TestExtendedTypesBind struct {
	// Slice types
	QueryStringSlice     []string  `in:"query" query:"qStrSlice"`
	HeaderIntSlice       []int     `in:"header" header:"X-Int-Slice"` // Comma-separated ints
	CookieBoolSlice      []bool    `in:"cookie" cookie:"ckBoolSlice"` // Comma-separated bools
	PathStringSlice      []string  `in:"path" path:"pStrSlice"`       // Note: Path slices are generally not standard, testing how it's (not) handled.
	QueryPtrIntSlice     []*int    `in:"query" query:"qPtrIntSlice"`
	HeaderPtrStringSlice []*string `in:"header" header:"X-PtrStr-Slice"`

	// Numeric types
	QueryInt8     int8    `in:"query" query:"qInt8"`
	QueryInt16    int16   `in:"query" query:"qInt16"`
	QueryInt32    int32   `in:"query" query:"qInt32"`
	QueryInt64    int64   `in:"query" query:"qInt64"`
	HeaderUint    uint    `in:"header" header:"X-Uint"`
	HeaderUint8   uint8   `in:"header" header:"X-Uint8"`
	HeaderUint16  uint16  `in:"header" header:"X-Uint16"`
	HeaderUint32  uint32  `in:"header" header:"X-Uint32"`
	HeaderUint64  uint64  `in:"header" header:"X-Uint64"`
	CookieFloat32 float32 `in:"cookie" cookie:"ckFloat32"`
	CookieFloat64 float64 `in:"cookie" cookie:"ckFloat64"`

	// Pointer to numeric types
	PathPtrInt64     *int64   `in:"path" path:"pPtrInt64"`
	QueryPtrUint     *uint    `in:"query" query:"qPtrUint"`
	HeaderPtrFloat32 *float32 `in:"header" header:"X-PtrFloat32"`

	// Required variations
	RequiredQueryStringSlice []string `in:"query" query:"reqQStrSlice" required:"true"`
	RequiredHeaderInt        int      `in:"header" header:"X-ReqInt" required:"true"`

	// Boolean specific tests
	QueryBoolTrue    bool `in:"query" query:"qBoolTrue"`    // Test "true"
	QueryBoolFalse   bool `in:"query" query:"qBoolFalse"`   // Test "false"
	QueryBoolOne     bool `in:"query" query:"qBoolOne"`     // Test "1"
	QueryBoolZero    bool `in:"query" query:"qBoolZero"`    // Test "0"
	QueryBoolYes     bool `in:"query" query:"qBoolYes"`     // Test "yes" (should be false by default strconv.ParseBool)
	QueryBoolCapTrue bool `in:"query" query:"qBoolCapTrue"` // Test "TRUE"
	QueryBoolInvalid bool `in:"query" query:"qBoolInvalid"` // Test invalid bool string

	// Empty value handling
	QueryStringEmptyOptional string `in:"query" query:"qStrEmptyOpt"`                 // ?qStrEmptyOpt=
	QueryIntEmptyOptional    int    `in:"query" query:"qIntEmptyOpt"`                 // ?qIntEmptyOpt=
	QueryBoolEmptyOptional   bool   `in:"query" query:"qBoolEmptyOpt"`                // ?qBoolEmptyOpt=
	QueryStringEmptyRequired string `in:"query" query:"qStrEmptyReq" required:"true"` // ?qStrEmptyReq=
	QueryIntEmptyRequired    int    `in:"query" query:"qIntEmptyReq" required:"true"` // ?qIntEmptyReq= (should error)

	QueryPtrStringEmptyOptional *string `in:"query" query:"qPtrStrEmptyOpt"` // ?qPtrStrEmptyOpt=
	QueryPtrIntEmptyOptional    *int    `in:"query" query:"qPtrIntEmptyOpt"` // ?qPtrIntEmptyOpt= (should be nil)

	// Slice with empty elements
	QueryStringSliceWithEmpty    []string  `in:"query" query:"qStrSliceEmpty"`    // ?qStrSliceEmpty=&qStrSliceEmpty=foo&qStrSliceEmpty=
	QueryIntSliceWithEmpty       []int     `in:"query" query:"qIntSliceEmpty"`    // ?qIntSliceEmpty=&qIntSliceEmpty=123&qIntSliceEmpty= (should error on empty)
	QueryPtrStringSliceWithEmpty []*string `in:"query" query:"qPtrStrSliceEmpty"` // ?qPtrStrSliceEmpty=&qPtrStrSliceEmpty=foo&qPtrStrSliceEmpty=
}

// @deriving:binding
// TestNewTypesBind is a struct for testing uintptr, complex64, complex128 types.
type TestNewTypesBind struct {
	// uintptr
	QueryUintptr    uintptr  `in:"query" query:"qUintptr"`
	PathUintptr     uintptr  `in:"path" path:"pUintptr"`
	HeaderUintptr   uintptr  `in:"header" header:"X-Uintptr"`
	CookieUintptr   uintptr  `in:"cookie" cookie:"cUintptr"`
	QueryPtrUintptr *uintptr `in:"query" query:"qPtrUintptr"`

	// complex64
	QueryComplex64    complex64  `in:"query" query:"qComplex64"`
	PathComplex64     complex64  `in:"path" path:"pComplex64"`
	HeaderComplex64   complex64  `in:"header" header:"X-Complex64"`
	CookieComplex64   complex64  `in:"cookie" cookie:"cComplex64"`
	QueryPtrComplex64 *complex64 `in:"query" query:"qPtrComplex64"`

	// complex128
	QueryComplex128    complex128  `in:"query" query:"qComplex128"`
	PathComplex128     complex128  `in:"path" path:"pComplex128"`
	HeaderComplex128   complex128  `in:"header" header:"X-Complex128"`
	CookieComplex128   complex128  `in:"cookie" cookie:"cComplex128"`
	QueryPtrComplex128 *complex128 `in:"query" query:"qPtrComplex128"`

	// Slices of new types
	QueryUintptrSlice     []uintptr    `in:"query" query:"qUintptrSlice"`
	HeaderComplex64Slice  []complex64  `in:"header" header:"X-Complex64-Slice"` // Comma-separated
	CookieComplex128Slice []complex128 `in:"cookie" cookie:"cComplex128-Slice"` // Comma-separated

	// Required fields for new types
	RequiredQueryUintptr    uintptr   `in:"query" query:"reqQUintptr" required:"true"`
	RequiredHeaderComplex64 complex64 `in:"header" header:"X-ReqComplex64" required:"true"`

	// Pointer slices of new types
	QueryPtrUintptrSlice    []*uintptr   `in:"query" query:"qPtrUintptrSlice"`
	HeaderPtrComplex64Slice []*complex64 `in:"header" header:"X-PtrComplex64-Slice"`
}
