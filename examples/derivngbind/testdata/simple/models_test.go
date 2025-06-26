package simple

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	// For a more realistic path parameter test, you might use a router like chi.
	// "github.com/go-chi/chi/v5"
)

// Helper function to create a request with context for path parameters (using chi as an example)
// func newTestRequestWithChiCtx(method, path string, body io.Reader, pathParams map[string]string) *http.Request {
// 	req := httptest.NewRequest(method, path, body)
// 	rctx := chi.NewRouteContext()
// 	for k, v := range pathParams {
// 		rctx.URLParams.Add(k, v)
// 	}
// 	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
// }


func TestComprehensiveBind_Bind(t *testing.T) {
	// Path parameters are tricky without a router. The generator currently has a TODO for this.
	// For this test, we'll simulate a value in the path. The generator would need to be
	// updated to parse this (e.g. by splitting the path or using a router's context).
	// For now, the generated code for path params is commented out.
	// We will test other bindings.
	rawBody := `{"description": "test item", "value": 123}`
	req := httptest.NewRequest("POST", "/items/test-path-id?name=Alice&age=30&active=true", strings.NewReader(rawBody))
	req.Header.Set("X-Auth-Token", "test-token")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "test-session"})

	// If using chi or similar for path params:
	// req = newTestRequestWithChiCtx("POST", "/items/test-path-id?name=Alice&age=30&active=true", strings.NewReader(rawBody), map[string]string{"id": "test-path-id"})


	var data ComprehensiveBind
	// Manually set PathString for now, as the generator's path binding is a TODO
	// In a real scenario with a router, this would be populated by the router middleware.
	// For the template to work without a router, it would need a simple split, which is not robust.
	// For now, we assume the generated code for path params is non-functional or relies on context set by a router.
	// To test the generated code for path params, you'd need to uncomment it in the template
	// and provide a mechanism for it to read `test-path-id` (e.g. via context).
	// For this test, we will skip asserting PathString until path param handling is finalized.
	// data.PathString = "test-path-id" // Simulate manual setting or router's work if path logic was active

	mockPathVar := func(name string) string {
		if name == "id" { // This matches the `path:"id"` tag in ComprehensiveBind struct
			return "test-path-id-from-func"
		}
		return ""
	}

	err := data.Bind(req, mockPathVar)
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}

	// Assertions
	if data.PathString != "test-path-id-from-func" {
		t.Errorf("expected PathString to be 'test-path-id-from-func', got '%s'", data.PathString)
	}
	if data.QueryName != "Alice" {
		t.Errorf("expected QueryName to be 'Alice', got '%s'", data.QueryName)
	}
	if data.QueryAge != 30 {
		t.Errorf("expected QueryAge to be 30, got %d", data.QueryAge)
	}
	if data.QueryActive != true {
		t.Errorf("expected QueryActive to be true, got %t", data.QueryActive)
	}
	if data.HeaderToken != "test-token" {
		t.Errorf("expected HeaderToken to be 'test-token', got '%s'", data.HeaderToken)
	}
	if data.CookieSession != "test-session" {
		t.Errorf("expected CookieSession to be 'test-session', got '%s'", data.CookieSession)
	}
	if data.Description != "test item" {
		t.Errorf("expected Description to be 'test item', got '%s'", data.Description)
	}
	if data.Value != 123 {
		t.Errorf("expected Value to be 123, got %d", data.Value)
	}
}

func TestSpecificBodyFieldBind_Bind(t *testing.T) {
	rawBody := `{"itemName": "widget", "quantity": 10, "isMember": true}`
	req := httptest.NewRequest("POST", "/submit?other=abc", strings.NewReader(rawBody))
	req.Header.Set("X-Request-ID", "req-123")
	req.Header.Set("Content-Type", "application/json")

	var data SpecificBodyFieldBind
	// This struct does not have path parameters, so we can pass a nil or dummy pathVar func
	dummyPathVar := func(name string) string { return "" }
	err := data.Bind(req, dummyPathVar)
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}

	if data.RequestID != "req-123" {
		t.Errorf("expected RequestID to be 'req-123', got '%s'", data.RequestID)
	}
	if data.OtherQueryParam != "abc" {
		t.Errorf("expected OtherQueryParam to be 'abc', got '%s'", data.OtherQueryParam)
	}
	if data.Payload.ItemName != "widget" {
		t.Errorf("expected Payload.ItemName to be 'widget', got '%s'", data.Payload.ItemName)
	}
	if data.Payload.Quantity != 10 {
		t.Errorf("expected Payload.Quantity to be 10, got %d", data.Payload.Quantity)
	}
	if data.Payload.IsMember != true {
		t.Errorf("expected Payload.IsMember to be true, got %t", data.Payload.IsMember)
	}
}

