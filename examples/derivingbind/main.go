package main

import (
	"bytes"
	"context"
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

const bindingAnnotation = "@deriving:binding" // Corrected here

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
	BitSize                 int    // Bit size for numeric types (e.g., 32, 64)
	IsNumeric               bool   // True if the field is a numeric type (int, float)
	IsFloat                 bool   // True if the field is a float type
	IsSigned                bool   // True if the field is a signed integer type
	IsComplex               bool   // True if the field is a complex type
}

const bindMethodTemplate = `
func (s *{{.StructName}}) Bind(req *http.Request, pathVar func(string) string) error {
	var errs []error

	{{range .Fields}}
	{{if eq .BindFrom "path"}}
	// Path parameter binding for field {{.FieldName}} ({{.OriginalFieldTypeString}}) from "{{.BindName}}"
	{{if .IsSlice}}
	// TODO: Path parameter slice binding is not typically supported directly.
	// Consider if this is a valid use case or should be an error/skipped.
	// For now, skipping slice binding from path.
	{{else}}
	if pathValueStr := pathVar("{{.BindName}}"); pathValueStr != "" {
		{{if .IsPointer}}
			{{if eq .FieldType "string"}}
				s.{{.FieldName}} = &pathValueStr
			{{else if .IsNumeric}}
				{{if .IsFloat}}
					v, convErr := strconv.ParseFloat(pathValueStr, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						convertedValue := {{.FieldType}}(v)
						// Corrected: s.FieldName is already a pointer type if .IsPointer is true.
						// For complex types, direct assignment of convertedValue (which is complex64/128)
						// to s.FieldName (which is *complex64/*complex128) is wrong.
						// We need to assign the address of convertedValue.
						s.{{.FieldName}} = &convertedValue
					}
				{{else if .IsComplex}}
					v, convErr := strconv.ParseComplex(pathValueStr, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						convertedValue := {{.FieldType}}(v)
						s.{{.FieldName}} = &convertedValue
					}
				{{else if .IsSigned}}
					v, convErr := strconv.ParseInt(pathValueStr, 10, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						convertedValue := {{.FieldType}}(v)
						s.{{.FieldName}} = &convertedValue
					}
				{{else}} // Unsigned Integer
					v, convErr := strconv.ParseUint(pathValueStr, 10, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						convertedValue := {{.FieldType}}(v)
						s.{{.FieldName}} = &convertedValue
					}
				{{end}}
			{{else if eq .FieldType "bool"}}
				v, convErr := strconv.ParseBool(pathValueStr)
				if convErr != nil {
					errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", pathValueStr, convErr))
				} else {
					s.{{.FieldName}} = &v
				}
			{{end}}
		{{else}} {{/* Not a pointer */}}
			{{if eq .FieldType "string"}}
				s.{{.FieldName}} = pathValueStr
			{{else if .IsNumeric}}
				{{if .IsFloat}}
					v, convErr := strconv.ParseFloat(pathValueStr, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						s.{{.FieldName}} = {{.FieldType}}(v) // This was already correct
					}
				{{else if .IsComplex}}
					v, convErr := strconv.ParseComplex(pathValueStr, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						s.{{.FieldName}} = {{.FieldType}}(v)
					}
				{{else if .IsSigned}}
					v, convErr := strconv.ParseInt(pathValueStr, 10, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						s.{{.FieldName}} = {{.FieldType}}(v)
					}
				{{else}} // Unsigned Integer
					v, convErr := strconv.ParseUint(pathValueStr, 10, {{.BitSize}})
					if convErr != nil {
						errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", pathValueStr, convErr))
					} else {
						s.{{.FieldName}} = {{.FieldType}}(v)
					}
				{{end}}
			{{else if eq .FieldType "bool"}}
				v, convErr := strconv.ParseBool(pathValueStr)
				if convErr != nil {
					errs = append(errs, fmt.Errorf("failed to convert path parameter \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", pathValueStr, convErr))
				} else {
					s.{{.FieldName}} = v
				}
			{{end}}
		{{end}}
	} else { // Path value string is empty
		{{if .IsRequired}}
			errs = append(errs, fmt.Errorf("required path parameter \"{{.BindName}}\" for field {{.FieldName}} is missing"))
		{{else if .IsPointer}}
			s.{{.FieldName}} = nil
		{{end}}
	}
	{{end}} // End of not .IsSlice for Path
	{{else if eq .BindFrom "query"}}
	// Query parameter binding for field {{.FieldName}} ({{.OriginalFieldTypeString}}) from "{{.BindName}}"
	{{if .IsSlice}}
		if values, ok := req.URL.Query()["{{.BindName}}"]; ok && len(values) > 0 {
			sliceCap := len(values)
			slice := make([]{{.SliceElementType}}, 0, sliceCap) // Use SliceElementType, e.g., "int", "*string"
			for _, valStrLoop := range values { // Renamed valStr to valStrLoop to avoid conflict if defined outside
				// Directly use valStrLoop and field properties like .SliceElementType, .BindName, .FieldName
				// {{ $IsElemPointer := stringsHasPrefix .SliceElementType "*" }} // This logic will be inlined or handled by nested if
				// {{ $ElemType := stringsTrimPrefix .SliceElementType "*" }} // This logic will be inlined

				if valStrLoop == "" { // Handle empty string in multi-value query param
					{{if stringsHasPrefix .SliceElementType "*"}} // IsElemPointer
						{{if eq (stringsTrimPrefix .SliceElementType "*") "string"}} // ElemType is string
							emptyStr := ""
							slice = append(slice, &emptyStr)
						{{else}} // *int, *bool etc.
							var typedNil {{.SliceElementType}}
							slice = append(slice, typedNil)
						{{end}}
					{{else if eq (stringsTrimPrefix .SliceElementType "*") "string"}} // ElemType is string
						slice = append(slice, valStrLoop)
					{{else}} // int, bool etc. (non-pointer, non-string)
						errs = append(errs, fmt.Errorf("empty value for slice element of {{.FieldName}} (param \"{{.BindName}}\") cannot be converted to {{(stringsTrimPrefix .SliceElementType "*")}} from %q", valStrLoop))
					{{end}}
				} else { // Value is not empty, proceed with conversion
					{{ $OriginalElemType := .SliceElementType }}
					{{ $IsElemPointerForBlock := stringsHasPrefix $OriginalElemType "*" }}
					{{ $ElemTypeForBlock := stringsTrimPrefix $OriginalElemType "*" }}

					{{ if eq $ElemTypeForBlock "string" }}
						{{ if $IsElemPointerForBlock }}
							sPtr := valStrLoop
							slice = append(slice, &sPtr)
						{{ else }}
							slice = append(slice, valStrLoop)
						{{ end }}
					{{ else if eq $ElemTypeForBlock "int" }}
						v, convErr := strconv.Atoi(valStrLoop)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to int for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int8" }}
						var v64 int64
						var convErr error
						v64, convErr = strconv.ParseInt(valStrLoop, 10, 8)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to int8 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := int8(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int16" }}
						var v64 int64
						var convErr error
						v64, convErr = strconv.ParseInt(valStrLoop, 10, 16)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to int16 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := int16(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int32" }}
						var v64 int64
						var convErr error
						v64, convErr = strconv.ParseInt(valStrLoop, 10, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to int32 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := int32(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int64" }}
						var v int64
						var convErr error
						v, convErr = strconv.ParseInt(valStrLoop, 10, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to int64 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(valStrLoop, 10, 0)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to uint for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := uint(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint8" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(valStrLoop, 10, 8)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to uint8 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := uint8(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint16" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(valStrLoop, 10, 16)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to uint16 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := uint16(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint32" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(valStrLoop, 10, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to uint32 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := uint32(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint64" }}
						var v uint64
						var convErr error
						v, convErr = strconv.ParseUint(valStrLoop, 10, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to uint64 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "float32" }}
						var v64 float64
						var convErr error
						v64, convErr = strconv.ParseFloat(valStrLoop, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to float32 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { v := float32(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "float64" }}
						var v float64
						var convErr error
						v, convErr = strconv.ParseFloat(valStrLoop, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to float64 for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "complex64" }}
						var vComplex complex128
						var convErr error
						vComplex, convErr = strconv.ParseComplex(valStrLoop, 64)
						if convErr != nil {
							errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to complex64 for field {{.FieldName}}: %w", valStrLoop, convErr))
						} else {
							c := complex64(vComplex);
							{{if $IsElemPointerForBlock}}
								slice = append(slice, &c)
							{{else}}
								slice = append(slice, c)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "complex128" }}
						var vComplex complex128
						var convErr error
						vComplex, convErr = strconv.ParseComplex(valStrLoop, 128)
						if convErr != nil {
							errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to complex128 for field {{.FieldName}}: %w", valStrLoop, convErr))
						} else {
							{{if $IsElemPointerForBlock}}
								slice = append(slice, &vComplex)
							{{else}}
								slice = append(slice, vComplex)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "bool" }}
						var v bool
						var convErr error
						v, convErr = strconv.ParseBool(valStrLoop)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" element (value: %q) to bool for field {{.FieldName}}: %w", valStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else }}
						errs = append(errs, fmt.Errorf("unsupported slice element type %q for field {{.FieldName}} (param \"{{.BindName}}\")", "{{$ElemTypeForBlock}}"))
					{{ end }}
				}
			}
			s.{{.FieldName}} = slice
		} else { // Parameter not found or no values
			// This entire block (previously lines 255-307) was problematic as valStr is not defined here.
			// The logic for when a slice parameter is missing or empty is handled below.
			{{if .IsRequired}}
				errs = append(errs, fmt.Errorf("required query parameter slice \"{{.BindName}}\" for field {{.FieldName}} is missing"))
			{{else}}
				s.{{.FieldName}} = nil // Or empty slice: make([]{{.SliceElementType}}, 0)
			{{end}}
		}
	{{else}} // Not a slice for Query
		if req.URL.Query().Has("{{.BindName}}") {
			valStr := req.URL.Query().Get("{{.BindName}}")
			if valStr == "" {
				{{if .IsRequired}}
					{{if eq .FieldType "string"}}
						// Required string can be empty unless specific validation says otherwise
						{{if .IsPointer}} s.{{.FieldName}} = &valStr {{else}} s.{{.FieldName}} = valStr {{end}}
					{{else}}
						errs = append(errs, fmt.Errorf("required query parameter \"{{.BindName}}\" for field {{.FieldName}} received an empty value which cannot be converted to {{.FieldType}}"))
					{{end}}
				{{else if .IsPointer}}
					{{if eq .FieldType "string"}} s.{{.FieldName}} = &valStr {{else}} s.{{.FieldName}} = nil {{end}} // Empty value for non-string pointer is nil
				{{else if eq .FieldType "string"}}
					s.{{.FieldName}} = valStr // Empty value for string is itself
				{{else}}
					// Empty value for non-pointer, non-string, non-required. Stays zero/default. Or error?
					// Let's be strict: if it's not string and empty, it's a conversion problem unless specifically allowed.
					// However, current logic for non-slice non-required non-pointer is to leave as zero value if key exists but val is empty and conv fails.
					// This behavior should be consistent. For now, if not string, treat empty as potential conversion error handled by strconv.
				{{end}}
			}
			// If valStr is not empty, or if it's an empty string for a string type, proceed with conversion
			if valStr != "" || {{eq .FieldType "string"}} {
				{{if .IsPointer}}
					{{if eq .FieldType "string"}}
						s.{{.FieldName}} = &valStr
					{{else if .IsNumeric}}
						{{if .IsFloat}}
							v, convErr := strconv.ParseFloat(valStr, {{.BitSize}})
							if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
						{{else if .IsSigned}}
							v, convErr := strconv.ParseInt(valStr, 10, {{.BitSize}})
							if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
						{{else if .IsComplex}}
							v, convErr := strconv.ParseComplex(valStr, {{.BitSize}})
							if convErr != nil {
								{{if .IsRequired}}
									errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr))
								{{else if .IsPointer}}
									s.{{.FieldName}} = nil
								{{end}}
							} else {
								convertedValue := {{.FieldType}}(v);
								{{if .IsPointer}}
									s.{{.FieldName}} = &convertedValue
								{{else}}
									s.{{.FieldName}} = convertedValue
								{{end}}
							}
						{{else}} // Unsigned
							v, convErr := strconv.ParseUint(valStr, 10, {{.BitSize}})
							if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
						{{end}}
					{{else if eq .FieldType "bool"}}
						v, convErr := strconv.ParseBool(valStr)
						if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { s.{{.FieldName}} = &v }
					{{end}}
				{{else}} {{/* Not a pointer */}}
					{{if eq .FieldType "string"}}
						s.{{.FieldName}} = valStr
					{{else if .IsNumeric}}
						{{if .IsFloat}}
							v, convErr := strconv.ParseFloat(valStr, {{.BitSize}})
							if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
						{{else if .IsSigned}}
							v, convErr := strconv.ParseInt(valStr, 10, {{.BitSize}})
							if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
						{{else if .IsComplex}}
							v, convErr := strconv.ParseComplex(valStr, {{.BitSize}})
							if convErr != nil {
								errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr))
							} else {
								s.{{.FieldName}} = {{.FieldType}}(v)
							}
						{{else}} // Unsigned
							v, convErr := strconv.ParseUint(valStr, 10, {{.BitSize}})
							if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
						{{end}}
					{{else if eq .FieldType "bool"}}
						v, convErr := strconv.ParseBool(valStr)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = v }
					{{end}}
				{{end}}
			} // End valStr not empty or is string
		} else { // Key does not exist for query
			{{if .IsRequired}}
				errs = append(errs, fmt.Errorf("required query parameter \"{{.BindName}}\" for field {{.FieldName}} is missing"))
			{{else if .IsPointer}}
				s.{{.FieldName}} = nil
			{{end}}
		}
	{{end}} // End of not .IsSlice for Query
	{{else if eq .BindFrom "header"}}
	// Header binding for field {{.FieldName}} ({{.OriginalFieldTypeString}}) from "{{.BindName}}"
	{{if .IsSlice}}
		headerValStr := req.Header.Get("{{.BindName}}")
		if headerValStr != "" {
			valuesStr := strings.Split(headerValStr, ",") // Assuming comma-separated for simple style
			slice := make([]{{.SliceElementType}}, 0, len(valuesStr))
			for _, valStrLoop := range valuesStr {
				trimmedValStrLoop := strings.TrimSpace(valStrLoop)
				// Directly use trimmedValStrLoop and field properties
				if trimmedValStrLoop == "" {
					{{if stringsHasPrefix .SliceElementType "*"}} // IsElemPointer
						{{if eq (stringsTrimPrefix .SliceElementType "*") "string"}} // ElemType is string
							emptyStr := ""
							slice = append(slice, &emptyStr)
						{{else}} // *int, *bool etc.
							var typedNil {{.SliceElementType}}
							slice = append(slice, typedNil)
						{{end}}
					{{else if eq (stringsTrimPrefix .SliceElementType "*") "string"}} // ElemType is string
						slice = append(slice, trimmedValStrLoop)
					{{else}} // int, bool etc. (non-pointer, non-string)
						errs = append(errs, fmt.Errorf("empty value for trimmed slice element of {{.FieldName}} (header \"{{.BindName}}\") cannot be converted to {{(stringsTrimPrefix .SliceElementType "*")}} from %q", trimmedValStrLoop))
					{{end}}
				} else {
					{{ $OriginalElemType := .SliceElementType }}
					{{ $IsElemPointerForBlock := stringsHasPrefix $OriginalElemType "*" }}
					{{ $ElemTypeForBlock := stringsTrimPrefix $OriginalElemType "*" }}

					{{ if eq $ElemTypeForBlock "string" }}
						{{ if $IsElemPointerForBlock }}
							sPtr := trimmedValStrLoop
							slice = append(slice, &sPtr)
						{{ else }}
							slice = append(slice, trimmedValStrLoop)
						{{ end }}
					{{ else if eq $ElemTypeForBlock "int" }}
						var v int
						var convErr error
						v, convErr = strconv.Atoi(trimmedValStrLoop)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to int for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int8" }}
						var v64 int64
						var convErr error
						v64, convErr = strconv.ParseInt(trimmedValStrLoop, 10, 8)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to int8 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := int8(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int16" }}
						var v64 int64
						var convErr error
						v64, convErr = strconv.ParseInt(trimmedValStrLoop, 10, 16)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to int16 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := int16(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int32" }}
						var v64 int64
						var convErr error
						v64, convErr = strconv.ParseInt(trimmedValStrLoop, 10, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to int32 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := int32(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int64" }}
						var v int64
						var convErr error
						v, convErr = strconv.ParseInt(trimmedValStrLoop, 10, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to int64 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(trimmedValStrLoop, 10, 0)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to uint for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := uint(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint8" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(trimmedValStrLoop, 10, 8)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to uint8 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := uint8(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint16" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(trimmedValStrLoop, 10, 16)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to uint16 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := uint16(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint32" }}
						var v64 uint64
						var convErr error
						v64, convErr = strconv.ParseUint(trimmedValStrLoop, 10, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to uint32 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := uint32(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "uint64" }}
						var v uint64
						var convErr error
						v, convErr = strconv.ParseUint(trimmedValStrLoop, 10, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to uint64 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "float32" }}
						var v64 float64
						var convErr error
						v64, convErr = strconv.ParseFloat(trimmedValStrLoop, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to float32 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { v := float32(v64); {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "float64" }}
						var v float64
						var convErr error
						v, convErr = strconv.ParseFloat(trimmedValStrLoop, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to float64 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "bool" }}
						var v bool
						var convErr error
						v, convErr = strconv.ParseBool(trimmedValStrLoop)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" element (value: %q) to bool for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else }}
						errs = append(errs, fmt.Errorf("unsupported slice element type %q for field {{.FieldName}} (param \"{{.BindName}}\")", "{{$ElemTypeForBlock}}"))
					{{ end }}
				}
			}
			s.{{.FieldName}} = slice
		} else { // Header not found
			{{if .IsRequired}}
				errs = append(errs, fmt.Errorf("required header slice \"{{.BindName}}\" for field {{.FieldName}} is missing"))
			{{else}}
				s.{{.FieldName}} = nil // Or empty slice
			{{end}}
		}
	{{else}} // Not a slice for Header
		if valStr := req.Header.Get("{{.BindName}}"); valStr != "" {
			{{if .IsPointer}}
				{{if eq .FieldType "string"}}
					s.{{.FieldName}} = &valStr
				{{else if .IsNumeric}}
					{{if .IsFloat}}
						v, convErr := strconv.ParseFloat(valStr, {{.BitSize}})
						if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
					{{else if .IsSigned}}
						v, convErr := strconv.ParseInt(valStr, 10, {{.BitSize}})
						if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
						{{else if .IsComplex}}
							v, convErr := strconv.ParseComplex(valStr, {{.BitSize}})
							if convErr != nil {
								{{if .IsRequired}}
									errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr))
								{{else if .IsPointer}}
									s.{{.FieldName}} = nil
								{{end}}
							} else {
								convertedValue := {{.FieldType}}(v);
								{{if .IsPointer}}
									s.{{.FieldName}} = &convertedValue
								{{else}}
									s.{{.FieldName}} = convertedValue
								{{end}}
							}
					{{else}} // Unsigned
						v, convErr := strconv.ParseUint(valStr, 10, {{.BitSize}})
						if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
					{{end}}
				{{else if eq .FieldType "bool"}}
					v, convErr := strconv.ParseBool(valStr)
					if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { s.{{.FieldName}} = &v }
				{{end}}
			{{else}} {{/* Not a pointer */}}
				{{if eq .FieldType "string"}}
					s.{{.FieldName}} = valStr
				{{else if .IsNumeric}}
					{{if .IsFloat}}
						v, convErr := strconv.ParseFloat(valStr, {{.BitSize}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
					{{else if .IsSigned}}
						v, convErr := strconv.ParseInt(valStr, 10, {{.BitSize}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
						{{else if .IsComplex}}
							v, convErr := strconv.ParseComplex(valStr, {{.BitSize}})
							if convErr != nil {
								errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr))
							} else {
								s.{{.FieldName}} = {{.FieldType}}(v)
							}
					{{else}} // Unsigned
						v, convErr := strconv.ParseUint(valStr, 10, {{.BitSize}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
					{{end}}
				{{else if eq .FieldType "bool"}}
					v, convErr := strconv.ParseBool(valStr)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = v }
				{{end}}
			{{end}}
		} else { // Header value is empty or header not found
			{{if .IsRequired}}
				errs = append(errs, fmt.Errorf("required header \"{{.BindName}}\" for field {{.FieldName}} is missing"))
			{{else if .IsPointer}}
				s.{{.FieldName}} = nil
			{{end}}
		}
	{{end}} // End of not .IsSlice for Header
	{{else if eq .BindFrom "cookie"}}
	// Cookie binding for field {{.FieldName}} ({{.OriginalFieldTypeString}}) from "{{.BindName}}"
	{{if .IsSlice}}
		if cookie, cerr := req.Cookie("{{.BindName}}"); cerr == nil && cookie.Value != "" {
			valuesStr := strings.Split(cookie.Value, ",") // Assuming comma-separated for form style
			slice := make([]{{.SliceElementType}}, 0, len(valuesStr))
			for _, valStrLoop := range valuesStr {
				trimmedValStrLoop := strings.TrimSpace(valStrLoop)
				// Directly use trimmedValStrLoop and field properties
				if trimmedValStrLoop == "" {
					{{if stringsHasPrefix .SliceElementType "*"}} // IsElemPointer
						{{if eq (stringsTrimPrefix .SliceElementType "*") "string"}} // ElemType is string
							emptyStr := ""
							slice = append(slice, &emptyStr)
						{{else}} // *int, *bool etc.
							var typedNil {{.SliceElementType}}
							slice = append(slice, typedNil)
						{{end}}
					{{else if eq (stringsTrimPrefix .SliceElementType "*") "string"}} // ElemType is string
						slice = append(slice, trimmedValStrLoop)
					{{else}} // int, bool etc. (non-pointer, non-string)
						errs = append(errs, fmt.Errorf("empty value for trimmed slice element of {{.FieldName}} (cookie \"{{.BindName}}\") cannot be converted to {{(stringsTrimPrefix .SliceElementType "*")}} from %q", trimmedValStrLoop))
					{{end}}
				} else {
					{{ $OriginalElemType := .SliceElementType }}
					{{ $IsElemPointerForBlock := stringsHasPrefix $OriginalElemType "*" }}
					{{ $ElemTypeForBlock := stringsTrimPrefix $OriginalElemType "*" }}

					{{ if eq $ElemTypeForBlock "string" }}
						{{ if $IsElemPointerForBlock }}
							sPtr := trimmedValStrLoop
							slice = append(slice, &sPtr)
						{{ else }}
							slice = append(slice, trimmedValStrLoop)
						{{ end }}
					{{ else if eq $ElemTypeForBlock "int" }}
						v, convErr := strconv.Atoi(trimmedValStrLoop)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to int for field {{.FieldName}}: %w", trimmedValStrLoop, convErr)) } else { {{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemTypeForBlock "int8" }}
						var v64_int8 int64; var convErr_int8 error
						v64_int8, convErr_int8 = strconv.ParseInt(trimmedValStrLoop, 10, 8)
						if convErr_int8 != nil {
							errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to int8 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_int8))
						} else {
							var v int8
							v = int8(v64_int8)
							{{if $IsElemPointerForBlock}}
								slice = append(slice, &v)
							{{else}}
								slice = append(slice, v)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "int16" }}
						var v64_int16 int64; var convErr_int16 error // Declaration
						v64_int16, convErr_int16 = strconv.ParseInt(trimmedValStrLoop, 10, 16) // Assignment
						if convErr_int16 != nil {
							errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to int16 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_int16))
						} else {
							v_final_int16 := int16(v64_int16) // Use a distinct variable name for the converted value
							{{if $IsElemPointerForBlock}}
								slice = append(slice, &v_final_int16)
							{{else}}
								slice = append(slice, v_final_int16)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "int32" }}
						var v64_int32 int64; var convErr_int32 error
						v64_int32, convErr_int32 = strconv.ParseInt(trimmedValStrLoop, 10, 32)
						if convErr_int32 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to int32 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_int32))
						} else {
							var v int32
							v = int32(v64_int32)
							{{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}}
						}
					{{ else if eq $ElemTypeForBlock "int64" }}
						var v_int64 int64; var convErr_int64 error
						v_int64, convErr_int64 = strconv.ParseInt(trimmedValStrLoop, 10, 64)
						if convErr_int64 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to int64 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_int64))
						} else {
							{{if $IsElemPointerForBlock}}
								v_ptr := v_int64; slice = append(slice, &v_ptr)
							{{else}}
								slice = append(slice, v_int64)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "uint" }}
						var v64_uint uint64; var convErr_uint error
						v64_uint, convErr_uint = strconv.ParseUint(trimmedValStrLoop, 10, 0)
						if convErr_uint != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to uint for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_uint))
						} else {
							var v uint
							v = uint(v64_uint)
							{{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}}
						}
					{{ else if eq $ElemTypeForBlock "uint8" }}
						var v64_uint8 uint64; var convErr_uint8 error
						v64_uint8, convErr_uint8 = strconv.ParseUint(trimmedValStrLoop, 10, 8)
						if convErr_uint8 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to uint8 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_uint8))
						} else {
							var v uint8
							v = uint8(v64_uint8)
							{{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}}
						}
					{{ else if eq $ElemTypeForBlock "uint16" }}
						var v64_uint16 uint64; var convErr_uint16 error
						v64_uint16, convErr_uint16 = strconv.ParseUint(trimmedValStrLoop, 10, 16)
						if convErr_uint16 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to uint16 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_uint16))
						} else {
							var v uint16
							v = uint16(v64_uint16)
							{{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}}
						}
					{{ else if eq $ElemTypeForBlock "uint32" }}
						var v64_uint32 uint64; var convErr_uint32 error
						v64_uint32, convErr_uint32 = strconv.ParseUint(trimmedValStrLoop, 10, 32)
						if convErr_uint32 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to uint32 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_uint32))
						} else {
							var v uint32
							v = uint32(v64_uint32)
							{{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}}
						}
					{{ else if eq $ElemTypeForBlock "uint64" }}
						var v_uint64 uint64; var convErr_uint64 error
						v_uint64, convErr_uint64 = strconv.ParseUint(trimmedValStrLoop, 10, 64)
						if convErr_uint64 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to uint64 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_uint64))
						} else {
							{{if $IsElemPointerForBlock}}
								v_ptr := v_uint64; slice = append(slice, &v_ptr)
							{{else}}
								slice = append(slice, v_uint64)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "float32" }}
						var v64_float32 float64; var convErr_float32 error
						v64_float32, convErr_float32 = strconv.ParseFloat(trimmedValStrLoop, 32)
						if convErr_float32 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to float32 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_float32))
						} else {
							var v float32
							v = float32(v64_float32)
							{{if $IsElemPointerForBlock}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}}
						}
					{{ else if eq $ElemTypeForBlock "float64" }}
						var v_float64 float64; var convErr_float64 error
						v_float64, convErr_float64 = strconv.ParseFloat(trimmedValStrLoop, 64)
						if convErr_float64 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to float64 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_float64))
						} else {
							{{if $IsElemPointerForBlock}}
								v_ptr := v_float64; slice = append(slice, &v_ptr)
							{{else}}
								slice = append(slice, v_float64)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "complex64" }}
						var vComplex_complex64 complex128; var convErr_complex64 error
						vComplex_complex64, convErr_complex64 = strconv.ParseComplex(trimmedValStrLoop, 64)
						if convErr_complex64 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to complex64 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_complex64))
						} else {
							c := complex64(vComplex_complex64);
							{{if $IsElemPointerForBlock}} slice = append(slice, &c) {{else}} slice = append(slice, c) {{end}}
						}
					{{ else if eq $ElemTypeForBlock "complex128" }}
						var vComplex_complex128 complex128; var convErr_complex128 error
						vComplex_complex128, convErr_complex128 = strconv.ParseComplex(trimmedValStrLoop, 128)
						if convErr_complex128 != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" element (value: %q) to complex128 for field {{.FieldName}}: %w", trimmedValStrLoop, convErr_complex128))
						} else {
							{{if $IsElemPointerForBlock}}
								v_ptr := vComplex_complex128; slice = append(slice, &v_ptr)
							{{else}}
								slice = append(slice, vComplex_complex128)
							{{end}}
						}
					{{ else if eq $ElemTypeForBlock "bool" }}
						var v bool; var convErr error
						v, convErr = strconv.ParseBool(trimmedValStrLoop)
					{{ else }}
						errs = append(errs, fmt.Errorf("unsupported slice element type %q for field {{.FieldName}} (param \"{{.BindName}}\")", "{{$ElemTypeForBlock}}"))
					{{ end }}
				}
			}
			s.{{.FieldName}} = slice
		} else { // Cookie not found or value is empty
			if cerr != http.ErrNoCookie && cerr != nil { // Report actual errors other than not found
				errs = append(errs, fmt.Errorf("error retrieving cookie \"{{.BindName}}\" for field {{.FieldName}}: %w", cerr))
			}
			{{if .IsRequired}}
				errs = append(errs, fmt.Errorf("required cookie slice \"{{.BindName}}\" for field {{.FieldName}} is missing or empty (underlying error: %v)", cerr))
			{{else}}
				s.{{.FieldName}} = nil // Or empty slice
			{{end}}
		}
	{{else}} // Not a slice for Cookie
		if cookie, cerr := req.Cookie("{{.BindName}}"); cerr == nil && cookie.Value != "" {
			valStr := cookie.Value
			{{if .IsPointer}}
				{{if eq .FieldType "string"}}
					s.{{.FieldName}} = &valStr
				{{else if .IsNumeric}}
					{{if .IsFloat}}
						v, convErr := strconv.ParseFloat(valStr, {{.BitSize}})
						if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
					{{else if .IsSigned}}
						v, convErr := strconv.ParseInt(valStr, 10, {{.BitSize}})
						if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
						{{else if .IsComplex}}
							v, convErr := strconv.ParseComplex(valStr, {{.BitSize}})
							if convErr != nil {
								{{if .IsRequired}}
									errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr))
								{{else if .IsPointer}}
									s.{{.FieldName}} = nil
								{{end}}
							} else {
								convertedValue := {{.FieldType}}(v);
								{{if .IsPointer}}
									s.{{.FieldName}} = &convertedValue
								{{else}}
									s.{{.FieldName}} = convertedValue
								{{end}}
							}
					{{else}} // Unsigned
						v, convErr := strconv.ParseUint(valStr, 10, {{.BitSize}})
						if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
					{{end}}
				{{else if eq .FieldType "bool"}}
					v, convErr := strconv.ParseBool(valStr)
					if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { s.{{.FieldName}} = &v }
				{{end}}
			{{else}} {{/* Not a pointer */}}
				{{if eq .FieldType "string"}}
					s.{{.FieldName}} = valStr
				{{else if .IsNumeric}}
					{{if .IsFloat}}
						v, convErr := strconv.ParseFloat(valStr, {{.BitSize}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
					{{else if .IsSigned}}
						v, convErr := strconv.ParseInt(valStr, 10, {{.BitSize}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
						{{else if .IsComplex}}
							v, convErr := strconv.ParseComplex(valStr, {{.BitSize}})
							if convErr != nil {
								errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr))
							} else {
								s.{{.FieldName}} = {{.FieldType}}(v)
							}
					{{else}} // Unsigned
						v, convErr := strconv.ParseUint(valStr, 10, {{.BitSize}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
					{{end}}
				{{else if eq .FieldType "bool"}}
					v, convErr := strconv.ParseBool(valStr)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to bool for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = v }
				{{end}}
			{{end}}
		} else { // Cookie not found or value is empty
			if cerr != http.ErrNoCookie && cerr != nil {
				errs = append(errs, fmt.Errorf("error retrieving cookie \"{{.BindName}}\" for field {{.FieldName}}: %w", cerr))
			}
			{{if .IsRequired}}
				errs = append(errs, fmt.Errorf("required cookie \"{{.BindName}}\" for field {{.FieldName}} is missing, empty, or could not be retrieved (underlying error: %v)", cerr))
			{{else if .IsPointer}}
				s.{{.FieldName}} = nil
			{{end}}
		}
	{{end}} // End of not .IsSlice for Cookie
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
			if decErr := json.NewDecoder(req.Body).Decode(&s.{{.FieldName}}); decErr != nil {
				if decErr != io.EOF { // EOF might be acceptable if body is optional
					errs = append(errs, fmt.Errorf("failed to decode request body into field {{.FieldName}}: %w", decErr))
				}
			}
			goto afterBodyProcessing // Process only one 'in:"body"' field
			{{end}}
			{{end}}
		} else {
			// The struct {{.StructName}} itself is the target for the request body
			if decErr := json.NewDecoder(req.Body).Decode(s); decErr != nil {
				if decErr != io.EOF { // EOF might be acceptable
					errs = append(errs, fmt.Errorf("failed to decode request body into {{.StructName}}: %w", decErr))
				}
			}
		}
		{{if .HasSpecificBodyFieldTarget}}
		afterBodyProcessing: // Label for goto only if there was a specific body field target that could jump here
		{{end}}
	}
	{{end}}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
`
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

	var generatedCodeForAllStructs bytes.Buffer
	allFileImports := make(map[string]string) // path -> alias
	needsImportStrconv := false
	needsImportNetHTTP := false
	needsImportFmt := false
	needsImportEncodingJson := false
	needsImportIO := false
	needsImportErrors := false


	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}
		hasBindingAnnotationOnStruct := strings.Contains(typeInfo.Doc, bindingAnnotation) // Uses corrected const
		structLevelInTag := ""
		if hasBindingAnnotationOnStruct {
			docLines := strings.Split(typeInfo.Doc, "\n")
			for _, line := range docLines {
				if strings.Contains(line, bindingAnnotation) { // Uses corrected const
					parts := strings.Fields(line)
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
		fmt.Printf("  Processing struct: %s for %s\n", typeInfo.Name, bindingAnnotation) // Uses corrected const

		data := TemplateData{
			PackageName:                pkgInfo.Name,
			StructName:                 typeInfo.Name,
			Imports:                    make(map[string]string),
			Fields:                     []FieldBindingInfo{},
			NeedsBody:                  (structLevelInTag == "body"),
			HasSpecificBodyFieldTarget: false,
			ErrNoCookie:                http.ErrNoCookie,
		}
		needsImportNetHTTP = true

		for _, field := range typeInfo.Struct.Fields {
			tag := reflect.StructTag(field.Tag)
			inTagVal := tag.Get("in")
			bindFrom := ""
			bindName := ""

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
					data.NeedsBody = true
				default:
					fmt.Printf("      Skipping field %s: unknown 'in' tag value '%s'\n", field.Name, inTagVal)
					continue
				}
				if bindFrom != "body" && bindName == "" {
					fmt.Printf("      Skipping field %s: 'in:\"%s\"' tag requires corresponding '%s:\"name\"' tag\n", field.Name, bindFrom, bindFrom)
					continue
				}
			} else if data.NeedsBody {
				continue
			} else {
				continue
			}

			fInfo := FieldBindingInfo{
				FieldName:               field.Name,
				BindFrom:                bindFrom,
				BindName:                bindName,
				IsRequired:              (tag.Get("required") == "true"),
				OriginalFieldTypeString: field.Type.String(),
				IsPointer:               field.Type.IsPointer,
			}

			currentScannerType := field.Type
			baseTypeForConversion := ""

			if currentScannerType.IsSlice {
				fInfo.IsSlice = true
				if currentScannerType.Elem != nil {
					fInfo.SliceElementType = currentScannerType.Elem.String()
					sliceElemScannerType := currentScannerType.Elem
					if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem != nil {
						baseTypeForConversion = sliceElemScannerType.Elem.Name
					} else if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem == nil {
						baseTypeForConversion = sliceElemScannerType.Name
					} else {
						baseTypeForConversion = sliceElemScannerType.Name
					}
				} else {
					fmt.Printf("      Skipping field %s: slice with nil Elem type\n", field.Name)
					continue
				}
			} else if currentScannerType.IsPointer {
				if currentScannerType.Elem != nil {
					baseTypeForConversion = currentScannerType.Elem.Name
				} else {
					baseTypeForConversion = currentScannerType.Name
					if baseTypeForConversion == "" {
						fmt.Printf("      Warning: Pointer field %s (%s) - field.Type.Elem is nil and field.Type.Name is empty. Original type: %s. Skipping.\n", field.Name, fInfo.OriginalFieldTypeString, field.Type.String())
						if bindFrom != "body" {
							continue
						}
					}
				}
			} else {
				baseTypeForConversion = currentScannerType.Name
				if baseTypeForConversion == "" {
					fmt.Printf("      Warning: Field %s (%s) - field.Type.Name is empty for non-slice/non-pointer. Original type: %s. Skipping.\n", field.Name, fInfo.OriginalFieldTypeString, field.Type.String())
					if bindFrom != "body" {
						continue
					}
				}
			}

			fInfo.FieldType = baseTypeForConversion

			switch baseTypeForConversion {
			case "int", "int8", "int16", "int32", "int64":
				fInfo.IsNumeric = true
				fInfo.IsSigned = true
				if size, ok := map[string]int{"int": 0, "int8": 8, "int16": 16, "int32": 32, "int64": 64}[baseTypeForConversion]; ok {
					fInfo.BitSize = size
				}
			case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
				fInfo.IsNumeric = true
				fInfo.IsSigned = false
				if size, ok := map[string]int{"uint": 0, "uint8": 8, "uint16": 16, "uint32": 32, "uint64": 64, "uintptr": 0}[baseTypeForConversion]; ok {
					fInfo.BitSize = size
				}
			case "float32", "float64":
				fInfo.IsNumeric = true
				fInfo.IsFloat = true
				if size, ok := map[string]int{"float32": 32, "float64": 64}[baseTypeForConversion]; ok {
					fInfo.BitSize = size
				}
			case "complex64", "complex128":
				fInfo.IsNumeric = true
				fInfo.IsComplex = true
				if size, ok := map[string]int{"complex64": 64, "complex128": 128}[baseTypeForConversion]; ok {
					fInfo.BitSize = size
				}
			case "string", "bool":
				break
			default:
				if bindFrom != "body" && !fInfo.IsSlice {
					fmt.Printf("      Skipping field %s: unhandled base type '%s' (original: %s, slice: %t) for %s binding\n", field.Name, baseTypeForConversion, fInfo.OriginalFieldTypeString, fInfo.IsSlice, bindFrom)
					continue
				} else if bindFrom != "body" && fInfo.IsSlice && !isWellKnownSliceElementType(fInfo.SliceElementType) {
					fmt.Printf("      Skipping field %s: slice of unhandled element type '%s' (original: %s) for %s binding\n", field.Name, fInfo.SliceElementType, fInfo.OriginalFieldTypeString, bindFrom)
					continue
				}
			}

			if bindFrom != "body" {
				needsImportNetHTTP = true
				needsConv := fInfo.IsNumeric || fInfo.IsComplex || baseTypeForConversion == "bool"
				if fInfo.IsSlice {
					elemBaseTypeForConv := baseTypeForConversion
					if isNumericOrBool(elemBaseTypeForConv) {
						needsConv = true
					}
				}
				if needsConv {
					needsImportStrconv = true
				}

				if fInfo.IsSlice && (bindFrom == "header" || bindFrom == "cookie") {
					allFileImports["strings"] = ""
				}
				needsImportFmt = true
			} else {
				fInfo.IsBody = true
				data.NeedsBody = true
				data.HasSpecificBodyFieldTarget = true
				needsImportEncodingJson = true
				needsImportIO = true
			}
			data.Fields = append(data.Fields, fInfo)
		}

		if len(data.Fields) == 0 && !data.NeedsBody {
			fmt.Printf("  Skipping struct %s: no bindable fields found and not a global body target.\n", typeInfo.Name)
			continue
		}

		if needsImportNetHTTP {
			allFileImports["net/http"] = ""
		}
		if needsImportStrconv {
			allFileImports["strconv"] = ""
		}
		if needsImportFmt {
			allFileImports["fmt"] = ""
		}
		if needsImportEncodingJson {
			allFileImports["encoding/json"] = ""
		}
		if needsImportIO {
			allFileImports["io"] = ""
		}
		if data.NeedsBody && !needsImportEncodingJson {
			allFileImports["encoding/json"] = ""
			needsImportEncodingJson = true
		}
		if data.NeedsBody && !needsImportIO {
			allFileImports["io"] = ""
			needsImportIO = true
		}

		funcMap := template.FuncMap{
			"stringsHasPrefix":  strings.HasPrefix,
			"stringsTrimPrefix": strings.TrimPrefix,
		}

		tmpl, err := template.New("bind").Funcs(funcMap).Parse(bindMethodTemplate)
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}
		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, data); err != nil {
			return fmt.Errorf("failed to execute template for struct %s: %w", typeInfo.Name, err)
		}
		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")
		needsImportErrors = true
	}

	if generatedCodeForAllStructs.Len() == 0 {
		fmt.Println("No structs found requiring Bind method generation.")
		return nil
	}

	finalOutput := bytes.Buffer{}
	finalOutput.WriteString(fmt.Sprintf("// Code generated by derivingbind for package %s. DO NOT EDIT.\n\n", pkgInfo.Name))
	finalOutput.WriteString(fmt.Sprintf("package %s\n\n", pkgInfo.Name))

	if needsImportErrors {
		allFileImports["errors"] = ""
	}

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
