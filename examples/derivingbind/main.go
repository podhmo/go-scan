package main

import (
	"bytes"
	"context"
	"embed" // Added
	"fmt"
	"go/format"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

//go:embed bind_method.tmpl
var bindMethodTemplateFS embed.FS

//go:embed bind_method.tmpl
var bindMethodTemplateString string

func main() {
	ctx := context.Background() // Or your application's context

	if len(os.Args) <= 1 {
		slog.ErrorContext(ctx, "Usage: derivingbind <package_path>")
		slog.ErrorContext(ctx, "Example: derivingbind examples/derivingbind/testdata/simple")
		os.Exit(1)
	}
	pkgPath := os.Args[1]

	stat, err := os.Stat(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.ErrorContext(ctx, "Package path does not exist", slog.String("package_path", pkgPath))
		} else {
			slog.ErrorContext(ctx, "Error accessing package path", slog.String("package_path", pkgPath), slog.Any("error", err))
		}
		os.Exit(1)
	}
	if !stat.IsDir() {
		slog.ErrorContext(ctx, "Package path is not a directory", slog.String("package_path", pkgPath))
		os.Exit(1)
	}

	slog.InfoContext(ctx, "Generating Bind method for package", slog.String("package_path", pkgPath))
	if err := Generate(ctx, pkgPath); err != nil { // Generate will be in generator.go
		slog.ErrorContext(ctx, "Error generating code", slog.Any("error", err))
		os.Exit(1)
	}
	slog.InfoContext(ctx, "Successfully generated Bind methods for package", slog.String("package_path", pkgPath))
}

const bindingAnnotation = "@derivng:binding"

type TemplateData struct {
	PackageName                string
	StructName                 string
	Fields                     []FieldBindingInfo
	Imports                    map[string]string // alias -> path
	NeedsBody                  bool
	HasSpecificBodyFieldTarget bool
	ErrNoCookie                error // For template: http.ErrNoCookie
	// IsGo122     bool // No longer needed directly in template for path vars
}

