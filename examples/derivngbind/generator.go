package main

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"net/http" // Added for http.ErrNoCookie

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

const bindingAnnotation = "@derivng:binding"

type TemplateData struct {
	PackageName string
	StructName  string
	Fields      []FieldBindingInfo
	Imports     map[string]string // alias -> path
	NeedsBody   bool
	HasSpecificBodyFieldTarget bool
	ErrNoCookie error // To allow comparison with http.ErrNoCookie in template
	// IsGo122     bool // No longer needed directly in template for path vars
}

type FieldBindingInfo struct {
	FieldName    string // Name of the field in the struct (e.g., "UserID")
	FieldType    string // Go type of the field (e.g., "string", "int", "bool")
	BindFrom     string // "path", "query", "header", "cookie", "body"
	BindName     string // Name used for binding (e.g., path param name, query key, header key, cookie name)
	IsPointer    bool
	IsRequired   bool
	IsBody       bool   // True if this field represents the entire request body
	BodyJSONName string // json tag name if this field is part of a larger body struct
}

const bindMethodTemplate = `
func (s *{{.StructName}}) Bind(req *http.Request, pathVar func(string) string) error {
	var err error
	_ = err // prevent unused var error if no error handling is needed below

	{{range .Fields}}
	{{if eq .BindFrom "path"}}
	// Path parameter binding for field {{.FieldName}} ({{.FieldType}}) from "{{.BindName}}"
	if pathValueStr := pathVar("{{.BindName}}"); pathValueStr != "" {
		{{if .IsPointer}}
			{{if eq .FieldType "string"}}
			s.{{.FieldName}} = &pathValueStr
			{{else if eq .FieldType "int"}}
			v, err := strconv.Atoi(pathValueStr)
			if err != nil {
				return fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to int for field {{.FieldName}}: %w", pathValueStr, err)
			}
			s.{{.FieldName}} = &v
			{{else if eq .FieldType "bool"}}
			v, err := strconv.ParseBool(pathValueStr)
			if err != nil {
				return fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", pathValueStr, err)
			}
			s.{{.FieldName}} = &v
			{{end}}
		{{else}} {{/* Not a pointer */}}
			{{if eq .FieldType "string"}}
			s.{{.FieldName}} = pathValueStr
			{{else if eq .FieldType "int"}}
			s.{{.FieldName}}, err = strconv.Atoi(pathValueStr)
			if err != nil {
				return fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to int for field {{.FieldName}}: %w", pathValueStr, err)
			}
			{{else if eq .FieldType "bool"}}
			s.{{.FieldName}}, err = strconv.ParseBool(pathValueStr)
			if err != nil {
				return fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", pathValueStr, err)
			}
			{{end}}
		{{end}}
	} else {
		{{if .IsRequired}}
		return fmt.Errorf("required path parameter \"{{.BindName}}\" for field {{.FieldName}} is missing")
		{{else if .IsPointer}}
		s.{{.FieldName}} = nil // Explicitly set to nil for clarity, though it's default
		{{end}}
		// For non-pointer, non-required, missing path param means field remains zero-value.
	}
	{{else if eq .BindFrom "query"}}
	// Query parameter binding for field {{.FieldName}} ({{.FieldType}}) from "{{.BindName}}"
	{{$bindName := .BindName}}
	{{$fieldName := .FieldName}}
	{{$fieldType := .FieldType}}
	{{$isPointer := .IsPointer}}
	{{$isRequired := .IsRequired}}
	if req.URL.Query().Has("{{$bindName}}") {
		val := req.URL.Query().Get("{{$bindName}}")
		{{if eq $fieldType "string"}}
			{{if $isPointer}}
		s.{{$fieldName}} = &val
			{{else}}
		s.{{$fieldName}} = val
			{{end}}
		{{else if eq $fieldType "int"}}
			v, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("failed to convert query parameter \"{{$bindName}}\" (value: %q) to int for field {{$fieldName}}: %w", val, err)
			}
			{{if $isPointer}}
		s.{{$fieldName}} = &v
			{{else}}
		s.{{$fieldName}} = v
			{{end}}
		{{else if eq $fieldType "bool"}}
			v, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("failed to convert query parameter \"{{$bindName}}\" (value: %q) to bool for field {{$fieldName}}: %w", val, err)
			}
			{{if $isPointer}}
		s.{{$fieldName}} = &v
			{{else}}
		s.{{$fieldName}} = v
			{{end}}
		{{end}}
	} else { // Key does not exist
		{{if $isRequired}}
		return fmt.Errorf("required query parameter \"{{$bindName}}\" for field {{$fieldName}} is missing")
		{{else if $isPointer}}
		s.{{$fieldName}} = nil
		{{end}}
		// For non-pointer, non-required, missing param means field remains zero-value.
	}
	{{else if eq .BindFrom "header"}}
	// Header binding for field {{.FieldName}} ({{.FieldType}}) from "{{.BindName}}"
	if val := req.Header.Get("{{.BindName}}"); val != "" {
		{{if .IsPointer}}
			{{if eq .FieldType "string"}}
			s.{{.FieldName}} = &val
			{{else if eq .FieldType "int"}}
			v, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to int for field {{.FieldName}}: %w", val, err)
			}
			s.{{.FieldName}} = &v
			{{else if eq .FieldType "bool"}}
			v, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", val, err)
			}
			s.{{.FieldName}} = &v
			{{end}}
		{{else}} {{/* Not a pointer */}}
			{{if eq .FieldType "string"}}
			s.{{.FieldName}} = val
			{{else if eq .FieldType "int"}}
			s.{{.FieldName}}, err = strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to int for field {{.FieldName}}: %w", val, err)
			}
			{{else if eq .FieldType "bool"}}
			s.{{.FieldName}}, err = strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", val, err)
			}
			{{end}}
		{{end}}
	} else {
		{{if .IsRequired}}
		return fmt.Errorf("required header \"{{.BindName}}\" for field {{.FieldName}} is missing")
		{{else if .IsPointer}}
		s.{{.FieldName}} = nil
		{{end}}
	}
	{{else if eq .BindFrom "cookie"}}
	// Cookie binding for field {{.FieldName}} ({{.FieldType}}) from "{{.BindName}}"
	if cookie, cerr := req.Cookie("{{.BindName}}"); cerr == nil && cookie.Value != "" {
		val := cookie.Value
		{{if .IsPointer}}
			{{if eq .FieldType "string"}}
			s.{{.FieldName}} = &val
			{{else if eq .FieldType "int"}}
			v, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to int for field {{.FieldName}}: %w", val, err)
			}
			s.{{.FieldName}} = &v
			{{else if eq .FieldType "bool"}}
			v, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", val, err)
			}
			s.{{.FieldName}} = &v
			{{end}}
		{{else}} {{/* Not a pointer */}}
			{{if eq .FieldType "string"}}
			s.{{.FieldName}} = val
			{{else if eq .FieldType "int"}}
			s.{{.FieldName}}, err = strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to int for field {{.FieldName}}: %w", val, err)
			}
			{{else if eq .FieldType "bool"}}
			s.{{.FieldName}}, err = strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", val, err)
			}
			{{end}}
		{{end}}
	} else { // Cookie not found or value is empty
		{{if .IsRequired}}
return fmt.Errorf("required cookie \"{{.BindName}}\" for field {{.FieldName}} is missing, empty, or could not be retrieved")
		{{else if .IsPointer}}
		s.{{.FieldName}} = nil
		{{end}}
		// If cerr is .ErrNoCookie and not required, it's fine. Field remains nil/zero.
		// If other cerr, it might be an issue even if not required, but current logic is to ignore.
	}
	{{end}}
	{{end}}

	{{if .NeedsBody}}
	if req.Body != nil && req.Body != http.NoBody {
		isSpecificFieldBodyTarget := false
		{{range .Fields}}
		{{if .IsBody}}
		isSpecificFieldBodyTarget = true
		{{end}}
		{{end}}

		if isSpecificFieldBodyTarget {
			{{range .Fields}}
			{{if .IsBody}}
			// Field {{.FieldName}} is the target for the entire request body
			if err := json.NewDecoder(req.Body).Decode(&s.{{.FieldName}}); err != nil {
				if err != io.EOF { // EOF might be acceptable if body is optional
					return fmt.Errorf("failed to decode request body into field {{.FieldName}}: %w", err)
				}
			}
			goto afterBodyProcessing // Process only one 'in:"body"' field
			{{end}}
			{{end}}
		} else {
			// The struct {{.StructName}} itself is the target for the request body
			if err := json.NewDecoder(req.Body).Decode(s); err != nil {
				if err != io.EOF { // EOF might be acceptable
					return fmt.Errorf("failed to decode request body into {{.StructName}}: %w", err)
				}
			}
		}
		{{if .HasSpecificBodyFieldTarget}}
		afterBodyProcessing: // Label for goto only if there was a specific body field target that could jump here
		{{end}}
	}
	{{end}}
	return nil
}
`

