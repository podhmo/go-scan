package integrationtest

// String types
// @deriving:binding
type TestBindStringQueryOptional struct {
	Value string `in:"query" query:"value"`
}

// @deriving:binding
type TestBindStringQueryRequired struct {
	Value string `in:"query" query:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrStringQueryOptional struct {
	Value *string `in:"query" query:"value"`
}

// @deriving:binding
type TestBindPtrStringQueryRequired struct {
	Value *string `in:"query" query:"value" required:"true"`
}

// @deriving:binding
type TestBindStringHeaderOptional struct {
	Value string `in:"header" header:"X-Value"`
}

// @deriving:binding
type TestBindStringHeaderRequired struct {
	Value string `in:"header" header:"X-Value" required:"true"`
}

// @deriving:binding
type TestBindPtrStringHeaderOptional struct {
	Value *string `in:"header" header:"X-Value"`
}

// @deriving:binding
type TestBindPtrStringHeaderRequired struct {
	Value *string `in:"header" header:"X-Value" required:"true"`
}

// @deriving:binding
type TestBindStringCookieOptional struct {
	Value string `in:"cookie" cookie:"value"`
}

// @deriving:binding
type TestBindStringCookieRequired struct {
	Value string `in:"cookie" cookie:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrStringCookieOptional struct {
	Value *string `in:"cookie" cookie:"value"`
}

// @deriving:binding
type TestBindPtrStringCookieRequired struct {
	Value *string `in:"cookie" cookie:"value" required:"true"`
}

// @deriving:binding
type TestBindStringPathOptional struct {
	Value string `in:"path" path:"value"`
}

// @deriving:binding
type TestBindStringPathRequired struct {
	Value string `in:"path" path:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrStringPathOptional struct {
	Value *string `in:"path" path:"value"`
}

// @deriving:binding
type TestBindPtrStringPathRequired struct {
	Value *string `in:"path" path:"value" required:"true"`
}

// Int types
// @deriving:binding
type TestBindIntQueryOptional struct {
	Value int `in:"query" query:"value"`
}

// @deriving:binding
type TestBindIntQueryRequired struct {
	Value int `in:"query" query:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrIntQueryOptional struct {
	Value *int `in:"query" query:"value"`
}

// @deriving:binding
type TestBindPtrIntQueryRequired struct {
	Value *int `in:"query" query:"value" required:"true"`
}

// @deriving:binding
type TestBindIntHeaderOptional struct {
	Value int `in:"header" header:"X-Value"`
}

// @deriving:binding
type TestBindIntHeaderRequired struct {
	Value int `in:"header" header:"X-Value" required:"true"`
}

// @deriving:binding
type TestBindPtrIntHeaderOptional struct {
	Value *int `in:"header" header:"X-Value"`
}

// @deriving:binding
type TestBindPtrIntHeaderRequired struct {
	Value *int `in:"header" header:"X-Value" required:"true"`
}

// @deriving:binding
type TestBindIntCookieOptional struct {
	Value int `in:"cookie" cookie:"value"`
}

// @deriving:binding
type TestBindIntCookieRequired struct {
	Value int `in:"cookie" cookie:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrIntCookieOptional struct {
	Value *int `in:"cookie" cookie:"value"`
}

// @deriving:binding
type TestBindPtrIntCookieRequired struct {
	Value *int `in:"cookie" cookie:"value" required:"true"`
}

// @deriving:binding
type TestBindIntPathOptional struct {
	Value int `in:"path" path:"value"`
}

// @deriving:binding
type TestBindIntPathRequired struct {
	Value int `in:"path" path:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrIntPathOptional struct {
	Value *int `in:"path" path:"value"`
}

// @deriving:binding
type TestBindPtrIntPathRequired struct {
	Value *int `in:"path" path:"value" required:"true"`
}

// Bool types
// @deriving:binding
type TestBindBoolQueryOptional struct {
	Value bool `in:"query" query:"value"`
}

// @deriving:binding
type TestBindBoolQueryRequired struct {
	Value bool `in:"query" query:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrBoolQueryOptional struct {
	Value *bool `in:"query" query:"value"`
}

// @deriving:binding
type TestBindPtrBoolQueryRequired struct {
	Value *bool `in:"query" query:"value" required:"true"`
}

// @deriving:binding
type TestBindBoolHeaderOptional struct {
	Value bool `in:"header" header:"X-Value"`
}

// @deriving:binding
type TestBindBoolHeaderRequired struct {
	Value bool `in:"header" header:"X-Value" required:"true"`
}

// @deriving:binding
type TestBindPtrBoolHeaderOptional struct {
	Value *bool `in:"header" header:"X-Value"`
}

// @deriving:binding
type TestBindPtrBoolHeaderRequired struct {
	Value *bool `in:"header" header:"X-Value" required:"true"`
}

// @deriving:binding
type TestBindBoolCookieOptional struct {
	Value bool `in:"cookie" cookie:"value"`
}

// @deriving:binding
type TestBindBoolCookieRequired struct {
	Value bool `in:"cookie" cookie:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrBoolCookieOptional struct {
	Value *bool `in:"cookie" cookie:"value"`
}

// @deriving:binding
type TestBindPtrBoolCookieRequired struct {
	Value *bool `in:"cookie" cookie:"value" required:"true"`
}

// @deriving:binding
type TestBindBoolPathOptional struct {
	Value bool `in:"path" path:"value"`
}

// @deriving:binding
type TestBindBoolPathRequired struct {
	Value bool `in:"path" path:"value" required:"true"`
}

// @deriving:binding
type TestBindPtrBoolPathOptional struct {
	Value *bool `in:"path" path:"value"`
}

// @deriving:binding
type TestBindPtrBoolPathRequired struct {
	Value *bool `in:"path" path:"value" required:"true"`
}

// TODO: Add other numeric types (int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64, complex64, complex128)
// For brevity, I will add a few representative examples for other types.
// The full list would be very long.

// Int64 types
// @deriving:binding
type TestBindInt64QueryOptional struct {
	Value int64 `in:"query" query:"value"`
}

// @deriving:binding
type TestBindPtrInt64PathRequired struct {
	Value *int64 `in:"path" path:"value" required:"true"`
}

// Uint32 types
// @deriving:binding
type TestBindUint32HeaderOptional struct {
	Value uint32 `in:"header" header:"X-Value"`
}

// @deriving:binding
type TestBindPtrUint32CookieRequired struct {
	Value *uint32 `in:"cookie" cookie:"value" required:"true"`
}

// Float64 types
// @deriving:binding
type TestBindFloat64QueryOptional struct {
	Value float64 `in:"query" query:"value"`
}

// @deriving:binding
type TestBindPtrFloat64HeaderRequired struct {
	Value *float64 `in:"header" header:"X-Value" required:"true"`
}

// Complex128 types
// @deriving:binding
type TestBindComplex128CookieOptional struct {
	Value complex128 `in:"cookie" cookie:"value"`
}

// @deriving:binding
type TestBindPtrComplex128PathRequired struct {
	Value *complex128 `in:"path" path:"value" required:"true"`
}

// Uintptr types
// @deriving:binding
type TestBindUintptrQueryOptional struct {
	Value uintptr `in:"query" query:"value"`
}

// @deriving:binding
type TestBindPtrUintptrHeaderRequired struct {
	Value *uintptr `in:"header" header:"X-Value" required:"true"`
}

// A struct with multiple fields to test interactions
// @deriving:binding
type TestBindMixedFields struct {
	Name      string  `in:"query" query:"name" required:"true"`
	Age       *int    `in:"query" query:"age"`
	SessionID string  `in:"cookie" cookie:"session_id" required:"true"`
	AuthToken *string `in:"header" header:"X-Auth-Token"`
	UserID    string  `in:"path" path:"userID" required:"true"`
	IsEnabled *bool   `in:"query" query:"enabled"`
	Factor    float64 `in:"header" header:"X-Factor"`
}
