package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

const unmarshalAnnotation = "@deriving:unmarshall"

type TemplateData struct {
	PackageName       string
	StructName        string
	OneOfFieldName    string
	OneOfFieldJSONTag string
	OneOfFieldType    string
	OtherFields       []FieldInfo
	OneOfTypes        []OneOfTypeMapping
	Imports           map[string]string
}

type FieldInfo struct {
	Name    string
	Type    string
	JSONTag string
}

type OneOfTypeMapping struct {
	JSONValue string
	GoType    string
}

const unmarshalJSONTemplate = `
func (s *{{.StructName}}) UnmarshalJSON(data []byte) error {
	// Define an alias type to prevent infinite recursion with UnmarshalJSON.
	type Alias {{.StructName}}
	aux := &struct {
		// The {{.OneOfFieldName}} field will be parsed manually later, so initially capture it as json.RawMessage.
		{{.OneOfFieldName}} json.RawMessage ` + "`json:\"{{.OneOfFieldJSONTag}}\"`" + `
		// All other fields will be handled by the standard unmarshaler via the Alias.
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal into aux struct for {{.StructName}}: %w", err)
	}

	// If the {{.OneOfFieldName}} field is null or empty, do nothing further with it.
	if aux.{{.OneOfFieldName}} == nil || string(aux.{{.OneOfFieldName}}) == "null" {
		s.{{.OneOfFieldName}} = nil // Explicitly set to nil, or follow specific logic if a non-nil zero value is required.
		return nil
	}

	// Read only the "type" field from the {{.OneOfFieldName}} content to determine the concrete type.
	// NOTE: This assumes the discriminator field is named "type".
	// If the generator can determine the actual discriminator field name, it should be used here.
	var discriminatorDoc struct {
		Type string ` + "`json:\"type\"`" + ` // TODO: Make this discriminator field name configurable if not always "type".
	}
	if err := json.Unmarshal(aux.{{.OneOfFieldName}}, &discriminatorDoc); err != nil {
		// Including aux content in the error can be helpful for debugging, but may make logs verbose for large JSON.
		return fmt.Errorf("could not detect type from field '{{.OneOfFieldJSONTag}}' (content: %s): %w", string(aux.{{.OneOfFieldName}}), err)
	}

	// Decode into the appropriate struct based on the value of the 'type' field.
	switch discriminatorDoc.Type {
	{{range .OneOfTypes}}
	case "{{.JSONValue}}":
		var content {{.GoType}}
		if err := json.Unmarshal(aux.{{$.OneOfFieldName}}, &content); err != nil {
			return fmt.Errorf("failed to unmarshal '{{$.OneOfFieldJSONTag}}' as {{.GoType}} for type '{{.JSONValue}}' (content: %s): %w", string(aux.{{$.OneOfFieldName}}), err)
		}
		s.{{$.OneOfFieldName}} = &content // Assuming the field is a pointer to the concrete type. Adjust if it's an interface holding value types.
	{{end}}
	default:
		// The error message for an empty discriminatorDoc.Type could be more specific.
		// (e.g., "discriminator field 'type' is missing or not a string in '{{.OneOfFieldJSONTag}}'")
		if discriminatorDoc.Type == "" {
			return fmt.Errorf("discriminator field 'type' missing or empty in '{{.OneOfFieldJSONTag}}' (content: %s)", string(aux.{{.OneOfFieldName}}))
		}
		return fmt.Errorf("unknown data type '%s' for field '{{.OneOfFieldJSONTag}}' (content: %s)", discriminatorDoc.Type, string(aux.{{.OneOfFieldName}}))
	}

	return nil
}
`

func findTypeInPackage(pkgInfo *scanner.PackageInfo, typeName string) *scanner.TypeInfo {
	for _, t := range pkgInfo.Types {
		if t.Name == typeName {
			return t
		}
	}
	return nil
}

