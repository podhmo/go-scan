package complex_test

import (
	"context"
	"strings"
	"testing"
	"time"

	// "example.com/convert2/internal/model" // No longer needed directly in test
	"example.com/convert2/testdata/complex/gen" // Import the generated package
	// Assuming 'complex' is the package name of models.go and custom_funcs.go
	// If custom_funcs are in 'complex_test', this needs adjustment or they need to be in 'complex'.
	// For this test, assume custom_funcs.go is part of package 'complex'.
	// We are testing the _generated_ code, which will be in package 'gen'.
	// The models and custom funcs are in package 'complex'.
	. "example.com/convert2/testdata/complex"
)

func TestUserConversion(t *testing.T) {
	src := UserRequest{
		FirstName:  "John",
		LastName:   "Doe",
		Email:      "john.doe@example.com",
		RawAddress: &AddressRequest{Street: "123 Main St", City: "Anytown", Zip: "12345"},
		Nicknames:  []string{"Johnny", "JD"},
	}

	// Modify User struct in models.go to include expected tags for 'using'
	// User.FullName: `convert:",using=complex.CombineFullName"`
	// User.HomeAddress: `convert:"RawAddress,using=complex.ConvertAddressWithCountryCtx"`
	// This means custom functions must be prefixed with their package if not in the generated package.
	// The generator should handle this by ensuring `complex.CombineFullName` is called.

	ctx := context.WithValue(context.Background(), "country", "Testland")

	// Assuming your top-level generated function is named ConvertUserRequestToUser
	// and is in the "gen" package.
	dest, err := gen.ConvertUserRequestToUser(ctx, src)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if dest.FullName != "John Doe" {
		t.Errorf("Expected FullName '%s', got '%s'", "John Doe", dest.FullName)
	}
	if dest.UserEmail != src.Email {
		t.Errorf("Expected UserEmail '%s', got '%s'", src.Email, dest.UserEmail)
	}
	if dest.HomeAddress == nil {
		t.Fatalf("Expected HomeAddress to be non-nil")
	}
	if dest.HomeAddress.FullStreet != src.RawAddress.Street {
		t.Errorf("Expected FullStreet '%s', got '%s'", src.RawAddress.Street, dest.HomeAddress.FullStreet)
	}
	if dest.HomeAddress.CityName != src.RawAddress.City {
		t.Errorf("Expected CityName '%s', got '%s'", src.RawAddress.City, dest.HomeAddress.CityName)
	}
	if dest.HomeAddress.PostalCode != src.RawAddress.Zip {
		t.Errorf("Expected PostalCode '%s', got '%s'", src.RawAddress.Zip, dest.HomeAddress.PostalCode)
	}
	if dest.HomeAddress.Country != "Testland" {
		t.Errorf("Expected Country 'Testland' (from context), got '%s'", dest.HomeAddress.Country)
	}

	if len(dest.Nicknames) != len(src.Nicknames) {
		t.Fatalf("Expected %d nicknames, got %d", len(src.Nicknames), len(dest.Nicknames))
	}
	for i, v := range src.Nicknames {
		if dest.Nicknames[i] != v {
			t.Errorf("Expected Nicknames[%d] to be '%s', got '%s'", i, v, dest.Nicknames[i])
		}
	}
	// Add UserID and Roles test once their conversion is defined via tags or global rules
}

