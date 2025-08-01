package converter

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"
)

func TestConvertUser(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	phone := "123-456-7890"
	updatedAt := now.Add(time.Hour)

	srcUser := source.SrcUser{
		ID:        101,
		FirstName: "John",
		LastName:  "Doe",
		Address: source.SrcAddress{
			Street: "123 Main St",
			City:   "Anytown",
		},
		ContactInfo: source.SrcContact{
			Email: "john.doe@example.com",
			Phone: &phone,
		},
		Details: []source.SrcInternalDetail{
			{Code: 1, Description: "Needs setup"},
			{Code: 2, Description: "Pending review"},
		},
		CreatedAt: now,
		UpdatedAt: &updatedAt,
	}

	expectedDstUser := destination.DstUser{
		UserID:   "user-101",
		FullName: "John Doe",
		Address: destination.DstAddress{
			FullStreet: "123 Main St",
			CityName:   "Anytown",
		},
		Contact: destination.DstContact{
			EmailAddress: "john.doe@example.com",
			PhoneNumber:  "123-456-7890",
		},
		Details: []destination.DstInternalDetail{
			{ItemCode: 1, LocalizedDesc: "翻訳済み (JP): Needs setup"},
			{ItemCode: 2, LocalizedDesc: "翻訳済み (JP): Pending review"},
		},
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}

	dstUser := ConvertUser(ctx, srcUser)

	if !reflect.DeepEqual(dstUser, expectedDstUser) {
		t.Errorf("ConvertUser() got = %v, want %v", dstUser, expectedDstUser)
	}
}

func TestConvertUser_NilFields(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	srcUser := source.SrcUser{
		ID:        102,
		FirstName: "Jane",
		LastName:  "Doe",
		Address: source.SrcAddress{
			Street: "456 Oak St",
			City:   "Otherville",
		},
		ContactInfo: source.SrcContact{
			Email: "jane.doe@example.com",
			Phone: nil, // Nil phone
		},
		Details: []source.SrcInternalDetail{
			{Code: 3, Description: "Urgent"},
		},
		CreatedAt: now,
		UpdatedAt: nil, // Nil UpdatedAt
	}

	expectedDstUser := destination.DstUser{
		UserID:   "user-102",
		FullName: "Jane Doe",
		Address: destination.DstAddress{
			FullStreet: "456 Oak St",
			CityName:   "Otherville",
		},
		Contact: destination.DstContact{
			EmailAddress: "jane.doe@example.com",
			PhoneNumber:  "N/A", // Default for nil phone
		},
		Details: []destination.DstInternalDetail{
			{ItemCode: 3, LocalizedDesc: "翻訳済み (JP): Urgent"},
		},
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: "", // Empty string for nil UpdatedAt
	}

	dstUser := ConvertUser(ctx, srcUser)

	if !reflect.DeepEqual(dstUser, expectedDstUser) {
		t.Errorf("ConvertUser() with nil fields got = %v, want %v", dstUser, expectedDstUser)
	}
}

func TestConvertOrder(t *testing.T) {
	ctx := context.Background()
	srcOrder := source.SrcOrder{
		OrderID: "ORD-001",
		Amount:  199.99,
		Items: []source.SrcItem{
			{SKU: "ITEM001", Quantity: 2},
			{SKU: "ITEM002", Quantity: 1},
		},
	}

	expectedDstOrder := destination.DstOrder{
		ID:          "ORD-001",
		TotalAmount: 199.99,
		LineItems: []destination.DstItem{
			{ProductCode: "ITEM001", Count: 2},
			{ProductCode: "ITEM002", Count: 1},
		},
	}

	dstOrder := ConvertOrder(ctx, srcOrder)

	if !reflect.DeepEqual(dstOrder, expectedDstOrder) {
		t.Errorf("ConvertOrder() got = %v, want %v", dstOrder, expectedDstOrder)
	}
}

func TestSrcAddressToDstAddress(t *testing.T) {
	ctx := context.Background()
	src := source.SrcAddress{Street: "123 Main St", City: "Anytown"}
	expected := destination.DstAddress{FullStreet: "123 Main St", CityName: "Anytown"}
	got := srcAddressToDstAddress(ctx, src)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("srcAddressToDstAddress() = %v, want %v", got, expected)
	}
}

func TestSrcContactToDstContact(t *testing.T) {
	ctx := context.Background()
	phone := "555-0100"
	tests := []struct {
		name     string
		src      source.SrcContact
		expected destination.DstContact
	}{
		{
			name:     "with phone",
			src:      source.SrcContact{Email: "test@example.com", Phone: &phone},
			expected: destination.DstContact{EmailAddress: "test@example.com", PhoneNumber: "555-0100"},
		},
		{
			name:     "nil phone",
			src:      source.SrcContact{Email: "test2@example.com", Phone: nil},
			expected: destination.DstContact{EmailAddress: "test2@example.com", PhoneNumber: "N/A"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := srcContactToDstContact(ctx, tt.src); !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("srcContactToDstContact() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSrcInternalDetailToDstInternalDetail(t *testing.T) {
	ctx := context.Background()
	src := source.SrcInternalDetail{Code: 10, Description: "Test Desc"}
	expected := destination.DstInternalDetail{ItemCode: 10, LocalizedDesc: "翻訳済み (JP): Test Desc"}
	got := srcInternalDetailToDstInternalDetail(ctx, src)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("srcInternalDetailToDstInternalDetail() = %v, want %v", got, expected)
	}
}

func TestSrcItemToDstItem(t *testing.T) {
	ctx := context.Background()
	src := source.SrcItem{SKU: "SKU007", Quantity: 3}
	expected := destination.DstItem{ProductCode: "SKU007", Count: 3}
	got := srcItemToDstItem(ctx, src)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("srcItemToDstItem() = %v, want %v", got, expected)
	}
}
