package parser

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/convert/model"
	"github.com/podhmo/go-scan/scanner"
)

var (
	reDerivingConvert = regexp.MustCompile(`@derivingconvert\(([^,)]+)(?:,\s*([^)]+))?\)`)
	reConvertRule     = regexp.MustCompile(`// convert:rule "([^"]+)"(?: -> "([^"]+)")?, (?:using=([a-zA-Z0-9_.]+)|validator=([a-zA-Z0-9_.]+))`)
	reConvertImport   = regexp.MustCompile(`// convert:import ([a-zA-Z0-9_.]+) "([^"]+)"`)
	reConvertVariable = regexp.MustCompile(`// convert:variable (\w+)\s+(.+)`)
)

func Parse(ctx context.Context, s *goscan.Scanner, scannedPkg *scanner.PackageInfo) (*model.ParsedInfo, error) {
	info := &model.ParsedInfo{
		PackageName:     scannedPkg.Name,
		PackagePath:     scannedPkg.ImportPath,
		Imports:         make(map[string]string),
		Structs:         make(map[string]*model.StructInfo),
		NamedTypes:      make(map[string]*scanner.TypeInfo),
		ConversionPairs: []model.ConversionPair{},
		GlobalRules:     []model.TypeRule{},
	}

	// Pre-pass for imports, as they are needed for resolution
	for _, astFile := range scannedPkg.AstFiles {
		for _, commentGroup := range astFile.Comments {
			for _, comment := range commentGroup.List {
				if m := reConvertImport.FindStringSubmatch(comment.Text); m != nil {
					alias := m[1]
					path := m[2]
					if _, ok := info.Imports[alias]; ok {
						return nil, fmt.Errorf("duplicate import alias %q", alias)
					}
					info.Imports[alias] = path
				}
			}
		}
	}

	for _, t := range scannedPkg.Types {
		info.NamedTypes[t.Name] = t
		if t.Kind == scanner.StructKind {
			modelStructInfo := &model.StructInfo{
				Name: t.Name,
				Type: t,
			}
			fields, err := collectFields(ctx, s, t, scannedPkg, make(map[string]struct{}))
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

			dstTypeNameRaw := strings.Trim(m[1], `"`)
			optionsStr := ""
			if len(m) > 2 {
				optionsStr = m[2]
			}

			srcTypeInfo, ok := info.NamedTypes[t.Name]
			if !ok {
				return nil, fmt.Errorf("internal error: source type %q not found after initial pass", t.Name)
			}

			dstTypeInfo, err := resolveType(ctx, s, info, scannedPkg, dstTypeNameRaw)
			if err != nil {
				return nil, fmt.Errorf("destination type %q for source %q could not be resolved: %w", dstTypeNameRaw, t.Name, err)
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

			variables := []model.Variable{}
			for _, docLine := range strings.Split(t.Doc, "\n") {
				if m := reConvertVariable.FindStringSubmatch(docLine); m != nil {
					variables = append(variables, model.Variable{Name: m[1], Type: m[2]})
				}
			}
			pair.Variables = variables

			info.ConversionPairs = append(info.ConversionPairs, pair)
		}
	}

	for _, astFile := range scannedPkg.AstFiles {
		for _, commentGroup := range astFile.Comments {
			for _, comment := range commentGroup.List {
				if m := reConvertRule.FindStringSubmatch(comment.Text); m != nil {
					rule := model.TypeRule{}
					type1Name := m[1]
					type2Name := m[2]
					usingFunc := m[3]
					validatorFunc := m[4]

					if validatorFunc != "" {
						rule.ValidatorFunc = validatorFunc
						rule.DstTypeName = type1Name
						dstTypeInfo, err := resolveType(ctx, s, info, scannedPkg, rule.DstTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving validator rule destination type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					} else if usingFunc != "" {
						rule.UsingFunc = usingFunc
						rule.SrcTypeName = type1Name
						rule.DstTypeName = type2Name

						srcTypeInfo, err := resolveType(ctx, s, info, scannedPkg, rule.SrcTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving global rule source type %q: %w", rule.SrcTypeName, err)
						}
						rule.SrcTypeInfo = srcTypeInfo

						dstTypeInfo, err := resolveType(ctx, s, info, scannedPkg, rule.DstTypeName)
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
	return info, nil
}

func isBuiltin(name string) bool {
	switch name {
	case "bool", "byte", "complex128", "complex64", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}

func resolveType(ctx context.Context, s *goscan.Scanner, info *model.ParsedInfo, p *scanner.PackageInfo, typeNameStr string) (*scanner.TypeInfo, error) {
	// Workaround: The scanner has a bug when resolving `time.Time` from within a test
	// running as `package main`. It gets confused about the package name.
	// To avoid this, we treat time.Time as a special case and return a synthetic TypeInfo,
	// similar to how the old parser behaved.
	if typeNameStr == "time.Time" {
		return &scanner.TypeInfo{
			Name:    "Time",
			PkgPath: "time",
			Kind:    scanner.InterfaceKind, // or StructKind, doesn't matter much for the generator
			Underlying: &scanner.FieldType{
				Name:               "Time",
				PkgName:            "time",
				IsResolvedByConfig: true, // This prevents further resolution attempts
			},
		}, nil
	}

	if !strings.Contains(typeNameStr, ".") {
		t := p.Lookup(typeNameStr)
		if t != nil {
			return t, nil
		}
		if isBuiltin(typeNameStr) {
			return &scanner.TypeInfo{
				Name: typeNameStr,
				Kind: scanner.AliasKind,
				Underlying: &scanner.FieldType{
					Name:      typeNameStr,
					IsBuiltin: true,
				},
			}, nil
		}
		return nil, fmt.Errorf("unqualified type %q not found in package %s", typeNameStr, p.Name)
	}

	parts := strings.Split(typeNameStr, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("unsupported type format for resolution: %q, expected 'pkg.Type'", typeNameStr)
	}
	pkgAlias := parts[0]
	name := parts[1]

	pkgPath, found := info.Imports[pkgAlias]
	if !found {
		for _, f := range p.AstFiles {
			for _, i := range f.Imports {
				path := strings.Trim(i.Path.Value, `"`)
				if i.Name != nil && i.Name.Name == pkgAlias {
					pkgPath = path
					found = true
					break
				}
				if i.Name == nil && strings.HasSuffix(path, "/"+pkgAlias) {
					pkgPath = path
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		if pkgAlias == "time" {
			pkgPath = "time"
			found = true
		}
	}

	if !found {
		return nil, fmt.Errorf("could not resolve package path for alias %q in type %q", pkgAlias, typeNameStr)
	}

	resolvableType := &scanner.FieldType{
		Resolver:       s,
		FullImportPath: pkgPath,
		TypeName:       name,
	}

	resolvedTypeInfo, err := s.ResolveType(ctx, resolvableType)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve type %q: %w", typeNameStr, err)
	}
	return resolvedTypeInfo, nil
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

func collectFields(ctx context.Context, s *goscan.Scanner, t *scanner.TypeInfo, p *scanner.PackageInfo, visited map[string]struct{}) ([]model.FieldInfo, error) {
	if _, ok := visited[t.Name]; ok {
		return nil, nil
	}
	visited[t.Name] = struct{}{}

	var fields []model.FieldInfo
	if t.Struct == nil {
		return nil, fmt.Errorf("type %s is not a struct", t.Name)
	}

	for _, f := range t.Struct.Fields {
		if f.Embedded {
			embeddedTypeInfo, err := s.ResolveType(ctx, f.Type)
			if err != nil {
				log.Printf("Could not resolve embedded struct type %s, skipping: %v", f.Type.String(), err)
				continue
			}
			if embeddedTypeInfo == nil || embeddedTypeInfo.Struct == nil {
				log.Printf("Resolved embedded type %s is not a struct, skipping", f.Type.String())
				continue
			}

			embeddedPkgInfo, err := s.ScanPackageByImport(ctx, embeddedTypeInfo.PkgPath)
			if err != nil {
				return nil, fmt.Errorf("could not scan package %q for embedded struct %s: %w", embeddedTypeInfo.PkgPath, embeddedTypeInfo.Name, err)
			}

			embeddedFields, err := collectFields(ctx, s, embeddedTypeInfo, embeddedPkgInfo, visited)
			if err != nil {
				return nil, fmt.Errorf("collecting fields from embedded struct %s: %w", embeddedTypeInfo.Name, err)
			}
			fields = append(fields, embeddedFields...)
		} else {
			structTag := reflect.StructTag(f.Tag)
			tag, err := parseConvertTag(structTag)
			if err != nil {
				return nil, fmt.Errorf("parsing tag for %s.%s: %w", t.Name, f.Name, err)
			}
			fieldInfo := model.FieldInfo{
				Name:         f.Name,
				OriginalName: f.Name,
				JSONTag:      parseJSONTag(structTag),
				TypeInfo:     nil,
				FieldType:    f.Type,
				Tag:          tag,
			}
			fields = append(fields, fieldInfo)
		}
	}
	return fields, nil
}
