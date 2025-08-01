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

	// Initial population of types from the main scanned package
	for _, t := range scannedPkg.Types {
		info.NamedTypes[t.Name] = t
	}

	// Worklist for types to process for annotations. Start with types from the initial package.
	var worklist []*scanner.TypeInfo
	worklist = append(worklist, scannedPkg.Types...)

	processed := make(map[string]bool) // Key: PkgPath.TypeName

	for len(worklist) > 0 {
		t := worklist[0]
		worklist = worklist[1:]

		typeIdentifier := fmt.Sprintf("%s.%s", t.PkgPath, t.Name)
		if processed[typeIdentifier] {
			continue
		}
		processed[typeIdentifier] = true

		// Ensure the type is processed as a struct if it is one
		if _, err := processStruct(ctx, info, t); err != nil {
			return nil, fmt.Errorf("processing initial struct %q: %w", t.Name, err)
		}

		if t.Doc == "" {
			continue
		}

		for _, line := range strings.Split(t.Doc, "\n") {
			m := reDerivingConvert.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			dstTypeStr := strings.Trim(m[1], `"`)
			optionsStr := ""
			if len(m) > 2 {
				optionsStr = m[2]
			}

			// Resolve the destination type using the on-demand scanner
			dstTypeInfo, err := resolveTypeByName(ctx, s, info, scannedPkg, t.PkgPath, dstTypeStr)
			if err != nil {
				return nil, fmt.Errorf("resolving destination type %q for source %q: %w", dstTypeStr, t.Name, err)
			}
			if dstTypeInfo == nil {
				return nil, fmt.Errorf("destination type %q for source %q not found", dstTypeStr, t.Name)
			}

			// Process the newly resolved type if it's a struct and we haven't seen it before
			wasNewlyProcessed, err := processStruct(ctx, info, dstTypeInfo)
			if err != nil {
				return nil, fmt.Errorf("processing resolved destination struct %q: %w", dstTypeInfo.Name, err)
			}
			if wasNewlyProcessed {
				worklist = append(worklist, dstTypeInfo)
			}

			pair := model.ConversionPair{
				SrcTypeName: t.Name,
				DstTypeName: dstTypeInfo.Name,
				SrcTypeInfo: t,
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

	// Parse global rules from comments, as before
	for _, astFile := range scannedPkg.AstFiles {
		for _, commentGroup := range astFile.Comments {
			for _, comment := range commentGroup.List {
				// Import rule
				if m := reConvertImport.FindStringSubmatch(comment.Text); m != nil {
					alias, path := m[1], m[2]
					if _, ok := info.Imports[alias]; ok {
						return nil, fmt.Errorf("duplicate import alias %q", alias)
					}
					info.Imports[alias] = path
					continue
				}

				// Conversion/validation rule
				if m := reConvertRule.FindStringSubmatch(comment.Text); m != nil {
					rule := model.TypeRule{}
					type1Name, type2Name, usingFunc, validatorFunc := m[1], m[2], m[3], m[4]

					if validatorFunc != "" {
						rule.ValidatorFunc = validatorFunc
						rule.DstTypeName = type1Name
						dstTypeInfo, err := resolveTypeByName(ctx, s, info, scannedPkg, scannedPkg.ImportPath, rule.DstTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving validator rule destination type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					} else if usingFunc != "" {
						rule.UsingFunc = usingFunc
						rule.SrcTypeName = type1Name
						rule.DstTypeName = type2Name
						srcTypeInfo, err := resolveTypeByName(ctx, s, info, scannedPkg, scannedPkg.ImportPath, rule.SrcTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving global rule source type %q: %w", rule.SrcTypeName, err)
						}
						rule.SrcTypeInfo = srcTypeInfo
						dstTypeInfo, err := resolveTypeByName(ctx, s, info, scannedPkg, scannedPkg.ImportPath, rule.DstTypeName)
						if err != nil {
							return nil, fmt.Errorf("resolving global rule destination type %q: %w", rule.DstTypeName, err)
						}
						rule.DstTypeInfo = dstTypeInfo
					}
					info.GlobalRules = append(info.GlobalRules, rule)
				}
			}
		}
	}

	return info, nil
}

func processStruct(ctx context.Context, info *model.ParsedInfo, t *scanner.TypeInfo) (bool, error) {
	if t.Kind != scanner.StructKind {
		return false, nil
	}
	if _, exists := info.Structs[t.Name]; exists {
		return false, nil
	}

	log.Printf("Processing newly discovered struct: %s.%s", t.PkgPath, t.Name)
	modelStructInfo := &model.StructInfo{
		Name: t.Name,
		Type: t,
	}

	fields, err := collectFields(ctx, t, make(map[string]struct{}))
	if err != nil {
		return false, fmt.Errorf("collecting fields for struct %s: %w", t.Name, err)
	}
	modelStructInfo.Fields = fields
	for i := range modelStructInfo.Fields {
		modelStructInfo.Fields[i].ParentStruct = modelStructInfo
	}
	info.Structs[t.Name] = modelStructInfo
	info.NamedTypes[t.Name] = t
	return true, nil
}

func resolveTypeByName(ctx context.Context, s *goscan.Scanner, info *model.ParsedInfo, initialPkg *scanner.PackageInfo, currentPkgPath, typeName string) (*scanner.TypeInfo, error) {
	// Handle built-in types
	switch typeName {
	case "string", "int", "bool": // Add other primitives as needed
		return &scanner.TypeInfo{Name: typeName, Underlying: &scanner.FieldType{Name: typeName, IsBuiltin: true}}, nil
	}

	var pkgPath, name string
	lastDot := strings.LastIndex(typeName, ".")

	if lastDot == -1 {
		// Local type, e.g., "MyType". It must be in the current package scope.
		name = typeName
		// Check already parsed types first.
		if t, ok := info.NamedTypes[name]; ok {
			return t, nil
		}
		// Check the initial package info.
		if t := initialPkg.Lookup(name); t != nil {
			return t, nil
		}
		return nil, fmt.Errorf("local type %q not found in package %q", name, currentPkgPath)
	}

	// Potentially qualified type, e.g., "pkg.Type" or "path/to/pkg.Type"
	pkgStr := typeName[:lastDot]
	name = typeName[lastDot+1:]

	// Now resolve pkgStr to a full import path.
	pkgPath, found := info.Imports[pkgStr] // Check //convert:import aliases
	if !found {
		// Fallback to searching file imports of the initial package
		for _, f := range initialPkg.AstFiles {
			for _, i := range f.Imports {
				path := strings.Trim(i.Path.Value, `"`)
				if i.Name != nil && i.Name.Name == pkgStr { // "alias"
					pkgPath = path
					found = true
					break
				}
				if i.Name == nil && (strings.HasSuffix(path, "/"+pkgStr) || path == pkgStr) { // "pkg" or "path/pkg"
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
		// If it's not an alias, it might be a full import path already.
		// Or a built-in package like "time".
		if strings.Contains(pkgStr, "/") || pkgStr == "time" {
			pkgPath = pkgStr
			found = true
		}
	}

	if !found {
		return nil, fmt.Errorf("could not resolve package path for alias/path %q in type %q", pkgStr, typeName)
	}

	resolvableType := &scanner.FieldType{
		Resolver:       s,
		FullImportPath: pkgPath,
		TypeName:       name,
		Name:           name,
	}

	resolvedTypeInfo, err := resolvableType.Resolve(ctx, make(map[string]struct{}))
	if err != nil {
		return nil, fmt.Errorf("resolution failed for type %q in package %q: %w", name, pkgPath, err)
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

func collectFields(ctx context.Context, t *scanner.TypeInfo, visited map[string]struct{}) ([]model.FieldInfo, error) {
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
			if _, err := f.Type.Resolve(ctx, make(map[string]struct{})); err != nil {
				return nil, fmt.Errorf("resolving embedded field type %q: %w", f.Type.Name, err)
			}
			embeddedTypeInfo := f.Type.Definition
			if embeddedTypeInfo == nil {
				log.Printf("Could not resolve embedded struct type %s, skipping", f.Type.Name)
				continue
			}

			embeddedFields, err := collectFields(ctx, embeddedTypeInfo, visited)
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
				TypeInfo:     f.Type.Definition,
				FieldType:    f.Type,
				Tag:          tag,
			}
			fields = append(fields, fieldInfo)
		}
	}
	return fields, nil
}
