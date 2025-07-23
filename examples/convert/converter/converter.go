package converter

import (
	"context"
	"fmt"
	"time"

	"example.com/convert/models/destination"
	"example.com/convert/models/source"
)

// translateDescription is a helper function simulating internal processing.
func translateDescription(ctx context.Context, text string, targetLang string) string {
	if targetLang == "jp" {
		return "翻訳済み (JP): " + text
	}
	return text
}

// --- User Conversion Functions ---

// ConvertSrcUserToDstUser converts a source.SrcUser to a destination.DstUser.
func ConvertSrcUserToDstUser(ctx context.Context, src source.SrcUser) destination.DstUser {
	if ctx == nil {
		ctx = context.Background()
	}

	dst := destination.DstUser{}

	dst.UserID = fmt.Sprintf("user-%d", src.ID)
	dst.FullName = src.FirstName + " " + src.LastName
	dst.Address = convertSrcAddressToDstAddress(ctx, src.SrcAddress)
	dst.Contact = convertSrcContactToDstContact(ctx, src.ContactInfo)

	if src.Details != nil {
		dst.Details = make([]destination.DstInternalDetail, len(src.Details))
		for i, sDetail := range src.Details {
			dst.Details[i] = convertSrcInternalDetailToDstInternalDetail(ctx, sDetail)
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

// convertSrcAddressToDstAddress converts source.SrcAddress to destination.DstAddress.
func convertSrcAddressToDstAddress(ctx context.Context, src source.SrcAddress) destination.DstAddress {
	return destination.DstAddress{
		FullStreet: src.Street,
		CityName:   src.City,
	}
}

// convertSrcContactToDstContact converts source.SrcContact to destination.DstContact.
func convertSrcContactToDstContact(ctx context.Context, src source.SrcContact) destination.DstContact {
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

// convertSrcInternalDetailToDstInternalDetail converts source.SrcInternalDetail to destination.DstInternalDetail.
func convertSrcInternalDetailToDstInternalDetail(ctx context.Context, src source.SrcInternalDetail) destination.DstInternalDetail {
	localizedDescription := translateDescription(ctx, src.Description, "jp")
	return destination.DstInternalDetail{
		ItemCode:      src.Code,
		LocalizedDesc: localizedDescription,
	}
}

// --- Order Conversion Functions ---

// ConvertSrcOrderToDstOrder converts a source.SrcOrder to a destination.DstOrder.
func ConvertSrcOrderToDstOrder(ctx context.Context, src source.SrcOrder) destination.DstOrder {
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
			dst.LineItems[i] = convertSrcItemToDstItem(ctx, sItem)
		}
	}
	return dst
}

// convertSrcItemToDstItem converts source.SrcItem to destination.DstItem.
func convertSrcItemToDstItem(ctx context.Context, src source.SrcItem) destination.DstItem {
	return destination.DstItem{
		ProductCode: src.SKU,
		Count:       src.Quantity,
	}
}
