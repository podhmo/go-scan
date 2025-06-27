package anotherpkg

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Helper to create pointers for test cases
func ptr[T any](v T) *T {
	return &v
}

func TestAnotherModel_Bind(t *testing.T) {
	tests := []struct {
		name        string
		queryParams string
		headers     map[string]string
		want        AnotherModel
		expectError bool
		errorMsg    string
	}{
		{
			name:        "all present",
			queryParams: "item_name=gizmo&quantity=10",
			headers:     map[string]string{"X-Special": "true"},
			want: AnotherModel{
				ItemName:  "gizmo",
				Quantity:  ptr(10),
				IsSpecial: true,
			},
			expectError: false,
		},
		{
			name:        "required item_name missing",
			queryParams: "quantity=5",
			headers:     map[string]string{"X-Special": "false"},
			expectError: true,
			errorMsg:    "binding: query key 'item_name' is required",
		},
		{
			name:        "optional quantity missing, header missing",
			queryParams: "item_name=widget",
			// No X-Special header
			want: AnotherModel{
				ItemName:  "widget",
				Quantity:  nil, // Default for *int
				IsSpecial: false, // Default for bool
			},
			expectError: false,
		},
		{
			name:        "quantity invalid",
			queryParams: "item_name=thing&quantity=lots",
			headers:     map[string]string{"X-Special": "true"},
			expectError: true,
			errorMsg:    "binding: failed to parse query key 'quantity' with value \"lots\": strconv.Atoi: parsing \"lots\": invalid syntax",
		},
		{
			name:        "isSpecial invalid",
			queryParams: "item_name=stuff",
			headers:     map[string]string{"X-Special": "maybe"},
			expectError: true,
			errorMsg:    "binding: failed to parse header key 'X-Special' with value \"maybe\": strconv.ParseBool: parsing \"maybe\": invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/?"+tt.queryParams, nil)
			for key, val := range tt.headers {
				req.Header.Set(key, val)
			}

			var data AnotherModel
			// The pathValueExtractor is not used by these fields, so it can be nil or a dummy.
			err := data.Bind(req, func(s string) string { return "" })

			if (err != nil) != tt.expectError {
				t.Errorf("Bind() error = %v, wantErr %v", err, tt.expectError)
				return
			}

			if tt.expectError {
				if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Bind() expected error containing %q, got %v", tt.errorMsg, err)
				}
			} else {
				if data.ItemName != tt.want.ItemName {
					t.Errorf("Bind() ItemName = %q, want %q", data.ItemName, tt.want.ItemName)
				}
				if (data.Quantity == nil && tt.want.Quantity != nil) || (data.Quantity != nil && tt.want.Quantity == nil) || (data.Quantity != nil && tt.want.Quantity != nil && *data.Quantity != *tt.want.Quantity) {
					t.Errorf("Bind() Quantity = %v, want %v", data.Quantity, tt.want.Quantity)
				}
				if data.IsSpecial != tt.want.IsSpecial {
					t.Errorf("Bind() IsSpecial = %v, want %v", data.IsSpecial, tt.want.IsSpecial)
				}
			}
		})
	}
}
