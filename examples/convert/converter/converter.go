package converter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"example.com/convert/models" // Assuming your module path is example.com/convert
)

// translateDescription is a helper function simulating internal processing.
// In a real scenario, this could be a more complex logic, e.g., calling a translation service.
func translateDescription(ctx context.Context, text string, targetLang string) string {
	// Simulate a call to a translation service or internal logic
	// For demonstration, we'll just prepend a string.
	// The context could be used here for deadlines, cancellation, or passing request-scoped values.
	if targetLang == "jp" {
		return "翻訳済み (JP): " + text
	}
	return text
}

// --- User Conversion Functions ---

// ConvertUser converts a models.SrcUser to a models.DstUser.
// This function is EXPORTED because DstUser is specified as a "top-level type".
func ConvertUser(ctx context.Context, src models.SrcUser) models.DstUser {
	if ctx == nil {
		ctx = context.Background() // Ensure context is never nil
	}

	dst := models.DstUser{}

	// ID: int64 to string with formatting
	dst.UserID = fmt.Sprintf("user-%d", src.ID)

	// FullName: Combine FirstName and LastName
	dst.FullName = src.FirstName + " " + src.LastName

	// Address: Embedded struct conversion (calls unexported converter)
	dst.Address = srcAddressToDstAddress(ctx, src.SrcAddress)

	// Contact: Nested struct conversion (calls unexported converter)
	dst.Contact = srcContactToDstContact(ctx, src.ContactInfo)

	// Details: Slice of structs conversion (calls unexported converter for elements)
	if src.Details != nil {
		dst.Details = make([]models.DstInternalDetail, len(src.Details))
		for i, sDetail := range src.Details {
			dst.Details[i] = srcInternalDetailToDstInternalDetail(ctx, sDetail)
		}
	}

	// CreatedAt: time.Time to string
	dst.CreatedAt = src.CreatedAt.Format(time.RFC3339)

	// UpdatedAt: *time.Time to string (handle nil)
	if src.UpdatedAt != nil {
		dst.UpdatedAt = src.UpdatedAt.Format(time.RFC3339)
	} else {
		dst.UpdatedAt = "" // Or handle as per specific requirements for nil
	}

	return dst
}

// srcAddressToDstAddress converts models.SrcAddress to models.DstAddress.
// This function is UNEXPORTED as models.DstAddress is not a "top-level type" by itself,
// but rather a component of DstUser.
func srcAddressToDstAddress(ctx context.Context, src models.SrcAddress) models.DstAddress {
	return models.DstAddress{
		FullStreet: src.Street, // Renamed
		CityName:   src.City,   // Renamed
	}
}

// srcContactToDstContact converts models.SrcContact to models.DstContact.
// This function is UNEXPORTED.
func srcContactToDstContact(ctx context.Context, src models.SrcContact) models.DstContact {
	dst := models.DstContact{
		EmailAddress: src.Email, // Renamed
	}
	if src.Phone != nil {
		dst.PhoneNumber = *src.Phone // Pointer to value
	} else {
		dst.PhoneNumber = "N/A" // Default value for nil phone
	}
	return dst
}

// srcInternalDetailToDstInternalDetail converts models.SrcInternalDetail to models.DstInternalDetail.
// This function is UNEXPORTED and includes internal processing.
func srcInternalDetailToDstInternalDetail(ctx context.Context, src models.SrcInternalDetail) models.DstInternalDetail {
	// Simulate internal processing (e.g., translation)
	localizedDescription := translateDescription(ctx, src.Description, "jp")

	return models.DstInternalDetail{
		ItemCode:        src.Code,               // Renamed
		LocalizedDesc: localizedDescription, // Processed and renamed
	}
}

// --- Order Conversion Functions ---

// ConvertOrder converts a models.SrcOrder to a models.DstOrder.
// This function is EXPORTED because DstOrder is specified as a "top-level type".
func ConvertOrder(ctx context.Context, src models.SrcOrder) models.DstOrder {
	if ctx == nil {
		ctx = context.Background() // Ensure context is never nil
	}

	dst := models.DstOrder{
		ID:          src.OrderID,
		TotalAmount: src.Amount,
	}

	if src.Items != nil {
		dst.LineItems = make([]models.DstItem, len(src.Items)) // Renamed field: Items -> LineItems
		for i, sItem := range src.Items {
			dst.LineItems[i] = srcItemToDstItem(ctx, sItem) // Calls unexported converter
		}
	}
	return dst
}

// srcItemToDstItem converts models.SrcItem to models.DstItem.
// This function is UNEXPORTED.
func srcItemToDstItem(ctx context.Context, src models.SrcItem) models.DstItem {
	return models.DstItem{
		ProductCode: src.SKU,    // Renamed
		Count:       src.Quantity, // Renamed
	}
}

// Example of a function that might be generated or defined if SrcUser itself
// needed to be converted from another type, perhaps with different rules.
// For now, this is just to show how one might organize multiple converters.
// func ConvertLegacyUserToSrcUser(ctx context.Context, legacy LegacyUser) models.SrcUser {
//    // ... conversion logic ...
//    return models.SrcUser{}
//}
