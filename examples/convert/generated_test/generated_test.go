package generated_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/generated"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"
)

func TestGeneratedUserConversion(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	updatedAt := now.Add(time.Hour)

	src := &source.SrcUser{
		ID:        101,
		FirstName: "John",
		LastName:  "Doe",
		SrcAddress: source.SrcAddress{
			Street: "123 Main St",
			City:   "Anytown",
		},
		ContactInfo: source.SrcContact{
			Email: "john.doe@example.com",
		},
		Details: []source.SrcInternalDetail{
			{Code: 1, Description: "Detail 1"},
		},
		CreatedAt: now,
		UpdatedAt: &updatedAt,
	}

	// Expected result from the *generated* converter.
	// Note that many fields will be zero-valued because the generator
	// doesn't handle name mismatches or custom logic.
	expected := &destination.DstUser{
		// UserID is not mapped (ID vs UserID)
		// FullName is not mapped (requires combining FirstName and LastName)
		// Address is not mapped (SrcAddress vs Address)
		// Contact is not mapped (ContactInfo vs Contact)
		Details: []destination.DstInternalDetail{
			{}, // Inner fields are not mapped (Code vs ItemCode, etc.)
		},
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}

	got, err := generated.ConvertSrcUserToDstUser(ctx, src)
	if err != nil {
		t.Fatalf("ConvertSrcUserToDstUser() failed: %v", err)
	}

	// We can't use reflect.DeepEqual because of the unexported fields in time.Time
	// and the fact that we have zero-valued structs with potentially unexported fields.
	// Let's check the fields we care about.
	if got.UserID != expected.UserID {
		t.Errorf("got UserID %q, want %q", got.UserID, expected.UserID)
	}
	if got.FullName != expected.FullName {
		t.Errorf("got FullName %q, want %q", got.FullName, expected.FullName)
	}
	if !reflect.DeepEqual(got.Address, expected.Address) {
		t.Errorf("got Address %+v, want %+v", got.Address, expected.Address)
	}
	if !reflect.DeepEqual(got.Contact, expected.Contact) {
		t.Errorf("got Contact %+v, want %+v", got.Contact, expected.Contact)
	}
	if got.CreatedAt != expected.CreatedAt {
		t.Errorf("got CreatedAt %q, want %q", got.CreatedAt, expected.CreatedAt)
	}
	if got.UpdatedAt != expected.UpdatedAt {
		t.Errorf("got UpdatedAt %q, want %q", got.UpdatedAt, expected.UpdatedAt)
	}

	// Check inner slice details
	if len(got.Details) != len(expected.Details) {
		t.Fatalf("got %d details, want %d", len(got.Details), len(expected.Details))
	}
	if !reflect.DeepEqual(got.Details[0], expected.Details[0]) {
		t.Errorf("got Details[0] %+v, want %+v", got.Details[0], expected.Details[0])
	}
}

func TestGeneratedOrderConversion(t *testing.T) {
	ctx := context.Background()
	src := &source.SrcOrder{
		OrderID: "ORD-001",
		Amount:  99.99,
		Items: []source.SrcItem{
			{SKU: "item-1", Quantity: 2},
		},
	}

	// The generated function is currently empty, so all fields should be zero-valued.
	expected := &destination.DstOrder{}

	got, err := generated.ConvertSrcOrderToDstOrder(ctx, src)
	if err != nil {
		t.Fatalf("ConvertSrcOrderToDstOrder() failed: %v", err)
	}

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("ConvertSrcOrderToDstOrder() mismatch (-want +got):\n%s", diff)
	}
}
