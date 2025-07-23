package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"example.com/convert/converter" // Adjust module path if different
	"example.com/convert/models"    // Adjust module path if different
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

func main() {
	// Part 1: Run existing examples
	runConversionExamples()

	// Part 2: Generator Prototype
	fmt.Println("\n--- Generator Prototype ---")
	if err := runGenerate(context.Background()); err != nil {
		log.Fatalf("Error running generator: %v", err)
	}
}

func runGenerate(ctx context.Context) error {
	modelsPath := "./models"
	s, err := goscan.New(goscan.WithWorkDir(modelsPath))
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Scan the package containing the models.
	pkg, err := s.ScanPackage(ctx, modelsPath)
	if err != nil {
		return fmt.Errorf("failed to scan package %s: %w", modelsPath, err)
	}

	return Generate(ctx, s, pkg)
}

// Generate produces converter code for the given package.
func Generate(ctx context.Context, s *goscan.Scanner, pkgInfo *scanner.PackageInfo) error {
	var srcUserType, dstUserType *scanner.TypeInfo
	var srcOrderType, dstOrderType *scanner.TypeInfo
	// We'll also need other types for sub-converters
	var srcAddressType, dstAddressType *scanner.TypeInfo
	var srcContactType, dstContactType *scanner.TypeInfo
	var srcInternalDetailType, dstInternalDetailType *scanner.TypeInfo
	var srcItemType, dstItemType *scanner.TypeInfo

	for _, t := range pkgInfo.Types {
		switch t.Name {
		case "SrcUser":
			srcUserType = t
		case "DstUser":
			dstUserType = t
		case "SrcOrder":
			srcOrderType = t
		case "DstOrder":
			dstOrderType = t
		case "SrcAddress":
			srcAddressType = t
		case "DstAddress":
			dstAddressType = t
		case "SrcContact":
			srcContactType = t
		case "DstContact":
			dstContactType = t
		case "SrcInternalDetail":
			srcInternalDetailType = t
		case "DstInternalDetail":
			dstInternalDetailType = t
		case "SrcItem":
			srcItemType = t
		case "DstItem":
			dstItemType = t
		}
	}

	if srcUserType == nil || dstUserType == nil || srcOrderType == nil || dstOrderType == nil {
		return fmt.Errorf("one or more top-level source or destination types not found in models package")
	}
	// Create a go-scan GoFile for the generated code.
	im := goscan.NewImportManager(pkgInfo)
	im.Add("context", "")
	im.Add("fmt", "")
	im.Add("time", "")
	im.Add(pkgInfo.ImportPath, "")

	// Create a string builder to accumulate the generated code.
	var sb strings.Builder

	// Generate User converter
	if srcUserType != nil && dstUserType != nil {
		generateStructConverter(&sb, srcUserType, dstUserType, pkgInfo)
	}
	// Generate Order converter
	if srcOrderType != nil && dstOrderType != nil {
		generateStructConverter(&sb, srcOrderType, dstOrderType, pkgInfo)
	}

	// Generate necessary sub-converters (helper functions)
	if srcAddressType != nil && dstAddressType != nil {
		generateStructConverter(&sb, srcAddressType, dstAddressType, pkgInfo)
	}
	if srcContactType != nil && dstContactType != nil {
		generateStructConverter(&sb, srcContactType, dstContactType, pkgInfo)
	}
	if srcInternalDetailType != nil && dstInternalDetailType != nil {
		generateStructConverter(&sb, srcInternalDetailType, dstInternalDetailType, pkgInfo)
	}
	if srcItemType != nil && dstItemType != nil {
		generateStructConverter(&sb, srcItemType, dstItemType, pkgInfo)
	}

	// Add the translateDescription helper as it's used by the manual converter
	sb.WriteString(`
// translateDescription is a helper function simulating internal processing.
func translateDescription(ctx context.Context, text string, targetLang string) string {
	if targetLang == "jp" {
		return "翻訳済み (JP): " + text
	}
	return text
}
`)

	gf := goscan.GoFile{
		PackageName: "converter",
		Imports:     im.Imports(),
		CodeSet:     sb.String(),
	}

	// Use goscan.SaveGoFile to allow interception by scantest
	converterPkgDir := goscan.NewPackageDirectory(filepath.Join(pkgInfo.Path, "..", "converter"), "converter")
	return converterPkgDir.SaveGoFile(ctx, gf, "generated_converters.go")
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

// generateStructConverter generates conversion function code for a single struct pair.
func generateStructConverter(sb *strings.Builder, srcType *scanner.TypeInfo, dstType *scanner.TypeInfo, pkgInfo *scanner.PackageInfo) {
	if srcType.Struct == nil || dstType.Struct == nil {
		log.Printf("Warning: Skipping generation for %s -> %s as one or both are not structs.\n", srcType.Name, dstType.Name)
		return
	}

	// Determine function name (e.g., ConvertSrcUserToDstUser or srcAddressToDstAddress)
	// A simple heuristic: if Dst type starts with "Dst", assume it's a top-level exported converter.
	funcName := ""
	if strings.HasPrefix(dstType.Name, "Dst") && strings.ToUpper(dstType.Name[0:1]) == dstType.Name[0:1] { // Exported
		funcName = fmt.Sprintf("Convert%sTo%s", stripPrefix(srcType.Name, "Src"), dstType.Name)
	} else { // Unexported helper
		funcName = fmt.Sprintf("%sTo%s", camelCase(srcType.Name), strings.Title(dstType.Name))
	}
	// Correct common unexported names based on manual converter
	if srcType.Name == "SrcAddress" && dstType.Name == "DstAddress" {
		funcName = "srcAddressToDstAddress"
	}
	if srcType.Name == "SrcContact" && dstType.Name == "DstContact" {
		funcName = "srcContactToDstContact"
	}
	if srcType.Name == "SrcInternalDetail" && dstType.Name == "DstInternalDetail" {
		funcName = "srcInternalDetailToDstInternalDetail"
	}
	if srcType.Name == "SrcItem" && dstType.Name == "DstItem" {
		funcName = "srcItemToDstItem"
	}

	sb.WriteString(fmt.Sprintf("// %s converts models.%s to models.%s\n", funcName, srcType.Name, dstType.Name))
	sb.WriteString(fmt.Sprintf("func %s(ctx context.Context, src models.%s) models.%s {\n", funcName, srcType.Name, dstType.Name))
	sb.WriteString(fmt.Sprintf("\tif ctx == nil { ctx = context.Background() }\n")) // Ensure context is not nil
	sb.WriteString(fmt.Sprintf("\tdst := models.%s{}\n", dstType.Name))

	// Field mapping logic (simplified, uses some hardcoded rules from manual_converter.go)
	for _, dstField := range dstType.Struct.Fields {
		// Try to find a corresponding source field or logic
		mapped := false
		// Rule 1: Specific known mappings (from manual converter)
		if srcType.Name == "SrcUser" && dstType.Name == "DstUser" {
			if dstField.Name == "UserID" {
				sb.WriteString(fmt.Sprintf("\tdst.UserID = fmt.Sprintf(\"user-%%d\", src.ID)\n"))
				mapped = true
			} else if dstField.Name == "FullName" {
				sb.WriteString(fmt.Sprintf("\tdst.FullName = src.FirstName + \" \" + src.LastName\n"))
				mapped = true
			} else if dstField.Name == "CreatedAt" {
				sb.WriteString(fmt.Sprintf("\tdst.CreatedAt = src.CreatedAt.Format(time.RFC3339)\n"))
				mapped = true
			} else if dstField.Name == "UpdatedAt" {
				sb.WriteString(fmt.Sprintf("\tif src.UpdatedAt != nil {\n"))
				sb.WriteString(fmt.Sprintf("\t\tdst.UpdatedAt = src.UpdatedAt.Format(time.RFC3339)\n"))
				sb.WriteString(fmt.Sprintf("\t} else {\n"))
				sb.WriteString(fmt.Sprintf("\t\tdst.UpdatedAt = \"\"\n"))
				sb.WriteString(fmt.Sprintf("\t}\n"))
				mapped = true
			}
		} else if srcType.Name == "SrcAddress" && dstType.Name == "DstAddress" {
			if dstField.Name == "FullStreet" {
				sb.WriteString(fmt.Sprintf("\tdst.FullStreet = src.Street\n"))
				mapped = true
			}
			if dstField.Name == "CityName" {
				sb.WriteString(fmt.Sprintf("\tdst.CityName = src.City\n"))
				mapped = true
			}
		} else if srcType.Name == "SrcContact" && dstType.Name == "DstContact" {
			if dstField.Name == "EmailAddress" {
				sb.WriteString(fmt.Sprintf("\tdst.EmailAddress = src.Email\n"))
				mapped = true
			}
			if dstField.Name == "PhoneNumber" {
				sb.WriteString(fmt.Sprintf("\tif src.Phone != nil {\n"))
				sb.WriteString(fmt.Sprintf("\t\tdst.PhoneNumber = *src.Phone\n"))
				sb.WriteString(fmt.Sprintf("\t} else {\n"))
				sb.WriteString(fmt.Sprintf("\t\tdst.PhoneNumber = \"N/A\"\n"))
				sb.WriteString(fmt.Sprintf("\t}\n"))
				mapped = true
			}
		} else if srcType.Name == "SrcInternalDetail" && dstType.Name == "DstInternalDetail" {
			if dstField.Name == "ItemCode" {
				sb.WriteString(fmt.Sprintf("\tdst.ItemCode = src.Code\n"))
				mapped = true
			}
			if dstField.Name == "LocalizedDesc" {
				sb.WriteString(fmt.Sprintf("\tdst.LocalizedDesc = translateDescription(ctx, src.Description, \"jp\") // TODO: Make lang configurable\n"))
				mapped = true
			}
		} else if srcType.Name == "SrcOrder" && dstType.Name == "DstOrder" {
			if dstField.Name == "ID" {
				sb.WriteString(fmt.Sprintf("\tdst.ID = src.OrderID\n"))
				mapped = true
			}
			if dstField.Name == "TotalAmount" {
				sb.WriteString(fmt.Sprintf("\tdst.TotalAmount = src.Amount\n"))
				mapped = true
			}
		} else if srcType.Name == "SrcItem" && dstType.Name == "DstItem" {
			if dstField.Name == "ProductCode" {
				sb.WriteString(fmt.Sprintf("\tdst.ProductCode = src.SKU\n"))
				mapped = true
			}
			if dstField.Name == "Count" {
				sb.WriteString(fmt.Sprintf("\tdst.Count = src.Quantity\n"))
				mapped = true
			}
		}

		// Rule 2: Embedded struct conversion
		if !mapped && dstField.Type.Name == "DstAddress" && srcType.Name == "SrcUser" { // Specific to User embedding Address
			var srcAddrField *scanner.FieldInfo
			for _, sf := range srcType.Struct.Fields {
				if sf.Embedded && sf.Type.Name == "SrcAddress" {
					srcAddrField = sf
					break
				}
			}
			if srcAddrField != nil {
				sb.WriteString(fmt.Sprintf("\tdst.%s = srcAddressToDstAddress(ctx, src.%s)\n", dstField.Name, srcAddrField.Type.Name /* Usually src.SrcAddress */))
				mapped = true
			}
		}

		// Rule 3: Nested struct conversion
		if !mapped && dstField.Type.Name == "DstContact" && srcType.Name == "SrcUser" { // Specific to User having ContactInfo
			var srcContactField *scanner.FieldInfo
			for _, sf := range srcType.Struct.Fields {
				if sf.Name == "ContactInfo" && sf.Type.Name == "SrcContact" {
					srcContactField = sf
					break
				}
			}
			if srcContactField != nil {
				sb.WriteString(fmt.Sprintf("\tdst.%s = srcContactToDstContact(ctx, src.%s)\n", dstField.Name, srcContactField.Name))
				mapped = true
			}
		}

		// Rule 4: Slice of structs conversion
		if !mapped && dstField.Type.IsSlice {
			srcFieldName := dstField.Name // Try direct name match first for slice
			if srcType.Name == "SrcOrder" && dstField.Name == "LineItems" {
				srcFieldName = "Items"
			} // Specific rename for Order:Items -> LineItems

			var srcSliceField *scanner.FieldInfo
			for _, sf := range srcType.Struct.Fields {
				if sf.Name == srcFieldName && sf.Type.IsSlice {
					srcSliceField = sf
					break
				}
			}
			if srcSliceField != nil && srcSliceField.Type.Elem != nil && dstField.Type.Elem != nil {
				elemConvertFunc := fmt.Sprintf("%sTo%s", camelCase(srcSliceField.Type.Elem.Name), strings.Title(dstField.Type.Elem.Name))
				// Correct common unexported names based on manual converter
				if srcSliceField.Type.Elem.Name == "SrcInternalDetail" && dstField.Type.Elem.Name == "DstInternalDetail" {
					elemConvertFunc = "srcInternalDetailToDstInternalDetail"
				}
				if srcSliceField.Type.Elem.Name == "SrcItem" && dstField.Type.Elem.Name == "DstItem" {
					elemConvertFunc = "srcItemToDstItem"
				}

				sb.WriteString(fmt.Sprintf("\tif src.%s != nil {\n", srcSliceField.Name))
				sb.WriteString(fmt.Sprintf("\t\tdst.%s = make([]models.%s, len(src.%s))\n", dstField.Name, dstField.Type.Elem.Name, srcSliceField.Name))
				sb.WriteString(fmt.Sprintf("\t\tfor i, sElem := range src.%s {\n", srcSliceField.Name))
				sb.WriteString(fmt.Sprintf("\t\t\tdst.%s[i] = %s(ctx, sElem)\n", dstField.Name, elemConvertFunc))
				sb.WriteString(fmt.Sprintf("\t\t}\n"))
				sb.WriteString(fmt.Sprintf("\t}\n"))
				mapped = true
			}
		}

		// Rule 5: Direct name and type match (fallback)
		if !mapped {
			for _, srcField := range srcType.Struct.Fields {
				if normalizeFieldName(dstField.Name) == normalizeFieldName(srcField.Name) {
					if srcField.Type.Name == dstField.Type.Name && !srcField.Type.IsMap && !srcField.Type.IsSlice && !dstField.Type.IsPointer && !srcField.Type.IsPointer { // Simple types
						sb.WriteString(fmt.Sprintf("\tdst.%s = src.%s\n", dstField.Name, srcField.Name))
						mapped = true
						break
					}
				}
			}
		}

		if !mapped {
			sb.WriteString(fmt.Sprintf("\t// TODO: No mapping found for dst.%s (Type: %s)\n", dstField.Name, dstField.Type.Name))
		}
	}

	sb.WriteString("\treturn dst\n")
	sb.WriteString("}\n\n")
}

func stripPrefix(name, prefix string) string {
	if strings.HasPrefix(name, prefix) {
		return name[len(prefix):]
	}
	return name
}

func camelCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[0:1]) + s[1:]
}

func normalizeFieldName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}