func TestAddressStandaloneConversion(t *testing.T) {
	src := AddressRequest{Street: "456 Sub St", City: "Otherville", Zip: "67890"}
	ctx := context.WithValue(context.Background(), "country", "ContextCountry")

	// Assumes `convert:pair complex.AddressRequest -> complex.Address` and `using` on Country field
	// User.HomeAddress: `convert:"RawAddress,using=complex.ConvertAddressWithCountryCtx"`
	// The 'using' on Address.Country in models.go should be `convert:",using=complex.ConvertAddressWithCountryCtx"`
	// but this function actually converts the whole struct.
	// For this test, let's assume the 'using' is on the field Address.Country
	// Or, more likely, the pair AddressRequest -> Address uses a global 'using' or field tags.
	// The function ConvertAddressWithCountryCtx converts *AddressRequest -> *Address
	// So, the generated function ConvertAddressRequestToAddress will call it.
	// Let's assume Address.Country field has a using tag that somehow gets context.
	// For this specific test, a direct call to ConvertAddressWithCountryCtx might be what's generated
	// if there was a global rule: "complex.AddressRequest" -> "complex.Address", using=complex.ConvertAddressWithCountryCtx
	// However, ConvertAddressWithCountryCtx takes *AddressRequest. The generated function would be ConvertAddressRequestToAddress.
	// The generator needs to handle the * for the 'using' function argument.

	// Let's assume the field Address.Country has a tag like `convert:"-"` and we rely on a struct-level `using`
	// or the `ConvertAddressWithCountryCtx` is used for the field mapping from `UserRequest.RawAddress` to `User.HomeAddress`.

	// To test Address by itself, we'd need a ConvertAddressRequestToAddress.
	// If this uses ConvertAddressWithCountryCtx, the signature of that func is *AddressRequest -> *Address.
	// So, the generated ConvertAddressRequestToAddress would likely return *Address.

	// This test is tricky without seeing the exact field tags on Address.
	// Let's assume a simple direct field mapping for Address for this standalone test,
	// and Country is set via a field-level 'using' that needs context.
	// This means Address.Country would need `convert:",using=GetCountryFromContext"`
	// And GetCountryFromContext(ctx context.Context, ec *model.ErrorCollector, src AddressRequest) string
	// This is getting too complex.

	// Simpler: Assume ConvertAddressRequestToAddress is generated and internally it calls
	// ConvertAddressWithCountryCtx if a global rule `AddressRequest -> Address, using=ConvertAddressWithCountryCtx` exists.
	// The current ConvertAddressWithCountryCtx converts the whole struct.

	// The generator will create `addressRequestToAddress(ec, src)` which needs context for its sub-calls.
	// The top level ConvertAddressRequestToAddress(ctx, src) calls this.

	dest, err := gen.ConvertAddressRequestToAddress(ctx, src) // This should use the one that calls ConvertAddressWithCountryCtx
	if err != nil {
		t.Fatalf("Address conversion failed: %v", err)
	}
	// If ConvertAddressRequestToAddress returns Address (not *Address)
	if dest.FullStreet != src.Street {
		t.Errorf("Expected FullStreet '%s', got '%s'", src.Street, dest.FullStreet)
	}
	if dest.Country != "ContextCountry" {
		t.Errorf("Expected Country 'ContextCountry', got '%s'", dest.Country)
	}
}


func TestOrderConversion(t *testing.T) {
	isRush := true
	notes1 := "Fragile"
	src := OrderRequest{
		CustomerEmail: "order@example.com",
		Items: []OrderItemRequest{
			{ProdID: "PROD123", Amount: 2, Notes: &notes1},
			{ProdID: "PROD456", Amount: 1},
		},
		OrderTags: []string{"urgent", "gift"},
		IsRush:    &isRush,
	}

	// OrderItem.Comment `convert:"Notes,using=complex.PtrStringToString"`
	// Order.RushOrder `convert:"IsRush,using=complex.PtrBoolToBool"`
	// Order.Tags `convert:"OrderTags"`
	// OrderItem.ItemProductID `convert:"ProdID"`
	// OrderItem.Quantity `convert:"Amount"`


	ctx := context.Background()
	dest, err := gen.ConvertOrderRequestToOrder(ctx, src)
	if err != nil {
		t.Fatalf("Order conversion failed: %v", err)
	}

	if dest.CustomerEmail != src.CustomerEmail {
		t.Errorf("Expected CustomerEmail '%s', got '%s'", src.CustomerEmail, dest.CustomerEmail)
	}
	if len(dest.LineItems) != len(src.Items) {
		t.Fatalf("Expected %d line items, got %d", len(src.Items), len(dest.LineItems))
	}
	if dest.LineItems[0].ItemProductID != "PROD123" {
		t.Errorf("Expected ItemProductID '%s', got '%s'", "PROD123", dest.LineItems[0].ItemProductID)
	}
	if dest.LineItems[0].Quantity != 2 {
		t.Errorf("Expected Quantity %d, got %d", 2, dest.LineItems[0].Quantity)
	}
	if dest.LineItems[0].Comment != "Fragile" {
		t.Errorf("Expected Comment '%s', got '%s'", "Fragile", dest.LineItems[0].Comment)
	}
	if dest.LineItems[1].Comment != "" { // Nil *string should become empty string
		t.Errorf("Expected empty Comment for nil notes, got '%s'", dest.LineItems[1].Comment)
	}

	if len(dest.Tags) != len(src.OrderTags) || dest.Tags[0] != src.OrderTags[0] {
		t.Errorf("Tag mismatch: expected %v, got %v", src.OrderTags, dest.Tags)
	}
	if dest.RushOrder != true {
		t.Errorf("Expected RushOrder true, got %v", dest.RushOrder)
	}
	if dest.CreatedAt.IsZero() { // Assuming generator might set a default or it's part of a 'using'
		t.Log("Warning: Order.CreatedAt is zero. This might be expected if not set by converter.")
	}
	// Test OrderStatus if set by a 'using' or default
}

