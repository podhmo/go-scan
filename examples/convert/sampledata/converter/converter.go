package converter

import (
	"context"
	"fmt"
	"time"

	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"
)

// This file contains manually written converter functions.
// These would be used for complex conversions that the generator doesn't handle,
// or as custom functions invoked by the generator via tags in the future.

// translateDescription is a helper function simulating internal processing.
func translateDescription(ctx context.Context, text string, targetLang string) string {
	if targetLang == "jp" {
		return "翻訳済み (JP): " + text
	}
	return text
}

// --- User Conversion Functions ---

// This is a manually implemented converter. The test `TestConvertUser` verifies its behavior.
// The generated code will produce `ConvertSrcUserToDstUser` and `convertSrcUserToDstUser` in a separate file.
// To avoid conflicts, we could name this differently, but for the test, we rely on the test calling this specific implementation.
func ConvertUser(ctx context.Context, src source.SrcUser) destination.DstUser {
	if ctx == nil {
		ctx = context.Background()
	}

	dst := destination.DstUser{}

	dst.UserID = fmt.Sprintf("user-%d", src.ID)
	dst.FullName = src.FirstName + " " + src.LastName
	dst.Address = srcAddressToDstAddress(ctx, src.Address)
	dst.Contact = srcContactToDstContact(ctx, src.ContactInfo)

	if src.Details != nil {
		dst.Details = make([]destination.DstInternalDetail, len(src.Details))
		for i, sDetail := range src.Details {
			dst.Details[i] = srcInternalDetailToDstInternalDetail(ctx, sDetail)
		}
	}

	dst.CreatedAt = src.CreatedAt.Format(time.RFC3339)

	if src.UpdatedAt != nil {
		dst.UpdatedAt = src.UpdatedAt.Format(time.RFC3339)
	} else {
		dst.UpdatedAt = ""
	}

	return dst
}

func srcAddressToDstAddress(ctx context.Context, src source.SrcAddress) destination.DstAddress {
	return destination.DstAddress{
		FullStreet: src.Street,
		CityName:   src.City,
	}
}

func srcContactToDstContact(ctx context.Context, src source.SrcContact) destination.DstContact {
	dst := destination.DstContact{
		EmailAddress: src.Email,
	}
	if src.Phone != nil {
		dst.PhoneNumber = *src.Phone
	} else {
		dst.PhoneNumber = "N/A"
	}
	return dst
}

func srcInternalDetailToDstInternalDetail(ctx context.Context, src source.SrcInternalDetail) destination.DstInternalDetail {
	localizedDescription := translateDescription(ctx, src.Description, "jp")
	return destination.DstInternalDetail{
		ItemCode:      src.Code,
		LocalizedDesc: localizedDescription,
	}
}

// --- Order Conversion Functions ---

func ConvertOrder(ctx context.Context, src source.SrcOrder) destination.DstOrder {
	if ctx == nil {
		ctx = context.Background()
	}

	dst := destination.DstOrder{
		ID:          src.OrderID,
		TotalAmount: src.Amount,
	}

	if src.Items != nil {
		dst.LineItems = make([]destination.DstItem, len(src.Items))
		for i, sItem := range src.Items {
			dst.LineItems[i] = srcItemToDstItem(ctx, sItem)
		}
	}
	return dst
}

func srcItemToDstItem(ctx context.Context, src source.SrcItem) destination.DstItem {
	return destination.DstItem{
		ProductCode: src.SKU,
		Count:       src.Quantity,
	}
}
