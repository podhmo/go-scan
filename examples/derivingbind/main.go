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
						s.{{.FieldName}} = {{.FieldType}}(v)
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
				// Define variables inside the loop
				{{ $Value := "valStrLoop" }} // Use the loop variable name
				{{ $IsElemPointer := stringsHasPrefix .SliceElementType "*" }}
				{{ $ElemType := stringsTrimPrefix .SliceElementType "*" }}
				{{ $OutputSliceVar := "slice" }} // Template variable for slice name
				{{ $ParamName := .BindName }}
				{{ $FieldName := .FieldName }}
				{{ $Source := "query parameter" }}

				if {{ $Value }} == "" { // Handle empty string in multi-value query param
					{{if $IsElemPointer }} // Pointer element type
						{{if eq $ElemType "string"}} // *string
							emptyStr := ""
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, &emptyStr)
						{{else}} // *int, *bool etc.
							// For optional non-string pointers, empty value means nil for the element
							// If the field itself is required, this might still be an issue overall, but element can be nil.
							var typedNil {{.SliceElementType}} // e.g. var typedNil *int
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, typedNil)
						{{end}}
					{{else if eq $ElemType "string"}} // string
						{{$OutputSliceVar}} = append({{$OutputSliceVar}}, {{ $Value }})
					{{else}} // int, bool etc. (non-pointer, non-string)
						errs = append(errs, fmt.Errorf("empty value for slice element of {{$FieldName}} (param \"{{$ParamName}}\") cannot be converted to {{$ElemType}} from %q", {{ $Value }}))
					{{end}}
				} else { // Value is not empty, proceed with conversion
					{{ if eq $ElemType "string" }}
						{{ if $IsElemPointer }}
							sPtr := {{$Value}}
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, &sPtr)
						{{ else }}
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, {{$Value}})
						{{ end }}
					{{ else if eq $ElemType "int" }}
						v, convErr := strconv.Atoi({{$Value}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "int8" }}
						v64, convErr := strconv.ParseInt({{$Value}}, 10, 8)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int8 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int8(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "int16" }}
						v64, convErr := strconv.ParseInt({{$Value}}, 10, 16)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int16 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int16(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "int32" }}
						v64, convErr := strconv.ParseInt({{$Value}}, 10, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int32(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "int64" }}
						v, convErr := strconv.ParseInt({{$Value}}, 10, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "uint" }}
						v64, convErr := strconv.ParseUint({{$Value}}, 10, 0)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "uint8" }}
						v64, convErr := strconv.ParseUint({{$Value}}, 10, 8)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint8 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint8(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "uint16" }}
						v64, convErr := strconv.ParseUint({{$Value}}, 10, 16)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint16 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint16(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "uint32" }}
						v64, convErr := strconv.ParseUint({{$Value}}, 10, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint32(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "uint64" }}
						v, convErr := strconv.ParseUint({{$Value}}, 10, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "float32" }}
						v64, convErr := strconv.ParseFloat({{$Value}}, 32)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to float32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := float32(v64); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "float64" }}
						v, convErr := strconv.ParseFloat({{$Value}}, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to float64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "complex64" }}
						v, convErr := strconv.ParseComplex({{$Value}}, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to complex64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { c := complex64(v); {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &c) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, c) {{end}} }
					{{ else if eq $ElemType "complex128" }}
						v, convErr := strconv.ParseComplex({{$Value}}, 128)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to complex128 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else if eq $ElemType "bool" }}
						v, convErr := strconv.ParseBool({{$Value}})
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to bool for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, &v) {{else}} {{$OutputSliceVar}} = append({{$OutputSliceVar}}, v) {{end}} }
					{{ else }}
						errs = append(errs, fmt.Errorf("unsupported slice element type %q for field {{$FieldName}} (param \"{{$ParamName}}\")", "{{$ElemType}}"))
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
							if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
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
							if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert query parameter \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
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
				{{ $Value := "trimmedValStrLoop" }}
				{{ $IsElemPointer := stringsHasPrefix .SliceElementType "*" }}
				{{ $ElemType := stringsTrimPrefix .SliceElementType "*" }}
				{{ $OutputSliceVar := "slice" }}
				{{ $ParamName := .BindName }}
				{{ $FieldName := .FieldName }}
				{{ $Source := "header" }}

				if {{ $Value }} == "" {
					{{if $IsElemPointer }}
						{{if eq $ElemType "string"}}
							emptyStr := ""
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, &emptyStr)
						{{else}}
							var typedNil {{.SliceElementType}}
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, typedNil)
						{{end}}
					{{else if eq $ElemType "string"}}
						{{$OutputSliceVar}} = append({{$OutputSliceVar}}, {{$Value}})
					{{else}}
						errs = append(errs, fmt.Errorf("empty value for trimmed slice element of {{$FieldName}} (header \"{{$ParamName}}\") cannot be converted to {{$ElemType}} from %q", {{ $Value }}))
					{{end}}
				} else {
					{{ if eq $ElemType "string" }}
					{{ if $IsElemPointer }}
						sPtr := {{$Value}}
						slice = append(slice, &sPtr)
					{{ else }}
						slice = append(slice, {{$Value}})
					{{ end }}
				{{ else if eq $ElemType "int" }}
					v, convErr := strconv.Atoi({{$Value}})
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int8" }}
					v64, convErr := strconv.ParseInt({{$Value}}, 10, 8)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int8 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int8(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int16" }}
					v64, convErr := strconv.ParseInt({{$Value}}, 10, 16)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int16 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int16(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int32" }}
					v64, convErr := strconv.ParseInt({{$Value}}, 10, 32)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int32(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int64" }}
					v, convErr := strconv.ParseInt({{$Value}}, 10, 64)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 0)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint8" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 8)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint8 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint8(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint16" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 16)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint16 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint16(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint32" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 32)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint32(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint64" }}
					v, convErr := strconv.ParseUint({{$Value}}, 10, 64)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "float32" }}
					v64, convErr := strconv.ParseFloat({{$Value}}, 32)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to float32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := float32(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "float64" }}
					v, convErr := strconv.ParseFloat({{$Value}}, 64)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to float64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "bool" }}
					v, convErr := strconv.ParseBool({{$Value}})
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to bool for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else }}
					errs = append(errs, fmt.Errorf("unsupported slice element type %q for field {{$FieldName}} (param \"{{$ParamName}}\")", "{{$ElemType}}"))
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
							if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
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
							if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert cookie \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
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
				{{ $Value := "trimmedValStrLoop" }}
				{{ $IsElemPointer := stringsHasPrefix .SliceElementType "*" }}
				{{ $ElemType := stringsTrimPrefix .SliceElementType "*" }}
				{{ $OutputSliceVar := "slice" }}
				{{ $ParamName := .BindName }}
				{{ $FieldName := .FieldName }}
				{{ $Source := "cookie" }}

				if {{ $Value }} == "" {
					{{if $IsElemPointer }}
						{{if eq $ElemType "string"}}
							emptyStr := ""
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, &emptyStr)
						{{else}}
							var typedNil {{.SliceElementType}}
							{{$OutputSliceVar}} = append({{$OutputSliceVar}}, typedNil)
						{{end}}
					{{else if eq $ElemType "string"}}
						{{$OutputSliceVar}} = append({{$OutputSliceVar}}, {{$Value}})
					{{else}}
						errs = append(errs, fmt.Errorf("empty value for trimmed slice element of {{$FieldName}} (cookie \"{{$ParamName}}\") cannot be converted to {{$ElemType}} from %q", {{ $Value }}))
					{{end}}
				} else {
					{{ if eq $ElemType "string" }}
					{{ if $IsElemPointer }}
						sPtr := {{$Value}}
						slice = append(slice, &sPtr)
					{{ else }}
						slice = append(slice, {{$Value}})
					{{ end }}
				{{ else if eq $ElemType "int" }}
					v, convErr := strconv.Atoi({{$Value}})
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int8" }}
					v64, convErr := strconv.ParseInt({{$Value}}, 10, 8)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int8 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int8(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int16" }}
					v64, convErr := strconv.ParseInt({{$Value}}, 10, 16)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int16 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int16(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int32" }}
					v64, convErr := strconv.ParseInt({{$Value}}, 10, 32)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := int32(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "int64" }}
					v, convErr := strconv.ParseInt({{$Value}}, 10, 64)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to int64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 0)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint8" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 8)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint8 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint8(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint16" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 16)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint16 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint16(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint32" }}
					v64, convErr := strconv.ParseUint({{$Value}}, 10, 32)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := uint32(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "uint64" }}
					v, convErr := strconv.ParseUint({{$Value}}, 10, 64)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to uint64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "float32" }}
					v64, convErr := strconv.ParseFloat({{$Value}}, 32)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to float32 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { v := float32(v64); {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else if eq $ElemType "float64" }}
					v, convErr := strconv.ParseFloat({{$Value}}, 64)
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to float64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
					{{ else if eq $ElemType "complex64" }}
						vComplex, convErr := strconv.ParseComplex({{$Value}}, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to complex64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { c := complex64(vComplex); {{if $IsElemPointer}} slice = append(slice, &c) {{else}} slice = append(slice, c) {{end}} }
					{{ else if eq $ElemType "complex128" }}
						vComplex, convErr := strconv.ParseComplex({{$Value}}, 128)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to complex128 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &vComplex) {{else}} slice = append(slice, vComplex) {{end}} }
					{{ else if eq $ElemType "complex64" }}
						vComplex, convErr := strconv.ParseComplex({{$Value}}, 64)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to complex64 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { c := complex64(vComplex); {{if $IsElemPointer}} slice = append(slice, &c) {{else}} slice = append(slice, c) {{end}} }
					{{ else if eq $ElemType "complex128" }}
						vComplex, convErr := strconv.ParseComplex({{$Value}}, 128)
						if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to complex128 for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &vComplex) {{else}} slice = append(slice, vComplex) {{end}} }
				{{ else if eq $ElemType "bool" }}
					v, convErr := strconv.ParseBool({{$Value}})
					if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert %s \"{{$ParamName}}\" element (value: %q) to bool for field {{$FieldName}}: %w", "{{$Source}}", {{$Value}}, convErr)) } else { {{if $IsElemPointer}} slice = append(slice, &v) {{else}} slice = append(slice, v) {{end}} }
				{{ else }}
					errs = append(errs, fmt.Errorf("unsupported slice element type %q for field {{$FieldName}} (param \"{{$ParamName}}\")", "{{$ElemType}}"))
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
							if convErr != nil { errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) } else { s.{{.FieldName}} = {{.FieldType}}(v) }
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
							if convErr != nil { {{if .IsRequired}} errs = append(errs, fmt.Errorf("failed to convert header \"{{.BindName}}\" (value: %q) to {{.FieldType}} for field {{.FieldName}}: %w", valStr, convErr)) {{else}} s.{{.FieldName}} = nil {{end}} } else { convertedValue := {{.FieldType}}(v); s.{{.FieldName}} = &convertedValue }
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
					fInfo.SliceElementType = currentScannerType.Elem.String() // e.g., "int", "*string", "pkg.MyType", "*pkg.MyType"

					// Determine the baseTypeForConversion from the slice element
					sliceElemScannerType := currentScannerType.Elem
					if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem != nil { // e.g. []*int, Elem is *int, Elem.Elem is int
						baseTypeForConversion = sliceElemScannerType.Elem.Name // "int"
						// isElementPointer = true
					} else if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem == nil { // e.g. []*ExternalType
						baseTypeForConversion = sliceElemScannerType.Name // "ExternalType"
						// isElementPointer = true
					} else { // e.g. []int or []ExternalType
						baseTypeForConversion = sliceElemScannerType.Name // "int" or "ExternalType"
						// isElementPointer = false
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
					if baseTypeForConversion == "" {
						fmt.Printf("      Warning: Pointer field %s (%s) - field.Type.Elem is nil and field.Type.Name is empty. Original type: %s. Skipping.\n", field.Name, fInfo.OriginalFieldTypeString, field.Type.String())
						if bindFrom != "body" { // body might handle it via JSON unmarshal
							continue
						}
					}
				}
			} else { // Not a slice, not a pointer (e.g. int, string, ExternalType)
				baseTypeForConversion = currentScannerType.Name // "int", "string", "ExternalType"
				if baseTypeForConversion == "" {
					fmt.Printf("      Warning: Field %s (%s) - field.Type.Name is empty for non-slice/non-pointer. Original type: %s. Skipping.\n", field.Name, fInfo.OriginalFieldTypeString, field.Type.String())
					if bindFrom != "body" {
						continue
					}
				}
			}

			fInfo.FieldType = baseTypeForConversion // This is what template's {{.FieldType}} will be, e.g., "int", "string", "MyStruct"

			// Populate numeric type details based on 'baseTypeForConversion'
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
				fInfo.IsNumeric = true // Technically, but for template logic, IsComplex is more specific
				fInfo.IsComplex = true
				if size, ok := map[string]int{"complex64": 64, "complex128": 128}[baseTypeForConversion]; ok {
					fInfo.BitSize = size
				}
			case "string", "bool":
				// These are handled. IsNumeric/IsFloat/IsSigned remain false.
				break // Explicitly break, though it's the end of switch cases for these.
			default:
				// This is for non-basic types when not binding from body.
				// Includes custom struct types, etc.
				if bindFrom != "body" && !fInfo.IsSlice {
					fmt.Printf("      Skipping field %s: unhandled base type '%s' (original: %s, slice: %t) for %s binding\n", field.Name, baseTypeForConversion, fInfo.OriginalFieldTypeString, fInfo.IsSlice, bindFrom)
					continue
				} else if bindFrom != "body" && fInfo.IsSlice && !isWellKnownSliceElementType(fInfo.SliceElementType) {
					// isWellKnownSliceElementType checks if SliceElementType is like "string", "int", "*bool" etc.
					fmt.Printf("      Skipping field %s: slice of unhandled element type '%s' (original: %s) for %s binding\n", field.Name, fInfo.SliceElementType, fInfo.OriginalFieldTypeString, bindFrom)
					continue
				}
			}

			// Determine if conversion is needed and manage imports
			if bindFrom != "body" {
				needsImportNetHTTP = true

				needsConv := fInfo.IsNumeric || fInfo.IsComplex || baseTypeForConversion == "bool"
				if fInfo.IsSlice {
					// Determine element base type for conversion check
					elemBaseTypeForConv := baseTypeForConversion // This was already set based on Elem
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

		if len(data.Fields) == 0 && !data.NeedsBody { // If no fields to bind and struct is not body target
			fmt.Printf("  Skipping struct %s: no bindable fields found and not a global body target.\n", typeInfo.Name)
			continue
		}

		// Manage imports (ensure "strings" is included if added above)
		// This check is implicitly handled as allFileImports is a map.
		// if _, ok := allFileImports["strings"]; ok {
		// }

		// if needsImportNetHTTP {  // THIS BLOCK IS DUPLICATED AND CAUSES THE ERROR
		// 		fieldBindingInfo.IsBody = true
		// 		data.NeedsBody = true
		// 		data.HasSpecificBodyFieldTarget = true
		// 		needsImportEncodingJson = true
		// 		needsImportIO = true
		// 	}

		// 	if bindFrom != "body" { // THIS BLOCK IS DUPLICATED
		// 		needsImportNetHTTP = true
		// 		if needsConversion {
		// 			needsImportStrconv = true
		// 		}
		// 		needsImportFmt = true
		// 	}

		// 	data.Fields = append(data.Fields, fieldBindingInfo) // THIS LINE IS DUPLICATED
		// } // THIS BRACE IS EXTRA

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

		funcMap := template.FuncMap{
			"stringsHasPrefix":  strings.HasPrefix,
			"stringsTrimPrefix": strings.TrimPrefix,
			// "stringsSplit":      strings.Split, // No longer needed
			// "stringsTrimSpace":  strings.TrimSpace, // No longer needed
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
		needsImportErrors = true // If a Bind method is generated, errors package might be needed
	}

	if generatedCodeForAllStructs.Len() == 0 {
		fmt.Println("No structs found requiring Bind method generation.")
		return nil
	}

	finalOutput := bytes.Buffer{}
	finalOutput.WriteString(fmt.Sprintf("// Code generated by derivingbind for package %s. DO NOT EDIT.\n\n", pkgInfo.Name))
	finalOutput.WriteString(fmt.Sprintf("package %s\n\n", pkgInfo.Name))

	if needsImportErrors { // Ensure errors is in allFileImports if needed
		allFileImports["errors"] = ""
	}

	if len(allFileImports) > 0 {
		finalOutput.WriteString("import (\n")
		// Correctly sort imports for stability, including "strings" if present
		paths := make([]string, 0, len(allFileImports))
		for path := range allFileImports {
			paths = append(paths, path)
		}
		sort.Strings(paths) // Sort the import paths
		for _, path := range paths {
			alias := allFileImports[path] // This is always "" in current code
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
