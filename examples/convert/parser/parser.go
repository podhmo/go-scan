package parser

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/podhmo/go-scan/examples/convert/model"
	"github.com/podhmo/go-scan/scanner"
)

var (
	reDerivingConvert = regexp.MustCompile(`@derivingconvert\(([^,)]+)(?:,\s*([^)]+))?\)`)
	reConvertRule     = regexp.MustCompile(`// convert:rule "([^"]+)"(?: -> "([^"]+)")?, (?:using=([a-zA-Z0-9_.]+)|validator=([a-zA-Z0-9_.]+))`)
	reConvertImport   = regexp.MustCompile(`// convert:import "([^"]+)" "([^"]+)"`)
)

func Parse(ctx context.Context, scannedPkg *scanner.PackageInfo) (*model.ParsedInfo, error) {
	info := &model.ParsedInfo{
		PackageName:     scannedPkg.Name,
		PackagePath:     scannedPkg.ImportPath,
		Imports:         []model.Import{},
		Structs:         make(map[string]*model.StructInfo),
		NamedTypes:      make(map[string]*scanner.TypeInfo),
		ConversionPairs: []model.ConversionPair{},
		GlobalRules:     []model.TypeRule{},
	}

	for _, t := range scannedPkg.Types {
		info.NamedTypes[t.Name] = t
		if t.Kind == scanner.StructKind {
			modelStructInfo := &model.StructInfo{
				Name: t.Name,
				Type: t,
			}
			fields, err := collectFields(ctx, t, scannedPkg, make(map[string]struct{}))
			if err != nil {
				return nil, fmt.Errorf("collecting fields for struct %s: %w", t.Name, err)
			}
			modelStructInfo.Fields = fields
			for i := range modelStructInfo.Fields {
				modelStructInfo.Fields[i].ParentStruct = modelStructInfo
			}
			info.Structs[t.Name] = modelStructInfo
		}
	}

	for _, t := range scannedPkg.Types {
		if t.Doc == "" {
			continue
		}
		for _, line := range strings.Split(t.Doc, "\n") {
			m := reDerivingConvert.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			dstTypeName := strings.Trim(m[1], `"`)
			optionsStr := ""
			if len(m) > 2 {
				optionsStr = m[2]
			}

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

			if optionsStr != "" {
				parts := strings.Split(strings.TrimSpace(optionsStr), "=")
				if len(parts) == 2 && strings.TrimSpace(parts[0]) == "max_errors" {
					maxErrors, err := strconv.Atoi(strings.TrimSpace(parts[1]))
					if err != nil {
						return nil, fmt.Errorf("invalid max_errors value %q for %s: %w", parts[1], t.Name, err)
					}
					pair.MaxErrors = maxErrors
				}
			}
			info.ConversionPairs = append(info.ConversionPairs, pair)
		}
	}

	for _, astFile := range scannedPkg.AstFiles {
		for _, commentGroup := range astFile.Comments {
			for _, comment := range commentGroup.List {
				// Try parsing as import rule
				if m := reConvertImport.FindStringSubmatch(comment.Text); m != nil {
					info.Imports = append(info.Imports, model.Import{
						Alias: m[1],
						Path:  m[2],
					})
					continue
				}

				// Try parsing as conversion/validation rule
				if m := reConvertRule.FindStringSubmatch(comment.Text); m != nil {
					rule := model.TypeRule{}
					type1Name := m[1]
					type2Name := m[2]
					usingFunc := m[3]
					validatorFunc := m[4]

					if validatorFunc != "" {
						rule.ValidatorFunc = validatorFunc
						rule.DstTypeName = type1Name
						dstTypeInfo, err := resolveType(scannedPkg, info, rule.DstTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving validator rule destination type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					} else if usingFunc != "" {
						rule.UsingFunc = usingFunc
						rule.SrcTypeName = type1Name
						rule.DstTypeName = type2Name

						srcTypeInfo, err := resolveType(scannedPkg, info, rule.SrcTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving global rule source type %q: %w", rule.SrcTypeName, err)
						}
						rule.SrcTypeInfo = srcTypeInfo

						dstTypeInfo, err := resolveType(scannedPkg, info, rule.DstTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving global rule destination type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					} else {
						continue
					}
					info.GlobalRules = append(info.GlobalRules, rule)
				}
			}
		}
	}

	for _, rule := range info.GlobalRules {
		log.Printf("GlobalRule: Src=%q, Dst=%q, Using=%q", rule.SrcTypeName, rule.DstTypeName, rule.UsingFunc)
	}
	return info, nil
}

func resolveType(p *scanner.PackageInfo, info *model.ParsedInfo, typeName string) (*scanner.TypeInfo, error) {
	// Check for primitive types first
	if !strings.Contains(typeName, ".") {
		t := p.Lookup(typeName)
		if t != nil {
			return t, nil
		}
		// It might be a primitive type like "string", "int", etc.
		return &scanner.TypeInfo{
			Name: typeName,
			Kind: scanner.AliasKind, // Treat as alias to underlying primitive
			Underlying: &scanner.FieldType{
				Name:      typeName,
				IsBuiltin: true,
			},
		}, nil
	}

	parts := strings.Split(typeName, ".")
	pkgAlias := parts[0]
	name := strings.Join(parts[1:], ".") // Handle cases like v.New().Var

	// Find the full import path from the alias
	pkgPath, found := "", false

	// Priority 1: Check `// convert:import` annotations
	for _, imp := range info.Imports {
		if imp.Alias == pkgAlias {
			pkgPath = imp.Path
			found = true
			break
		}
	}

	// Priority 2: Check file imports
	if !found {
		for _, f := range p.AstFiles {
			for _, i := range f.Imports {
				path := strings.Trim(i.Path.Value, `"`)
				if i.Name != nil && i.Name.Name == pkgAlias {
					pkgPath = path
					found = true
					break
				}
				// This logic is tricky and can be incorrect for complex cases.
				// e.g. import "github.com/a/b" and alias is "b"
				if i.Name == nil {
					importParts := strings.Split(path, "/")
					if len(importParts) > 0 && importParts[len(importParts)-1] == pkgAlias {
						pkgPath = path
						found = true
						break
					}
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		// Fallback for built-in packages like "time"
		if pkgAlias == "time" {
			pkgPath = "time"
		} else {
			return nil, fmt.Errorf("could not resolve package path for alias %q in type %q", pkgAlias, typeName)
		}
	}

	// Create a synthetic TypeInfo for the external type.
	isBuiltIn := (pkgPath == "time") // A bit of a hack, but works for "time"

	return &scanner.TypeInfo{
		Name:    name,
		PkgPath: pkgPath,
		Kind:    scanner.InterfaceKind, // Use a generic kind
		Underlying: &scanner.FieldType{
			Name:               name,
			PkgName:            pkgAlias,
			IsResolvedByConfig: isBuiltIn, // Prevent further resolution for these types
		},
	}, nil
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

func parseJSONTag(tag reflect.StructTag) string {
	jsonTag := tag.Get("json")
	if jsonTag == "" {
		return ""
	}
	parts := strings.Split(jsonTag, ",")
	return parts[0]
}

func collectFields(ctx context.Context, t *scanner.TypeInfo, p *scanner.PackageInfo, visited map[string]struct{}) ([]model.FieldInfo, error) {
	if _, ok := visited[t.Name]; ok {
		return nil, nil // cycle detected
	}
	visited[t.Name] = struct{}{}

	var fields []model.FieldInfo
	if t.Struct == nil {
		return nil, fmt.Errorf("type %s is not a struct", t.Name)
	}

	for _, f := range t.Struct.Fields {
		if f.Embedded {
			// Resolve the embedded field's type to get its own fields.
			// This requires the FieldType to be resolved to a TypeInfo.
			embeddedTypeName := f.Type.Name
			if f.Type.IsPointer {
				if f.Type.Elem != nil {
					embeddedTypeName = f.Type.Elem.Name
				}
			}

			// Look up the TypeInfo for the embedded struct.
			embeddedTypeInfo := p.Lookup(embeddedTypeName)
			if embeddedTypeInfo == nil || embeddedTypeInfo.Struct == nil {
				// If not found in the current package, it might be in an external package.
				// This part requires resolving external types, which might be complex.
				// For now, we assume embedded types are within the same scanned package.
				log.Printf("Could not resolve embedded struct type %s, skipping", embeddedTypeName)
				continue
			}

			// Recursively collect fields from the embedded struct.
			embeddedFields, err := collectFields(ctx, embeddedTypeInfo, p, visited)
			if err != nil {
				return nil, fmt.Errorf("collecting fields from embedded struct %s: %w", embeddedTypeName, err)
			}
			fields = append(fields, embeddedFields...)
		} else {
			// This is a regular field.
			structTag := reflect.StructTag(f.Tag)
			tag, err := parseConvertTag(structTag)
			if err != nil {
				return nil, fmt.Errorf("parsing tag for %s.%s: %w", t.Name, f.Name, err)
			}

			fieldTypeInfo := p.Lookup(f.Type.Name)
			if fieldTypeInfo == nil {
				fieldTypeInfo = &scanner.TypeInfo{Name: f.Type.Name}
			}

			fieldInfo := model.FieldInfo{
				Name:         f.Name,
				OriginalName: f.Name,
				JSONTag:      parseJSONTag(structTag),
				TypeInfo:     fieldTypeInfo,
				FieldType:    f.Type,
				Tag:          tag,
				// ParentStruct is set by the caller
			}
			fields = append(fields, fieldInfo)
		}
	}
	return fields, nil
}