func TestFullBodyBind_Bind(t *testing.T) {
	// This struct is tagged `@derivng:binding in:"body"`
	rawBody := `{"title": "My Document", "count": 5, "is_published": false}`
	req := httptest.NewRequest("PUT", "/documents/1", strings.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Source", "external-system")


	var data FullBodyBind
	// This struct does not have path parameters, so we can pass a nil or dummy pathVar func
	dummyPathVar := func(name string) string { return "" }
	err := data.Bind(req, dummyPathVar)
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}

	if data.Title != "My Document" {
		t.Errorf("expected Title to be 'My Document', got '%s'", data.Title)
	}
	if data.Count != 5 {
		t.Errorf("expected Count to be 5, got %d", data.Count)
	}
	if data.IsPublished != false {
		t.Errorf("expected IsPublished to be false, got %t", data.IsPublished)
	}
	if data.SourceHeader != "external-system" {
		t.Errorf("expected SourceHeader to be 'external-system', got '%s'", data.SourceHeader)
	}
}

func TestEmptyValues_Bind(t *testing.T) {
	// Test with missing optional values
	req := httptest.NewRequest("GET", "/items/empty-id", nil) // No query, no body, no relevant headers/cookies

	var data ComprehensiveBind
	// PathString is "id"
	mockPathVar := func(name string) string {
		if name == "id" {
			return "empty-path-id" // or "" if we want to test empty path param
		}
		return ""
	}
	err := data.Bind(req, mockPathVar)
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}

	if data.PathString != "empty-path-id" {
		t.Errorf("expected PathString to be 'empty-path-id', got '%s'", data.PathString)
	}
	// Expect zero values for non-provided fields
	if data.QueryName != "" {
		t.Errorf("expected QueryName to be empty, got '%s'", data.QueryName)
	}
	if data.QueryAge != 0 {
		t.Errorf("expected QueryAge to be 0, got %d", data.QueryAge)
	}
	if data.QueryActive != false {
		t.Errorf("expected QueryActive to be false, got %t", data.QueryActive)
	}
	if data.HeaderToken != "" {
		t.Errorf("expected HeaderToken to be empty, got '%s'", data.HeaderToken)
	}
	if data.CookieSession != "" {
		t.Errorf("expected CookieSession to be empty, got '%s'", data.CookieSession)
	}
	if data.Description != "" { // From body, which is nil
		t.Errorf("expected Description to be empty, got '%s'", data.Description)
	}
	if data.Value != 0 { // From body
		t.Errorf("expected Value to be 0, got %d", data.Value)
	}
}

