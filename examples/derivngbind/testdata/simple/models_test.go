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
		if !strings.Contains(err.Error(), "failed to bind query parameter \"age\"") {
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
		if !strings.Contains(err.Error(), "failed to bind query parameter \"active\"") {
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