// isGo122orLater checks the go.mod file for the Go version.
// This function is kept for now as it might be useful for other features,
// but it's not strictly necessary for the current path parameter handling.
// func isGo122orLater(gscn *goscan.Scanner) bool {
// 	if gscn.Module == nil || gscn.Module.GoVersion == "" {
// 		// Fallback or warning if go.mod isn't parsed or version isn't found
// 		// For safety, assume older version if undetermined.
// 		fmt.Println("Warning: Go version not found in go.mod, assuming pre-1.22 for path parameter binding.")
// 		return false
// 	}
// 	versionStr := gscn.Module.GoVersion
// 	// Expecting format like "1.22" or "1.22.0"
// 	parts := strings.Split(versionStr, ".")
// 	if len(parts) < 2 {
// 		return false // Invalid format
// 	}
// 	major, errMajor := strconv.Atoi(parts[0])
// 	minor, errMinor := strconv.Atoi(parts[1])
// 	if errMajor != nil || errMinor != nil {
// 		return false // Invalid format
// 	}

// 	return major > 1 || (major == 1 && minor >= 22)
// }

func Generate(ctx context.Context, pkgPath string) error {
	gscn, err := goscan.New(".")
	if err != nil {
		return fmt.Errorf("failed to create go-scan scanner: %w", err)
	}
	// Scan the package to get its info.
	pkgInfo, err := gscn.ScanPackage(ctx, pkgPath)
	if err != nil {
		return fmt.Errorf("go-scan failed to scan package at %s: %w", pkgPath, err)
	}

	// isGo122 := isGo122orLater(gscn) // No longer strictly needed for path vars
	// if isGo122 {
	// 	fmt.Println("Detected Go version 1.22 or later.") // Info message can be removed or adapted
	// } else {
	// 	fmt.Println("Detected Go version < 1.22.") // Info message can be removed or adapted
	// }

	var generatedCodeForAllStructs bytes.Buffer
	allFileImports := make(map[string]string) // path -> alias
	needsImportStrconv := false
	needsImportNetHTTP := false
	needsImportFmt := false
	needsImportEncodingJson := false
	needsImportIO := false


	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}
		hasBindingAnnotationOnStruct := strings.Contains(typeInfo.Doc, bindingAnnotation)
		structLevelInTag := ""
		if hasBindingAnnotationOnStruct {
			// Extract in:"xxx" from struct doc comment if present
			// Example: @derivng:binding in:"body"
			docLines := strings.Split(typeInfo.Doc, "\n")
			for _, line := range docLines {
				if strings.Contains(line, bindingAnnotation) {
					parts := strings.Fields(line) // Split by space
					for _, part := range parts {
						if strings.HasPrefix(part, "in:") {
							structLevelInTag = strings.TrimSuffix(strings.SplitN(part, ":", 2)[1], `"`)
							structLevelInTag = strings.TrimPrefix(structLevelInTag, `"`)
							break
						}
					}
				}
				if structLevelInTag != "" {
					break
				}
			}
		}


		if !hasBindingAnnotationOnStruct {
			continue
		}
		fmt.Printf("  Processing struct: %s for %s\n", typeInfo.Name, bindingAnnotation)

		data := TemplateData{
			PackageName: pkgInfo.Name,
			StructName:  typeInfo.Name,
			Imports:     make(map[string]string),
			Fields:      []FieldBindingInfo{},
			NeedsBody:   (structLevelInTag == "body"),
			HasSpecificBodyFieldTarget: false, // Initialize
			ErrNoCookie: http.ErrNoCookie, // Set http.ErrNoCookie for template
			// IsGo122:     isGo122, // No longer needed here
		}
		needsImportNetHTTP = true // For http.ErrNoCookie

		for _, field := range typeInfo.Struct.Fields {
			tag := reflect.StructTag(field.Tag)
			inTagVal := tag.Get("in")
			bindFrom := ""
			bindName := "" // This will be sourced from specific tags like path:"<name>", query:"<name>"

			if inTagVal != "" {
				bindFrom = strings.ToLower(strings.TrimSpace(inTagVal))
				switch bindFrom {
				case "path":
					bindName = tag.Get("path")
				case "query":
					bindName = tag.Get("query")
				case "header":
					bindName = tag.Get("header")
				case "cookie":
					bindName = tag.Get("cookie")
				case "body":
					// For `in:"body"`, bindName is not used from another tag for the field itself.
					// The field *is* the body.
					data.NeedsBody = true // Ensure NeedsBody is true if any field is in:body
				default:
					fmt.Printf("      Skipping field %s: unknown 'in' tag value '%s'\n", field.Name, inTagVal)
					continue
				}
				if bindFrom != "body" && bindName == "" {
					fmt.Printf("      Skipping field %s: 'in:\"%s\"' tag requires corresponding '%s:\"name\"' tag\n", field.Name, bindFrom, bindFrom)
					continue
				}
			} else if data.NeedsBody { // structLevelInTag was "body", and this field has no specific "in" tag
				// This field is part of the JSON body. Its JSON name comes from the "json" tag.
				// The template handles this by decoding into the whole struct 's'.
				// We don't need to add it to Fields for individual binding logic here,
				// unless the template becomes more granular for struct-level body.
				// For now, skip adding to data.Fields if it's just part of a struct-level body.
				continue
			} else {
				// No "in" tag and struct is not "in:body" globally.
				continue
			}

			fieldTypeStr := field.Type.Name // Simplified: does not show package for external types.
                                       // For `*pkg.Type`, it's `Type`. `scanner.FieldType.String()` would be more complete.
                                       // Let's use field.Type.String() for better accuracy.
			fieldTypeStr = field.Type.String()
			isPointer := strings.HasPrefix(fieldTypeStr, "*")
			// baseFieldType := field.Type.Name // This is the name of the type, e.g. "String" for "*string" if it's a defined type, or "string" for "*string" (primitive)
                                          // For primitive types like *string, *int, Type.Name will be the underlying type name ("string", "int")
                                          // For named types like *models.MyString, Type.Name will be "MyString"
                                          // We need the underlying primitive type for conversion logic.

			actualFieldTypeForTemplate := ""
			if isPointer {
				// For a pointer type like "*string", field.Type.Elem.Name might give "string"
				// or field.Type.Elem.String() might give "string" or "pkg.Type"
				// We need the simple name for the template's switch cases.
				// Let's try to get the name of the element type.
				if field.Type.Elem != nil {
					elemName := field.Type.Elem.Name
					elemString := field.Type.Elem.String()
					fmt.Printf("DEBUG: Field %s, Pointer Elem Name: '%s', Elem String: '%s'\n", field.Name, elemName, elemString)
					actualFieldTypeForTemplate = elemName
					if actualFieldTypeForTemplate == "" && elemString != "" {
						actualFieldTypeForTemplate = elemString
					}
					// If still empty, or if it contains ".", it might be a qualified type like "pkg.Type"
					// For basic types, we want "string", "int", "bool".
					// The String() method on scanner.FieldType usually gives the correct representation.
					if actualFieldTypeForTemplate == "" || strings.Contains(actualFieldTypeForTemplate, ".") {
					    // Let's prefer Elem.String() if Name is complex or empty, assuming String() gives a good representation
					    // For primitive types, Elem.Name should be like "string", Elem.String() also "string"
					    // For named types like *myType (where myType is string), Name="myType", String="pkg.myType"
					    // For template switch, we need the base primitive name if it's an alias of one.
					    // This part is tricky without knowing scanner.FieldType's exact behavior for aliased primitives.
					    // For now, rely on Elem.Name and fallback to Elem.String(), then check against known types.
					    // The current logic is: Name, then String(). If it results in "string", "int", "bool", it's fine.
					    // If it's "mypkg.MyInt", the switch below will skip it unless "mypkg.MyInt" is added.
					    // We are trying to get "string", "int", "bool" for actualFieldTypeForTemplate.
					    // Let's assume Elem.Name is the primary source for the simple name if available.
						if field.Type.Elem.Name != "" { // Prefer simple name if available
							actualFieldTypeForTemplate = field.Type.Elem.Name
						} else {
							actualFieldTypeForTemplate = field.Type.Elem.String() // Fallback to String()
						}
					}
				} else {
					fmt.Printf("      Skipping field %s: pointer type %s has nil Elem (field.Type.Elem is nil)\n", field.Name, fieldTypeStr)
					continue
				}
			} else {
				actualFieldTypeForTemplate = field.Type.Name
				if actualFieldTypeForTemplate == "" && field.Type.String() != "" { // For non-pointer built-in types
					actualFieldTypeForTemplate = field.Type.String()
				}
			}


			// For external packages, Name() might be "MyType" and String() "mypkg.MyType"
			// The template expects simple types like "string", "int", "bool".
			// This logic needs to be robust. For now, we assume Name() is sufficient if not pointer,
			// and Elem().Name() or Elem().String() if pointer, for basic supported types.
			// This might need refinement for complex or aliased types from other packages.

			isRequiredTag := tag.Get("required")
			isRequired := (isRequiredTag == "true")

			needsConversion := false
			switch actualFieldTypeForTemplate {
			case "string", "int", "bool":
				needsConversion = (actualFieldTypeForTemplate == "int" || actualFieldTypeForTemplate == "bool")
			default:
				if bindFrom != "body" { // Body binding uses json.Unmarshal, which handles various types.
					fmt.Printf("      Skipping field %s of unhandled type %s (template type %s) for %s binding\n", field.Name, fieldTypeStr, actualFieldTypeForTemplate, bindFrom)
					continue
				}
			}

			fieldBindingInfo := FieldBindingInfo{
				FieldName:  field.Name,
				FieldType:  actualFieldTypeForTemplate, // Use the determined simple type name
				BindFrom:   bindFrom,
				BindName:   bindName,
				IsPointer:  isPointer,
				IsRequired: isRequired,
			}

			if bindFrom == "body" {
				fieldBindingInfo.IsBody = true // This field itself is the target for the body
				data.NeedsBody = true
				data.HasSpecificBodyFieldTarget = true // Set this flag
				needsImportEncodingJson = true
				needsImportIO = true
			}

			if bindFrom != "body" {
				needsImportNetHTTP = true
				if needsConversion {
					needsImportStrconv = true
				}
				needsImportFmt = true // For error messages
			}

			data.Fields = append(data.Fields, fieldBindingInfo)
		}

		if len(data.Fields) == 0 && !data.NeedsBody { // If no fields to bind and struct is not body target
			fmt.Printf("  Skipping struct %s: no bindable fields found and not a global body target.\n", typeInfo.Name)
			continue
		}

		// Manage imports
		if needsImportNetHTTP {
			allFileImports["net/http"] = ""
		}
		if needsImportStrconv {
			allFileImports["strconv"] = ""
		}
		if needsImportFmt {
			allFileImports["fmt"] = ""
		}
		if needsImportEncodingJson { // This might also be true if data.NeedsBody is true from struct level
			allFileImports["encoding/json"] = ""
		}
		if needsImportIO { // This might also be true if data.NeedsBody is true from struct level
			allFileImports["io"] = ""
		}
		if data.NeedsBody && !needsImportEncodingJson { // Ensure json/io are imported if struct is body target
		    allFileImports["encoding/json"] = ""
			needsImportEncodingJson = true
		}
		if data.NeedsBody && !needsImportIO {
		    allFileImports["io"] = ""
			needsImportIO = true
		}


		tmpl, err := template.New("bind").Parse(bindMethodTemplate)
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}
		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, data); err != nil {
			return fmt.Errorf("failed to execute template for struct %s: %w", typeInfo.Name, err)
		}
		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")
	}

	if generatedCodeForAllStructs.Len() == 0 {
		fmt.Println("No structs found requiring Bind method generation.")
		return nil
	}

	finalOutput := bytes.Buffer{}
	finalOutput.WriteString(fmt.Sprintf("// Code generated by derivngbind for package %s. DO NOT EDIT.\n\n", pkgInfo.Name))
	finalOutput.WriteString(fmt.Sprintf("package %s\n\n", pkgInfo.Name))

	if len(allFileImports) > 0 {
		finalOutput.WriteString("import (\n")
		sortedImports := []string{}
		for path := range allFileImports {
			sortedImports = append(sortedImports, path)
		}
		// Sort imports for consistent output
		// For now, direct iteration is fine, but sorting would be better for stability.
		// sort.Strings(sortedImports) // if stability is needed
		for path, alias := range allFileImports { // Use allFileImports to get alias if any (though current setup is no alias)
			if alias == "" {
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
		// fmt.Printf("Error formatting generated code for package %s: %v\n--- Unformatted Code ---\n%s\n--- End Unformatted Code ---\n", pkgInfo.Name, err, finalOutput.String())
		// return fmt.Errorf("failed to format generated code for package %s: %w", pkgInfo.Name, err)
		// If formatting fails, write the unformatted code for debugging
		fmt.Printf("Warning: Error formatting generated code for package %s: %v. Writing unformatted code.\n", pkgInfo.Name, err)
		formattedCode = finalOutput.Bytes()
	}
	outputFileName := filepath.Join(pkgPath, fmt.Sprintf("%s_deriving.go", strings.ToLower(pkgInfo.Name)))
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
