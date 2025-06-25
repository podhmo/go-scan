package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"reflect" // For parsing struct tags
	"strings"
	"text/template"

	"github.com/podhmo/go-scan"       // goscanパッケージ (ルート)
	"github.com/podhmo/go-scan/scanner" // scannerパッケージをインポート
)

const unmarshalAnnotation = "@deriving:unmarshall"

// TemplateData holds data for the UnmarshalJSON template
type TemplateData struct {
	PackageName              string
	StructName               string
	DiscriminatorFieldName     string
	DiscriminatorFieldType     string
	DiscriminatorFieldJSONTag  string
	OneOfFieldName           string
	OneOfFieldJSONTag        string
	OneOfFieldType           string // Interface type name for oneOf field
	OtherFields              []FieldInfo
	OneOfTypes               []OneOfTypeMapping
}

// FieldInfo holds information about a struct field
type FieldInfo struct {
	Name    string
	Type    string
	JSONTag string
}

// OneOfTypeMapping maps a JSON discriminator value to a Go type
type OneOfTypeMapping struct {
	JSONValue string
	GoType    string
}

const unmarshalJSONTemplate = `
func (s *{{.StructName}}) UnmarshalJSON(data []byte) error {
	var discriminatorDoc struct {
		Value {{.DiscriminatorFieldType}} ` + "`json:\"{{.DiscriminatorFieldJSONTag}}\"`" + `
	}
	if err := json.Unmarshal(data, &discriminatorDoc); err != nil {
		return fmt.Errorf("failed to unmarshal discriminator value for field '{{.DiscriminatorFieldJSONTag}}': %w", err)
	}

	s.{{.DiscriminatorFieldName}} = discriminatorDoc.Value

	var raw{{.OneOfFieldName}} json.RawMessage
	var tempKnownFields struct {
		TargetContent *json.RawMessage ` + "`json:\"{{.OneOfFieldJSONTag}}\"`" + `
	}

	var fullMap map[string]*json.RawMessage
	if err := json.Unmarshal(data, &fullMap); err != nil {
		return fmt.Errorf("failed to unmarshal full object: %w", err)
	}

	if rawVal, ok := fullMap["{{.OneOfFieldJSONTag}}"]; ok && rawVal != nil {
		raw{{.OneOfFieldName}} = *rawVal
	}

	{{range .OtherFields}}
	if rawVal, ok := fullMap["{{.JSONTag}}"]; ok && rawVal != nil {
		var val {{.Type}}
		if err := json.Unmarshal(*rawVal, &val); err != nil {
			return fmt.Errorf("failed to unmarshal field '{{.JSONTag}}' into {{.Type}}: %w", err)
		}
		s.{{.Name}} = val
	}
	{{end}}

	switch discriminatorDoc.Value {
	{{range .OneOfTypes}}
	case "{{.JSONValue}}":
		var content {{.GoType}}
		if raw{{$.OneOfFieldName}} != nil {
			if err := json.Unmarshal(raw{{$.OneOfFieldName}}, &content); err != nil {
				return fmt.Errorf("failed to unmarshal {{$.OneOfFieldName}} as {{.GoType}} for discriminator '{{.JSONValue}}': %w", err)
			}
		}
		s.{{$.OneOfFieldName}} = content
	{{end}}
	default:
		if raw{{.OneOfFieldName}} != nil && len(raw{{$.OneOfFieldName}}) > 0 && string(raw{{$.OneOfFieldName}}) != "null" {
			return fmt.Errorf("unknown value for discriminator '{{.DiscriminatorFieldJSONTag}}': %s, cannot unmarshal '{{.OneOfFieldJSONTag}}'", discriminatorDoc.Value)
		}
	}
	return nil
}
`

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

	fmt.Println("DEBUG: All scanned types in the package:")
	for _, t := range pkgInfo.Types {
		fmt.Printf("  - Name: %s, Kind: %v, FilePath: %s, Doc: %q\n", t.Name, t.Kind, t.FilePath, t.Doc)
		if t.Kind == scanner.InterfaceKind && t.Interface != nil {
			fmt.Printf("    Interface Methods:\n")
			for _, m := range t.Interface.Methods {
				fmt.Printf("      - %s()\n", m.Name)
			}
		}
	}
	fmt.Println("--- END DEBUG ---")

	if pkgInfo.Name == "" {
	    files, _ := os.ReadDir(pkgPath)
	    isSinglePackage := false
	    for _, f := range files {
	        if !f.IsDir() && strings.HasSuffix(f.Name(), ".go") {
	            isSinglePackage = true;
	            break
	        }
	    }
	    if !isSinglePackage {
	         return fmt.Errorf("path %s does not appear to be a single Go package directory", pkgPath)
	    }
	}

	var generatedCodeForAllStructs bytes.Buffer
	needsImportEncodingJson := false
	needsImportFmt := false

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind {
			continue
		}
		if typeInfo.Doc == "" || !strings.Contains(typeInfo.Doc, unmarshalAnnotation) {
			continue
		}

		fmt.Printf("  Processing struct: %s for %s\n", typeInfo.Name, unmarshalAnnotation)

		data := TemplateData{
			PackageName: pkgInfo.Name,
			StructName:  typeInfo.Name,
		}
		oneOfInterfaceName := ""

		if typeInfo.Struct == nil {
		    fmt.Printf("  Warning: Struct %s has nil StructInfo, skipping\n", typeInfo.Name)
		    continue
		}

		for _, field := range typeInfo.Struct.Fields {
			jsonTag := ""
			if field.Tag != "" {
				var structTag reflect.StructTag = reflect.StructTag(field.Tag)
				jsonTagVal := structTag.Get("json")
				if commaIdx := strings.Index(jsonTagVal, ","); commaIdx != -1 {
					jsonTag = jsonTagVal[:commaIdx]
				} else {
					jsonTag = jsonTagVal
				}
			}

			var fieldKind scanner.Kind = -1
			var actualResolvedTypeInfo *scanner.TypeInfo

			if field.Type.PkgName == "" && !field.Type.IsPointer && !field.Type.IsSlice && !field.Type.IsMap {
				isPrimitive := false
				switch field.Type.Name {
				case "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "bool", "byte", "rune":
					isPrimitive = true
				}
				if !isPrimitive {
					for _, ti := range pkgInfo.Types {
						if ti.Name == field.Type.Name {
							actualResolvedTypeInfo = ti
							fieldKind = ti.Kind
							break
						}
					}
					if actualResolvedTypeInfo == nil {
						fmt.Printf("    Warning: Could not find local definition for type %s. Treating as opaque.\n", field.Type.Name)
					}
				}
			} else {
				var errResolve error
				actualResolvedTypeInfo, errResolve = field.Type.Resolve()
				if errResolve != nil {
					fmt.Printf("    Warning: Could not resolve type for field %s (%s): %v. Using raw name.\n", field.Name, field.Type.Name, errResolve)
				}
				if actualResolvedTypeInfo != nil {
					fieldKind = actualResolvedTypeInfo.Kind
				}
			}

			isDiscriminatorType := field.Type.Name == "string" && field.Type.PkgName == "" && !field.Type.IsPointer && !field.Type.IsSlice && !field.Type.IsMap
			if strings.ToLower(jsonTag) == "type" && isDiscriminatorType {
				data.DiscriminatorFieldName = field.Name
				data.DiscriminatorFieldType = "string"
				data.DiscriminatorFieldJSONTag = jsonTag
				fmt.Printf("      Found Discriminator: FieldName=%s, Type=%s, JSONTag=%s\n", field.Name, "string", jsonTag)
			} else if actualResolvedTypeInfo != nil && fieldKind == scanner.InterfaceKind {
				data.OneOfFieldName = field.Name
				data.OneOfFieldJSONTag = jsonTag
				data.OneOfFieldType = field.Type.Name
				oneOfInterfaceName = field.Type.Name
				fmt.Printf("      Found OneOf Field: FieldName=%s, Type=%s, JSONTag=%s\n", field.Name, field.Type.Name, jsonTag)
			} else {
				typeNameBuilder := &strings.Builder{}
				currentFieldType := field.Type
				for currentFieldType.IsPointer {
					typeNameBuilder.WriteString("*")
					if currentFieldType.Elem != nil {
						currentFieldType = currentFieldType.Elem
					} else {
						break
					}
				}
				if currentFieldType.IsSlice {
					typeNameBuilder.WriteString("[]")
					if currentFieldType.Elem != nil {
						elemTypeName := currentFieldType.Elem.Name
						if currentFieldType.Elem.IsPointer { elemTypeName = "*" + elemTypeName }
						typeNameBuilder.WriteString(elemTypeName)
					} else {
						typeNameBuilder.WriteString("interface{}")
					}
				} else if currentFieldType.IsMap {
					typeNameBuilder.WriteString("map[")
					if currentFieldType.MapKey != nil {
						keyTypeName := currentFieldType.MapKey.Name
						if currentFieldType.MapKey.IsPointer { keyTypeName = "*" + keyTypeName }
						typeNameBuilder.WriteString(keyTypeName)
					} else {
						typeNameBuilder.WriteString("interface{}")
					}
					typeNameBuilder.WriteString("]")
					if currentFieldType.Elem != nil {
						valueTypeName := currentFieldType.Elem.Name
						if currentFieldType.Elem.IsPointer { valueTypeName = "*" + valueTypeName }
						typeNameBuilder.WriteString(valueTypeName)
					} else {
						typeNameBuilder.WriteString("interface{}")
					}
				} else {
					typeNameBuilder.WriteString(currentFieldType.Name)
				}
				actualFieldTypeString := typeNameBuilder.String()
				data.OtherFields = append(data.OtherFields, FieldInfo{
					Name:    field.Name,
					Type:    actualFieldTypeString,
					JSONTag: jsonTag,
				})
				fmt.Printf("      Found Other Field: FieldName=%s, Type=%s, JSONTag=%s\n", field.Name, actualFieldTypeString, jsonTag)
			}
		}

		if data.DiscriminatorFieldName == "" || data.OneOfFieldName == "" {
			fmt.Printf("  Skipping struct %s: missing discriminator or oneOf field\n", typeInfo.Name)
			continue
		}

		if oneOfInterfaceName != "" {
			var oneOfInterfaceDef *scanner.TypeInfo
			for _, t := range pkgInfo.Types {
				if t.Name == oneOfInterfaceName && t.Kind == scanner.InterfaceKind {
					oneOfInterfaceDef = t
					break
				}
			}

			if oneOfInterfaceDef == nil {
				fmt.Printf("    Warning: Could not find TypeInfo for interface %s in package %s. Skipping oneOf mapping.\n", oneOfInterfaceName, pkgInfo.Name)
			} else if oneOfInterfaceDef.Interface == nil {
				fmt.Printf("    Warning: InterfaceInfo for interface %s in package %s is nil. Skipping oneOf mapping.\n", oneOfInterfaceName, pkgInfo.Name)
			} else {
				fmt.Printf("    Found definition for interface %s. Methods: %d\n", oneOfInterfaceName, len(oneOfInterfaceDef.Interface.Methods))
				for _, structCandidate := range pkgInfo.Types {
					if structCandidate.Kind != scanner.StructKind || structCandidate.Name == typeInfo.Name {
						continue
					}
					if goscan.Implements(structCandidate, oneOfInterfaceDef, pkgInfo) {
						jsonVal := strings.ToLower(structCandidate.Name)
						data.OneOfTypes = append(data.OneOfTypes, OneOfTypeMapping{
							JSONValue: jsonVal,
							GoType:    structCandidate.Name,
						})
						fmt.Printf("        Mapping: type \"%s\" -> %s (implements %s)\n", jsonVal, structCandidate.Name, oneOfInterfaceName)
					}
				}
			}
		}

		if oneOfInterfaceName != "" && len(data.OneOfTypes) == 0 {
			fmt.Printf("  Warning: For struct %s, no types found in package %s that implement the interface %s. Generated UnmarshalJSON might be incomplete.\n", typeInfo.Name, pkgInfo.Name, oneOfInterfaceName)
			// Not skipping generation for this warning, to see if at least an empty switch is made.
		}

		// Only skip if critical fields are missing, not if no impls found for a valid interface.
		// The check for missing discriminator/oneOf field is already above.

		tmpl, err := template.New("unmarshal").Parse(unmarshalJSONTemplate)
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}

		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, data); err != nil {
			return fmt.Errorf("failed to execute template for struct %s: %w", typeInfo.Name, err)
		}

		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")
		needsImportEncodingJson = true
		needsImportFmt = true
	} // This brace correctly closes the "for _, typeInfo" loop.

	if generatedCodeForAllStructs.Len() == 0 {
		fmt.Println("No structs found requiring UnmarshalJSON generation.")
		return nil
	}

	finalOutput := bytes.Buffer{}
	finalOutput.WriteString(fmt.Sprintf("// Code generated by derivingjson for package %s. DO NOT EDIT.\n\n", pkgInfo.Name))
	finalOutput.WriteString(fmt.Sprintf("package %s\n\n", pkgInfo.Name))

	if needsImportEncodingJson || needsImportFmt {
		finalOutput.WriteString("import (\n")
		if needsImportEncodingJson {
			finalOutput.WriteString("\t\"encoding/json\"\n")
		}
		if needsImportFmt {
			finalOutput.WriteString("\t\"fmt\"\n")
		}
		finalOutput.WriteString(")\n\n")
	}
	finalOutput.Write(generatedCodeForAllStructs.Bytes())

	formattedCode, err := format.Source(finalOutput.Bytes())
	if err != nil {
		fmt.Printf("Error formatting generated code for package %s: %v\nUnformatted code:\n%s\n", pkgInfo.Name, err, finalOutput.String())
		return fmt.Errorf("failed to format generated code for package %s: %w", pkgInfo.Name, err)
	}

	outputFileName := filepath.Join(pkgPath, fmt.Sprintf("%s_deriving.go", pkgInfo.Name))
	if _, err := os.Stat(outputFileName); err == nil {
		os.Remove(outputFileName)
	}

	err = os.WriteFile(outputFileName, formattedCode, 0644)
	if err != nil {
		return fmt.Errorf("failed to write generated code to %s: %w", outputFileName, err)
	}
	fmt.Printf("Generated code written to %s\n", outputFileName)

	return nil
}
