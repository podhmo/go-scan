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
	IsGo122     bool // True if Go version is 1.22 or later (for req.PathValue)
}

type FieldBindingInfo struct {
	FieldName    string // Name of the field in the struct (e.g., "UserID")
	FieldType    string // Go type of the field (e.g., "string", "int", "bool")
	BindFrom     string // "path", "query", "header", "cookie", "body"
	BindName     string // Name used for binding (e.g., path param name, query key, header key, cookie name)
	IsPointer    bool   // TODO: for pointer type support
	IsBody       bool   // True if this field represents the entire request body
	BodyJSONName string // json tag name if this field is part of a larger body struct
}

const bindMethodTemplate = `
func (s *{{.StructName}}) Bind(req *http.Request) error {
	var err error
	_ = err // prevent unused var error if no error handling is needed below

	{{range .Fields}}
	{{if eq .BindFrom "path"}}
	// Path parameter binding using req.PathValue (available in Go 1.22+)
	// If using Go < 1.22, this part will not compile.
	// You might need to use a router that puts path parameters in req.Context()
	// or handle path parsing manually for older Go versions.
	{{if $.IsGo122}}
	if pathValueStr := req.PathValue("{{.BindName}}"); pathValueStr != "" {
		{{if eq .FieldType "string"}}
		s.{{.FieldName}} = pathValueStr
		{{else if eq .FieldType "int"}}
		s.{{.FieldName}}, err = strconv.Atoi(pathValueStr)
		if err != nil {
			return fmt.Errorf("failed to bind path parameter \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{else if eq .FieldType "bool"}}
		s.{{.FieldName}}, err = strconv.ParseBool(pathValueStr)
		if err != nil {
			return fmt.Errorf("failed to bind path parameter \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{end}}
	} else {
		// Handle missing path parameter "{{.BindName}}" if necessary (e.g. return error or set default)
		// For now, if it's empty, we do nothing, field remains zero-value.
	}
	{{else}}
	// TODO: Path parameter binding for Go < 1.22 requires custom logic or a router.
	// Example: chi.URLParam(req, "{{.BindName}}")
	// Or for manual parsing (less robust):
	// pathParts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	// if len(pathParts) > index_of_{{.BindName}}_in_path { pathValueStr := pathParts[index_of_{{.BindName}}_in_path] ... }
	_ = req // placeholder to use req if no other bindings use it and path is skipped.
	{{end}}
	{{else if eq .BindFrom "query"}}
	if val := req.URL.Query().Get("{{.BindName}}"); val != "" {
		{{if eq .FieldType "string"}}
		s.{{.FieldName}} = val
		{{else if eq .FieldType "int"}}
		s.{{.FieldName}}, err = strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("failed to bind query parameter \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{else if eq .FieldType "bool"}}
		s.{{.FieldName}}, err = strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("failed to bind query parameter \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{end}}
	}
	{{else if eq .BindFrom "header"}}
	if val := req.Header.Get("{{.BindName}}"); val != "" {
		{{if eq .FieldType "string"}}
		s.{{.FieldName}} = val
		{{else if eq .FieldType "int"}}
		s.{{.FieldName}}, err = strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("failed to bind header \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{else if eq .FieldType "bool"}}
		s.{{.FieldName}}, err = strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("failed to bind header \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{end}}
	}
	{{else if eq .BindFrom "cookie"}}
	if cookie, cerr := req.Cookie("{{.BindName}}"); cerr == nil && cookie.Value != "" {
		{{if eq .FieldType "string"}}
		s.{{.FieldName}} = cookie.Value
		{{else if eq .FieldType "int"}}
		s.{{.FieldName}}, err = strconv.Atoi(cookie.Value)
		if err != nil {
			return fmt.Errorf("failed to bind cookie \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{else if eq .FieldType "bool"}}
		s.{{.FieldName}}, err = strconv.ParseBool(cookie.Value)
		if err != nil {
			return fmt.Errorf("failed to bind cookie \"{{.BindName}}\" to field {{.FieldName}}: %w", err)
		}
		{{end}}
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
		afterBodyProcessing: // Label for goto
	}
	{{end}}
	return nil
}
`

// isGo122orLater checks the go.mod file for the Go version.
// This is a simplified check and might need refinement for more complex go.mod files.
func isGo122orLater(gscn *goscan.Scanner) bool {
	if gscn.Module == nil || gscn.Module.GoVersion == "" {
		// Fallback or warning if go.mod isn't parsed or version isn't found
		// For safety, assume older version if undetermined.
		fmt.Println("Warning: Go version not found in go.mod, assuming pre-1.22 for path parameter binding.")
		return false
	}
	versionStr := gscn.Module.GoVersion
	// Expecting format like "1.22" or "1.22.0"
	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return false // Invalid format
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return false // Invalid format
	}

	return major > 1 || (major == 1 && minor >= 22)
}

func Generate(ctx context.Context, pkgPath string) error {
	gscn, err := goscan.New(".")
	if err != nil {
		return fmt.Errorf("failed to create go-scan scanner: %w", err)
	}
	// Ensure module info is loaded to check Go version
	if _, err := gscn.ScanPackage(ctx, pkgPath); err != nil {
		// this initial scan is primarily to ensure module info is loaded by go-scan
	}


	pkgInfo, err := gscn.ScanPackage(ctx, pkgPath) // Rescan or use cached if available
	if err != nil {
		return fmt.Errorf("go-scan failed to scan package at %s: %w", pkgPath, err)
	}

	isGo122 := isGo122orLater(gscn)
	if isGo122 {
		fmt.Println("Detected Go version 1.22 or later. Path parameters will use req.PathValue().")
	} else {
		fmt.Println("Detected Go version < 1.22. Path parameter binding will be placeholder. Consider upgrading Go or using a router for path params.")
	}

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
			IsGo122:     isGo122,
		}

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
			if strings.HasPrefix(fieldTypeStr, "*") {
				// For now, we only handle non-pointer types for string, int, bool conversion.
				// Pointers to these types can be a TODO.
				// For body, pointers are fine as json.Unmarshal handles them.
				if bindFrom != "body" {
					fmt.Printf("      Skipping field %s: pointer type %s for %s binding is a TODO\n", field.Name, fieldTypeStr, bindFrom)
					continue
				}
			}
			// Extract base type name for switch, e.g. "string" from "string" or "mypkg.MyString" -> "MyString"
			baseFieldType := field.Type.Name


			isPointer := false // TODO
			needsConversion := false
			switch baseFieldType { // Use baseFieldType for simple types
			case "string", "int", "bool":
				needsConversion = (baseFieldType == "int" || baseFieldType == "bool")
			default:
				if bindFrom != "body" {
					fmt.Printf("      Skipping field %s of unhandled type %s (%s) for %s binding\n", field.Name, fieldTypeStr, baseFieldType, bindFrom)
					continue
				}
			}

			fieldBindingInfo := FieldBindingInfo{
				FieldName: field.Name,
				FieldType: baseFieldType, // Use the simple type name for template logic (string, int, bool)
				BindFrom:  bindFrom,
				BindName:  bindName,
				IsPointer: isPointer,
			}

			if bindFrom == "body" {
				fieldBindingInfo.IsBody = true // This field itself is the target for the body
				data.NeedsBody = true
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