func TestInvalidTypedValues_Bind(t *testing.T) {
	// Test query parameter with incorrect type
	reqInvalidAge := httptest.NewRequest("GET", "/items/typed?age=not-an-int", nil)
	var dataInvalidAge ComprehensiveBind
	dummyPathVar := func(name string) string { return "" }
	err := dataInvalidAge.Bind(reqInvalidAge, dummyPathVar)
	if err == nil {
		t.Errorf("expected error for invalid age type, got nil")
	} else {
		t.Logf("Got expected error for invalid age: %v", err) // Log error to see it
		if !strings.Contains(err.Error(), "failed to convert query parameter \"age\"") {
			t.Errorf("error message mismatch, got: %s", err.Error())
		}
	}

	reqInvalidActive := httptest.NewRequest("GET", "/items/typed?active=not-a-bool", nil)
	var dataInvalidActive ComprehensiveBind
	err = dataInvalidActive.Bind(reqInvalidActive, dummyPathVar)
	if err == nil {
		t.Errorf("expected error for invalid active type, got nil")
	} else {
		t.Logf("Got expected error for invalid active: %v", err)
		if !strings.Contains(err.Error(), "failed to convert query parameter \"active\"") {
			t.Errorf("error message mismatch, got: %s", err.Error())
		}
	}

	// Test invalid path parameter type (e.g. if pathVar returned "not-an-int" for an int field)
	// This requires a field with path binding and non-string type in ComprehensiveBind.
	// Let's assume PathIntField `path:"intField"` int exists.
	// For now, ComprehensiveBind only has PathString `path:"id"` string.
	// If we add PathIntField `path:"count" path_type:"int"` (hypothetical tag)
	// mockPathVarInvalidInt := func(name string) string {
	// 	if name == "count" { return "not-an-int-for-path" }
	// 	return ""
	// }
	// reqInvalidPathInt := httptest.NewRequest("GET", "/items/path-int-test", nil)
	// var dataInvalidPathInt ComprehensiveBind // Assume it has PathIntField
	// err = dataInvalidPathInt.Bind(reqInvalidPathInt, mockPathVarInvalidInt)
	// if err == nil {
	// 	t.Errorf("expected error for invalid path int type, got nil")
	// } else {
	// 	t.Logf("Got expected error for invalid path int: %v", err)
	// 	if !strings.Contains(err.Error(), "failed to bind path parameter \"count\"") {
	// 		t.Errorf("error message mismatch for invalid path int, got: %s", err.Error())
	// 	}
	// }


	// Test invalid JSON body
	reqInvalidBody := httptest.NewRequest("POST", "/items/body", strings.NewReader(`{"value": "not-an-int-for-value"}`))
	reqInvalidBody.Header.Set("Content-Type", "application/json")
	var dataInvalidBody ComprehensiveBind
	err = dataInvalidBody.Bind(reqInvalidBody, dummyPathVar)
	if err == nil {
		t.Errorf("expected error for invalid JSON body type, got nil")
	} else {
		t.Logf("Got expected error for invalid body: %v", err)
		// The exact error message from json.Unmarshal can vary, so check for a general failure.
		// Example: "json: cannot unmarshal string into Go struct field ComprehensiveBind.value of type int"
		if !strings.Contains(strings.ToLower(err.Error()), "json") || !strings.Contains(strings.ToLower(err.Error()), "value") {
			t.Errorf("error message mismatch for invalid JSON, got: %s", err.Error())
		}
	}
}

// ContextKey is a custom type for context keys to avoid collisions.
// type ContextKey string // Keep if other context tests exist, remove if not used.

// TestPathParameterSimulation_Bind is removed as the primary mechanism is now pathVar func.
// func TestPathParameterSimulation_Bind(t *testing.T) { ... }


func TestQueryAndPathOnlyBind_Bind(t *testing.T) {
	req := httptest.NewRequest("GET", "/users/test-user-id?itemCode=XYZ123&limit=10", nil)

	mockPathVar := func(name string) string {
		if name == "userID" { // Matches `path:"userID"` in QueryAndPathOnlyBind
			return "test-user-id-from-func"
		}
		return ""
	}

	var data QueryAndPathOnlyBind
	err := data.Bind(req, mockPathVar)
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}

	if data.UserID != "test-user-id-from-func" {
		t.Errorf("expected UserID to be 'test-user-id-from-func', got '%s'", data.UserID)
	}
	if data.ItemCode != "XYZ123" {
		t.Errorf("expected ItemCode to be 'XYZ123', got '%s'", data.ItemCode)
	}
	if data.Limit != 10 {
		t.Errorf("expected Limit to be 10, got %d", data.Limit)
	}
	// t.Log("QueryAndPathOnlyBind: UserID is now asserted using mockPathVar.") // Updated log
}

