package integrationtest

// String types
// @derivng:binding
type TestBindStringQueryOptional struct {
	Value string `in:"query" query:"value"`
}

// @derivng:binding
type TestBindStringQueryRequired struct {
	Value string `in:"query" query:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrStringQueryOptional struct {
	Value *string `in:"query" query:"value"`
}

// @derivng:binding
type TestBindPtrStringQueryRequired struct {
	Value *string `in:"query" query:"value" required:"true"`
}

// @derivng:binding
type TestBindStringHeaderOptional struct {
	Value string `in:"header" header:"X-Value"`
}

// @derivng:binding
type TestBindStringHeaderRequired struct {
	Value string `in:"header" header:"X-Value" required:"true"`
}

// @derivng:binding
type TestBindPtrStringHeaderOptional struct {
	Value *string `in:"header" header:"X-Value"`
}

// @derivng:binding
type TestBindPtrStringHeaderRequired struct {
	Value *string `in:"header" header:"X-Value" required:"true"`
}

// @derivng:binding
type TestBindStringCookieOptional struct {
	Value string `in:"cookie" cookie:"value"`
}

// @derivng:binding
type TestBindStringCookieRequired struct {
	Value string `in:"cookie" cookie:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrStringCookieOptional struct {
	Value *string `in:"cookie" cookie:"value"`
}

// @derivng:binding
type TestBindPtrStringCookieRequired struct {
	Value *string `in:"cookie" cookie:"value" required:"true"`
}

// @derivng:binding
type TestBindStringPathOptional struct {
	Value string `in:"path" path:"value"`
}

// @derivng:binding
type TestBindStringPathRequired struct {
	Value string `in:"path" path:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrStringPathOptional struct {
	Value *string `in:"path" path:"value"`
}

// @derivng:binding
type TestBindPtrStringPathRequired struct {
	Value *string `in:"path" path:"value" required:"true"`
}

// Int types
// @derivng:binding
type TestBindIntQueryOptional struct {
	Value int `in:"query" query:"value"`
}

// @derivng:binding
type TestBindIntQueryRequired struct {
	Value int `in:"query" query:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrIntQueryOptional struct {
	Value *int `in:"query" query:"value"`
}

// @derivng:binding
type TestBindPtrIntQueryRequired struct {
	Value *int `in:"query" query:"value" required:"true"`
}

// @derivng:binding
type TestBindIntHeaderOptional struct {
	Value int `in:"header" header:"X-Value"`
}

// @derivng:binding
type TestBindIntHeaderRequired struct {
	Value int `in:"header" header:"X-Value" required:"true"`
}

// @derivng:binding
type TestBindPtrIntHeaderOptional struct {
	Value *int `in:"header" header:"X-Value"`
}

// @derivng:binding
type TestBindPtrIntHeaderRequired struct {
	Value *int `in:"header" header:"X-Value" required:"true"`
}

// @derivng:binding
type TestBindIntCookieOptional struct {
	Value int `in:"cookie" cookie:"value"`
}

// @derivng:binding
type TestBindIntCookieRequired struct {
	Value int `in:"cookie" cookie:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrIntCookieOptional struct {
	Value *int `in:"cookie" cookie:"value"`
}

// @derivng:binding
type TestBindPtrIntCookieRequired struct {
	Value *int `in:"cookie" cookie:"value" required:"true"`
}

// @derivng:binding
type TestBindIntPathOptional struct {
	Value int `in:"path" path:"value"`
}

// @derivng:binding
type TestBindIntPathRequired struct {
	Value int `in:"path" path:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrIntPathOptional struct {
	Value *int `in:"path" path:"value"`
}

// @derivng:binding
type TestBindPtrIntPathRequired struct {
	Value *int `in:"path" path:"value" required:"true"`
}

// Bool types
// @derivng:binding
type TestBindBoolQueryOptional struct {
	Value bool `in:"query" query:"value"`
}

// @derivng:binding
type TestBindBoolQueryRequired struct {
	Value bool `in:"query" query:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrBoolQueryOptional struct {
	Value *bool `in:"query" query:"value"`
}

// @derivng:binding
type TestBindPtrBoolQueryRequired struct {
	Value *bool `in:"query" query:"value" required:"true"`
}

// @derivng:binding
type TestBindBoolHeaderOptional struct {
	Value bool `in:"header" header:"X-Value"`
}

// @derivng:binding
type TestBindBoolHeaderRequired struct {
	Value bool `in:"header" header:"X-Value" required:"true"`
}

// @derivng:binding
type TestBindPtrBoolHeaderOptional struct {
	Value *bool `in:"header" header:"X-Value"`
}

// @derivng:binding
type TestBindPtrBoolHeaderRequired struct {
	Value *bool `in:"header" header:"X-Value" required:"true"`
}

// @derivng:binding
type TestBindBoolCookieOptional struct {
	Value bool `in:"cookie" cookie:"value"`
}

// @derivng:binding
type TestBindBoolCookieRequired struct {
	Value bool `in:"cookie" cookie:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrBoolCookieOptional struct {
	Value *bool `in:"cookie" cookie:"value"`
}

// @derivng:binding
type TestBindPtrBoolCookieRequired struct {
	Value *bool `in:"cookie" cookie:"value" required:"true"`
}

// @derivng:binding
type TestBindBoolPathOptional struct {
	Value bool `in:"path" path:"value"`
}

// @derivng:binding
type TestBindBoolPathRequired struct {
	Value bool `in:"path" path:"value" required:"true"`
}

// @derivng:binding
type TestBindPtrBoolPathOptional struct {
	Value *bool `in:"path" path:"value"`
}

// @derivng:binding
type TestBindPtrBoolPathRequired struct {
	Value *bool `in:"path" path:"value" required:"true"`
}

// TODO: Add other numeric types (int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64, complex64, complex128)
// For brevity, I will add a few representative examples for other types.
// The full list would be very long.

// Int64 types
// @derivng:binding
type TestBindInt64QueryOptional struct {
	Value int64 `in:"query" query:"value"`
}

// @derivng:binding
type TestBindPtrInt64PathRequired struct {
	Value *int64 `in:"path" path:"value" required:"true"`
}

// Uint32 types
// @derivng:binding
type TestBindUint32HeaderOptional struct {
	Value uint32 `in:"header" header:"X-Value"`
}

// @derivng:binding
type TestBindPtrUint32CookieRequired struct {
	Value *uint32 `in:"cookie" cookie:"value" required:"true"`
}

// Float64 types
// @derivng:binding
type TestBindFloat64QueryOptional struct {
	Value float64 `in:"query" query:"value"`
}

// @derivng:binding
type TestBindPtrFloat64HeaderRequired struct {
	Value *float64 `in:"header" header:"X-Value" required:"true"`
}

// Complex128 types
// @derivng:binding
type TestBindComplex128CookieOptional struct {
	Value complex128 `in:"cookie" cookie:"value"`
}

// @derivng:binding
type TestBindPtrComplex128PathRequired struct {
	Value *complex128 `in:"path" path:"value" required:"true"`
}

// Uintptr types
// @derivng:binding
type TestBindUintptrQueryOptional struct {
	Value uintptr `in:"query" query:"value"`
}

// @derivng:binding
type TestBindPtrUintptrHeaderRequired struct {
	Value *uintptr `in:"header" header:"X-Value" required:"true"`
}

// A struct with multiple fields to test interactions
// @derivng:binding
type TestBindMixedFields struct {
	Name      string  `in:"query" query:"name" required:"true"`
	Age       *int    `in:"query" query:"age"`
	SessionID string  `in:"cookie" cookie:"session_id" required:"true"`
	AuthToken *string `in:"header" header:"X-Auth-Token"`
	UserID    string  `in:"path" path:"userID" required:"true"`
	IsEnabled *bool   `in:"query" query:"enabled"`
	Factor    float64 `in:"header" header:"X-Factor"`
}
