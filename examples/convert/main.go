package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"example.com/convert/converter" // Adjust module path if different
	"example.com/convert/models"    // Adjust module path if different
)

func main() {
	// Part 1: Run existing examples
	runConversionExamples()

	// Part 2: Generator Prototype
	fmt.Println("\n--- Generator Prototype ---")
	generateConverterPrototype()
}

func runConversionExamples() {
	ctx := context.Background() // Parent context
	// --- Example 1: User Conversion ---
	fmt.Println("--- User Conversion Example ---")
	phone := "123-456-7890"
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
		CreatedAt: time.Now().Add(-24 * time.Hour), // Yesterday
		UpdatedAt: func() *time.Time { t := time.Now(); return &t }(),
	}

	// Perform the conversion
	dstUser := converter.ConvertUser(ctx, srcUser)

	// Print the results (using JSON for readability)
	fmt.Println("Source User:")
	printJSON(srcUser)
	fmt.Println("\nDestination User:")
	printJSON(dstUser)
	fmt.Println("------------------------------")

	// --- Example 2: Order Conversion ---
	fmt.Println("--- Order Conversion Example ---")
	srcOrder := models.SrcOrder{
		OrderID: "ORD-001",
		Amount:  199.99,
		Items: []models.SrcItem{
			{SKU: "ITEM001", Quantity: 2},
			{SKU: "ITEM002", Quantity: 1},
		},
	}

	// Perform the conversion
	dstOrder := converter.ConvertOrder(ctx, srcOrder)

	// Print the results
	fmt.Println("Source Order:")
	printJSON(srcOrder)
	fmt.Println("\nDestination Order:")
	printJSON(dstOrder)
	fmt.Println("------------------------------")

	// --- Example 3: User with nil fields ---
	fmt.Println("--- User Conversion with Nil Phone and UpdatedAt ---")
	srcUserNil := models.SrcUser{
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
		CreatedAt: time.Now().Add(-48 * time.Hour),
		UpdatedAt: nil, // Nil UpdatedAt
	}

	dstUserNil := converter.ConvertUser(ctx, srcUserNil)
	fmt.Println("Source User (with nils):")
	printJSON(srcUserNil)
	fmt.Println("\nDestination User (with nils handled):")
	printJSON(dstUserNil)
	fmt.Println("------------------------------")
}

// printJSON is a helper to pretty-print structs as JSON.
func printJSON(data interface{}) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling to JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonData))
}