type FieldBindingInfo struct {
	FieldName    string // Name of the field in the struct (e.g., "UserID")
	FieldType    string // Go type of the field (e.g., "string", "int", "bool")
	BindFrom     string // "path", "query", "header", "cookie", "body"
	BindName     string // Name used for binding (e.g., path param name, query key, header key, cookie name)
	IsPointer    bool   // No longer TODO
	IsRequired   bool   // Added
	IsBody       bool   // True if this field represents the entire request body
	BodyJSONName string // json tag name if this field is part of a larger body struct

	// Extended fields for slice and numeric types
	IsSlice                 bool   // True if the field is a slice
	SliceElementType        string // Type of the elements in the slice (e.g., "string", "int", "*float64")
	OriginalFieldTypeString string // Full type string from scanner.FieldType.String()
	// BitSize                 int    // Bit size for numeric types (e.g., 32, 64) // Removed
	// IsNumeric               bool   // True if the field is a numeric type (int, float) // Removed
	// IsFloat                 bool   // True if the field is a float type - Will be removed // Removed
	// IsSigned                bool   // True if the field is a signed integer type - Will be removed // Removed
	// IsComplex               bool   // True if the field is a complex type - Will be removed // Removed
	ParserFunc            string // e.g. "parser.Int", "parser.String"
	IsSliceElementPointer bool   // True if the slice element is a pointer, e.g. []*int
}

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
	// Always import binding and parser for now to fix the test issue
	allFileImports["github.com/podhmo/go-scan/examples/derivingbind/binding"] = ""
	allFileImports["github.com/podhmo/go-scan/examples/derivingbind/parser"] = ""
	// needsImportStrconv := false // No longer needed at this scope
	needsImportNetHTTP := false
	needsImportFmt := false
	needsImportEncodingJson := false
	needsImportIO := false
	needsImportErrors := false // Added for errors.Join
	// needsImportStrings will be implicitly handled by adding to allFileImports

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
			PackageName:                pkgInfo.Name,
			StructName:                 typeInfo.Name,
			Imports:                    make(map[string]string),
			Fields:                     []FieldBindingInfo{},
			NeedsBody:                  (structLevelInTag == "body"),
			HasSpecificBodyFieldTarget: false, // Initialize
			ErrNoCookie:                http.ErrNoCookie,
			// IsGo122:     isGo122,
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

			fInfo := FieldBindingInfo{
				FieldName:               field.Name,
				BindFrom:                bindFrom,
				BindName:                bindName,
				IsRequired:              (tag.Get("required") == "true"),
				OriginalFieldTypeString: field.Type.String(),
				IsPointer:               field.Type.IsPointer, // Base IsPointer for the field itself
			}

			// Determine detailed type information
			currentScannerType := field.Type // This is a *scanner.FieldType

			// This is the type that will be used in the template's switch/if conditions for conversion logic.
			// For simple types (int, *int), it's the base type ("int").
			// For slice types ([]int, []*int), it's the slice's element's base type ("int").
			baseTypeForConversion := ""
			// isElementPointer := false // We can infer this from SliceElementType if it starts with "*"

			if currentScannerType.IsSlice {
				fInfo.IsSlice = true
				if currentScannerType.Elem != nil {
					fInfo.SliceElementType = currentScannerType.Elem.String()       // e.g., "int", "*string", "pkg.MyType", "*pkg.MyType"
					fInfo.IsSliceElementPointer = currentScannerType.Elem.IsPointer // Check if the element itself is a pointer

					// Determine the baseTypeForConversion from the slice element
					sliceElemScannerType := currentScannerType.Elem
					if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem != nil { // e.g. []*int, Elem is *int, Elem.Elem is int
						baseTypeForConversion = sliceElemScannerType.Elem.Name // "int"
					} else if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem == nil { // e.g. []*ExternalType
						baseTypeForConversion = sliceElemScannerType.Name // "ExternalType" - This case might need more thought for parser assignment if not a basic type
					} else { // e.g. []int or []ExternalType
						baseTypeForConversion = sliceElemScannerType.Name // "int" or "ExternalType"
					}
				} else {
					fmt.Printf("      Skipping field %s: slice with nil Elem type\n", field.Name)
					continue
				}
			} else if currentScannerType.IsPointer {
				// fInfo.IsPointer is already true
				if currentScannerType.Elem != nil { // e.g. *int, Elem is int
					baseTypeForConversion = currentScannerType.Elem.Name // "int"
				} else { // e.g. *ExternalType where ExternalType is not further broken down by go-scan's Elem
					baseTypeForConversion = currentScannerType.Name // "ExternalType"
					if baseTypeForConversion == "" && bindFrom != "body" {
						fmt.Printf("      Warning: Pointer field %s (%s) - field.Type.Elem is nil and field.Type.Name is empty. Original type: %s. Skipping non-body binding.\n", field.Name, fInfo.OriginalFieldTypeString, field.Type.String())
						continue
					}
				}
			} else { // Not a slice, not a pointer (e.g. int, string, ExternalType)
				baseTypeForConversion = currentScannerType.Name // "int", "string", "ExternalType"
				if baseTypeForConversion == "" && bindFrom != "body" {
					fmt.Printf("      Warning: Field %s (%s) - field.Type.Name is empty for non-slice/non-pointer. Original type: %s. Skipping non-body binding.\n", field.Name, fInfo.OriginalFieldTypeString, field.Type.String())
					continue
				}
			}

			fInfo.FieldType = baseTypeForConversion // This is what template's {{.FieldType}} will be, e.g., "int", "string", "MyStruct"

			// Assign ParserFunc based on baseTypeForConversion
			switch baseTypeForConversion {
			case "string":
				fInfo.ParserFunc = "parser.String"
			case "int":
				fInfo.ParserFunc = "parser.Int"
			case "int8":
				fInfo.ParserFunc = "parser.Int8"
			case "int16":
				fInfo.ParserFunc = "parser.Int16"
			case "int32":
				fInfo.ParserFunc = "parser.Int32"
			case "int64":
				fInfo.ParserFunc = "parser.Int64"
			case "uint":
				fInfo.ParserFunc = "parser.Uint"
			case "uint8":
				fInfo.ParserFunc = "parser.Uint8"
			case "uint16":
				fInfo.ParserFunc = "parser.Uint16"
			case "uint32":
				fInfo.ParserFunc = "parser.Uint32"
			case "uint64":
				fInfo.ParserFunc = "parser.Uint64"
			case "bool":
				fInfo.ParserFunc = "parser.Bool"
			case "float32":
				fInfo.ParserFunc = "parser.Float32"
			case "float64":
				fInfo.ParserFunc = "parser.Float64"
			case "uintptr":
				fInfo.ParserFunc = "parser.Uintptr"
			case "complex64":
				fInfo.ParserFunc = "parser.Complex64"
			case "complex128":
				fInfo.ParserFunc = "parser.Complex128"
			default:
				if bindFrom != "body" {
					fmt.Printf("      Skipping field %s: unhandled base type '%s' (original: %s, slice: %t) for %s binding. No parser available.\n", field.Name, baseTypeForConversion, fInfo.OriginalFieldTypeString, fInfo.IsSlice, bindFrom)
					continue
				}
				// For 'in:"body"', ParserFunc is not used, so allow it.
			}

			if bindFrom != "body" {
				allFileImports["github.com/podhmo/go-scan/examples/derivingbind/binding"] = ""
				allFileImports["github.com/podhmo/go-scan/examples/derivingbind/parser"] = ""
				needsImportNetHTTP = true   // For req *http.Request in Bind method signature
				needsImportErrors = true    // For errors.Join
				if fInfo.ParserFunc == "" { // Should not happen if logic above is correct for non-body
					fmt.Printf("      Skipping field %s: No parser function assigned for non-body binding.\n", field.Name)
					continue
				}
			} else { // This field is 'in:"body"'
				fInfo.IsBody = true
				data.NeedsBody = true
				data.HasSpecificBodyFieldTarget = true // A specific field is the body target
				needsImportEncodingJson = true
				needsImportIO = true
				needsImportNetHTTP = true // For req *http.Request
				needsImportFmt = true     // For fmt.Errorf potentially
				needsImportErrors = true  // For errors.Join potentially
			}
			data.Fields = append(data.Fields, fInfo)
		}

		if len(data.Fields) == 0 && !data.NeedsBody {
			fmt.Printf("  Skipping struct %s: no bindable fields found and not a global body target.\n", typeInfo.Name)
			continue
		}

		// If struct itself is body target (no specific field has in:body)
		if data.NeedsBody && !data.HasSpecificBodyFieldTarget {
			needsImportEncodingJson = true
			needsImportIO = true
			needsImportNetHTTP = true
			needsImportFmt = true
			needsImportErrors = true
		}

		// Consolidate import needs
		if needsImportNetHTTP {
			allFileImports["net/http"] = ""
		}
		if needsImportEncodingJson {
			allFileImports["encoding/json"] = ""
		}
		if needsImportIO {
			allFileImports["io"] = ""
		}
		if needsImportFmt { // Potentially used by body decoding errors
			allFileImports["fmt"] = ""
		}
		if needsImportErrors {
			allFileImports["errors"] = ""
		}
		// strconv and strings are no longer directly needed by the template for non-body fields.
		// If body processing uses them, they would be added there, but current body uses json.

		funcMap := template.FuncMap{
			"TitleCase": strings.Title, // Used for binding.Query, binding.Path etc.
		}

		tmpl, err := template.New("bind").Funcs(funcMap).Parse(bindMethodTemplateString)
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
	finalOutput.WriteString(fmt.Sprintf("// Code generated by derivingbind for package %s. DO NOT EDIT.\n\n", pkgInfo.Name))
	finalOutput.WriteString(fmt.Sprintf("package %s\n\n", pkgInfo.Name))

	// No need to add "errors" here again if needsImportErrors was managed correctly
	// if needsImportErrors {
	// 	allFileImports["errors"] = ""
	// }

	if len(allFileImports) > 0 {
		finalOutput.WriteString("import (\n")
		paths := make([]string, 0, len(allFileImports))
		for path := range allFileImports {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			alias := allFileImports[path]
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

// Helper function to check if a base type string is numeric or boolean
func isNumericOrBool(baseType string) bool {
	switch baseType {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr", // uintptr added here
		"float32", "float64", "complex64", "complex128", "bool": // complex types added here
		return true
	default:
		return false
	}
}

// Helper function to check if a slice element type is one of the directly convertible primitives
// (string, or numeric/bool that strconv can handle)
func isWellKnownSliceElementType(sliceElementType string) bool {
	// Check for pointer prefix and get base type
	base := sliceElementType
	if strings.HasPrefix(base, "*") && len(base) > 1 {
		base = base[1:]
	}
	switch base {
	case "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128", "bool", "uintptr": // Added complex, uintptr
		return true
	default:
		// Could also check against a list of registered external types if that becomes a feature
		return false
	}
}