// TODO: Temporarily commented out due to go-scan pointer type resolution issue - Re-enabling // Removing comment
func TestPointerFields_Bind(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name             string
		requestURL       string
		headers          map[string]string
		cookies          []*http.Cookie
		pathParams       map[string]string
		expected         TestPointerFields
		expectError      bool
		errorContains    string
	}{
		{
			name:       "all optional present",
			requestURL: "/?qStrOpt=test&qIntOpt=123&qBoolOpt=true&qStrReq=dummy&qIntReq=0&qBoolReq=false",
			headers:    map[string]string{"hStrOpt": "headerTest", "hStrReq": "dummy"},
			cookies:    []*http.Cookie{{Name: "cStrOpt", Value: "cookieTest"}, {Name: "cStrReq", Value: "dummy"}},
			pathParams: map[string]string{"pStrOpt": "pathTest", "pStrReq": "dummy"},
			expected: TestPointerFields{
				QueryStrOptional:  strPtr("test"),
				QueryIntOptional:  intPtr(123),
				QueryBoolOptional: boolPtr(true),
				HeaderStrOptional: strPtr("headerTest"),
				PathStrOptional:   strPtr("pathTest"),
				CookieStrOptional: strPtr("cookieTest"),
				QueryStrRequired:  strPtr("dummy"),
				QueryIntRequired:  intPtr(0),
				QueryBoolRequired: boolPtr(false),
				HeaderStrRequired: strPtr("dummy"),
				PathStrRequired:   strPtr("dummy"),
				CookieStrRequired: strPtr("dummy"),
			},
		},
		{
			name:       "all optional missing",
			requestURL: "/?qStrReq=dummy&qIntReq=0&qBoolReq=false", // Optional query params missing
			headers:    map[string]string{"hStrReq": "dummy"},      // Optional header missing
			cookies:    []*http.Cookie{{Name: "cStrReq", Value: "dummy"}}, // Optional cookie missing
			pathParams: map[string]string{"pStrReq": "dummy"},      // Optional path param missing
			expected: TestPointerFields{
				QueryStrOptional:  nil,
				QueryIntOptional:  nil,
				QueryBoolOptional: nil,
				HeaderStrOptional: nil,
				PathStrOptional:   nil,
				CookieStrOptional: nil,
				QueryStrRequired:  strPtr("dummy"),
				QueryIntRequired:  intPtr(0),
				QueryBoolRequired: boolPtr(false),
				HeaderStrRequired: strPtr("dummy"),
				PathStrRequired:   strPtr("dummy"),
				CookieStrRequired: strPtr("dummy"),
			},
		},
		{
			name:       "all required present",
			requestURL: "/?qStrReq=reqTest&qIntReq=456&qBoolReq=false",
			headers:    map[string]string{"hStrReq": "reqHeader"},
			cookies:    []*http.Cookie{{Name: "cStrReq", Value: "reqCookie"}},
			pathParams: map[string]string{"pStrReq": "reqPath"},
			expected: TestPointerFields{
				QueryStrRequired:  strPtr("reqTest"),
				QueryIntRequired:  intPtr(456),
				QueryBoolRequired: boolPtr(false),
				HeaderStrRequired: strPtr("reqHeader"),
				PathStrRequired:   strPtr("reqPath"),
				CookieStrRequired: strPtr("reqCookie"),
			},
		},
		{
			name:          "required query string missing",
			requestURL:    "/?qIntReq=1&qBoolReq=true", // qStrReq missing
			pathParams:    map[string]string{"pStrReq": "path"},
			headers:       map[string]string{"hStrReq": "header"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookie"}},
			expectError:   true,
			errorContains: "required query parameter \"qStrReq\" for field QueryStrRequired is missing",
		},
		{
			name:          "required query int missing",
			requestURL:    "/?qStrReq=test&qBoolReq=true", // qIntReq missing
			pathParams:    map[string]string{"pStrReq": "path"},
			headers:       map[string]string{"hStrReq": "header"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookie"}},
			expectError:   true,
			errorContains: "required query parameter \"qIntReq\" for field QueryIntRequired is missing",
		},
		{
			name:          "required header missing",
			requestURL:    "/?qStrReq=test&qIntReq=1&qBoolReq=true",
			pathParams:    map[string]string{"pStrReq": "path"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookie"}},
			// hStrReq missing
			expectError:   true,
			errorContains: "required header \"hStrReq\" for field HeaderStrRequired is missing",
		},
		{
			name:          "required path missing",
			requestURL:    "/?qStrReq=test&qIntReq=1&qBoolReq=true",
			headers:       map[string]string{"hStrReq": "header"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookie"}},
			// pStrReq missing from pathParams
			expectError:   true,
			errorContains: "required path parameter \"pStrReq\" for field PathStrRequired is missing",
		},
		{
			name:          "required cookie missing",
			requestURL:    "/?qStrReq=test&qIntReq=1&qBoolReq=true",
			headers:       map[string]string{"hStrReq": "header"},
			pathParams:    map[string]string{"pStrReq": "path"},
			// cStrReq missing
			expectError:   true,
			errorContains: "required cookie \"cStrReq\" for field CookieStrRequired is missing",
		},
		{
			name: "all values empty strings",
			// Optional pointers to non-string types should become nil due to conversion error.
			// Required pointers to non-string types should cause a conversion error.
			// String pointers should point to an empty string.
			requestURL: "/?qStrOpt=&qStrReq=&qIntOpt=&qIntReq=&qBoolOpt=&qBoolReq=",
			headers:    map[string]string{"hStrOpt": "", "hStrReq": "", "hIntOpt": "", "hIntReq": "", "hBoolOpt": "", "hBoolReq": ""}, // Assuming these could exist
			cookies:    []*http.Cookie{
				{Name: "cStrOpt", Value: ""}, {Name: "cStrReq", Value: ""},
				{Name: "cIntOpt", Value: ""}, {Name: "cIntReq", Value: ""}, // Assuming these could exist
				{Name: "cBoolOpt", Value: ""}, {Name: "cBoolReq", Value: ""}, // Assuming these could exist
			},
			pathParams: map[string]string{"pStrOpt": "", "pStrReq": "", "pIntOpt": "", "pIntReq": "", "pBoolOpt": "", "pBoolReq": ""}, // Assuming
			// Expected struct values if no error occurred (which is not the case here for required int/bool)
			// This test will expect an error from the first required non-string field that gets an empty string.
			// Based on field order in TestPointerFields: QueryIntRequired is the first such.
			expected: TestPointerFields{
				QueryStrOptional:  strPtr(""),
				QueryStrRequired:  strPtr(""),
				QueryIntOptional:  nil, // Optional *int with "" input -> nil
				QueryBoolOptional: nil, // Optional *bool with "" input -> nil
				HeaderStrOptional: strPtr(""),
				HeaderStrRequired: strPtr(""),
				PathStrOptional:   strPtr(""),
				PathStrRequired:   strPtr(""),
				CookieStrOptional: strPtr(""),
				CookieStrRequired: strPtr(""),
				// QueryIntRequired will error first.
			},
			expectError:   true, // Expecting error due to QueryIntRequired=""
			errorContains: "failed to convert query parameter \"qIntReq\" (value: \"\") to int for field QueryIntRequired",
		},
		// The test "required int query with empty string value" is now covered by the modified "all values empty strings"
		// if QueryIntRequired is the first one to cause error. We can remove it or make it more specific
		// if other required fields (e.g. bool) are tested for empty string errors.
		// For now, let's assume the above test covers the qIntReq="" scenario.
		// {
		// 	name:          "required int query with empty string value",
		// 	requestURL:    "/?qStrReq=s&qIntReq=&qBoolReq=true", // qIntReq is ""
		// 	pathParams:    map[string]string{"pStrReq": "p", "pIntReq": "0", "pBoolReq": "false"}, // Satisfy other path required
		// 	headers:       map[string]string{"hStrReq": "h", "hIntReq": "0", "hBoolReq": "false"}, // Satisfy other header required
		// 	cookies:       []*http.Cookie{ // Satisfy other cookie required
		// 		{Name: "cStrReq", Value: "c"}, {Name: "cIntReq", Value: "0"}, {Name: "cBoolReq", Value: "false"},
		// 	},
		// 	expectError:   true,
		// 	errorContains: "failed to convert query parameter \"qIntReq\" (value: \"\") to int for field QueryIntRequired",
		// },
		{
			name:          "required bool query with empty string value",
			requestURL:    "/?qStrReq=s&qIntReq=1&qBoolReq=", // qBoolReq is ""
			// Satisfy other required fields for different sources to isolate this error
			headers:    map[string]string{"hStrReq": "h", "hBoolReq": "true"}, // dummy for other required header
			cookies:    []*http.Cookie{{Name: "cStrReq", Value: "c"}, {Name: "cBoolReq", Value: "true"}}, // dummy for other required cookie
			pathParams: map[string]string{"pStrReq": "p", "pBoolReq": "true"},   // dummy for other required path
			expectError:   true,
			errorContains: "failed to convert query parameter \"qBoolReq\" (value: \"\") to bool for field QueryBoolRequired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.requestURL, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			for _, c := range tt.cookies {
				req.AddCookie(c)
			}

			mockPathVar := func(name string) string {
				if tt.pathParams != nil {
					if val, ok := tt.pathParams[name]; ok {
						return val
					}
				}
				return ""
			}

			var data TestPointerFields
			err := data.Bind(req, mockPathVar)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("expected error message to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("did not expect error, got %v", err)
				}
				// Compare Query
				assertStringPtrEqual(t, "QueryStrOptional", data.QueryStrOptional, tt.expected.QueryStrOptional)
				assertIntPtrEqual(t, "QueryIntOptional", data.QueryIntOptional, tt.expected.QueryIntOptional)
				assertBoolPtrEqual(t, "QueryBoolOptional", data.QueryBoolOptional, tt.expected.QueryBoolOptional)
				assertStringPtrEqual(t, "QueryStrRequired", data.QueryStrRequired, tt.expected.QueryStrRequired)
				assertIntPtrEqual(t, "QueryIntRequired", data.QueryIntRequired, tt.expected.QueryIntRequired)
				assertBoolPtrEqual(t, "QueryBoolRequired", data.QueryBoolRequired, tt.expected.QueryBoolRequired)
				// Compare Header
				assertStringPtrEqual(t, "HeaderStrOptional", data.HeaderStrOptional, tt.expected.HeaderStrOptional)
				assertStringPtrEqual(t, "HeaderStrRequired", data.HeaderStrRequired, tt.expected.HeaderStrRequired)
				// Compare Path
				assertStringPtrEqual(t, "PathStrOptional", data.PathStrOptional, tt.expected.PathStrOptional)
				assertStringPtrEqual(t, "PathStrRequired", data.PathStrRequired, tt.expected.PathStrRequired)
				// Compare Cookie
				assertStringPtrEqual(t, "CookieStrOptional", data.CookieStrOptional, tt.expected.CookieStrOptional)
				assertStringPtrEqual(t, "CookieStrRequired", data.CookieStrRequired, tt.expected.CookieStrRequired)
			}
		})
	}
}

