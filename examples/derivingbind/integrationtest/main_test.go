package integrationtest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFuncBindStringQueryOptional(t *testing.T) {
	tests := []struct {
		name        string
		queryParam  string
		wantValue   string
		expectError bool
	}{
		{"with value", "value=test", "test", false},
		{"empty value", "value=", "", false},
		{"no value", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/?"+tt.queryParam, nil)
			var data TestBindStringQueryOptional
			err := data.Bind(req, func(s string) string { return req.PathValue(s) })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if !tt.expectError && data.Value != tt.wantValue {
				t.Errorf("Bind() Value = %q, want %q", data.Value, tt.wantValue)
			}
		})
	}
}

func TestFuncBindStringQueryRequired(t *testing.T) {
	tests := []struct {
		name        string
		queryParam  string
		wantValue   string
		expectError bool
		errorMsg    string
	}{
		{"with value", "value=test", "test", false, ""},
		{"empty value", "value=", "", false, ""}, // Required but empty is still a provided value
		{"no value", "", "", true, "binding: query key 'value' is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/?"+tt.queryParam, nil)
			var data TestBindStringQueryRequired
			err := data.Bind(req, func(s string) string { return req.PathValue(s) })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Bind() expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else if data.Value != tt.wantValue {
				t.Errorf("Bind() Value = %q, want %q", data.Value, tt.wantValue)
			}
		})
	}
}

func TestFuncBindPtrStringHeaderOptional(t *testing.T) {
	tests := []struct {
		name        string
		headerValue *string
		wantValue   *string
		expectError bool
	}{
		{"with value", ptr("test"), ptr("test"), false},
		{"empty value", ptr(""), ptr(""), false},
		{"no value", nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.headerValue != nil {
				req.Header.Set("X-Value", *tt.headerValue)
			}
			var data TestBindPtrStringHeaderOptional
			err := data.Bind(req, func(s string) string { return req.PathValue(s) })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if !tt.expectError {
				if (data.Value == nil && tt.wantValue != nil) || (data.Value != nil && tt.wantValue == nil) || (data.Value != nil && tt.wantValue != nil && *data.Value != *tt.wantValue) {
					t.Errorf("Bind() Value = %v, want %v", data.Value, tt.wantValue)
				}
			}
		})
	}
}

func TestFuncBindIntPathRequired(t *testing.T) {
	tests := []struct {
		name        string
		pathValue   string
		setPath     bool
		wantValue   int
		expectError bool
		errorMsg    string
	}{
		{"with value", "123", true, 123, false, ""},
		{"invalid int", "abc", true, 0, true, "binding: failed to parse path key 'value' with value \"abc\": strconv.Atoi: parsing \"abc\": invalid syntax"},
		{"no value", "", false, 0, true, "binding: path key 'value' is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/somepath/"+tt.pathValue, nil)
			if tt.setPath {
				req.SetPathValue("value", tt.pathValue)
			}

			var data TestBindIntPathRequired
			err := data.Bind(req, func(s string) string { return req.PathValue(s) })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Bind() expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else if data.Value != tt.wantValue {
				t.Errorf("Bind() Value = %d, want %d", data.Value, tt.wantValue)
			}
		})
	}
}

func TestFuncBindBoolCookieOptional(t *testing.T) {
	tests := []struct {
		name        string
		cookieValue *string // Use string to represent cookie presence
		wantValue   bool
		expectError bool
		errorMsg    string
	}{
		{"true value", ptr("true"), true, false, ""},
		{"false value", ptr("false"), false, false, ""},
		{"1 value", ptr("1"), true, false, ""},
		{"0 value", ptr("0"), false, false, ""},
		{"t value", ptr("t"), true, false, ""}, // strconv.ParseBool standard
		{"f value", ptr("f"), false, false, ""}, // strconv.ParseBool standard
		{"TRUE value", ptr("TRUE"), true, false, ""},
		{"FALSE value", ptr("FALSE"), false, false, ""},
		{"empty value", ptr(""), false, true, "binding: failed to parse cookie key 'value' with value \"\": strconv.ParseBool: parsing \"\": invalid syntax"}, // Empty string is invalid for strconv.ParseBool
		{"invalid value", ptr("yes"), false, true, "binding: failed to parse cookie key 'value' with value \"yes\": strconv.ParseBool: parsing \"yes\": invalid syntax"},
		{"no value", nil, false, false, ""}, // Optional, so no error, default false
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.cookieValue != nil {
				req.AddCookie(&http.Cookie{Name: "value", Value: *tt.cookieValue})
			}
			var data TestBindBoolCookieOptional
			err := data.Bind(req, func(s string) string { return req.PathValue(s) })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Bind() expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else if data.Value != tt.wantValue {
				t.Errorf("Bind() Value = %v, want %v", data.Value, tt.wantValue)
			}
		})
	}
}

