package integrationtest

// @deriving:binding
type TestBindStringQuery struct {
	Val string `in:"query" query:"val"`
}

// @deriving:binding
type TestBindStringQueryRequired struct {
	Val string `in:"query" query:"val" required:"true"`
}

// @deriving:binding
type TestBindStringHeader struct {
	Val string `in:"header" header:"X-Val"`
}

// @deriving:binding
type TestBindStringHeaderRequired struct {
	Val string `in:"header" header:"X-Val" required:"true"`
}

// @deriving:binding
type TestBindStringCookie struct {
	Val string `in:"cookie" cookie:"val"`
}

// @deriving:binding
type TestBindStringCookieRequired struct {
	Val string `in:"cookie" cookie:"val" required:"true"`
}

// @deriving:binding
type TestBindStringPath struct {
	Val string `in:"path" path:"val"`
}

// @deriving:binding
type TestBindStringPathRequired struct {
	Val string `in:"path" path:"val" required:"true"`
}

// --- int ---
// @deriving:binding
type TestBindIntQuery struct {
	Val int `in:"query" query:"val"`
}

// @deriving:binding
type TestBindIntQueryRequired struct {
	Val int `in:"query" query:"val" required:"true"`
}

// @deriving:binding
type TestBindIntHeader struct {
	Val int `in:"header" header:"X-Val"`
}

// @deriving:binding
type TestBindIntHeaderRequired struct {
	Val int `in:"header" header:"X-Val" required:"true"`
}

// @deriving:binding
type TestBindIntCookie struct {
	Val int `in:"cookie" cookie:"val"`
}

// @deriving:binding
type TestBindIntCookieRequired struct {
	Val int `in:"cookie" cookie:"val" required:"true"`
}

// @deriving:binding
type TestBindIntPath struct {
	Val int `in:"path" path:"val"`
}

// @deriving:binding
type TestBindIntPathRequired struct {
	Val int `in:"path" path:"val" required:"true"`
}

// --- bool ---
// @deriving:binding
type TestBindBoolQuery struct {
	Val bool `in:"query" query:"val"`
}

// @deriving:binding
type TestBindBoolQueryRequired struct {
	Val bool `in:"query" query:"val" required:"true"`
}

// @deriving:binding
type TestBindBoolHeader struct {
	Val bool `in:"header" header:"X-Val"`
}

// @deriving:binding
type TestBindBoolHeaderRequired struct {
	Val bool `in:"header" header:"X-Val" required:"true"`
}

// @deriving:binding
type TestBindBoolCookie struct {
	Val bool `in:"cookie" cookie:"val"`
}

// @deriving:binding
type TestBindBoolCookieRequired struct {
	Val bool `in:"cookie" cookie:"val" required:"true"`
}

// @deriving:binding
type TestBindBoolPath struct {
	Val bool `in:"path" path:"val"`
}

// @deriving:binding
type TestBindBoolPathRequired struct {
	Val bool `in:"path" path:"val" required:"true"`
}

// --- Pointers ---
// @deriving:binding
type TestBindStringPointerQuery struct {
	Val *string `in:"query" query:"val"`
}

// @deriving:binding
type TestBindStringPointerQueryRequired struct {
	Val *string `in:"query" query:"val" required:"true"`
}

// @deriving:binding
type TestBindStringPointerHeader struct {
	Val *string `in:"header" header:"X-Val"`
}

// @deriving:binding
type TestBindStringPointerHeaderRequired struct {
	Val *string `in:"header" header:"X-Val" required:"true"`
}

// @deriving:binding
type TestBindStringPointerCookie struct {
	Val *string `in:"cookie" cookie:"val"`
}

// @deriving:binding
type TestBindStringPointerCookieRequired struct {
	Val *string `in:"cookie" cookie:"val" required:"true"`
}

// @deriving:binding
type TestBindStringPointerPath struct {
	Val *string `in:"path" path:"val"`
}

// @deriving:binding
type TestBindStringPointerPathRequired struct {
	Val *string `in:"path" path:"val" required:"true"`
}

// --- int pointer ---
// @deriving:binding
type TestBindIntPointerQuery struct {
	Val *int `in:"query" query:"val"`
}

// @deriving:binding
type TestBindIntPointerQueryRequired struct {
	Val *int `in:"query" query:"val" required:"true"`
}

// @deriving:binding
type TestBindIntPointerHeader struct {
	Val *int `in:"header" header:"X-Val"`
}

