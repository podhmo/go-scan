package complex

import "time"

// Basic types for underlying type tests
type UserID string
type ProductID string
type OrderStatus int

const (
	StatusPending OrderStatus = iota
	StatusShipped
	StatusDelivered
)

type AddressRequest struct {
	Street string
	City   string
	Zip    string
}

type UserRequest struct {
	ID          string // Added for UserID conversion
	FirstName   string
	LastName    string
	Email       string `convert:"UserEmail"` // Different name
	RawAddress  *AddressRequest
	Nicknames   []string
	Permissions map[string]bool
}

type Address struct {
	FullStreet  string `convert:"Street"`
	CityName    string `convert:"City"`
	PostalCode  string `convert:"Zip"`
	Country     string // This field's conversion is handled by the 'using' on User.HomeAddress
}

type User struct {
	ID          UserID            `convert:"ID,using=complex.ConvertStringToUserID"`
	FullName    string            `convert:"_struct,using=complex.CombineFullName"` // Uses the whole UserRequest
	UserEmail   string            // Direct map from UserRequest.Email via its tag (UserRequest.Email has `convert:"UserEmail"`)
	HomeAddress *Address          `convert:"RawAddress,using=complex.ConvertAddressWithCountryCtx"`
	Nicknames   []string          // Direct map
	Roles       map[string]string // Placeholder for map test
	LastLogin   time.Time         `convert:",using=complex.TimeToTime"` // Example for time.Time field
}

type OrderItemRequest struct {
	ProdID ProductID
	Amount int
	Notes  *string
}

type OrderItem struct {
	ItemProductID ProductID `convert:"ProdID"`
	Quantity      int       `convert:"Amount"`
	Comment       string    `convert:"Notes,using=complex.PtrStringToString"`
}

type OrderRequest struct {
	CustomerEmail string
	Items         []OrderItemRequest
	OrderTags     []string
	IsRush        *bool
	StatusCode    int // For testing conversion to OrderStatus
}

type Order struct {
	OrderID       string      `convert:"-"` // Usually not from request
	CustomerEmail string      // Direct
	Status        OrderStatus `convert:"StatusCode,using=complex.ConvertIntToOrderStatus"`
	LineItems     []OrderItem // Slice of structs
	Tags          []string    `convert:"OrderTags"`
	RushOrder     bool        `convert:"IsRush,using=complex.PtrBoolToBool"`
	CreatedAt     time.Time   `convert:"-"` // Typically set by system
}

// For validator testing
type ValidatedInput struct {
	Name  string
	Value int
}

// For testing using a function for the whole struct conversion
type SourceUsingStruct struct {
	Data string
}

type DestUsingStruct struct {
	ProcessedData string
	Timestamp     time.Time
}

// For testing dot imports (manual setup in a sub-package might be needed for true test)
// type ExternalType time.Time // Assume this is in another package dot-imported
//
// type DotImportTestSource struct {
//	ImportedTime ExternalType
// }
//
// type DotImportTestDest struct {
//	 StandardTime time.Time
// }
