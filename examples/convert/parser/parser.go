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
		PackageName:       scannedPkg.Name,
		PackagePath:       scannedPkg.ImportPath,
		Imports:           make(map[string]string),
		Structs:           make(map[string]*model.StructInfo),
		NamedTypes:        make(map[string]*scanner.TypeInfo),
		ConversionPairs:   []model.ConversionPair{},
		GlobalRules:       []model.TypeRule{},
		ProcessedPackages: make(map[string]bool),
	}

	if err := processPackage(ctx, s, info, scannedPkg); err != nil {
		return nil, fmt.Errorf("failed to process package %q: %w", scannedPkg.ImportPath, err)
	}

	return info, nil
}

func processPackage(ctx context.Context, s *goscan.Scanner, info *model.ParsedInfo, pkgInfo *scanner.PackageInfo) error {
	if pkgInfo == nil || info.ProcessedPackages[pkgInfo.ImportPath] {
		return nil
	}
	info.ProcessedPackages[pkgInfo.ImportPath] = true
	log.Printf("Processing package: %s", pkgInfo.ImportPath)

	for _, astFile := range pkgInfo.AstFiles {
		for _, commentGroup := range astFile.Comments {
			for _, comment := range commentGroup.List {
				if m := reConvertImport.FindStringSubmatch(comment.Text); m != nil {
					alias, path := m[1], m[2]
					if existingPath, ok := info.Imports[alias]; ok && existingPath != path {
						return fmt.Errorf("duplicate import alias %q with different paths: %q vs %q", alias, existingPath, path)
					}
					info.Imports[alias] = path
				}
			}
		}
	}

	for _, t := range pkgInfo.Types {
		if _, exists := info.NamedTypes[t.Name]; !exists {
			info.NamedTypes[t.Name] = t
		}
		if t.Kind == scanner.StructKind {
			if _, exists := info.Structs[t.Name]; exists {
				continue
			}
			modelStructInfo := &model.StructInfo{Name: t.Name, Type: t}
			info.Structs[t.Name] = modelStructInfo

			fields, err := collectFields(ctx, s, info, t, pkgInfo, make(map[string]struct{}))
			if err != nil {
				return fmt.Errorf("collecting fields for struct %s in pkg %s: %w", t.Name, pkgInfo.ImportPath, err)
			}
			modelStructInfo.Fields = fields
			for i := range modelStructInfo.Fields {
				modelStructInfo.Fields[i].ParentStruct = modelStructInfo
			}
		}
	}

	for _, t := range pkgInfo.Types {
		if t.Doc == "" {
			continue
		}
		for _, line := range strings.Split(t.Doc, "\n") {
			m := reDerivingConvert.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			dstTypeNameRaw, optionsStr := strings.Trim(m[1], `"`), ""
			if len(m) > 2 {
				optionsStr = m[2]
			}

			srcTypeInfo, ok := info.NamedTypes[t.Name]
			if !ok {
				return fmt.Errorf("internal error: source type %q not found", t.Name)
			}

			dstTypeInfo, err := resolveType(ctx, s, info, pkgInfo, dstTypeNameRaw)
			if err != nil {
				return fmt.Errorf("destination type %q for source %q could not be resolved: %w", dstTypeNameRaw, t.Name, err)
			}

			pair := model.ConversionPair{
				SrcTypeName: t.Name, DstTypeName: dstTypeInfo.Name,
				SrcTypeInfo: srcTypeInfo, DstTypeInfo: dstTypeInfo,
			}

			if optionsStr != "" {
				parts := strings.Split(strings.TrimSpace(optionsStr), "=")
				if len(parts) == 2 && strings.TrimSpace(parts[0]) == "max_errors" {
					if maxErrors, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
						pair.MaxErrors = maxErrors
					}
				}
			}

			for _, docLine := range strings.Split(t.Doc, "\n") {
				if m := reConvertVariable.FindStringSubmatch(docLine); m != nil {
					pair.Variables = append(pair.Variables, model.Variable{Name: m[1], Type: m[2]})
				}
			}
			info.ConversionPairs = append(info.ConversionPairs, pair)
		}
	}

	for _, astFile := range pkgInfo.AstFiles {
		for _, commentGroup := range astFile.Comments {
			for _, comment := range commentGroup.List {
				if m := reConvertRule.FindStringSubmatch(comment.Text); m != nil {
					rule := model.TypeRule{}
					type1Name, type2Name, usingFunc, validatorFunc := m[1], m[2], m[3], m[4]

					if validatorFunc != "" {
						rule.ValidatorFunc, rule.DstTypeName = validatorFunc, type1Name
						dstTypeInfo, err := resolveType(ctx, s, info, pkgInfo, rule.DstTypeName)
						if err != nil {
							return fmt.Errorf("resolving validator rule dst type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					} else if usingFunc != "" {
						rule.UsingFunc, rule.SrcTypeName, rule.DstTypeName = usingFunc, type1Name, type2Name
						srcTypeInfo, err := resolveType(ctx, s, info, pkgInfo, rule.SrcTypeName)
						if err != nil {
							return fmt.Errorf("resolving global rule src type %q: %w", rule.SrcTypeName, err)
						}
						rule.SrcTypeInfo = srcTypeInfo
						dstTypeInfo, err := resolveType(ctx, s, info, pkgInfo, rule.DstTypeName)
						if err != nil {
							return fmt.Errorf("resolving global rule dst type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					}
					info.GlobalRules = append(info.GlobalRules, rule)
				}
			}
		}
	}
	return nil
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
	// HACK: This is a temporary fix to avoid crashing the parser.
	// The rule matching logic in the generator will be updated to use the raw type strings.
	if strings.HasPrefix(typeNameStr, "*") {
		typeNameStr = typeNameStr[1:]
	}

	if ti, ok := s.LookupOverride(typeNameStr); ok {
		return ti, nil
	}

	if !strings.Contains(typeNameStr, ".") {
		if t := p.Lookup(typeNameStr); t != nil {
			return t, nil
		}
		if isBuiltin(typeNameStr) {
			return &scanner.TypeInfo{Name: typeNameStr, Kind: scanner.AliasKind, Underlying: &scanner.FieldType{Name: typeNameStr, IsBuiltin: true}}, nil
		}
		return nil, fmt.Errorf("unqualified type %q not found in package %s", typeNameStr, p.Name)
	}

	var pkgPath string
	var name string
	var found bool

	lastDotIndex := strings.LastIndex(typeNameStr, ".")
	// this should not happen due to the strings.Contains check above, but as a safeguard
	if lastDotIndex == -1 {
		return nil, fmt.Errorf("qualified type %q does not contain a '.'", typeNameStr)
	}

	pkgIdentifier := typeNameStr[:lastDotIndex]
	name = typeNameStr[lastDotIndex+1:]

	if strings.Contains(pkgIdentifier, "/") {
		// Assume it's a full package path
		pkgPath = pkgIdentifier
		found = true
	} else {
		// Assume it's a package alias
		pkgAlias := pkgIdentifier
		pkgPath, found = info.Imports[pkgAlias]
		if !found {
			for _, f := range p.AstFiles {
				for _, i := range f.Imports {
					path := strings.Trim(i.Path.Value, `"`)
					if i.Name != nil && i.Name.Name == pkgAlias {
						pkgPath, found = path, true
						break
					}
					if i.Name == nil {
						// Heuristic: match alias with package name if no explicit alias
						if path == pkgAlias || strings.HasSuffix(path, "/"+pkgAlias) {
							pkgPath, found = path, true
							break
						}
					}
				}
				if found {
					break
				}
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("could not resolve package path for identifier %q", pkgIdentifier)
	}

	resolvableType := &scanner.FieldType{Resolver: s, FullImportPath: pkgPath, TypeName: name}
	resolvedTypeInfo, err := s.ResolveType(ctx, resolvableType)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve type %q: %w", typeNameStr, err)
	}

	if resolvedTypeInfo != nil && resolvedTypeInfo.PkgPath != "" && resolvedTypeInfo.PkgPath != p.ImportPath {
		resolvedPkgInfo, err := s.ScanPackageByImport(ctx, resolvedTypeInfo.PkgPath)
		if err != nil {
			return nil, fmt.Errorf("could not scan package %q for resolved type %s: %w", resolvedTypeInfo.PkgPath, resolvedTypeInfo.Name, err)
		}
		if err := processPackage(ctx, s, info, resolvedPkgInfo); err != nil {
			return nil, fmt.Errorf("failed to process recursively discovered package %q: %w", resolvedTypeInfo.PkgPath, err)
		}
	}
	return resolvedTypeInfo, nil
}

func collectFields(ctx context.Context, s *goscan.Scanner, info *model.ParsedInfo, t *scanner.TypeInfo, p *scanner.PackageInfo, visited map[string]struct{}) ([]model.FieldInfo, error) {
	if _, ok := visited[t.Name]; ok {
		return nil, nil
	}
	visited[t.Name] = struct{}{}
	if t.Struct == nil {
		return nil, fmt.Errorf("type %s is not a struct", t.Name)
	}

	var fields []model.FieldInfo
	for _, f := range t.Struct.Fields {
		var fieldTypeInfo *scanner.TypeInfo

		// In unit tests, s might be a dummy scanner, so check if Fset is available.
		isTest := s == nil || s.Fset() == nil
		if !isTest && !f.Type.IsBuiltin {
			var err error
			fieldTypeInfo, err = s.ResolveType(ctx, f.Type)
			if err != nil {
				log.Printf("Could not resolve field type %s, skipping: %v", f.Type.String(), err)
			}

			if fieldTypeInfo != nil && fieldTypeInfo.PkgPath != "" && fieldTypeInfo.PkgPath != p.ImportPath && !f.Type.IsResolvedByConfig {
				fieldPkgInfo, err := s.ScanPackageByImport(ctx, fieldTypeInfo.PkgPath)
				if err != nil {
					return nil, fmt.Errorf("could not scan package for field type %s: %w", fieldTypeInfo.Name, err)
				}
				if err := processPackage(ctx, s, info, fieldPkgInfo); err != nil {
					return nil, fmt.Errorf("failed to process package for field type %s: %w", fieldTypeInfo.Name, err)
				}
			}
		}

		if f.Embedded {
			if isTest {
				// Cannot resolve in test environment without a real scanner
				log.Printf("Skipping embedded field %s in test environment", f.Type.String())
				continue
			}
			if fieldTypeInfo == nil || fieldTypeInfo.Struct == nil {
				log.Printf("Resolved embedded type %s is not a struct, skipping", f.Type.String())
				continue
			}
			embeddedPkgInfo, err := s.ScanPackageByImport(ctx, fieldTypeInfo.PkgPath)
			if err != nil {
				return nil, fmt.Errorf("could not scan package for embedded struct %s: %w", fieldTypeInfo.Name, err)
			}
			embeddedFields, err := collectFields(ctx, s, info, fieldTypeInfo, embeddedPkgInfo, visited)
			if err != nil {
				return nil, fmt.Errorf("collecting fields from embedded struct %s: %w", fieldTypeInfo.Name, err)
			}
			fields = append(fields, embeddedFields...)
		} else {
			tag, err := parseConvertTag(reflect.StructTag(f.Tag))
			if err != nil {
				return nil, fmt.Errorf("parsing tag for %s.%s: %w", t.Name, f.Name, err)
			}
			fields = append(fields, model.FieldInfo{
				Name: f.Name, OriginalName: f.Name, JSONTag: parseJSONTag(reflect.StructTag(f.Tag)),
				FieldType: f.Type, Tag: tag, TypeInfo: fieldTypeInfo,
			})
		}
	}
	return fields, nil
}

func parseConvertTag(tag reflect.StructTag) (model.ConvertTag, error) {
	value := tag.Get("convert")
	result := model.ConvertTag{RawValue: value}
	if value == "" {
		return result, nil
	}
	parts := strings.Split(value, ",")
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
	return strings.Split(jsonTag, ",")[0]
}