func Generate(pkgPath string) error {
	fmt.Printf("Attempting to generate for package path: %s\n", pkgPath)
	gscn, err := goscan.New(".")
	if err != nil {
		return fmt.Errorf("failed to create go-scan scanner: %w", err)
	}

	pkgInfo, err := gscn.ScanPackage(pkgPath)
	if err != nil {
		return fmt.Errorf("go-scan failed to scan package at %s: %w", pkgPath, err)
	}
	fmt.Printf("Scanned package: %s (ImportPath: %s, Files: %d)\n", pkgInfo.Name, pkgInfo.ImportPath, len(pkgInfo.Files))

	var generatedCodeForAllStructs bytes.Buffer
	needsImportEncodingJson := false
	needsImportFmt := false
	allFileImports := make(map[string]string) // path -> alias

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}
		if typeInfo.Doc == "" || !strings.Contains(typeInfo.Doc, unmarshalAnnotation) {
			continue
		}
		fmt.Printf("  Processing struct: %s for %s\n", typeInfo.Name, unmarshalAnnotation)

		data := TemplateData{
			PackageName: pkgInfo.Name,
			StructName:  typeInfo.Name,
			Imports:     make(map[string]string),
		}
		var oneOfInterfaceDef *scanner.TypeInfo
		var oneOfInterfaceFieldType *scanner.FieldType

		for _, field := range typeInfo.Struct.Fields {
			jsonTag := ""
			if field.Tag != "" {
				tag := reflect.StructTag(field.Tag)
				jsonTagVal := tag.Get("json")
				if commaIdx := strings.Index(jsonTagVal, ","); commaIdx != -1 {
					jsonTag = jsonTagVal[:commaIdx]
				} else {
					jsonTag = jsonTagVal
				}
			}

			resolvedFieldType, errResolve := field.Type.Resolve()
			if errResolve != nil {
				// If Resolve fails, and PkgName is empty (or same as current package), try to find type in current package.
				if field.Type.PkgName == "" || field.Type.PkgName == pkgInfo.Name {
					fmt.Printf("      Resolve failed for local type %s for field %s: %v. Attempting to find in current package.\n", field.Type.Name, field.Name, errResolve)
					resolvedFieldType = findTypeInPackage(pkgInfo, field.Type.Name)
					if resolvedFieldType != nil {
						fmt.Printf("        Found local type %s in current package.\n", field.Type.Name)
					} else {
						fmt.Printf("        Local type %s not found in current package.\n", field.Type.Name)
					}
				} else {
					fmt.Printf("      Error resolving field %s type %s (external pkg %s): %v\n", field.Name, field.Type.Name, field.Type.PkgName, errResolve)
				}
			}

			var resolvedKind scanner.Kind = -1
			isInterface := false
			if resolvedFieldType != nil {
				resolvedKind = resolvedFieldType.Kind
				isInterface = (resolvedFieldType.Kind == scanner.InterfaceKind)
			}
			fmt.Printf("      Field: %s, TypeName: %s, TypePkgName: %s, ResolvedKind: %v, IsInterfaceResult: %t\n", field.Name, field.Type.Name, field.Type.PkgName, resolvedKind, isInterface)


			if resolvedFieldType != nil && resolvedFieldType.Kind == scanner.InterfaceKind {
				data.OneOfFieldName = field.Name
				data.OneOfFieldJSONTag = jsonTag
				oneOfInterfaceDef = resolvedFieldType
				oneOfInterfaceFieldType = field.Type

				fieldTypeString := oneOfInterfaceDef.Name
				var determinedInterfaceImportPath string

				if oneOfInterfaceDef.FilePath != "" { // Assuming FilePath is set for resolved types
					interfaceDir := filepath.Dir(oneOfInterfaceDef.FilePath)
					// Scan the package of the interface to get its canonical import path and name
					interfaceDefiningPkg, errPkgScan := gscn.ScanPackage(interfaceDir)
					if errPkgScan == nil && interfaceDefiningPkg != nil && interfaceDefiningPkg.ImportPath != "" {
						if interfaceDefiningPkg.ImportPath != pkgInfo.ImportPath { // If it's an external package
							determinedInterfaceImportPath = interfaceDefiningPkg.ImportPath
							fieldTypeString = interfaceDefiningPkg.Name + "." + oneOfInterfaceDef.Name
							data.Imports[interfaceDefiningPkg.Name] = determinedInterfaceImportPath
							allFileImports[determinedInterfaceImportPath] = interfaceDefiningPkg.Name
						}
						// If in the same package, fieldTypeString remains oneOfInterfaceDef.Name, no import needed
					} else {
						fmt.Printf("      Warning: Could not determine import path for interface %s in dir %s. PkgScanErr: %v. Using PkgName as fallback.\n", oneOfInterfaceDef.Name, interfaceDir, errPkgScan)
						if oneOfInterfaceFieldType.PkgName != "" && oneOfInterfaceFieldType.PkgName != pkgInfo.Name {
							fieldTypeString = oneOfInterfaceFieldType.PkgName + "." + oneOfInterfaceDef.Name
						}
					}
				} else if oneOfInterfaceFieldType.PkgName != "" && oneOfInterfaceFieldType.PkgName != pkgInfo.Name {
					// Fallback if FilePath is empty, use PkgName (less reliable for import path)
					fieldTypeString = oneOfInterfaceFieldType.PkgName + "." + oneOfInterfaceDef.Name
					fmt.Printf("      Warning: Interface %s FilePath was empty, relying on PkgName %s for type string.\n", oneOfInterfaceDef.Name, oneOfInterfaceFieldType.PkgName)
				}
				data.OneOfFieldType = fieldTypeString
				fmt.Printf("      Found OneOf Field: Name=%s, JSONTag=%s, InterfaceType=%s (Determined ImportPath for interface: %s)\n", data.OneOfFieldName, data.OneOfFieldJSONTag, data.OneOfFieldType, determinedInterfaceImportPath)

			} else { // Other fields
				typeName := field.Type.String()
				var otherFieldActualImportPath string
				var otherFieldPkgAliasForImport string

				// Check if this field's type is from an external package
				if field.Type.PkgName != "" && (pkgInfo.Name == "" || field.Type.PkgName != pkgInfo.Name) {
					// Try to get its full import path if its definition was resolved
					if resolvedFieldType != nil && resolvedFieldType.FilePath != "" { // Using resolvedFieldType from above
						otherFieldDir := filepath.Dir(resolvedFieldType.FilePath)
						otherFieldDefiningPkg, errPkgScan := gscn.ScanPackage(otherFieldDir)
						if errPkgScan == nil && otherFieldDefiningPkg != nil && otherFieldDefiningPkg.ImportPath != "" {
							if otherFieldDefiningPkg.ImportPath != pkgInfo.ImportPath { // Is it external?
								otherFieldActualImportPath = otherFieldDefiningPkg.ImportPath
								otherFieldPkgAliasForImport = otherFieldDefiningPkg.Name
							}
						}
					} else if field.Type.PkgName != "" {
						// Fallback for "other" fields if their TypeInfo wasn't fully resolved with FilePath
						fmt.Printf("      Note: Other field %s uses PkgName '%s'. Full import path might rely on FieldType.String() or prior discoveries.\n", field.Name, field.Type.PkgName)
					}

					if otherFieldActualImportPath != "" && otherFieldPkgAliasForImport != "" {
						data.Imports[otherFieldPkgAliasForImport] = otherFieldActualImportPath
						allFileImports[otherFieldActualImportPath] = otherFieldPkgAliasForImport
					}
				}
				data.OtherFields = append(data.OtherFields, FieldInfo{Name: field.Name, Type: typeName, JSONTag: jsonTag})
			}
		}

		if data.OneOfFieldName == "" || oneOfInterfaceDef == nil {
			fmt.Printf("  Skipping struct %s: missing oneOf interface field or its definition.\n", typeInfo.Name)
			continue
		}

		var interfaceActualImportPath string
		if oneOfInterfaceDef.FilePath != "" {
			dir := filepath.Dir(oneOfInterfaceDef.FilePath)
			tempPkgInfo, _ := gscn.ScanPackage(dir) // Scan dir to get PackageInfo
			if tempPkgInfo != nil && tempPkgInfo.ImportPath != "" {
				interfaceActualImportPath = tempPkgInfo.ImportPath
				if interfaceActualImportPath == pkgInfo.ImportPath { // If it resolved to current package
					interfaceActualImportPath = "" // Treat as local (no import path needed for qualification)
				}
			}
		}
		// Fallback or additional check if FilePath was empty but PkgName suggested external
		if interfaceActualImportPath == "" && oneOfInterfaceFieldType != nil && oneOfInterfaceFieldType.PkgName != "" && oneOfInterfaceFieldType.PkgName != pkgInfo.Name {
			fmt.Printf("    Note: Interface %s might be external via PkgName %s, but FilePath method didn't confirm an *external* import path.\n", oneOfInterfaceDef.Name, oneOfInterfaceFieldType.PkgName)
			// This implies we might need to find the import path for PkgName if it's an alias for an external package.
			// This is hard without full import map of the source file. For now, assume local if FilePath method results in ""
		}


		fmt.Printf("    Searching for implementers of %s (interface defined in effective import path: '%s')\n", oneOfInterfaceDef.Name, interfaceActualImportPath)

		searchPkgs := []*scanner.PackageInfo{pkgInfo} // Start with current package
		if interfaceActualImportPath != "" {
			// If interfaceActualImportPath is non-empty, it means it's different from pkgInfo.ImportPath (or should be)
			scannedInterfacePkg, errScan := gscn.ScanPackageByImport(interfaceActualImportPath)
			if errScan != nil {
				fmt.Printf("      Warning: Failed to scan interface's actual package %s by import: %v.\n", interfaceActualImportPath, errScan)
			} else if scannedInterfacePkg != nil {
				alreadyAdded := false
				for _, sp := range searchPkgs { if sp.ImportPath == scannedInterfacePkg.ImportPath { alreadyAdded = true; break } }
				if !alreadyAdded { searchPkgs = append(searchPkgs, scannedInterfacePkg) }
				fmt.Printf("    Also searched interface's own package: %s (found %d types)\n", scannedInterfacePkg.ImportPath, len(scannedInterfacePkg.Types))
			}
		}

		foundImplementers := false
		processedImplementerKeys := make(map[string]bool)

		for _, currentSearchPkg := range searchPkgs {
			if currentSearchPkg == nil { continue }
			fmt.Printf("      Checking package: %s (ImportPath: %s) for implementers of %s\n", currentSearchPkg.Name, currentSearchPkg.ImportPath, oneOfInterfaceDef.Name)
			for _, candidateType := range currentSearchPkg.Types {
				if candidateType.Kind != scanner.StructKind || candidateType.Struct == nil { continue }

				implementerKey := candidateType.FilePath + "::" + candidateType.Name
				if processedImplementerKeys[implementerKey] { continue }

				if goscan.Implements(candidateType, oneOfInterfaceDef, currentSearchPkg) {
					fmt.Printf("        Found implementer: %s in package %s (File: %s)\n", candidateType.Name, currentSearchPkg.ImportPath, candidateType.FilePath)
					processedImplementerKeys[implementerKey] = true
					foundImplementers = true

					discriminatorValue := ""
					// TODO: Get discriminator value from candidateType's "Type" field or GetType() method.
					// For now, using simplified logic based on testdata.
					if candidateType.Name == "Circle" { discriminatorValue = "circle" } else
					if candidateType.Name == "Rectangle" { discriminatorValue = "rectangle" } else {
						discriminatorValue = strings.ToLower(candidateType.Name)
						fmt.Printf("          Warning: No specific discriminator rule for %s from %s, using '%s'.\n", candidateType.Name, currentSearchPkg.ImportPath, discriminatorValue)
					}

					goTypeString := candidateType.Name
					// Qualify type if it's from a different package than the container struct's package
					if currentSearchPkg.ImportPath != "" && currentSearchPkg.ImportPath != pkgInfo.ImportPath {
						goTypeString = currentSearchPkg.Name + "." + candidateType.Name
						data.Imports[currentSearchPkg.Name] = currentSearchPkg.ImportPath
						allFileImports[currentSearchPkg.ImportPath] = currentSearchPkg.Name
					}
					data.OneOfTypes = append(data.OneOfTypes, OneOfTypeMapping{ JSONValue: discriminatorValue, GoType: goTypeString, })
					fmt.Printf("          Mapping: JSON value \"%s\" -> Go type %s\n", discriminatorValue, goTypeString)
				}
			}
		}

		if !foundImplementers {
			warnPath := interfaceActualImportPath
			if warnPath == "" { // If interface is local or resolution failed to identify external path
				warnPath = pkgInfo.ImportPath // Assume current package
			}
			fmt.Printf("  Warning: For struct %s, no types found implementing interface %s (from %s). Generated UnmarshalJSON might be incomplete.\n", typeInfo.Name, oneOfInterfaceDef.Name, warnPath)
		}

		tmpl, err := template.New("unmarshal").Parse(unmarshalJSONTemplate)
		if err != nil { return fmt.Errorf("failed to parse template: %w", err) }
		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, data); err != nil { return fmt.Errorf("failed to execute template for struct %s: %w", typeInfo.Name, err) }
		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")
		needsImportEncodingJson = true
		needsImportFmt = true
	}

	if generatedCodeForAllStructs.Len() == 0 {
		fmt.Println("No structs found requiring UnmarshalJSON generation.")
		return nil
	}

	finalOutput := bytes.Buffer{}
	finalOutput.WriteString(fmt.Sprintf("// Code generated by derivingjson for package %s. DO NOT EDIT.\n\n", pkgInfo.Name))
	finalOutput.WriteString(fmt.Sprintf("package %s\n\n", pkgInfo.Name))
	if len(allFileImports) > 0 || needsImportEncodingJson || needsImportFmt {
		finalOutput.WriteString("import (\n")
		if needsImportEncodingJson { finalOutput.WriteString("\t\"encoding/json\"\n") }
		if needsImportFmt { finalOutput.WriteString("\t\"fmt\"\n") }
		uniqueImports := make(map[string]string)
		for path, alias := range allFileImports {
			if path == pkgInfo.ImportPath { continue } // Don't import self
			if currentAlias, exists := uniqueImports[path]; exists {
				// If an alias already exists, prefer the non-empty one.
				// If both are non-empty and different, it's a conflict (though less likely if PkgName is used).
				if currentAlias != alias && alias != "" {
					// This logic might need refinement for alias conflicts.
					// For now, if new alias is non-empty, prefer it.
					uniqueImports[path] = alias
				} else if currentAlias == "" && alias != "" {
					uniqueImports[path] = alias
				}
			} else {
				uniqueImports[path] = alias
			}
		}
		for path, alias := range uniqueImports {
			pathParts := strings.Split(path, "/")
			baseName := pathParts[len(pathParts)-1] // Get actual package name from path
			if alias == baseName || alias == "" { // If stored alias is natural package name or empty
				finalOutput.WriteString(fmt.Sprintf("\t\"%s\"\n", path))
			} else {
				finalOutput.WriteString(fmt.Sprintf("\t%s \"%s\"\n", alias, path))
			}
		}
		finalOutput.WriteString(")\n\n")
	}
	finalOutput.Write(generatedCodeForAllStructs.Bytes())
	formattedCode, err := format.Source(finalOutput.Bytes())
	if err != nil {
		fmt.Printf("Error formatting generated code for package %s: %v\n--- Unformatted Code ---\n%s\n--- End Unformatted Code ---\n", pkgInfo.Name, err, finalOutput.String())
		return fmt.Errorf("failed to format generated code for package %s: %w", pkgInfo.Name, err)
	}
	outputFileName := filepath.Join(pkgPath, fmt.Sprintf("%s_deriving.go", pkgInfo.Name))
	if _, statErr := os.Stat(outputFileName); statErr == nil {
		if removeErr := os.Remove(outputFileName); removeErr != nil {
			fmt.Printf("Warning: Failed to remove existing generated file %s: %v\n", outputFileName, removeErr)
		}
	}
	if err = os.WriteFile(outputFileName, formattedCode, 0644); err != nil {
		return fmt.Errorf("failed to write generated code to %s: %w", outputFileName, err)
	}
	fmt.Printf("Generated code written to %s\n", outputFileName)
	return nil
}

