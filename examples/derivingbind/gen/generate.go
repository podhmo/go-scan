package gen

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"text/template"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

//go:embed bind_method.tmpl
var bindMethodTemplateFS embed.FS

//go:embed bind_method.tmpl
var bindMethodTemplateString string

const bindingAnnotation = "deriving:binding"

type TemplateData struct {
	StructName                 string
	Fields                     []FieldBindingInfo
	NeedsBody                  bool
	HasSpecificBodyFieldTarget bool
	HasNonBodyFields           bool // New flag
	ErrNoCookie                error
}

type FieldBindingInfo struct {
	FieldName    string
	FieldType    string // This will store the base type name for parser lookup
	BindFrom     string
	BindName     string
	IsPointer    bool
	IsRequired   bool
	IsBody       bool
	BodyJSONName string

	IsSlice                 bool
	SliceElementType        string // This will store the element's base type name
	OriginalFieldTypeString string // The original full type string (e.g., "[]*models.Item", "string")
	ParserFunc              string
	IsSliceElementPointer   bool
}

func Generate(ctx context.Context, gscn *goscan.Scanner, pkgInfo *scanner.PackageInfo, importManager *goscan.ImportManager) ([]byte, error) {
	if pkgInfo == nil {
		return nil, fmt.Errorf("cannot generate code for a nil package")
	}
	var generatedCodeForAllStructs bytes.Buffer
	anyCodeGenerated := false

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}

		annotationValue, hasBindingAnnotationOnStruct := typeInfo.Annotation(bindingAnnotation)
		structLevelInTag := ""
		if hasBindingAnnotationOnStruct {
			parts := strings.Fields(annotationValue)
			for _, part := range parts {
				if strings.HasPrefix(part, "in:") {
					structLevelInTag = strings.TrimSuffix(strings.SplitN(part, ":", 2)[1], `"`)
					structLevelInTag = strings.TrimPrefix(structLevelInTag, `"`)
					break
				}
			}
		}

		if !hasBindingAnnotationOnStruct {
			continue
		}
		slog.DebugContext(ctx, "Processing struct for binding", slog.String("struct", typeInfo.Name))

		data := TemplateData{
			StructName:                 typeInfo.Name,
			Fields:                     []FieldBindingInfo{},
			NeedsBody:                  (structLevelInTag == "body"),
			HasSpecificBodyFieldTarget: false,
			HasNonBodyFields:           false, // Initialize
			ErrNoCookie:                http.ErrNoCookie,
		}
		importManager.Add("net/http", "") // For http.ErrNoCookie and request object (r *http.Request)

		structHasBindableFields := false
		for _, field := range typeInfo.Struct.Fields {
			bindFrom := field.TagValue("in")
			if bindFrom == "" {
				if data.NeedsBody && structLevelInTag == "body" {
					// Field is part of struct-level body, handled by overall JSON decode.
				}
				continue
			}
			bindFrom = strings.ToLower(strings.TrimSpace(bindFrom))
			bindName := field.TagValue(bindFrom)

			switch bindFrom {
			case "path", "query", "header", "cookie":
				if bindName == "" {
					slog.DebugContext(ctx, "Skipping field: tag requires corresponding name tag", "struct", typeInfo.Name, "field", field.Name, "in_tag", bindFrom)
					continue
				}
			case "body":
				data.NeedsBody = true
			default:
				slog.DebugContext(ctx, "Skipping field: unknown 'in' tag value", "struct", typeInfo.Name, "field", field.Name, "in_tag", bindFrom)
				continue
			}

			originalFieldTypeStr := field.Type.String()
			if field.Type.FullImportPath != "" && field.Type.FullImportPath != pkgInfo.ImportPath {
				originalFieldTypeStr = importManager.Qualify(field.Type.FullImportPath, field.Type.Name)
				if field.Type.IsSlice && field.Type.Elem != nil {
					sliceElemStr := importManager.Qualify(field.Type.Elem.FullImportPath, field.Type.Elem.Name)
					if field.Type.Elem.IsPointer {
						sliceElemStr = "*" + sliceElemStr
					}
					originalFieldTypeStr = "[]" + sliceElemStr
				} else if field.Type.IsPointer && field.Type.Elem != nil {
					originalFieldTypeStr = "*" + importManager.Qualify(field.Type.Elem.FullImportPath, field.Type.Elem.Name)
				}
			}

			fInfo := FieldBindingInfo{
				FieldName:               field.Name,
				BindFrom:                bindFrom,
				BindName:                bindName,
				IsRequired:              (field.TagValue("required") == "true"),
				OriginalFieldTypeString: originalFieldTypeStr,
				IsPointer:               field.Type.IsPointer,
			}

			currentScannerType := field.Type
			baseTypeForConversion := ""

			if currentScannerType.IsSlice {
				fInfo.IsSlice = true
				if currentScannerType.Elem != nil {
					fInfo.SliceElementType = importManager.Qualify(currentScannerType.Elem.FullImportPath, currentScannerType.Elem.Name)
					if currentScannerType.Elem.IsPointer {
						fInfo.SliceElementType = "*" + fInfo.SliceElementType
					}
					fInfo.IsSliceElementPointer = currentScannerType.Elem.IsPointer

					sliceElemForParser := currentScannerType.Elem
					if sliceElemForParser.IsPointer && sliceElemForParser.Elem != nil {
						baseTypeForConversion = sliceElemForParser.Elem.Name
					} else {
						baseTypeForConversion = sliceElemForParser.Name
					}
				} else {
					slog.DebugContext(ctx, "Skipping field: slice with nil Elem type", "struct", typeInfo.Name, "field", field.Name)
					continue
				}
			} else if currentScannerType.IsPointer {
				if currentScannerType.Elem != nil {
					baseTypeForConversion = currentScannerType.Elem.Name
				} else {
					baseTypeForConversion = currentScannerType.Name
				}
			} else {
				baseTypeForConversion = currentScannerType.Name
			}
			fInfo.FieldType = baseTypeForConversion

			switch baseTypeForConversion {
			case "string":
				fInfo.ParserFunc = "parser.String"
			case "int", "int8", "int16", "int32", "int64":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			case "bool":
				fInfo.ParserFunc = "parser.Bool"
			case "float32", "float64":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			case "complex64", "complex128":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			default:
				if bindFrom != "body" {
					slog.DebugContext(ctx, "Skipping field: unhandled base type for non-body binding", "struct", typeInfo.Name, "field", field.Name, "baseType", baseTypeForConversion, "bindFrom", bindFrom)
					continue
				}
			}

			if bindFrom != "body" {
				data.HasNonBodyFields = true
				importManager.Add("github.com/podhmo/go-scan/examples/derivingbind/binding", "")
				importManager.Add("github.com/podhmo/go-scan/examples/derivingbind/parser", "")
				importManager.Add("errors", "") // For errors.Join
				if fInfo.ParserFunc == "" {
					slog.DebugContext(ctx, "Skipping field: No parser func for non-body binding", "struct", typeInfo.Name, "field", field.Name)
					continue
				}
			} else {
				fInfo.IsBody = true
				data.NeedsBody = true
				data.HasSpecificBodyFieldTarget = true
				importManager.Add("encoding/json", "")
				importManager.Add("io", "")
				importManager.Add("fmt", "")
				importManager.Add("errors", "")
			}
			data.Fields = append(data.Fields, fInfo)
			structHasBindableFields = true
		}

		if !structHasBindableFields && !data.NeedsBody {
			slog.DebugContext(ctx, "Skipping struct: no bindable fields or global body target", "struct", typeInfo.Name)
			continue
		}
		anyCodeGenerated = true

		if data.NeedsBody && !data.HasSpecificBodyFieldTarget {
			importManager.Add("encoding/json", "")
			importManager.Add("io", "")
			importManager.Add("fmt", "")
			importManager.Add("errors", "")
		}

		funcMap := template.FuncMap{"TitleCase": strings.Title}
		tmpl, err := template.New("bind").Funcs(funcMap).Parse(bindMethodTemplateString)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template: %w", err)
		}
		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, data); err != nil {
			return nil, fmt.Errorf("failed to execute template for struct %s: %w", typeInfo.Name, err)
		}
		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")
	}

	if !anyCodeGenerated {
		slog.InfoContext(ctx, "No structs found requiring Bind method generation in package", slog.String("package_path", pkgInfo.Path))
		return nil, nil
	}

	return generatedCodeForAllStructs.Bytes(), nil
}