func TestValidatedInput(t *testing.T) {
	ctx := context.Background()

	// Test case 1: Valid input
	validSrc := ValidatedInput{Name: "Test", Value: 10}
	_, err := gen.ConvertValidatedInputToValidatedInput(ctx, validSrc)
	if err != nil {
		t.Errorf("Expected no error for valid input, got: %v", err)
	}

	// Test case 2: Invalid name
	invalidNameSrc := ValidatedInput{Name: " ", Value: 10}
	_, err = gen.ConvertValidatedInputToValidatedInput(ctx, invalidNameSrc)
	if err == nil {
		t.Errorf("Expected error for invalid name, got nil")
	} else {
		if !strings.Contains(err.Error(), "Name cannot be empty") {
			t.Errorf("Expected error message for name, got: %v", err.Error())
		}
		t.Logf("Got expected error for invalid name: %v", err)
	}

	// Test case 3: Invalid value
	invalidValueSrc := ValidatedInput{Name: "Test", Value: -5}
	_, err = gen.ConvertValidatedInputToValidatedInput(ctx, invalidValueSrc)
	if err == nil {
		t.Errorf("Expected error for invalid value, got nil")
	} else {
		if !strings.Contains(err.Error(), "Value cannot be negative") {
			t.Errorf("Expected error message for value, got: %v", err.Error())
		}
		t.Logf("Got expected error for invalid value: %v", err)
	}
}

func TestWholeStructUsingConversion(t *testing.T) {
	src := SourceUsingStruct{Data: "hello world"}
	ctx := context.Background()

	dest, err := gen.ConvertSourceUsingStructToDestUsingStruct(ctx, src)
	if err != nil {
		t.Fatalf("Whole struct conversion failed: %v", err)
	}

	expectedProcessedData := "Processed: hello world"
	if dest.ProcessedData != expectedProcessedData {
		t.Errorf("Expected ProcessedData '%s', got '%s'", expectedProcessedData, dest.ProcessedData)
	}
	if dest.Timestamp.IsZero() {
		t.Error("Expected Timestamp to be set, but it's zero")
	}
	if time.Since(dest.Timestamp) > time.Second { // Check if it's a recent time
		t.Errorf("Expected Timestamp to be recent, got %v", dest.Timestamp)
	}
}

// TODO: Add tests for:
// - Underlying type conversions not covered by fields (e.g. UserID -> string global rule)
// - More complex pointer to pointer conversions for structs.
// - Nil source struct for Convert* functions that take a value and return a pointer (should it panic or return nil with error?)
// - Error collector path correctness for nested structs and slices.
// - Import alias collision resolution if we can set up such a test case.
// - Dot import resolution (might require a more complex test setup with actual sub-packages).
// - Map conversions (once implemented).
