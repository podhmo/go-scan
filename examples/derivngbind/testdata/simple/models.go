package simple

// @derivng:binding in:"body"
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

// @derivng:binding
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


// @derivng:binding in:"body"
// This struct itself is the target for the request body because of `in:"body"` in the doc comment.
// Fields without other `in:` tags are expected from JSON.
type FullBodyBind struct {
	Title        string `json:"title"`         // From JSON body
	Count        int    `json:"count"`         // From JSON body
	IsPublished  bool   `json:"is_published"`  // From JSON body
	SourceHeader string `in:"header" header:"X-Source"` // This field explicitly comes from a header
}

// @derivng:binding
// Test case for a struct that is NOT a body target itself, but has query/path params.
// Used to ensure that non-body structs are handled correctly.
type QueryAndPathOnlyBind struct {
    UserID    string `in:"path" path:"userID"`
    ItemCode  string `in:"query" query:"itemCode"`
    Limit     int    `in:"query" query:"limit"`
}
