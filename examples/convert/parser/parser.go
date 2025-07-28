package parser

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"example.com/convert/model"
	"github.com/podhmo/go-scan/scanner"
)

var reDerivingConvert = regexp.MustCompile(`@derivingconvert\("([^"]+)"\)`)

func Parse(ctx context.Context, scannedPkg *scanner.PackageInfo) (*model.ParsedInfo, error) {
	info := &model.ParsedInfo{
		PackageName:     scannedPkg.Name,
		PackagePath:     scannedPkg.ImportPath,
		Structs:         make(map[string]*model.StructInfo),
		NamedTypes:      make(map[string]*scanner.TypeInfo),
		ConversionPairs: []model.ConversionPair{},
	}

	for _, t := range scannedPkg.Types {
		info.NamedTypes[t.Name] = t
		if t.Kind == scanner.StructKind {
			modelStructInfo := &model.StructInfo{
				Name: t.Name,
				Type: t,
			}
			for _, f := range t.Struct.Fields {
				tag, err := parseConvertTag(reflect.StructTag(f.Tag))
				if err != nil {
					return nil, fmt.Errorf("parsing tag for %s.%s: %w", t.Name, f.Name, err)
				}

				fieldTypeInfo := scannedPkg.Lookup(f.Type.Name)
				if fieldTypeInfo == nil {
					fieldTypeInfo = &scanner.TypeInfo{Name: f.Type.Name}
				}

				fieldInfo := model.FieldInfo{
					Name:         f.Name,
					OriginalName: f.Name,
					TypeInfo:     fieldTypeInfo,
					FieldType:    f.Type, // Store the original FieldType
					Tag:          tag,
					ParentStruct: modelStructInfo,
				}
				modelStructInfo.Fields = append(modelStructInfo.Fields, fieldInfo)
			}
			info.Structs[t.Name] = modelStructInfo
		}
	}

	for _, t := range scannedPkg.Types {
		if t.Doc == "" {
			continue
		}
		m := reDerivingConvert.FindStringSubmatch(t.Doc)
		if m == nil {
			continue
		}
		dstTypeName := m[1]
		srcTypeInfo, ok := info.NamedTypes[t.Name]
		if !ok {
			return nil, fmt.Errorf("internal error: source type %q not found after initial pass", t.Name)
		}
		dstTypeInfo := scannedPkg.Lookup(dstTypeName)
		if dstTypeInfo == nil {
			return nil, fmt.Errorf("destination type %q for source %q not found in scanned package", dstTypeName, t.Name)
		}
		pair := model.ConversionPair{
			SrcTypeName: t.Name,
			DstTypeName: dstTypeInfo.Name,
			SrcTypeInfo: srcTypeInfo,
			DstTypeInfo: dstTypeInfo,
		}
		info.ConversionPairs = append(info.ConversionPairs, pair)
	}

	return info, nil
}

func parseConvertTag(tag reflect.StructTag) (model.ConvertTag, error) {
	value := tag.Get("convert")
	result := model.ConvertTag{RawValue: value}
	if value == "" {
		return result, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return result, nil
	}
	if !strings.Contains(parts[0], "=") {
		result.DstFieldName = strings.TrimSpace(parts[0])
		parts = parts[1:]
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case part == "-":
			result.DstFieldName = "-"
		case part == "required":
			result.Required = true
		case strings.HasPrefix(part, "using="):
			result.UsingFunc = strings.TrimPrefix(part, "using=")
		default:
			if result.DstFieldName == "" && !strings.Contains(part, "=") {
				result.DstFieldName = part
			}
		}
	}
	return result, nil
}