// @deriving:binding
type TestBindIntPointerHeaderRequired struct {
	Val *int `in:"header" header:"X-Val" required:"true"`
}

// @deriving:binding
type TestBindIntPointerCookie struct {
	Val *int `in:"cookie" cookie:"val"`
}

// @deriving:binding
type TestBindIntPointerCookieRequired struct {
	Val *int `in:"cookie" cookie:"val" required:"true"`
}

// @deriving:binding
type TestBindIntPointerPath struct {
	Val *int `in:"path" path:"val"`
}

// @deriving:binding
type TestBindIntPointerPathRequired struct {
	Val *int `in:"path" path:"val" required:"true"`
}

// --- bool pointer ---
// @deriving:binding
type TestBindBoolPointerQuery struct {
	Val *bool `in:"query" query:"val"`
}

// @deriving:binding
type TestBindBoolPointerQueryRequired struct {
	Val *bool `in:"query" query:"val" required:"true"`
}

// @deriving:binding
type TestBindBoolPointerHeader struct {
	Val *bool `in:"header" header:"X-Val"`
}

// @deriving:binding
type TestBindBoolPointerHeaderRequired struct {
	Val *bool `in:"header" header:"X-Val" required:"true"`
}

// @deriving:binding
type TestBindBoolPointerCookie struct {
	Val *bool `in:"cookie" cookie:"val"`
}

// @deriving:binding
type TestBindBoolPointerCookieRequired struct {
	Val *bool `in:"cookie" cookie:"val" required:"true"`
}

// @deriving:binding
type TestBindBoolPointerPath struct {
	Val *bool `in:"path" path:"val"`
}

// @deriving:binding
type TestBindBoolPointerPathRequired struct {
	Val *bool `in:"path" path:"val" required:"true"`
}

