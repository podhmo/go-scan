package integrationtest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"fmt" // For debugging complex types
)

// --- Helper Functions ---
func strPtr(s string) *string { return &s }
func intPtr(i int) *int    { return &i }
func boolPtr(b bool) *bool  { return &b }
func float32Ptr(f float32) *float32 { return &f }
func float64Ptr(f float64) *float64 { return &f }
func uintPtr(u uint) *uint       { return &u }
func uint8Ptr(u uint8) *uint8    { return &u }
func uint16Ptr(u uint16) *uint16  { return &u }
func uint32Ptr(u uint32) *uint32  { return &u }
func uint64Ptr(u uint64) *uint64  { return &u }
func uintptrToval(p uintptr) *uintptr { return &p } // Renamed to avoid conflict
func complex64Ptr(c complex64) *complex64   { return &c }
func complex128Ptr(c complex128) *complex128 { return &c }


func assertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Not equal:\nexpected: %v\nactual  : %v\n%s", expected, actual, fmt.Sprint(msgAndArgs...))
	}
}

func assertNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		t.Fatalf("Expected no error, but got: %v\n%s", err, fmt.Sprint(msgAndArgs...))
	}
}

func assertError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error, but got nil\n%s", fmt.Sprint(msgAndArgs...))
	}
}

func assertErrorContains(t *testing.T, err error, substring string, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error, but got nil\n%s", fmt.Sprint(msgAndArgs...))
		return
	}
	if !strings.Contains(err.Error(), substring) {
		t.Errorf("Error message '%s' does not contain expected substring '%s'\n%s", err.Error(), substring, fmt.Sprint(msgAndArgs...))
	}
}

func newTestRequest(method, target string, queryParams url.Values, headers http.Header, cookies []*http.Cookie, body string) *http.Request {
	u, _ := url.Parse(target)
	if queryParams != nil {
		u.RawQuery = queryParams.Encode()
	}

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, u.String(), strings.NewReader(body))
		if method == "POST" || method == "PUT" || method == "PATCH" { // Common methods with bodies
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded") // Default, can be overridden by headers
		}
	} else {
		req = httptest.NewRequest(method, u.String(), nil)
	}

	if headers != nil {
		for k, v := range headers {
			req.Header[k] = v
		}
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

// --- String Tests ---

func TestBindStringQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "test_string")
	req := newTestRequest("GET", "/test", q, nil, nil, "")

	var data TestBindStringQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, "test_string", data.Val)
}

func TestBindStringQuery_MissingOptional(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, "", data.Val) // Default zero value for string
}

func TestBindStringQueryRequired_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "required_string")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindStringQueryRequired
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, "required_string", data.Val)
}

func TestBindStringQueryRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringQueryRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required query parameter \"val\"")
}

func TestBindStringHeader_Success(t *testing.T) {
	h := http.Header{}
	h.Set("X-Val", "header_val")
	req := newTestRequest("GET", "/test", nil, h, nil, "")
	var data TestBindStringHeader
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, "header_val", data.Val)
}

func TestBindStringHeaderRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringHeaderRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required header \"X-Val\"")
}


func TestBindStringCookie_Success(t *testing.T) {
	cookies := []*http.Cookie{{Name: "val", Value: "cookie_val"}}
	req := newTestRequest("GET", "/test", nil, nil, cookies, "")
	var data TestBindStringCookie
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, "cookie_val", data.Val)
}

func TestBindStringCookieRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringCookieRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required cookie \"val\"")
}

func TestBindStringPath_Success(t *testing.T) {
	req := newTestRequest("GET", "/test/path_val", nil, nil, nil, "")
	pathVarFunc := func(name string) string {
		if name == "val" { return "path_val" }
		return ""
	}
	var data TestBindStringPath
	err := data.Bind(req, pathVarFunc)
	assertNoError(t, err)
	assertEqual(t, "path_val", data.Val)
}

func TestBindStringPathRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test/", nil, nil, nil, "")
	pathVarFunc := func(name string) string { return "" } // Simulate missing path param
	var data TestBindStringPathRequired
	err := data.Bind(req, pathVarFunc)
	assertError(t, err)
	assertErrorContains(t, err, "required path parameter \"val\"")
}


// --- Pointer String Tests ---
func TestBindStringPointerQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "ptr_string")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindStringPointerQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, strPtr("ptr_string"), data.Val)
}

func TestBindStringPointerQuery_MissingOptional(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringPointerQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, (*string)(nil), data.Val)
}

func TestBindStringPointerQueryRequired_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "req_ptr_string")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindStringPointerQueryRequired
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, strPtr("req_ptr_string"), data.Val)
}

func TestBindStringPointerQueryRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringPointerQueryRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required query parameter \"val\"")
}

// --- Int Tests ---
func TestBindIntQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "123")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, 123, data.Val)
}

func TestBindIntQuery_Invalid(t *testing.T) {
	q := url.Values{}
	q.Set("val", "abc")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntQuery
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\"")
}

func TestBindIntQueryRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindIntQueryRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required query parameter \"val\"")
}


// --- Bool Tests ---
func TestBindBoolQuery_SuccessTrue(t *testing.T) {
	q := url.Values{}
	q.Set("val", "true")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindBoolQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, true, data.Val)
}

func TestBindBoolQuery_SuccessFalse(t *testing.T) {
	q := url.Values{}
	q.Set("val", "false")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindBoolQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, false, data.Val)
}

func TestBindBoolQuery_Success1(t *testing.T) {
	q := url.Values{}
	q.Set("val", "1")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindBoolQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, true, data.Val)
}

func TestBindBoolQuery_Invalid(t *testing.T) {
	q := url.Values{}
	q.Set("val", "notabool")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindBoolQuery
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\"")
}

func TestBindBoolQueryRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindBoolQueryRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required query parameter \"val\"")
}

// --- Pointer Int Tests ---
func TestBindIntPointerQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "456")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntPointerQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, intPtr(456), data.Val)
}

func TestBindIntPointerQuery_MissingOptional(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindIntPointerQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, (*int)(nil), data.Val)
}

func TestBindIntPointerQuery_Invalid(t *testing.T) {
	q := url.Values{}
	q.Set("val", "xyz")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntPointerQuery
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\"")
}


// --- Pointer Bool Tests ---
func TestBindBoolPointerQuery_SuccessTrue(t *testing.T) {
	q := url.Values{}
	q.Set("val", "true")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindBoolPointerQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, boolPtr(true), data.Val)
}

func TestBindBoolPointerQuery_MissingOptional(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindBoolPointerQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, (*bool)(nil), data.Val)
}

func TestBindBoolPointerQuery_Invalid(t *testing.T) {
	q := url.Values{}
	q.Set("val", "notbool")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindBoolPointerQuery
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\"")
}

// --- Float32 Tests ---
func TestBindFloat32Query_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "3.14")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindFloat32Query
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, float32(3.14), data.Val)
}

func TestBindFloat32Query_Invalid(t *testing.T) {
	q := url.Values{}
	q.Set("val", "notafloat")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindFloat32Query
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\"")
}

func TestBindFloat32QueryRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindFloat32QueryRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required query parameter \"val\"")
}

func TestBindFloat32PointerQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "2.71")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindFloat32PointerQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, float32Ptr(2.71), data.Val)
}

// --- Uint Tests ---
func TestBindUintQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "12345")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindUintQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, uint(12345), data.Val)
}

func TestBindUintQuery_Invalid(t *testing.T) {
	q := url.Values{}
	q.Set("val", "-100")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindUintQuery
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\"")
}

// --- Complex64 Tests ---
func TestBindComplex64Query_Success(t *testing.T) {
	q := url.Values{}
	q.Set("val", "1+2i")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindComplex64Query
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, complex64(complex(1, 2)), data.Val)
}

func TestBindComplex64Query_Invalid(t *testing.T) {
	q := url.Values{}
	q.Set("val", "notcomplex")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindComplex64Query
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\"")
}


// --- String Slice Tests ---
func TestBindStringSliceQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Add("val", "apple")
	q.Add("val", "banana")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindStringSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, []string{"apple", "banana"}, data.Val)
}

func TestBindStringSliceQuery_Empty(t *testing.T) {
	q := url.Values{}
	q.Add("val", "")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindStringSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, []string{""}, data.Val)
}

func TestBindStringSliceQuery_MultipleEmpty(t *testing.T) {
	q := url.Values{}
	q.Add("val", "")
	q.Add("val", "value")
	q.Add("val", "")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindStringSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, []string{"", "value", ""}, data.Val)
}


func TestBindStringSliceQueryRequired_Missing(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringSliceQueryRequired
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "required query parameter \"val\"")
}


// --- Int Slice Tests ---
func TestBindIntSliceQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Add("val", "1")
	q.Add("val", "20")
	q.Add("val", "-5")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, []int{1, 20, -5}, data.Val)
}

func TestBindIntSliceQuery_InvalidElement(t *testing.T) {
	q := url.Values{}
	q.Add("val", "10")
	q.Add("val", "abc")
	q.Add("val", "30")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntSliceQuery
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\" slice element")
}

