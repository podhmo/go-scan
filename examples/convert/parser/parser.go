package parser

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
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

			dstTypeName := strings.Trim(m[1], `"`)
			optionsStr := ""
			if len(m) > 2 {
				optionsStr = m[2]
			}

			srcTypeInfo, ok := info.NamedTypes[t.Name]
			if !ok {
				return nil, fmt.Errorf("internal error: source type %q not found after initial pass", t.Name)
			}
			dstType, err := resolveType(ctx, s, info, scannedPkg, dstTypeName)
			if err != nil {
				return nil, fmt.Errorf("could not resolve destination type %q for source %q: %w", dstTypeName, t.Name, err)
			}

			// If the resolved type is a struct and we haven't processed it yet, do so now.
			if _, ok := info.Structs[dstType.Name]; !ok && dstType.Kind == scanner.StructKind {
				if err := processNewlyResolvedStruct(ctx, s, info, scannedPkg, dstType); err != nil {
					return nil, err
				}
			}

			pair := model.ConversionPair{
				SrcTypeName: t.Name,
				DstTypeName: dstType.Name,
				SrcTypeInfo: srcTypeInfo,
				DstTypeInfo: dstType,
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

			// Also parse variables from the same doc comment block
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
				// Try parsing as an import rule
				if m := reConvertImport.FindStringSubmatch(comment.Text); m != nil {
					alias := m[1]
					path := m[2]
					if _, ok := info.Imports[alias]; ok {
						return nil, fmt.Errorf("duplicate import alias %q", alias)
					}
					info.Imports[alias] = path
					log.Printf("Found import rule: alias=%s, path=%s", alias, path)
					continue
				}

				// Try parsing as a conversion/validation rule
				if m := reConvertRule.FindStringSubmatch(comment.Text); m != nil {
					rule := model.TypeRule{}
					type1Name := m[1]
					type2Name := m[2]
					usingFunc := m[3]
					validatorFunc := m[4]

					if validatorFunc != "" {
						// Validator rule: // convert:rule "<DstType>", validator=<func>
						rule.ValidatorFunc = validatorFunc
						rule.DstTypeName = type1Name
						dstTypeInfo, err := resolveType(ctx, s, info, scannedPkg, rule.DstTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving validator rule destination type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					} else if usingFunc != "" {
						// Conversion rule: // convert:rule "<SrcType>" -> "<DstType>", using=<func>
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
						continue // Should not happen with the new regex
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

func resolveType(ctx context.Context, s *goscan.Scanner, info *model.ParsedInfo, currentPkg *scanner.PackageInfo, typeNameStr string) (*scanner.TypeInfo, error) {
	// Handle primitive types that don't need resolution.
	if !strings.Contains(typeNameStr, ".") {
		switch typeNameStr {
		case "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "bool", "byte", "rune":
			return &scanner.TypeInfo{
				Name: typeNameStr,
				Kind: scanner.AliasKind,
				Underlying: &scanner.FieldType{
					Name:      typeNameStr,
					IsBuiltin: true,
				},
			}, nil
		}
	}

	pkgAlias, typeNameOnly := "", typeNameStr
	if parts := strings.Split(typeNameStr, "."); len(parts) == 2 {
		pkgAlias = parts[0]
		typeNameOnly = parts[1]
	}

	ft := scanner.FieldType{
		Name:     typeNameOnly,
		TypeName: typeNameOnly,
		PkgName:  pkgAlias,
		Resolver: s,
	}

	if pkgAlias == "" {
		ft.FullImportPath = currentPkg.ImportPath
	} else {
		// Find the full import path from the alias.
		// First, check global imports from `// convert:import`.
		pkgPath, found := info.Imports[pkgAlias]

		// If not found, fall back to searching file-local imports.
		if !found {
			for _, astFile := range currentPkg.AstFiles {
				for _, imp := range astFile.Imports {
					path := strings.Trim(imp.Path.Value, `"`)
					if imp.Name != nil && imp.Name.Name == pkgAlias {
						pkgPath = path
						found = true
						break
					}
					if imp.Name == nil {
						if nameFromPath := filepath.Base(path); nameFromPath == pkgAlias {
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

		// Final fallback for built-in packages like "time".
		if !found && pkgAlias == "time" {
			pkgPath = "time"
			found = true
		}

		if !found {
			return nil, fmt.Errorf("could not resolve package path for alias %q", pkgAlias)
		}
		ft.FullImportPath = pkgPath
	}

	// For "time.Time", we can treat it as a special case that doesn't need full resolution
	// but isn't a simple primitive. We can return a synthetic type.
	if ft.FullImportPath == "time" && ft.TypeName == "Time" {
		return &scanner.TypeInfo{
			Name:    ft.TypeName,
			PkgPath: ft.FullImportPath,
			Kind:    scanner.InterfaceKind, // Treat as opaque
			Underlying: &scanner.FieldType{
				Name:               ft.TypeName,
				PkgName:            ft.PkgName,
				IsResolvedByConfig: true, // Prevent further resolution
			},
		}, nil
	}

	resolvedType, err := s.ResolveType(ctx, &ft)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve type %q: %w", typeNameStr, err)
	}
	if resolvedType == nil {
		// Lookup in the current package as a last resort if resolution returns nil.
		if t := currentPkg.Lookup(typeNameStr); t != nil {
			return t, nil
		}
		return nil, fmt.Errorf("type %q not found after resolution", typeNameStr)
	}
	return resolvedType, nil
}

func processNewlyResolvedStruct(ctx context.Context, s *goscan.Scanner, info *model.ParsedInfo, currentPkg *scanner.PackageInfo, t *scanner.TypeInfo) error {
	if _, ok := info.Structs[t.Name]; ok {
		return nil // Already processed
	}

	modelStructInfo := &model.StructInfo{
		Name: t.Name,
		Type: t,
	}
	fields, err := collectFields(ctx, s, t, currentPkg, make(map[string]struct{}))
	if err != nil {
		return fmt.Errorf("collecting fields for newly resolved struct %s: %w", t.Name, err)
	}
	modelStructInfo.Fields = fields
	for i := range modelStructInfo.Fields {
		modelStructInfo.Fields[i].ParentStruct = modelStructInfo
	}
	info.Structs[t.Name] = modelStructInfo
	info.NamedTypes[t.Name] = t

	// Now, check if this newly found struct also needs a conversion.
	if t.Doc == "" {
		return nil
	}
	for _, line := range strings.Split(t.Doc, "\n") {
		m := reDerivingConvert.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		dstTypeName := strings.Trim(m[1], `"`)
		dstType, err := resolveType(ctx, s, info, currentPkg, dstTypeName)
		if err != nil {
			return fmt.Errorf("could not resolve destination type %q for source %q: %w", dstTypeName, t.Name, err)
		}

		// Recursively process the new destination type if it's a struct we haven't seen.
		if _, ok := info.Structs[dstType.Name]; !ok && dstType.Kind == scanner.StructKind {
			if err := processNewlyResolvedStruct(ctx, s, info, currentPkg, dstType); err != nil {
				return err
			}
		}

		pair := model.ConversionPair{
			SrcTypeName: t.Name,
			DstTypeName: dstType.Name,
			SrcTypeInfo: t,
			DstTypeInfo: dstType,
		}
		info.ConversionPairs = append(info.ConversionPairs, pair)
	}

	return nil
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
		return nil, nil // cycle detected
	}
	visited[t.Name] = struct{}{}

	var fields []model.FieldInfo
	if t.Struct == nil {
		return nil, fmt.Errorf("type %s is not a struct", t.Name)
	}

	for _, f := range t.Struct.Fields {
		// Resolve the field's type to get its full definition.
		// This will trigger an on-demand scan if the type is in an external package.
		fieldTypeInfo, err := s.ResolveType(ctx, f.Type)
		if err != nil {
			log.Printf("Could not resolve type for field %s.%s (%s), skipping: %v", t.Name, f.Name, f.Type.Name, err)
			// Create a placeholder TypeInfo to allow processing to continue
			fieldTypeInfo = &scanner.TypeInfo{Name: f.Type.Name, PkgPath: f.Type.FullImportPath}
		}

		if f.Embedded {
			if fieldTypeInfo == nil || fieldTypeInfo.Struct == nil {
				log.Printf("Could not resolve embedded struct type %s, skipping", f.Type.Name)
				continue
			}
			// Recursively collect fields from the embedded struct.
			embeddedFields, err := collectFields(ctx, s, fieldTypeInfo, p, visited)
			if err != nil {
				return nil, fmt.Errorf("collecting fields from embedded struct %s: %w", f.Type.Name, err)
			}
			fields = append(fields, embeddedFields...)
		} else {
			// This is a regular field.
			structTag := reflect.StructTag(f.Tag)
			tag, err := parseConvertTag(structTag)
			if err != nil {
				return nil, fmt.Errorf("parsing tag for %s.%s: %w", t.Name, f.Name, err)
			}
			if fieldTypeInfo == nil {
				// if resolution failed, create a placeholder
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