// --- float32 ---
// @deriving:binding
type TestBindFloat32Query struct {
	Val float32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat32QueryRequired struct {
	Val float32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat32Header struct {
	Val float32 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindFloat32HeaderRequired struct {
	Val float32 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindFloat32Cookie struct {
	Val float32 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindFloat32CookieRequired struct {
	Val float32 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat32Path struct {
	Val float32 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindFloat32PathRequired struct {
	Val float32 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat32PointerQuery struct {
	Val *float32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat32PointerQueryRequired struct {
	Val *float32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat32PointerHeader struct {
	Val *float32 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindFloat32PointerHeaderRequired struct {
	Val *float32 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindFloat32PointerCookie struct {
	Val *float32 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindFloat32PointerCookieRequired struct {
	Val *float32 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat32PointerPath struct {
	Val *float32 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindFloat32PointerPathRequired struct {
	Val *float32 `in:"path" path:"val" required:"true"`
}


// --- float64 ---
// @deriving:binding
type TestBindFloat64Query struct {
	Val float64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat64QueryRequired struct {
	Val float64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat64Header struct {
	Val float64 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindFloat64HeaderRequired struct {
	Val float64 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindFloat64Cookie struct {
	Val float64 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindFloat64CookieRequired struct {
	Val float64 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat64Path struct {
	Val float64 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindFloat64PathRequired struct {
	Val float64 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat64PointerQuery struct {
	Val *float64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat64PointerQueryRequired struct {
	Val *float64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat64PointerHeader struct {
	Val *float64 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindFloat64PointerHeaderRequired struct {
	Val *float64 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindFloat64PointerCookie struct {
	Val *float64 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindFloat64PointerCookieRequired struct {
	Val *float64 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat64PointerPath struct {
	Val *float64 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindFloat64PointerPathRequired struct {
	Val *float64 `in:"path" path:"val" required:"true"`
}


// --- uint ---
// @deriving:binding
type TestBindUintQuery struct {
	Val uint `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintQueryRequired struct {
    Val uint `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUintHeader struct {
	Val uint `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUintHeaderRequired struct {
    Val uint `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUintCookie struct {
	Val uint `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUintCookieRequired struct {
    Val uint `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUintPath struct {
	Val uint `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUintPathRequired struct {
    Val uint `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindUintPointerQuery struct {
	Val *uint `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintPointerQueryRequired struct {
    Val *uint `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUintPointerHeader struct {
	Val *uint `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUintPointerHeaderRequired struct {
    Val *uint `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUintPointerCookie struct {
	Val *uint `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUintPointerCookieRequired struct {
    Val *uint `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUintPointerPath struct {
	Val *uint `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUintPointerPathRequired struct {
    Val *uint `in:"path" path:"val" required:"true"`
}


// --- uint8 ---
// @deriving:binding
type TestBindUint8Query struct {
	Val uint8 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint8QueryRequired struct {
    Val uint8 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint8Header struct {
	Val uint8 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint8HeaderRequired struct {
    Val uint8 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint8Cookie struct {
	Val uint8 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint8CookieRequired struct {
    Val uint8 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint8Path struct {
	Val uint8 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint8PathRequired struct {
    Val uint8 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindUint8PointerQuery struct {
	Val *uint8 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint8PointerQueryRequired struct {
    Val *uint8 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint8PointerHeader struct {
	Val *uint8 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint8PointerHeaderRequired struct {
    Val *uint8 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint8PointerCookie struct {
	Val *uint8 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint8PointerCookieRequired struct {
    Val *uint8 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint8PointerPath struct {
	Val *uint8 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint8PointerPathRequired struct {
    Val *uint8 `in:"path" path:"val" required:"true"`
}


// --- uint16 ---
// @deriving:binding
type TestBindUint16Query struct {
	Val uint16 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint16QueryRequired struct {
    Val uint16 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint16Header struct {
	Val uint16 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint16HeaderRequired struct {
    Val uint16 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint16Cookie struct {
	Val uint16 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint16CookieRequired struct {
    Val uint16 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint16Path struct {
	Val uint16 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint16PathRequired struct {
    Val uint16 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindUint16PointerQuery struct {
	Val *uint16 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint16PointerQueryRequired struct {
    Val *uint16 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint16PointerHeader struct {
	Val *uint16 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint16PointerHeaderRequired struct {
    Val *uint16 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint16PointerCookie struct {
	Val *uint16 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint16PointerCookieRequired struct {
    Val *uint16 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint16PointerPath struct {
	Val *uint16 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint16PointerPathRequired struct {
    Val *uint16 `in:"path" path:"val" required:"true"`
}


// --- uint32 ---
// @deriving:binding
type TestBindUint32Query struct {
	Val uint32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint32QueryRequired struct {
    Val uint32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint32Header struct {
	Val uint32 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint32HeaderRequired struct {
    Val uint32 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint32Cookie struct {
	Val uint32 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint32CookieRequired struct {
    Val uint32 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint32Path struct {
	Val uint32 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint32PathRequired struct {
    Val uint32 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindUint32PointerQuery struct {
	Val *uint32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint32PointerQueryRequired struct {
    Val *uint32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint32PointerHeader struct {
	Val *uint32 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint32PointerHeaderRequired struct {
    Val *uint32 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint32PointerCookie struct {
	Val *uint32 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint32PointerCookieRequired struct {
    Val *uint32 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint32PointerPath struct {
	Val *uint32 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint32PointerPathRequired struct {
    Val *uint32 `in:"path" path:"val" required:"true"`
}


// --- uint64 ---
// @deriving:binding
type TestBindUint64Query struct {
	Val uint64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint64QueryRequired struct {
    Val uint64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint64Header struct {
	Val uint64 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint64HeaderRequired struct {
    Val uint64 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint64Cookie struct {
	Val uint64 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint64CookieRequired struct {
    Val uint64 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint64Path struct {
	Val uint64 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint64PathRequired struct {
    Val uint64 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindUint64PointerQuery struct {
	Val *uint64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint64PointerQueryRequired struct {
    Val *uint64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint64PointerHeader struct {
	Val *uint64 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUint64PointerHeaderRequired struct {
    Val *uint64 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUint64PointerCookie struct {
	Val *uint64 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUint64PointerCookieRequired struct {
    Val *uint64 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUint64PointerPath struct {
	Val *uint64 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUint64PointerPathRequired struct {
    Val *uint64 `in:"path" path:"val" required:"true"`
}


// --- uintptr ---
// @deriving:binding
type TestBindUintptrQuery struct {
	Val uintptr `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintptrQueryRequired struct {
    Val uintptr `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUintptrHeader struct {
	Val uintptr `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUintptrHeaderRequired struct {
    Val uintptr `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUintptrCookie struct {
	Val uintptr `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUintptrCookieRequired struct {
    Val uintptr `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUintptrPath struct {
	Val uintptr `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUintptrPathRequired struct {
    Val uintptr `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindUintptrPointerQuery struct {
	Val *uintptr `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintptrPointerQueryRequired struct {
    Val *uintptr `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUintptrPointerHeader struct {
	Val *uintptr `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindUintptrPointerHeaderRequired struct {
    Val *uintptr `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindUintptrPointerCookie struct {
	Val *uintptr `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindUintptrPointerCookieRequired struct {
    Val *uintptr `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindUintptrPointerPath struct {
	Val *uintptr `in:"path" path:"val"`
}
// @deriving:binding
type TestBindUintptrPointerPathRequired struct {
    Val *uintptr `in:"path" path:"val" required:"true"`
}


// --- complex64 ---
// @deriving:binding
type TestBindComplex64Query struct {
	Val complex64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex64QueryRequired struct {
    Val complex64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex64Header struct {
	Val complex64 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindComplex64HeaderRequired struct {
    Val complex64 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindComplex64Cookie struct {
	Val complex64 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindComplex64CookieRequired struct {
    Val complex64 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex64Path struct {
	Val complex64 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindComplex64PathRequired struct {
    Val complex64 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex64PointerQuery struct {
	Val *complex64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex64PointerQueryRequired struct {
    Val *complex64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex64PointerHeader struct {
	Val *complex64 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindComplex64PointerHeaderRequired struct {
    Val *complex64 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindComplex64PointerCookie struct {
	Val *complex64 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindComplex64PointerCookieRequired struct {
    Val *complex64 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex64PointerPath struct {
	Val *complex64 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindComplex64PointerPathRequired struct {
    Val *complex64 `in:"path" path:"val" required:"true"`
}


// --- complex128 ---
// @deriving:binding
type TestBindComplex128Query struct {
	Val complex128 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex128QueryRequired struct {
    Val complex128 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex128Header struct {
	Val complex128 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindComplex128HeaderRequired struct {
    Val complex128 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindComplex128Cookie struct {
	Val complex128 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindComplex128CookieRequired struct {
    Val complex128 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex128Path struct {
	Val complex128 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindComplex128PathRequired struct {
    Val complex128 `in:"path" path:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex128PointerQuery struct {
	Val *complex128 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex128PointerQueryRequired struct {
    Val *complex128 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex128PointerHeader struct {
	Val *complex128 `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindComplex128PointerHeaderRequired struct {
    Val *complex128 `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindComplex128PointerCookie struct {
	Val *complex128 `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindComplex128PointerCookieRequired struct {
    Val *complex128 `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex128PointerPath struct {
	Val *complex128 `in:"path" path:"val"`
}
// @deriving:binding
type TestBindComplex128PointerPathRequired struct {
    Val *complex128 `in:"path" path:"val" required:"true"`
}


// --- Slice Types ---

// @deriving:binding
type TestBindStringSliceQuery struct {
	Val []string `in:"query" query:"val"`
}
// @deriving:binding
type TestBindStringSliceQueryRequired struct {
	Val []string `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindStringSliceHeader struct {
	Val []string `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindStringSliceHeaderRequired struct {
	Val []string `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindStringSliceCookie struct {
	Val []string `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindStringSliceCookieRequired struct {
	Val []string `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindStringSlicePath struct {
	Val []string `in:"path" path:"val"`
}
// @deriving:binding
type TestBindStringSlicePathRequired struct {
	Val []string `in:"path" path:"val" required:"true"`
}


// @deriving:binding
type TestBindIntSliceQuery struct {
	Val []int `in:"query" query:"val"`
}
// @deriving:binding
type TestBindIntSliceQueryRequired struct {
    Val []int `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindIntSliceHeader struct {
	Val []int `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindIntSliceHeaderRequired struct {
    Val []int `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindIntSliceCookie struct {
	Val []int `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindIntSliceCookieRequired struct {
    Val []int `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindIntSlicePath struct {
	Val []int `in:"path" path:"val"`
}
// @deriving:binding
type TestBindIntSlicePathRequired struct {
    Val []int `in:"path" path:"val" required:"true"`
}


// @deriving:binding
type TestBindBoolSliceQuery struct {
	Val []bool `in:"query" query:"val"`
}
// @deriving:binding
type TestBindBoolSliceQueryRequired struct {
    Val []bool `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindBoolSliceHeader struct {
	Val []bool `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindBoolSliceHeaderRequired struct {
    Val []bool `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindBoolSliceCookie struct {
	Val []bool `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindBoolSliceCookieRequired struct {
    Val []bool `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindBoolSlicePath struct {
	Val []bool `in:"path" path:"val"`
}
// @deriving:binding
type TestBindBoolSlicePathRequired struct {
    Val []bool `in:"path" path:"val" required:"true"`
}


// --- Pointer Slice Types ---
// @deriving:binding
type TestBindStringPointerSliceQuery struct {
	Val []*string `in:"query" query:"val"`
}
// @deriving:binding
type TestBindStringPointerSliceQueryRequired struct {
	Val []*string `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindStringPointerSliceHeader struct {
	Val []*string `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindStringPointerSliceHeaderRequired struct {
	Val []*string `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindStringPointerSliceCookie struct {
	Val []*string `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindStringPointerSliceCookieRequired struct {
	Val []*string `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindStringPointerSlicePath struct {
	Val []*string `in:"path" path:"val"`
}
// @deriving:binding
type TestBindStringPointerSlicePathRequired struct {
	Val []*string `in:"path" path:"val" required:"true"`
}


// @deriving:binding
type TestBindIntPointerSliceQuery struct {
	Val []*int `in:"query" query:"val"`
}
// @deriving:binding
type TestBindIntPointerSliceQueryRequired struct {
	Val []*int `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindIntPointerSliceHeader struct {
	Val []*int `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindIntPointerSliceHeaderRequired struct {
	Val []*int `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindIntPointerSliceCookie struct {
	Val []*int `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindIntPointerSliceCookieRequired struct {
	Val []*int `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindIntPointerSlicePath struct {
	Val []*int `in:"path" path:"val"`
}
// @deriving:binding
type TestBindIntPointerSlicePathRequired struct {
	Val []*int `in:"path" path:"val" required:"true"`
}


// @deriving:binding
type TestBindBoolPointerSliceQuery struct {
	Val []*bool `in:"query" query:"val"`
}
// @deriving:binding
type TestBindBoolPointerSliceQueryRequired struct {
	Val []*bool `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindBoolPointerSliceHeader struct {
	Val []*bool `in:"header" header:"X-Val"`
}
// @deriving:binding
type TestBindBoolPointerSliceHeaderRequired struct {
	Val []*bool `in:"header" header:"X-Val" required:"true"`
}
// @deriving:binding
type TestBindBoolPointerSliceCookie struct {
	Val []*bool `in:"cookie" cookie:"val"`
}
// @deriving:binding
type TestBindBoolPointerSliceCookieRequired struct {
	Val []*bool `in:"cookie" cookie:"val" required:"true"`
}
// @deriving:binding
type TestBindBoolPointerSlicePath struct {
	Val []*bool `in:"path" path:"val"`
}
// @deriving:binding
type TestBindBoolPointerSlicePathRequired struct {
	Val []*bool `in:"path" path:"val" required:"true"`
}


// --- Float Slices ---
// @deriving:binding
type TestBindFloat32SliceQuery struct {
	Val []float32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat32SliceQueryRequired struct {
	Val []float32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat64SliceQuery struct {
	Val []float64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat64SliceQueryRequired struct {
	Val []float64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat32PointerSliceQuery struct {
	Val []*float32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat32PointerSliceQueryRequired struct {
	Val []*float32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindFloat64PointerSliceQuery struct {
	Val []*float64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindFloat64PointerSliceQueryRequired struct {
	Val []*float64 `in:"query" query:"val" required:"true"`
}


// --- Uint Slices ---
// @deriving:binding
type TestBindUintSliceQuery struct {
	Val []uint `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintSliceQueryRequired struct {
	Val []uint `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint8SliceQuery struct {
	Val []uint8 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint8SliceQueryRequired struct {
	Val []uint8 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint16SliceQuery struct {
	Val []uint16 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint16SliceQueryRequired struct {
	Val []uint16 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint32SliceQuery struct {
	Val []uint32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint32SliceQueryRequired struct {
	Val []uint32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint64SliceQuery struct {
	Val []uint64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint64SliceQueryRequired struct {
	Val []uint64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUintptrSliceQuery struct {
	Val []uintptr `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintptrSliceQueryRequired struct {
	Val []uintptr `in:"query" query:"val" required:"true"`
}

// --- Uint Pointer Slices ---
// @deriving:binding
type TestBindUintPointerSliceQuery struct {
	Val []*uint `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintPointerSliceQueryRequired struct {
	Val []*uint `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint8PointerSliceQuery struct {
	Val []*uint8 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint8PointerSliceQueryRequired struct {
	Val []*uint8 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint16PointerSliceQuery struct {
	Val []*uint16 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint16PointerSliceQueryRequired struct {
	Val []*uint16 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint32PointerSliceQuery struct {
	Val []*uint32 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint32PointerSliceQueryRequired struct {
	Val []*uint32 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUint64PointerSliceQuery struct {
	Val []*uint64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUint64PointerSliceQueryRequired struct {
	Val []*uint64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindUintptrPointerSliceQuery struct {
	Val []*uintptr `in:"query" query:"val"`
}
// @deriving:binding
type TestBindUintptrPointerSliceQueryRequired struct {
	Val []*uintptr `in:"query" query:"val" required:"true"`
}


// --- Complex Slices ---
// @deriving:binding
type TestBindComplex64SliceQuery struct {
	Val []complex64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex64SliceQueryRequired struct {
	Val []complex64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex128SliceQuery struct {
	Val []complex128 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex128SliceQueryRequired struct {
	Val []complex128 `in:"query" query:"val" required:"true"`
}

// --- Complex Pointer Slices ---
// @deriving:binding
type TestBindComplex64PointerSliceQuery struct {
	Val []*complex64 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex64PointerSliceQueryRequired struct {
	Val []*complex64 `in:"query" query:"val" required:"true"`
}
// @deriving:binding
type TestBindComplex128PointerSliceQuery struct {
	Val []*complex128 `in:"query" query:"val"`
}
// @deriving:binding
type TestBindComplex128PointerSliceQueryRequired struct {
	Val []*complex128 `in:"query" query:"val" required:"true"`
}

// --- Multiple Fields of different types ---
// @deriving:binding
type TestBindMultipleQuery struct {
	StringVal  string  `in:"query" query:"sval"`
	IntVal     int     `in:"query" query:"ival"`
	BoolVal    bool    `in:"query" query:"bval"`
	FloatVal   float64 `in:"query" query:"fval"`
	UintVal    uint    `in:"query" query:"uval"`
	ComplexVal complex128 `in:"query" query:"cval"`
}

// @deriving:binding
type TestBindMultipleQueryRequired struct {
	StringVal  string  `in:"query" query:"sval" required:"true"`
	IntVal     int     `in:"query" query:"ival" required:"true"`
	BoolVal    bool    `in:"query" query:"bval" required:"true"`
	FloatVal   float64 `in:"query" query:"fval" required:"true"`
	UintVal    uint    `in:"query" query:"uval" required:"true"`
	ComplexVal complex128 `in:"query" query:"cval" required:"true"`
}

// @deriving:binding
type TestBindMultiplePointerQuery struct {
	StringVal  *string  `in:"query" query:"sval"`
	IntVal     *int     `in:"query" query:"ival"`
	BoolVal    *bool    `in:"query" query:"bval"`
	FloatVal   *float64 `in:"query" query:"fval"`
	UintVal    *uint    `in:"query" query:"uval"`
	ComplexVal *complex128 `in:"query" query:"cval"`
}

// @deriving:binding
type TestBindMultiplePointerQueryRequired struct {
	StringVal  *string  `in:"query" query:"sval" required:"true"`
	IntVal     *int     `in:"query" query:"ival" required:"true"`
	BoolVal    *bool    `in:"query" query:"bval" required:"true"`
	FloatVal   *float64 `in:"query" query:"fval" required:"true"`
	UintVal    *uint    `in:"query" query:"uval" required:"true"`
	ComplexVal *complex128 `in:"query" query:"cval" required:"true"`
}

// @deriving:binding
type TestBindMultipleMixed struct {
	QueryStr    string  `in:"query" query:"qStr"`
	HeaderInt   int     `in:"header" header:"X-HInt" required:"true"`
	CookieBool  *bool   `in:"cookie" cookie:"cBool"`
	PathFloat   float32 `in:"path" path:"pFloat" required:"true"`
	QueryUintP  *uint   `in:"query" query:"qUintP"`
	HeaderCmplx complex64 `in:"header" header:"X-HCmplx"`
}

// @deriving:binding
type TestBindMultipleSlicesMixed struct {
	QueryStrSlice    []string  `in:"query" query:"qStrSlice"`
	HeaderIntSlice   []int     `in:"header" header:"X-HIntSlice" required:"true"`
	CookieBoolSlice  []*bool   `in:"cookie" cookie:"cBoolSlice"`
	PathFloatSlice   []float32 `in:"path" path:"pFloatSlice" required:"true"`
	QueryUintPSlice  []*uint   `in:"query" query:"qUintPSlice"`
	HeaderCmplxSlice []complex64 `in:"header" header:"X-HCmplxSlice"`
}