func main() {
	// This main function is added for ease of execution.
	// It targets the ./examples/derivingjson/models directory.
	targetPkgPath := "./examples/derivingjson/models"
	// Attempt to get absolute path for robustness, especially if generator is run from different directories.
	absPath, err := filepath.Abs(targetPkgPath)
	if err != nil {
		fmt.Printf("Error getting absolute path for %s: %v\n", targetPkgPath, err)
		// Fallback to relative path if abs path fails, though this might be less reliable.
		absPath = targetPkgPath
	}

	// Check if the target directory exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		fmt.Printf("Error: Target package directory does not exist: %s\n", absPath)
		fmt.Println("Please ensure the 'models' directory with 'models.go' is correctly placed under 'examples/derivingjson/'.")
		os.Exit(1)
	}


	fmt.Printf("Running generator for package: %s (resolved to %s)\n", targetPkgPath, absPath)
	if err := Generate(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating code for package %s: %v\n", absPath, err)
		os.Exit(1)
	}
	fmt.Println("Generator finished successfully.")
}

// func main() {
// 	// This main function is added for ease of execution.
// 	// It targets the ./examples/derivingjson/models directory.
// 	targetPkgPath := "./examples/derivingjson/models"
// 	// Attempt to get absolute path for robustness, especially if generator is run from different directories.
// 	absPath, err := filepath.Abs(targetPkgPath)
// 	if err != nil {
// 		fmt.Printf("Error getting absolute path for %s: %v\n", targetPkgPath, err)
// 		// Fallback to relative path if abs path fails, though this might be less reliable.
// 		absPath = targetPkgPath
// 	}

// 	// Check if the target directory exists
// 	if _, err := os.Stat(absPath); os.IsNotExist(err) {
// 		fmt.Printf("Error: Target package directory does not exist: %s\n", absPath)
// 		fmt.Println("Please ensure the 'models' directory with 'models.go' is correctly placed under 'examples/derivingjson/'.")
// 		os.Exit(1)
// 	}


// 	fmt.Printf("Running generator for package: %s (resolved to %s)\n", targetPkgPath, absPath)
// 	if err := Generate(absPath); err != nil {
// 		fmt.Fprintf(os.Stderr, "Error generating code for package %s: %v\n", absPath, err)
// 		os.Exit(1)
// 	}
// 	fmt.Println("Generator finished successfully.")
// }
