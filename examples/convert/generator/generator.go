package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"text/template"

	"example.com/convert/parser"
	"github.com/podhmo/go-scan/scanner"
)

const codeTemplate = `
import (
	"context"
	{{- range .Imports }}
	"{{ . }}"
	{{- end }}
)

{{ range .Pairs }}
// {{ .ExportedFuncName }} converts {{ .SrcType.Name }} to {{ .DstType.Name }}.
func {{ .ExportedFuncName }}(ctx context.Context, src {{ .ModelsPackageName }}.{{ .SrcType.Name }}) ({{ .ModelsPackageName }}.{{ .DstType.Name }}, error) {
	// In the future, this will use an error collector.
	// For now, we just call the internal function.
	dst := {{ .InternalFuncName }}(ctx, src)
	return dst, nil
}

// {{ .InternalFuncName }} is the internal conversion function.
func {{ .InternalFuncName }}(ctx context.Context, src {{ .ModelsPackageName }}.{{ .SrcType.Name }}) {{ .ModelsPackageName }}.{{ .DstType.Name }} {
	dst := {{ .ModelsPackageName }}.{{ .DstType.Name }}{}

	{{- range .FieldMappings }}
	dst.{{ .DstField }} = src.{{ .SrcField }}
	{{- end }}

	return dst
}
{{ end }}
`

type TemplateData struct {
	PackageName string
	Imports     []string
	Pairs       []TemplatePair
}

type TemplatePair struct {
	ExportedFuncName  string
	InternalFuncName  string
	SrcType           *scanner.TypeInfo
	DstType           *scanner.TypeInfo
	FieldMappings     []FieldMapping
	ModelsPackageName string
}

type FieldMapping struct {
	SrcField string
	DstField string
}

// Generate generates the Go code for the conversion functions.
func Generate(packageName string, pairs []parser.ConversionPair, pkgInfo *scanner.PackageInfo) ([]byte, error) {
	imports := map[string]bool{
		"context":        true,
		pkgInfo.ImportPath: true,
	}

	templateData := TemplateData{
		PackageName: packageName,
	}

	for _, pair := range pairs {
		if pair.SrcType.Struct == nil || pair.DstType.Struct == nil {
			continue // Skip non-struct types
		}

		templatePair := TemplatePair{
			ExportedFuncName:  fmt.Sprintf("Convert%sTo%s", pair.SrcType.Name, pair.DstType.Name),
			InternalFuncName:  fmt.Sprintf("convert%sTo%s", pair.SrcType.Name, pair.DstType.Name),
			SrcType:           pair.SrcType,
			DstType:           pair.DstType,
			ModelsPackageName: pkgInfo.Name,
		}

		// Basic field mapping: match by name
		dstFields := make(map[string]bool)
		for _, field := range pair.DstType.Struct.Fields {
			dstFields[field.Name] = true
		}

		for _, srcField := range pair.SrcType.Struct.Fields {
			if _, exists := dstFields[srcField.Name]; exists {
				// TODO: Check if types are compatible
				templatePair.FieldMappings = append(templatePair.FieldMappings, FieldMapping{
					SrcField: srcField.Name,
					DstField: srcField.Name,
				})
			}
		}
		templateData.Pairs = append(templateData.Pairs, templatePair)
	}

	for imp := range imports {
		templateData.Imports = append(templateData.Imports, imp)
	}


	tmpl, err := template.New("converter").Parse(codeTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// Format the generated code
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to format generated code: %w", err)
	}

	return formatted, nil
}

func camelCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[0:1]) + s[1:]
}
