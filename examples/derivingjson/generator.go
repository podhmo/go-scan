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
	StructName    string
	OtherFields   []FieldInfo
	OneOfFields   []OneOfFieldDetail
	Imports       map[string]string
	DiscriminatorFieldJSONName string // Assuming this is global for the struct for now
}

type FieldInfo struct {
	Name    string
	Type    string
	JSONTag string
}

// OneOfFieldDetail holds information for a single oneOf field
type OneOfFieldDetail struct {
	FieldName    string             // Name of the field in the struct (e.g., "ShapeData", "EventPayload")
	FieldType    string             // Go type of the interface (e.g., "shapes.Shape", "events.Event")
	JSONTag      string             // JSON tag name (e.g., "shape_data", "payload")
	Implementers []OneOfTypeMapping // Mappings of JSON discriminator value to concrete Go type
}

type OneOfTypeMapping struct {
	JSONValue string // The value in the discriminator field (e.g., "circle", "user_created")
	GoType    string // The concrete Go type (e.g., "*shapes.Circle", "*events.UserCreatedEvent")
}

const unmarshalJSONTemplate = `
func (s *{{.StructName}}) UnmarshalJSON(data []byte) error {
	// Define an alias type to prevent infinite recursion with UnmarshalJSON.
	type Alias {{.StructName}}
	aux := &struct {
		{{range .OneOfFields}}
		{{.FieldName}} json.RawMessage ` + "`json:\"{{.JSONTag}}\"`" + `
		{{end}}
		// All other fields will be handled by the standard unmarshaler via the Alias.
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal into aux struct for {{.StructName}}: %w", err)
	}

	{{range $oneOfField := .OneOfFields}}
	// Process {{$oneOfField.FieldName}}
	if aux.{{$oneOfField.FieldName}} != nil && string(aux.{{$oneOfField.FieldName}}) != "null" {
		var discriminatorDoc struct {
			Type string ` + "`json:\"{{$.DiscriminatorFieldJSONName}}\"`" + ` // Discriminator field
		}
		if err := json.Unmarshal(aux.{{$oneOfField.FieldName}}, &discriminatorDoc); err != nil {
			return fmt.Errorf("could not detect type from field '{{$oneOfField.JSONTag}}' (content: %s): %w", string(aux.{{$oneOfField.FieldName}}), err)
		}

		switch discriminatorDoc.Type {
		{{range .Implementers}}
		case "{{.JSONValue}}":
			var content {{.GoType}}
			if err := json.Unmarshal(aux.{{$oneOfField.FieldName}}, &content); err != nil {
				return fmt.Errorf("failed to unmarshal '{{$oneOfField.JSONTag}}' as {{.GoType}} for type '{{.JSONValue}}' (content: %s): %w", string(aux.{{$oneOfField.FieldName}}), err)
			}
			s.{{$oneOfField.FieldName}} = content
		{{end}}
		default:
			if discriminatorDoc.Type == "" {
				return fmt.Errorf("discriminator field '{{$.DiscriminatorFieldJSONName}}' missing or empty in '{{$oneOfField.JSONTag}}' (content: %s)", string(aux.{{$oneOfField.FieldName}}))
			}
			return fmt.Errorf("unknown data type '%s' for field '{{$oneOfField.JSONTag}}' (content: %s)", discriminatorDoc.Type, string(aux.{{$oneOfField.FieldName}}))
		}
	} else {
		s.{{$oneOfField.FieldName}} = nil // Explicitly set to nil if null or empty
	}
	{{end}}

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
			PackageName:       pkgInfo.Name,
			StructName:        typeInfo.Name,
			Imports:           make(map[string]string),
			OneOfFields:       []OneOfFieldDetail{},
			OtherFields:       []FieldInfo{},
			DiscriminatorFieldJSONName: "type", // Hardcoded for now
		}

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
				if field.Type.PkgName == "" || field.Type.PkgName == pkgInfo.Name {
					resolvedFieldType = findTypeInPackage(pkgInfo, field.Type.Name)
				}
				if resolvedFieldType == nil { // if still nil after local lookup or if external
					fmt.Printf("      Warning: Error resolving field %s type %s (pkg %s): %v. Will proceed if it's an interface for oneOf.\n", field.Name, field.Type.Name, field.Type.PkgName, errResolve)
				}
			}

			isInterfaceField := false
			if resolvedFieldType != nil && resolvedFieldType.Kind == scanner.InterfaceKind {
				isInterfaceField = true
			} else if resolvedFieldType == nil && strings.Contains(field.Type.Name, "interface{") { // Heuristic for anonymous interfaces, though less robust
				// This case is tricky as anonymous interfaces don't have a TypeInfo directly from Resolve() in the same way.
				// For derivingjson, we typically expect named interfaces.
				// Goscan's Implements check might also struggle with anonymous interfaces if they are not fully parsed.
				// For now, we'll primarily focus on named interfaces.
				fmt.Printf("      Field %s is an anonymous interface. Support for these as oneOf targets is limited.\n", field.Name)
			}


			if isInterfaceField {
				fmt.Printf("      Processing potential OneOf Field: %s (Interface: %s)\n", field.Name, field.Type.String())
				oneOfDetail := OneOfFieldDetail{
					FieldName:    field.Name,
					JSONTag:      jsonTag,
					Implementers: []OneOfTypeMapping{},
				}

				// Determine interface type string and manage imports
				interfaceDef := resolvedFieldType // This is the TypeInfo for the interface
				fieldTypeString := interfaceDef.Name // Default to local name for the interface's type string in `oneOfDetail.FieldType`
				var interfaceDefiningPkgImportPath string // This will be the canonical import path of the package defining the interface
				var interfaceDefiningPkgNameForImport string // This will be the package name (or alias) to use in import statements and qualified type names

				// Priority 1: Use FullImportPath from the FieldType, as it's derived directly from the import statement.
				if field.Type.FullImportPath() != "" && field.Type.FullImportPath() != pkgInfo.ImportPath {
					interfaceDefiningPkgImportPath = field.Type.FullImportPath()
					interfaceDefiningPkgNameForImport = field.Type.PkgName // PkgName from FieldType is the alias/name used in the source file.

					fieldTypeString = interfaceDefiningPkgNameForImport + "." + interfaceDef.Name
					data.Imports[interfaceDefiningPkgNameForImport] = interfaceDefiningPkgImportPath
					allFileImports[interfaceDefiningPkgImportPath] = interfaceDefiningPkgNameForImport
					fmt.Printf("      Derived interface import path '%s' and package alias/name '%s' from FieldType.FullImportPath(). FieldType string: %s\n", interfaceDefiningPkgImportPath, interfaceDefiningPkgNameForImport, fieldTypeString)
				} else if interfaceDef.FilePath != "" {
					// Priority 2: Fallback if FullImportPath is not available or local. Derive from interface's FilePath.
					// This is less direct as it re-scans the directory of the interface's definition.
					fmt.Printf("      FieldType.FullImportPath() was empty or local for interface %s. Falling back to FilePath-based scan.\n", interfaceDef.Name)
					interfaceDir := filepath.Dir(interfaceDef.FilePath)
					scannedPkgForInterfaceFile, errPkgScan := gscn.ScanPackage(interfaceDir) // ScanPackage derives import path based on its own module logic.

					if errPkgScan == nil && scannedPkgForInterfaceFile != nil && scannedPkgForInterfaceFile.ImportPath != "" {
						if scannedPkgForInterfaceFile.ImportPath != pkgInfo.ImportPath { // Is it external to the current struct's package?
							interfaceDefiningPkgImportPath = scannedPkgForInterfaceFile.ImportPath
							interfaceDefiningPkgNameForImport = scannedPkgForInterfaceFile.Name // Actual name of the package

							fieldTypeString = interfaceDefiningPkgNameForImport + "." + interfaceDef.Name
							data.Imports[interfaceDefiningPkgNameForImport] = interfaceDefiningPkgImportPath
							allFileImports[interfaceDefiningPkgImportPath] = interfaceDefiningPkgNameForImport
							fmt.Printf("      Derived interface import path '%s' and package name '%s' from FilePath scan. FieldType string: %s\n", interfaceDefiningPkgImportPath, interfaceDefiningPkgNameForImport, fieldTypeString)
						} else {
							// Interface is in the same package as the struct using it. No import needed for its types.
							// fieldTypeString remains interfaceDef.Name (local). interfaceDefiningPkgImportPath remains empty or local.
							interfaceDefiningPkgImportPath = pkgInfo.ImportPath // Set to current package's import path
							interfaceDefiningPkgNameForImport = pkgInfo.Name    // Current package name
							fmt.Printf("      Interface %s is in the same package %s. No special import path needed.\n", interfaceDef.Name, pkgInfo.ImportPath)
						}
					} else {
						// If scanning by FilePath also fails to yield a clear import path.
						fmt.Printf("      Warning: Could not determine import path for interface %s in dir %s via FilePath scan. PkgScanErr: %v. Type string may be incorrect.\n", interfaceDef.Name, interfaceDir, errPkgScan)
						// fieldTypeString remains interfaceDef.Name (local). interfaceDefiningPkgImportPath will be empty.
						// This might be okay if the interface is somehow globally known or a built-in, though unlikely for user types.
					}
				} else {
					// Priority 3: If neither FullImportPath nor FilePath gives an external package path.
					// This implies the interface might be local to the package being scanned (pkgInfo), or a built-in.
					// In this case, fieldTypeString remains interfaceDef.Name.
					// interfaceDefiningPkgImportPath should reflect the current package or be empty if truly local/unqualified.
					interfaceDefiningPkgImportPath = pkgInfo.ImportPath // Assume it's part of the current package
					interfaceDefiningPkgNameForImport = pkgInfo.Name
					fmt.Printf("      Warning: Interface %s has no FullImportPath from FieldType and no FilePath. Assuming local to %s or built-in.\n", interfaceDef.Name, pkgInfo.ImportPath)
				}
				oneOfDetail.FieldType = fieldTypeString // Set the string representation for the interface type


				// Find implementers for this specific interface
				searchPkgs := []*scanner.PackageInfo{pkgInfo}
				if interfaceDefiningPkgImportPath != "" && interfaceDefiningPkgImportPath != pkgInfo.ImportPath {
					scannedInterfacePkg, errScan := gscn.ScanPackageByImport(interfaceDefiningPkgImportPath)
					if errScan == nil && scannedInterfacePkg != nil {
						fmt.Printf("        Successfully scanned interface's package: %s, Found %d types.\n", scannedInterfacePkg.ImportPath, len(scannedInterfacePkg.Types))
						for _, t := range scannedInterfacePkg.Types {
						fmt.Printf("          Type in interface pkg: %s (Kind: %v)\n", t.Name, t.Kind)
						}
						alreadyAdded := false
						for _, sp := range searchPkgs { if sp.ImportPath == scannedInterfacePkg.ImportPath { alreadyAdded = true; break } }
						if !alreadyAdded {
							searchPkgs = append(searchPkgs, scannedInterfacePkg)
							fmt.Printf("        Added %s to searchPkgs. Total searchPkgs: %d\n", scannedInterfacePkg.ImportPath, len(searchPkgs))
						}
					} else {
						fmt.Printf("        Warning: Failed to scan interface's (%s) own package %s by import: %v.\n", interfaceDef.Name, interfaceDefiningPkgImportPath, errScan)
					}
				}

				fmt.Printf("        Searching for implementers of %s (from %s) in %d packages\n", interfaceDef.Name, interfaceDefiningPkgImportPath, len(searchPkgs))
				foundImplementersForThisInterface := false
				processedImplementerKeys := make(map[string]bool) //Scoped per interface

				for i, currentSearchPkg := range searchPkgs {
					if currentSearchPkg == nil { continue }
					fmt.Printf("        Searching in pkg %d: %s (%s), %d types\n", i+1, currentSearchPkg.Name, currentSearchPkg.ImportPath, len(currentSearchPkg.Types))
					for _, candidateType := range currentSearchPkg.Types {
						if candidateType.Kind != scanner.StructKind || candidateType.Struct == nil { continue }
						fmt.Printf("          Checking candidate: %s.%s (isStruct: %v)\n", currentSearchPkg.Name, candidateType.Name, candidateType.Struct != nil)

						implementerKey := candidateType.FilePath + "::" + candidateType.Name
						if processedImplementerKeys[implementerKey] { continue }

						// Debug: Print details of interfaceDef and candidateType before calling Implements
						fmt.Printf("            Interface: %s (Package: %s, Kind: %v)\n", interfaceDef.Name, interfaceDef.FilePath, interfaceDef.Kind)
						if interfaceDef.Interface != nil {
							for _, m := range interfaceDef.Interface.Methods {
								fmt.Printf("              InterfaceMethod: %s\n", m.Name)
							}
						}
						fmt.Printf("            Candidate: %s (Package: %s, Kind: %v)\n", candidateType.Name, currentSearchPkg.ImportPath, candidateType.Kind)


						if goscan.Implements(candidateType, interfaceDef, currentSearchPkg) {
							fmt.Printf("          Found implementer for %s: %s in package %s (File: %s)\n", field.Name, candidateType.Name, currentSearchPkg.ImportPath, candidateType.FilePath)
							processedImplementerKeys[implementerKey] = true
							foundImplementersForThisInterface = true

							discriminatorValue := strings.ToLower(candidateType.Name) // Simplified: use struct name
							// TODO: Allow customization of discriminator value, e.g., via a method or struct tag on the implementer.
							// For now, using simplified logic based on testdata.
							if candidateType.Name == "Circle" { discriminatorValue = "circle" } else
							if candidateType.Name == "Rectangle" { discriminatorValue = "rectangle" } else {
								// Keep using ToLower as default
								fmt.Printf("            Warning: No specific discriminator rule for %s from %s, using '%s'.\n", candidateType.Name, currentSearchPkg.ImportPath, discriminatorValue)
							}


							goTypeString := candidateType.Name
							if currentSearchPkg.ImportPath != "" && currentSearchPkg.ImportPath != pkgInfo.ImportPath {
								goTypeString = currentSearchPkg.Name + "." + candidateType.Name
								data.Imports[currentSearchPkg.Name] = currentSearchPkg.ImportPath
								allFileImports[currentSearchPkg.ImportPath] = currentSearchPkg.Name
							}
							// Ensure the GoType includes a pointer if the field is expected to hold a pointer to an interface implementer
							// For now, assuming all implementers will be pointer types in the field.
							if !strings.HasPrefix(goTypeString, "*") {
								goTypeString = "*" + goTypeString
							}

							oneOfDetail.Implementers = append(oneOfDetail.Implementers, OneOfTypeMapping{
								JSONValue: discriminatorValue,
								GoType:    goTypeString,
							})
						}
					}
				}
				if !foundImplementersForThisInterface {
					warnPath := interfaceDefiningPkgImportPath
					if warnPath == "" { warnPath = pkgInfo.ImportPath }
					fmt.Printf("        Warning: For field %s (interface %s from %s), no implementing types found. UnmarshalJSON might be incomplete for this field.\n", field.Name, interfaceDef.Name, warnPath)
				}
				data.OneOfFields = append(data.OneOfFields, oneOfDetail)

			} else { // Other fields (non-interface or non-oneOf)
				typeName := field.Type.String() // This should give a reasonable representation, e.g. *pkg.Type, []int
				// Handle imports for types of other fields if necessary
				if resolvedFieldType != nil && resolvedFieldType.FilePath != "" {
					fieldDir := filepath.Dir(resolvedFieldType.FilePath)
					fieldDefiningPkg, errPkgScan := gscn.ScanPackage(fieldDir)
					if errPkgScan == nil && fieldDefiningPkg != nil && fieldDefiningPkg.ImportPath != "" {
						if fieldDefiningPkg.ImportPath != pkgInfo.ImportPath { // Is it external?
							// Ensure this import is added to data.Imports and allFileImports
							// The alias used would typically be fieldDefiningPkg.Name
							data.Imports[fieldDefiningPkg.Name] = fieldDefiningPkg.ImportPath
							allFileImports[fieldDefiningPkg.ImportPath] = fieldDefiningPkg.Name
						}
					}
				} else if field.Type.PkgName != "" && field.Type.PkgName != pkgInfo.Name {
					// If FilePath wasn't available but PkgName suggests an external package,
					// we rely on the PkgName being either the actual package name or an alias
					// whose import path has been (or will be) discovered.
					// This part is tricky and relies on consistent import aliasing or go-scan resolving them.
					// For now, assume FieldType.String() handles qualification if necessary,
					// and required imports are caught by other mechanisms or direct PkgName usage.
					fmt.Printf("      Note: Other field %s (%s) might be from external package '%s'. Ensure imports are handled.\n", field.Name, typeName, field.Type.PkgName)
				}
				data.OtherFields = append(data.OtherFields, FieldInfo{Name: field.Name, Type: typeName, JSONTag: jsonTag})
			}
		}

		if len(data.OneOfFields) == 0 {
			fmt.Printf("  Skipping struct %s: no oneOf interface fields found.\n", typeInfo.Name)
			continue
		}

		// No longer need this old block for single oneOf field
		// var interfaceActualImportPath string
		// ... (old logic for single oneOfInterfaceDef) ...

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