func assertStringPtrEqual(t *testing.T, fieldName string, got, want *string) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil && want != nil {
		t.Errorf("%s: expected %v, got nil", fieldName, *want)
		return
	}
	if got != nil && want == nil {
		t.Errorf("%s: expected nil, got %v", fieldName, *got)
		return
	}
	if *got != *want {
		t.Errorf("%s: expected %v, got %v", fieldName, *want, *got)
	}
}

func assertIntPtrEqual(t *testing.T, fieldName string, got, want *int) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil && want != nil {
		t.Errorf("%s: expected %v, got nil", fieldName, *want)
		return
	}
	if got != nil && want == nil {
		t.Errorf("%s: expected nil, got %v", fieldName, *got)
		return
	}
	if *got != *want {
		t.Errorf("%s: expected %v, got %v", fieldName, *want, *got)
	}
}

func assertBoolPtrEqual(t *testing.T, fieldName string, got, want *bool) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil && want != nil {
		t.Errorf("%s: expected %v, got nil", fieldName, *want)
		return
	}
	if got != nil && want == nil {
		t.Errorf("%s: expected nil, got %v", fieldName, *got)
		return
	}
	if *got != *want {
		t.Errorf("%s: expected %v, got %v", fieldName, *want, *got)
	}
}
// */ // Removing comment
func TestRequiredNonPointerFields_Bind(t *testing.T) {
	tests := []struct {
		name          string
		requestURL    string
		headers       map[string]string
		cookies       []*http.Cookie
		pathParams    map[string]string
		expected      TestRequiredNonPointerFields
		expectError   bool
		errorContains string
	}{
		{
			name:       "all required present",
			requestURL: "/?qStrReq=test&qIntReq=123",
			headers:    map[string]string{"hStrReq": "headerTest"},
			cookies:    []*http.Cookie{{Name: "cStrReq", Value: "cookieTest"}},
			pathParams: map[string]string{"pStrReq": "pathTest"},
			expected: TestRequiredNonPointerFields{
				QueryStrRequired:  "test",
				QueryIntRequired:  123,
				HeaderStrRequired: "headerTest",
				PathStrRequired:   "pathTest",
				CookieStrRequired: "cookieTest",
			},
		},
		{
			name:          "required query string missing",
			requestURL:    "/?qIntReq=123", // qStrReq missing
			headers:       map[string]string{"hStrReq": "headerTest"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookieTest"}},
			pathParams:    map[string]string{"pStrReq": "pathTest"},
			expectError:   true,
			errorContains: "required query parameter \"qStrReq\" for field QueryStrRequired is missing",
		},
		{
			name:          "required query int missing",
			requestURL:    "/?qStrReq=test", // qIntReq missing
			headers:       map[string]string{"hStrReq": "headerTest"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookieTest"}},
			pathParams:    map[string]string{"pStrReq": "pathTest"},
			expectError:   true,
			errorContains: "required query parameter \"qIntReq\" for field QueryIntRequired is missing",
		},
		{
			name:          "required header missing",
			requestURL:    "/?qStrReq=test&qIntReq=123",
			// hStrReq missing
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookieTest"}},
			pathParams:    map[string]string{"pStrReq": "pathTest"},
			expectError:   true,
			errorContains: "required header \"hStrReq\" for field HeaderStrRequired is missing",
		},
		{
			name:          "required path missing",
			requestURL:    "/?qStrReq=test&qIntReq=123",
			headers:       map[string]string{"hStrReq": "headerTest"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookieTest"}},
			// pStrReq missing
			expectError:   true,
			errorContains: "required path parameter \"pStrReq\" for field PathStrRequired is missing",
		},
		{
			name:          "required cookie missing",
			requestURL:    "/?qStrReq=test&qIntReq=123",
			headers:       map[string]string{"hStrReq": "headerTest"},
			pathParams:    map[string]string{"pStrReq": "pathTest"},
			// cStrReq missing
			expectError:   true,
			errorContains: "required cookie \"cStrReq\" for field CookieStrRequired is missing",
		},
		{
			name:          "required query int with empty string value (conversion error)",
			requestURL:    "/?qStrReq=test&qIntReq=", // qIntReq is empty string
			headers:       map[string]string{"hStrReq": "headerTest"},
			cookies:       []*http.Cookie{{Name: "cStrReq", Value: "cookieTest"}},
			pathParams:    map[string]string{"pStrReq": "pathTest"},
			expectError:   true,
			errorContains: "failed to convert query parameter \"qIntReq\" (value: \"\") to int for field QueryIntRequired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.requestURL, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			for _, c := range tt.cookies {
				req.AddCookie(c)
			}

			mockPathVar := func(name string) string {
				if tt.pathParams != nil {
					if val, ok := tt.pathParams[name]; ok {
						return val
					}
				}
				return ""
			}

			var data TestRequiredNonPointerFields
			err := data.Bind(req, mockPathVar)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("expected error message to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("did not expect error, got %v", err)
				}
				if data.QueryStrRequired != tt.expected.QueryStrRequired {
					t.Errorf("QueryStrRequired: expected %v, got %v", tt.expected.QueryStrRequired, data.QueryStrRequired)
				}
				if data.QueryIntRequired != tt.expected.QueryIntRequired {
					t.Errorf("QueryIntRequired: expected %v, got %v", tt.expected.QueryIntRequired, data.QueryIntRequired)
				}
				if data.HeaderStrRequired != tt.expected.HeaderStrRequired {
					t.Errorf("HeaderStrRequired: expected %v, got %v", tt.expected.HeaderStrRequired, data.HeaderStrRequired)
				}
				if data.PathStrRequired != tt.expected.PathStrRequired {
					t.Errorf("PathStrRequired: expected %v, got %v", tt.expected.PathStrRequired, data.PathStrRequired)
				}
				if data.CookieStrRequired != tt.expected.CookieStrRequired {
					t.Errorf("CookieStrRequired: expected %v, got %v", tt.expected.CookieStrRequired, data.CookieStrRequired)
				}
			}
		})
	}
}