func TestFuncBindPtrBoolCookieRequired(t *testing.T) {
	tests := []struct {
		name        string
		cookieValue *string // Use string to represent cookie presence
		wantValue   *bool
		expectError bool
		errorMsg    string
	}{
		{"true value", ptr("true"), ptr(true), false, ""},
		{"false value", ptr("false"), ptr(false), false, ""},
		{"empty value", ptr(""), nil, true, "binding: failed to parse cookie key 'value' with value \"\": strconv.ParseBool: parsing \"\": invalid syntax"}, // Empty string is invalid for strconv.ParseBool
		{"no value", nil, nil, true, "binding: cookie key 'value' is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.cookieValue != nil {
				req.AddCookie(&http.Cookie{Name: "value", Value: *tt.cookieValue})
			}
			var data TestBindPtrBoolCookieRequired
			err := data.Bind(req, func(s string) string { return req.PathValue(s) })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Bind() expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				if (data.Value == nil && tt.wantValue != nil) || (data.Value != nil && tt.wantValue == nil) || (data.Value != nil && tt.wantValue != nil && *data.Value != *tt.wantValue) {
					t.Errorf("Bind() Value = %v, want %v", data.Value, tt.wantValue)
				}
			}
		})
	}
}


func TestFuncBindMixedFields(t *testing.T) {
	type testCase struct {
		name        string
		queryParams string
		cookies     []*http.Cookie
		headers     map[string]string
		pathParams  map[string]string
		want        TestBindMixedFields
		expectError bool
		errorMsg    string
	}

	tests := []testCase{
		{
			name:        "all present and valid",
			queryParams: "name=tester&age=30&enabled=true",
			cookies:     []*http.Cookie{{Name: "session_id", Value: "sid123"}},
			headers:     map[string]string{"X-Auth-Token": "tokenXYZ", "X-Factor": "1.5"},
			pathParams:  map[string]string{"userID": "user1"},
			want: TestBindMixedFields{
				Name:      "tester",
				Age:       ptr(30),
				SessionID: "sid123",
				AuthToken: ptr("tokenXYZ"),
				UserID:    "user1",
				IsEnabled: ptr(true),
				Factor:    1.5,
			},
			expectError: false,
		},
		{
			name:        "required name missing",
			queryParams: "age=30", // name is missing
			cookies:     []*http.Cookie{{Name: "session_id", Value: "sid123"}},
			headers:     map[string]string{"X-Auth-Token": "tokenXYZ"},
			pathParams:  map[string]string{"userID": "user1"},
			expectError: true,
			errorMsg:    "binding: query key 'name' is required",
		},
		{
			name:        "required session_id missing",
			queryParams: "name=tester&age=30",
			// cookies:     No session_id cookie
			headers:     map[string]string{"X-Auth-Token": "tokenXYZ"},
			pathParams:  map[string]string{"userID": "user1"},
			expectError: true,
			errorMsg:    "binding: cookie key 'session_id' is required",
		},
		{
			name:        "required userID missing",
			queryParams: "name=tester&age=30",
			cookies:     []*http.Cookie{{Name: "session_id", Value: "sid123"}},
			headers:     map[string]string{"X-Auth-Token": "tokenXYZ"},
			// pathParams:  userID is missing
			expectError: true,
			errorMsg:    "binding: path key 'userID' is required",
		},
		{
			name:        "optional fields missing",
			queryParams: "name=testerOnly",
			cookies:     []*http.Cookie{{Name: "session_id", Value: "sOnly"}},
			pathParams:  map[string]string{"userID": "uOnly"},
			// AuthToken (header), Age (query), IsEnabled (query), Factor (header) are missing
			want: TestBindMixedFields{
				Name:      "testerOnly",
				Age:       nil,
				SessionID: "sOnly",
				AuthToken: nil,
				UserID:    "uOnly",
				IsEnabled: nil, // default for *bool is nil
				Factor:    0.0, // default for float64
			},
			expectError: false,
		},
		{
			name:        "age invalid",
			queryParams: "name=tester&age=thirty", // age is not int
			cookies:     []*http.Cookie{{Name: "session_id", Value: "sid123"}},
			pathParams:  map[string]string{"userID": "user1"},
			expectError: true,
			errorMsg:    "binding: failed to parse query key 'age' with value \"thirty\": strconv.Atoi: parsing \"thirty\": invalid syntax",
		},
		{
			name:        "factor invalid",
			queryParams: "name=tester",
			cookies:     []*http.Cookie{{Name: "session_id", Value: "sid123"}},
			headers:     map[string]string{"X-Factor": "onepointfive"},
			pathParams:  map[string]string{"userID": "user1"},
			expectError: true,
			errorMsg:    "binding: failed to parse header key 'X-Factor' with value \"onepointfive\": strconv.ParseFloat: parsing \"onepointfive\": invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test?"+tt.queryParams, nil)

			for _, cookie := range tt.cookies {
				req.AddCookie(cookie)
			}
			for key, val := range tt.headers {
				req.Header.Set(key, val)
			}

			// Set path values on the request object for req.PathValue() to find them.
			// The actual URL path string in httptest.NewRequest doesn't need to be templated
			// if we are directly using req.SetPathValue and the Bind method uses req.PathValue.
			if len(tt.pathParams) > 0 {
				for key, val := range tt.pathParams {
					req.SetPathValue(key, val)
				}
			}

			var data TestBindMixedFields
			err := data.Bind(req, func(s string) string { return req.PathValue(s) })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, expectError %v for input %+v", err, tt.expectError, tt)
				return
			}

			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Bind() expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				// Compare Name
				if data.Name != tt.want.Name {
					t.Errorf("Bind() Name = %q, want %q", data.Name, tt.want.Name)
				}
				// Compare Age
				if (data.Age == nil && tt.want.Age != nil) || (data.Age != nil && tt.want.Age == nil) || (data.Age != nil && tt.want.Age != nil && *data.Age != *tt.want.Age) {
					t.Errorf("Bind() Age = %v, want %v", data.Age, tt.want.Age)
				}
				// Compare SessionID
				if data.SessionID != tt.want.SessionID {
					t.Errorf("Bind() SessionID = %q, want %q", data.SessionID, tt.want.SessionID)
				}
				// Compare AuthToken
				if (data.AuthToken == nil && tt.want.AuthToken != nil) || (data.AuthToken != nil && tt.want.AuthToken == nil) || (data.AuthToken != nil && tt.want.AuthToken != nil && *data.AuthToken != *tt.want.AuthToken) {
					t.Errorf("Bind() AuthToken = %v, want %v", data.AuthToken, tt.want.AuthToken)
				}
				// Compare UserID
				if data.UserID != tt.want.UserID {
					t.Errorf("Bind() UserID = %q, want %q", data.UserID, tt.want.UserID)
				}
				// Compare IsEnabled
				if (data.IsEnabled == nil && tt.want.IsEnabled != nil) || (data.IsEnabled != nil && tt.want.IsEnabled == nil) || (data.IsEnabled != nil && tt.want.IsEnabled != nil && *data.IsEnabled != *tt.want.IsEnabled) {
					t.Errorf("Bind() IsEnabled = %v, want %v", data.IsEnabled, tt.want.IsEnabled)
				}
				// Compare Factor
				if data.Factor != tt.want.Factor {
					t.Errorf("Bind() Factor = %v, want %v", data.Factor, tt.want.Factor)
				}
			}
		})
	}
}
