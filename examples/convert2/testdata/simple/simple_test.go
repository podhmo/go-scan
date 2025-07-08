//go:build convert2_test_target
// +build convert2_test_target

package simple

import (
	"context"
	"fmt" // Added for pointerValue
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConvertSrcSimple(t *testing.T) {
	// Helper functions to create pointers
	strPtr := func(s string) *string { return &s }
	float32Ptr := func(f float32) *float32 { return &f }
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name          string
		src           SrcSimple
		expectedDst   DstSimple
		expectError   bool
		errorContains []string // Substrings to check for in the error message
	}{
		{
			name: "basic conversion with T -> *T, *T -> T, required",
			src: SrcSimple{
				ID:                 1,
				Name:               "Test Name",
				Description:        "This should be skipped",
				Value:              123.45,
				Timestamp:          time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				NoMatchDst:         "Source specific",
				PtrString:          strPtr("Hello Pointer"),
				StringPtr:          "Value To Pointer", // T -> *T
				PtrToValue:         float32Ptr(3.14),   // *T -> T (default)
				RequiredPtrToValue: intPtr(100),        // *T -> T (required)
			},
			expectedDst: DstSimple{
				ID:                 1,
				Name:               "Test Name",
				Value:              123.45,
				CreationTime:       time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				NoMatchSrc:         "",
				PtrString:          strPtr("Hello Pointer"),
				StringPtr:          strPtr("Value To Pointer"), // Expect address of source value
				PtrToValue:         3.14,
				RequiredPtrToValue: 100,
				CustomStr:          "converted_0_from_models",
			},
			expectError: false,
		},
		{
			name: "nil pointer source for *T -> T (default)",
			src: SrcSimple{
				ID:                 2,
				Name:               "Nil PtrToValue",
				Timestamp:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				PtrToValue:         nil, // *T (nil) -> T (default)
				RequiredPtrToValue: intPtr(200),
				StringPtr:          "MakeItPointer",
			},
			expectedDst: DstSimple{
				ID:                 2,
				Name:               "Nil PtrToValue",
				CreationTime:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				PtrToValue:         0, // Expect zero value for float32
				RequiredPtrToValue: 200,
				StringPtr:          strPtr("MakeItPointer"),
				CustomStr:          "converted_0_from_models",
			},
			expectError: false,
		},
		{
			name: "nil pointer source for *T -> T (required)",
			src: SrcSimple{
				ID:                 3,
				Name:               "Nil RequiredPtrToValue",
				Timestamp:          time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				PtrToValue:         float32Ptr(1.0),
				RequiredPtrToValue: nil, // *T (nil) -> T (required)
				StringPtr:          "Another",
			},
			expectedDst: DstSimple{ // Dst fields will be partially populated before error
				ID:                 3,
				Name:               "Nil RequiredPtrToValue",
				CreationTime:       time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				PtrToValue:         1.0,
				StringPtr:          strPtr("Another"),
				RequiredPtrToValue: 0, // Expect zero value as conversion error occurs
				CustomStr:          "converted_0_from_models",
			},
			expectError:   true,
			errorContains: []string{"RequiredPtrToValue", "is required", "source field RequiredPtrToValue is nil"},
		},
		{
			name: "all pointers nil where possible",
			src: SrcSimple{
				ID:                 4,
				Name:               "All Pointers Nil",
				Timestamp:          time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				PtrString:          nil,
				StringPtr:          "WillBePointer", // T -> *T
				PtrToValue:         nil,             // *T -> T (default)
				RequiredPtrToValue: intPtr(400),     // *T -> T (required)
			},
			expectedDst: DstSimple{
				ID:                 4,
				Name:               "All Pointers Nil",
				CreationTime:       time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				PtrString:          nil,
				StringPtr:          strPtr("WillBePointer"),
				PtrToValue:         0, // default for nil
				RequiredPtrToValue: 400,
				CustomStr:          "converted_0_from_models",
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst, err := ConvertSrcSimple(context.Background(), tc.src)

			if tc.expectError {
				if err == nil {
					t.Errorf("ConvertSrcSimple() expected an error, but got nil")
				} else {
					for _, sub := range tc.errorContains {
						if !strings.Contains(err.Error(), sub) {
							t.Errorf("ConvertSrcSimple() error = %v, expected to contain %q", err, sub)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("ConvertSrcSimple() unexpected error: %v", err)
				}
			}

			if !reflect.DeepEqual(dst, tc.expectedDst) {
				t.Errorf("ConvertSrcSimple() got = %#v, want %#v", dst, tc.expectedDst)
				// Provide more detailed diff
				if dst.ID != tc.expectedDst.ID {
					t.Errorf("ID: got %v, want %v", dst.ID, tc.expectedDst.ID)
				}
				if dst.Name != tc.expectedDst.Name {
					t.Errorf("Name: got %v, want %v", dst.Name, tc.expectedDst.Name)
				}
				if dst.Value != tc.expectedDst.Value {
					t.Errorf("Value: got %v, want %v", dst.Value, tc.expectedDst.Value)
				}
				if !dst.CreationTime.Equal(tc.expectedDst.CreationTime) {
					t.Errorf("CreationTime: got %v, want %v", dst.CreationTime, tc.expectedDst.CreationTime)
				}

				if (dst.PtrString == nil && tc.expectedDst.PtrString != nil) || (dst.PtrString != nil && tc.expectedDst.PtrString == nil) || (dst.PtrString != nil && tc.expectedDst.PtrString != nil && *dst.PtrString != *tc.expectedDst.PtrString) {
					t.Errorf("PtrString: got %v, want %v", pointerValue(dst.PtrString), pointerValue(tc.expectedDst.PtrString))
				}
				if (dst.StringPtr == nil && tc.expectedDst.StringPtr != nil) || (dst.StringPtr != nil && tc.expectedDst.StringPtr == nil) || (dst.StringPtr != nil && tc.expectedDst.StringPtr != nil && *dst.StringPtr != *tc.expectedDst.StringPtr) {
					t.Errorf("StringPtr: got %v, want %v", pointerValue(dst.StringPtr), pointerValue(tc.expectedDst.StringPtr))
				}
				if dst.PtrToValue != tc.expectedDst.PtrToValue {
					t.Errorf("PtrToValue: got %v, want %v", dst.PtrToValue, tc.expectedDst.PtrToValue)
				}
				if dst.RequiredPtrToValue != tc.expectedDst.RequiredPtrToValue {
					t.Errorf("RequiredPtrToValue: got %v, want %v", dst.RequiredPtrToValue, tc.expectedDst.RequiredPtrToValue)
				}
			}
		})
	}
}

// Helper to get value of pointer for logging, or "nil"
func pointerValue(ptr interface{}) string {
	val := reflect.ValueOf(ptr)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return "<nil>"
		}
		return fmt.Sprintf("%v", val.Elem().Interface())
	}
	return fmt.Sprintf("%v", ptr) // Should not happen if used for pointers
}

// Test for SrcWithAlias is still commented out as `using` is not implemented.
/*
func TestConvertSrcWithAlias(t *testing.T) {
    // ...
}
*/
