package converter

import (
	"context"
	"reflect"
	"testing"
	"time"

	"example.com/convert/models"
)

func TestConvertUser(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	phone := "123-456-7890"
	updatedAt := now.Add(time.Hour)

	srcUser := models.SrcUser{
		ID:        101,
		FirstName: "John",
		LastName:  "Doe",
		SrcAddress: models.SrcAddress{
			Street: "123 Main St",
			City:   "Anytown",
		},
		ContactInfo: models.SrcContact{
			Email: "john.doe@example.com",
			Phone: &phone,
		},
		Details: []models.SrcInternalDetail{
			{Code: 1, Description: "Needs setup"},
			{Code: 2, Description: "Pending review"},
		},
		CreatedAt: now,
		UpdatedAt: &updatedAt,
	}

	expectedDstUser := models.DstUser{
		UserID:   "user-101",
		FullName: "John Doe",
		Address: models.DstAddress{
			FullStreet: "123 Main St",
			CityName:   "Anytown",
		},
		Contact: models.DstContact{
			EmailAddress: "john.doe@example.com",
			PhoneNumber:  "123-456-7890",
		},
		Details: []models.DstInternalDetail{
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

	srcUser := models.SrcUser{
		ID:        102,
		FirstName: "Jane",
		LastName:  "Doe",
		SrcAddress: models.SrcAddress{
			Street: "456 Oak St",
			City:   "Otherville",
		},
		ContactInfo: models.SrcContact{
			Email: "jane.doe@example.com",
			Phone: nil, // Nil phone
		},
		Details: []models.SrcInternalDetail{
			{Code: 3, Description: "Urgent"},
		},
		CreatedAt: now,
		UpdatedAt: nil, // Nil UpdatedAt
	}

	expectedDstUser := models.DstUser{
		UserID:   "user-102",
		FullName: "Jane Doe",
		Address: models.DstAddress{
			FullStreet: "456 Oak St",
			CityName:   "Otherville",
		},
		Contact: models.DstContact{
			EmailAddress: "jane.doe@example.com",
			PhoneNumber:  "N/A", // Default for nil phone
		},
		Details: []models.DstInternalDetail{
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
	srcOrder := models.SrcOrder{
		OrderID: "ORD-001",
		Amount:  199.99,
		Items: []models.SrcItem{
			{SKU: "ITEM001", Quantity: 2},
			{SKU: "ITEM002", Quantity: 1},
		},
	}

	expectedDstOrder := models.DstOrder{
		ID:          "ORD-001",
		TotalAmount: 199.99,
		LineItems: []models.DstItem{
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
	src := models.SrcAddress{Street: "123 Main St", City: "Anytown"}
	expected := models.DstAddress{FullStreet: "123 Main St", CityName: "Anytown"}
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
		src      models.SrcContact
		expected models.DstContact
	}{
		{
			name:     "with phone",
			src:      models.SrcContact{Email: "test@example.com", Phone: &phone},
			expected: models.DstContact{EmailAddress: "test@example.com", PhoneNumber: "555-0100"},
		},
		{
			name:     "nil phone",
			src:      models.SrcContact{Email: "test2@example.com", Phone: nil},
			expected: models.DstContact{EmailAddress: "test2@example.com", PhoneNumber: "N/A"},
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
	src := models.SrcInternalDetail{Code: 10, Description: "Test Desc"}
	expected := models.DstInternalDetail{ItemCode: 10, LocalizedDesc: "翻訳済み (JP): Test Desc"}
	got := srcInternalDetailToDstInternalDetail(ctx, src)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("srcInternalDetailToDstInternalDetail() = %v, want %v", got, expected)
	}
}

func TestSrcItemToDstItem(t *testing.T) {
	ctx := context.Background()
	src := models.SrcItem{SKU: "SKU007", Quantity: 3}
	expected := models.DstItem{ProductCode: "SKU007", Count: 3}
	got := srcItemToDstItem(ctx, src)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("srcItemToDstItem() = %v, want %v", got, expected)
	}
}
