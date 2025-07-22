package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	typescanner "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

func generateConverterPrototype() {
	// Assuming models are in a 'models' subdirectory relative to this file's package.
	// Adjust this path as necessary.
	modelsPath := "./models" // Or an absolute path if needed

	// Get the current working directory to resolve the modelsPath relative to the project root.
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %v", err)
	}

	// Construct the full path to the models directory
	// This assumes the 'examples/convert' directory is the current context for path resolution.
	// If running from project root, this path needs to be 'examples/convert/models'.
	// For simplicity, let's assume we are in 'examples/convert' or can resolve it.
	// A more robust solution would use build tags or configuration.

	// Let's try to locate the 'go-scan' module root to make path resolution more stable.
	// This is a simplified lookup. A real generator might need more sophisticated path finding.
	currentDir := wd
	var projectRoot string
	for {
		if _, err := os.Stat(filepath.Join(currentDir, "go.mod")); err == nil {
			// Check if this go.mod is the main project's go.mod
			// For this example, we'll assume if it contains "github.com/podhmo/go-scan" it's the one.
			// This is a heuristic.
			b, _ := os.ReadFile(filepath.Join(currentDir, "go.mod"))
			if strings.Contains(string(b), "github.com/podhmo/go-scan") {
				projectRoot = currentDir
				break
			}
		}
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			log.Println("Could not reliably find project root containing 'github.com/podhmo/go-scan' in go.mod. Trying relative path.")
			// Fallback or error if not found, for now, assume relative path works from 'examples/convert'
			// If running `go run examples/convert/main.go` from project root, wd is projectRoot
			// and modelsPath should be "examples/convert/models"
			if strings.HasSuffix(wd, "examples/convert") {
				modelsPath = filepath.Join(wd, "models")
			} else {
				// Try to construct path assuming wd is project root
				modelsPath = filepath.Join(wd, "examples", "convert", "models")
				if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
					log.Fatalf("Models directory not found at %s. Please ensure paths are correct.", modelsPath)
				}
			}
			break // Exit loop if heuristic fails or relative path assumed
		}
		currentDir = parent
	}
	if projectRoot != "" {
		modelsPath = filepath.Join(projectRoot, "examples", "convert", "models")
	}

	log.Printf("Scanning models in: %s\n", modelsPath)

	// Create a go-scan Scanner instance.
	// The first argument to New is a starting path to find the module root (go.mod).
	// For this example, we'll use the modelsPath itself, assuming it's within a module.
	s, err := typescanner.New(typescanner.WithWorkDir(modelsPath))
	if err != nil {
		log.Fatalf("Failed to create scanner: %v", err)
	}

	// Scan the package containing the models.
	pkgInfo, err := s.ScanPackage(context.Background(), modelsPath)
	if err != nil {
		log.Fatalf("Failed to scan package %s: %v", modelsPath, err)
	}

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
		log.Fatal("One or more top-level source or destination types not found in models package.")
	}

	// Create a string builder to accumulate the generated code.
	var sb strings.Builder

	sb.WriteString("package converter\n\n")
	sb.WriteString("import (\n")
	sb.WriteString("\t\"context\"\n")
	sb.WriteString("\t\"fmt\"\n")
	sb.WriteString("\t\"time\"\n")
	sb.WriteString("\t\"example.com/convert/models\"\n") // Assuming models are in this path
	sb.WriteString(")\n\n")

	// Generate User converter
	if srcUserType != nil && dstUserType != nil {
		generateStructConverter(&sb, srcUserType, dstUserType, pkgInfo)
	}
	// Generate Order converter
	if srcOrderType != nil && dstOrderType != nil {
		generateStructConverter(&sb, srcOrderType, dstOrderType, pkgInfo)
	}

	// Generate necessary sub-converters (helper functions)
	// In a real generator, we'd only generate these if they are actually needed by top-level converters
	// and manage their names to avoid conflicts.
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
// In a real scenario, this could be a more complex logic, e.g., calling a translation service.
func translateDescription(ctx context.Context, text string, targetLang string) string {
	if targetLang == "jp" {
		return "翻訳済み (JP): " + text
	}
	return text
}
`)

	// Write the generated code to a file.
	generatedFilePath := filepath.Join(filepath.Dir(modelsPath), "converter", "generated_converters.go")
	if err := os.MkdirAll(filepath.Dir(generatedFilePath), 0755); err != nil {
		log.Fatalf("Failed to create directory for generated converters: %v", err)
	}
	err = os.WriteFile(generatedFilePath, []byte(sb.String()), 0644)
	if err != nil {
		log.Fatalf("Failed to write generated converters: %v", err)
	}

	log.Printf("Generated converters successfully at: %s\n", generatedFilePath)
	fmt.Println("--- Generator Prototype Finished ---")
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