// --- Bool Slice Tests ---
func TestBindBoolSliceQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Add("val", "true")
	q.Add("val", "0")
	q.Add("val", "FALSE")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindBoolSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, []bool{true, false, false}, data.Val)
}


// --- Pointer Slice Tests ---
func TestBindStringPointerSliceQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Add("val", "hello")
	q.Add("val", "")
	q.Add("val", "world")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindStringPointerSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, []*string{strPtr("hello"), strPtr(""), strPtr("world")}, data.Val)
}

func TestBindStringPointerSliceQuery_MissingOptional(t *testing.T) {
	req := newTestRequest("GET", "/test", nil, nil, nil, "")
	var data TestBindStringPointerSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, ([]*string)(nil), data.Val)
}


func TestBindIntPointerSliceQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Add("val", "100")
	q.Add("val", "200")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntPointerSliceQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, []*int{intPtr(100), intPtr(200)}, data.Val)
}

func TestBindIntPointerSliceQuery_InvalidElement(t *testing.T) {
	q := url.Values{}
	q.Add("val", "10")
	q.Add("val", "badint")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindIntPointerSliceQuery
	err := data.Bind(req, nil)
	assertError(t, err)
	assertErrorContains(t, err, "failed to convert query parameter \"val\" slice element")
}


// --- Multiple Fields Struct ---
func TestBindMultipleQuery_Success(t *testing.T) {
	q := url.Values{}
	q.Set("sval", "test")
	q.Set("ival", "123")
	q.Set("bval", "true")
	q.Set("fval", "3.14159")
	q.Set("uval", "987")
	q.Set("cval", "2+3i")
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindMultipleQuery
	err := data.Bind(req, nil)
	assertNoError(t, err)
	assertEqual(t, "test", data.StringVal)
	assertEqual(t, 123, data.IntVal)
	assertEqual(t, true, data.BoolVal)
	assertEqual(t, 3.14159, data.FloatVal)
	assertEqual(t, uint(987), data.UintVal)
	assertEqual(t, complex128(complex(2,3)), data.ComplexVal)
}

func TestBindMultipleQueryRequired_PartialMissing(t *testing.T) {
	q := url.Values{}
	q.Set("sval", "test")
	// ival is missing
	q.Set("bval", "true")
	// fval, uval, cval are also missing
	req := newTestRequest("GET", "/test", q, nil, nil, "")
	var data TestBindMultipleQueryRequired
	err := data.Bind(req, nil)
	assertError(t, err)

	errStr := err.Error()
	if !(strings.Contains(errStr, "ival") &&
	     strings.Contains(errStr, "fval") &&
		 strings.Contains(errStr, "uval") &&
		 strings.Contains(errStr, "cval") &&
		 strings.Contains(errStr, "required query parameter")) {
		t.Errorf("Error message %q does not mention all missing required fields (ival, fval, uval, cval)", errStr)
	}
}


func TestBindMultipleMixed_Success(t *testing.T) {
    q := url.Values{}
    q.Set("qStr", "query string")
    q.Set("qUintP", "12345")

    h := http.Header{}
    h.Set("X-HInt", "99")
    h.Set("X-HCmplx", "1.1+2.2i")

    cookies := []*http.Cookie{{Name: "cBool", Value: "false"}}

    pathVarFunc := func(name string) string {
        if name == "pFloat" { return "3.14" }
        return ""
    }

    req := newTestRequest("GET", "/path/3.14", q, h, cookies, "")

    var data TestBindMultipleMixed
    err := data.Bind(req, pathVarFunc)
    assertNoError(t, err)
    assertEqual(t, "query string", data.QueryStr)
    assertEqual(t, 99, data.HeaderInt)
    assertEqual(t, boolPtr(false), data.CookieBool)
    assertEqual(t, float32(3.14), data.PathFloat)
    assertEqual(t, uintPtr(12345), data.QueryUintP)
    assertEqual(t, complex64(complex(1.1, 2.2)), data.HeaderCmplx)
}

func TestBindMultipleMixed_RequiredMissing(t *testing.T) {
    q := url.Values{}
    h := http.Header{}
    h.Set("X-HCmplx", "1+1i")

    cookies := []*http.Cookie{{Name: "cBool", Value: "true"}}

    pathVarFunc := func(name string) string {
        return ""
    }
    req := newTestRequest("GET", "/path/some_value_not_pFloat", q, h, cookies, "")
    var data TestBindMultipleMixed
    err := data.Bind(req, pathVarFunc)
    assertError(t, err)
    errStr := err.Error()
    if !(strings.Contains(errStr, "X-HInt") && strings.Contains(errStr, "pFloat")) {
        t.Errorf("Error message '%s' does not contain expected missing required fields 'X-HInt' or 'pFloat'", errStr)
    }
}
