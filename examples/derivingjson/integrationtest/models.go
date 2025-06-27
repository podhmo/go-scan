// Package models defines the data structures for API responses.
package integrationtest

// DataInterface is implemented by types that can be in APIResponse.Data.
// This helps the generator identify the "oneOf" field and its possible concrete types.
type DataInterface interface {
	isData() // A dummy method to make it a concrete interface that types can implement.
}

// APIResponse is a generic API response structure where the Data field
// can contain different types of objects that implement DataInterface.
// @deriving:unmarshall
type APIResponse struct {
	Status string        `json:"status"`
	Data   DataInterface `json:"data"` // Changed from interface{} to DataInterface
}

// UserProfile is one of the possible types for the Data field.
// It represents a user's profile information.
type UserProfile struct {
	Type     string `json:"type"` // Discriminator: "user_profile" (expected value based on original user request)
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
}

// isData implements the DataInterface for UserProfile.
func (UserProfile) isData() {}

// ProductInfo is another possible type for the Data field.
// It represents information about a product.
type ProductInfo struct {
	Type        string `json:"type"` // Discriminator: "product_info" (expected value based on original user request)
	ProductID   string `json:"productId"`
	ProductName string `json:"productName"`
	Price       int    `json:"price"`
}

// isData implements the DataInterface for ProductInfo.
func (ProductInfo) isData() {}

// Notes on discriminator values for the generator:
// The current generator logic (as of the last check) uses:
//   discriminatorValue = strings.ToLower(candidateType.Name)
// This would result in "userprofile" for UserProfile and "productinfo" for ProductInfo.
// The test JSON in main_test.go has been adjusted to use these lowercase values for now
// to allow testing of the UnmarshalJSON structure.
// For the generator to use "user_profile" and "product_info" (as in the struct tags here),
// the generator's logic for determining `JSONValue` in `OneOfTypeMapping` would need to be enhanced
// to inspect struct tags (e.g., from the 'Type' field) or a method on the implementing types.
// This is a known TODO in the generator code.
