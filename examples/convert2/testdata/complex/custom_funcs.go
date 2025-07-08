package complex

import (
	"context"
	"strings"
	"time"

	"example.com/convert2/internal/model"
)

// CombineFullName example 'using' function
func CombineFullName(ec model.ErrorCollectorInterface, srcUser UserRequest) string {
	// In a real scenario, ec might be used if srcUser fields were optional and required
	// For example: if srcUser.FirstName == "" { ec.Add("FirstName is required for FullName") }
	return srcUser.FirstName + " " + srcUser.LastName
}

// ConvertAddressWithCountryCtx example 'using' function with context
func ConvertAddressWithCountryCtx(ctx context.Context, ec model.ErrorCollectorInterface, srcAddr *AddressRequest) *Address {
	if srcAddr == nil {
		return nil
	}
	country := "Default Country"
	if val, ok := ctx.Value("country").(string); ok {
		country = val
	}
	return &Address{
		FullStreet: srcAddr.Street,
		CityName:   srcAddr.City,
		PostalCode: srcAddr.Zip,
		Country:    country,
	}
}

// PtrStringToString converts *string to string, returning empty if nil.
func PtrStringToString(ec model.ErrorCollectorInterface, notes *string) string {
	if notes == nil {
		return ""
	}
	return *notes
}

// PtrBoolToBool converts *bool to bool, returning false if nil.
func PtrBoolToBool(ec model.ErrorCollectorInterface, val *bool) bool {
    if val == nil {
        return false // Default value for bool
    }
    return *val
}


// TimeToTime is a dummy 'using' function for time.Time -> time.Time if one were needed for some reason
// (e.g. to enforce a specific timezone or format, though that's usually for string conversion)
func TimeToTime(ec model.ErrorCollectorInterface, t time.Time) time.Time {
	return t // No change, just to test 'using' with external types
}

// ValidateMyInput example validator function
func ValidateMyInput(ec model.ErrorCollectorInterface, input *ValidatedInput) {
	if input == nil {
		ec.Add("ValidatedInput cannot be nil")
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		ec.Add("Name cannot be empty")
	}
	if input.Value < 0 {
		ec.Addf("Value cannot be negative, got %d", input.Value)
	}
}

// WholeStructConversionUsingFunc example 'using' for a whole struct pair
func WholeStructConversionUsingFunc(ec model.ErrorCollectorInterface, src SourceUsingStruct) DestUsingStruct {
	return DestUsingStruct{
		ProcessedData: "Processed: " + src.Data,
		Timestamp:     time.Now(),
	}
}

// UppercaseOrderTag an example for a slice element conversion (if we had 'usingElem' or similar)
// For now, this would be used if the 'using' was on the whole slice field.
func UppercaseOrderTag(ec model.ErrorCollectorInterface, tag string) string {
	return strings.ToUpper(tag)
}

// ConvertUserIDToString example for underlying types
func ConvertUserIDToString(ec model.ErrorCollectorInterface, id UserID) string {
    return string(id)
}

func ConvertStringToUserID(ec model.ErrorCollectorInterface, s string) UserID {
    if s == "INVALID" {
        ec.Add("invalid user id string")
        return UserID("")
    }
    return UserID(s)
}

func ConvertOrderStatusToInt(ec model.ErrorCollectorInterface, status OrderStatus) int {
	return int(status)
}

func ConvertIntToOrderStatus(ec model.ErrorCollectorInterface, val int) OrderStatus {
	if val < int(StatusPending) || val > int(StatusDelivered) {
		ec.Addf("invalid order status value: %d", val)
		return StatusPending // return a default
	}
	return OrderStatus(val)
}
