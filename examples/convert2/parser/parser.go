package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"example.com/convert2/internal/model"
)

// ParseDirectory scans the given directory for Go files, parses them,
// and extracts conversion rules and type information.
func ParseDirectory(dirPath string) (*model.ParsedInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dirPath, func(fi os.FileInfo) bool {
		return !fi.IsDir() && strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go") && !strings.HasSuffix(fi.Name(), "_gen.go")
	}, parser.ParseComments)

	if err != nil {
		return nil, fmt.Errorf("failed to parse directory %s: %w", dirPath, err)
	}

	// Assuming a single package in the directory for simplicity.
	for pkgName, pkg := range pkgs {
		// Determine package import path (heuristic, might need improvement for complex module structures)
		// For now, assume dirPath is within GOPATH/src or is a module path.
		// A more robust way would be to find go.mod and derive from module path.
		// For this example, let's assume the last part of dirPath can be appended to a base module path
		// or use a placeholder. If dirPath is 'example.com/mymodule/pkg1', then pkgPath is that.
		// This is a simplification.
		packagePath := derivePackagePath(dirPath) // Placeholder for actual package path resolution
		if packagePath == "" {
			// Fallback or error. For now, use pkgName, but this is not an import path.
			fmt.Printf("Warning: Could not reliably determine import path for package %s in dir %s. Using package name as placeholder.\n", pkgName, dirPath)
			packagePath = pkgName
		}


		parsedInfo := model.NewParsedInfo(pkgName, packagePath)

		for filePath, file := range pkg.Files {
			// Collect file imports
			fileImports := collectFileImports(file)
			parsedInfo.FileImports[filePath] = fileImports

			// Parse package-level annotations
			if file.Doc != nil {
				for _, comment := range file.Doc.List {
					parsePackageComment(comment.Text, parsedInfo, fileImports, pkgName, packagePath)
				}
			}

			ast.Inspect(file, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.GenDecl:
					if decl.Tok == token.TYPE {
						for _, spec := range decl.Specs {
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
								parseTypeSpec(typeSpec, parsedInfo, fileImports, pkgName, packagePath)
							}
						}
					}
				}
				return true
			})
		}
		return parsedInfo, nil
	}

	return nil, fmt.Errorf("no Go packages found in directory %s", dirPath)
}

// derivePackagePath tries to derive an import path from a directory path.
// This is a simplified heuristic and might not work for all project layouts.
// A robust solution would involve parsing go.mod.
func derivePackagePath(dirPath string) string {
    // Try to find go.mod upwards to get module path
    currentPath, err := filepath.Abs(dirPath)
    if err != nil {
        return ""
    }

    for {
        goModPath := filepath.Join(currentPath, "go.mod")
        if _, err := os.Stat(goModPath); err == nil {
            // Found go.mod, read module line
            content, err := os.ReadFile(goModPath)
            if err != nil {
                return "" // Failed to read go.mod
            }
            lines := strings.Split(string(content), "\n")
            for _, line := range lines {
                if strings.HasPrefix(line, "module ") {
                    modulePath := strings.TrimSpace(strings.TrimPrefix(line, "module "))
                    // Relative path from module root to dirPath
                    relativePath, err := filepath.Rel(currentPath, dirPath)
                    if err != nil {
                        return modulePath // Cannot get relative, return module path itself
                    }
                    return filepath.ToSlash(filepath.Join(modulePath, relativePath))
                }
            }
            return "" // go.mod found but no module line?
        }
        parent := filepath.Dir(currentPath)
        if parent == currentPath {
            break // Reached root
        }
        currentPath = parent
    }
    return "" // Could not find go.mod
}


// collectFileImports gathers all import declarations from a file.
// Returns a map of import alias/name to full import path.
// e.g., {"fmt": "fmt", "custom_alias": "example.com/custom/pkg", "pkg": "example.com/other/pkg"}
func collectFileImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, importSpec := range file.Imports {
		path := strings.Trim(importSpec.Path.Value, `"`)
		name := ""
		if importSpec.Name != nil {
			name = importSpec.Name.Name
		} else {
			// If no explicit alias, name is the last part of the import path
			parts := strings.Split(path, "/")
			name = parts[len(parts)-1]
		}
		if name == "." { // Dot import
			// Handling dot imports is complex for type resolution without go/types.
			// For now, we can record it, but resolving types from dot imports is tricky.
			// We might need to treat types from dot imports as if they are in the current package.
			// Or, more accurately, the generator would need to know not to prefix them.
			// For now, let's use a special marker or just the path.
			imports["."] = path // Or perhaps a more unique key if multiple dot imports were allowed/meaningful here
		} else {
			imports[name] = path
		}
	}
	return imports
}

// resolveTypeExpr converts an ast.Expr representing a type into a model.TypeInfo.
func resolveTypeExpr(expr ast.Expr, currentPkgName, currentPkgPath string, fileImports map[string]string, parsedInfo *model.ParsedInfo) *model.TypeInfo {
	ti := &model.TypeInfo{AstExpr: expr, PackageName: currentPkgName, PackagePath: currentPkgPath, FullName: ""}

	switch e := expr.(type) {
	case *ast.Ident:
		ti.Name = e.Name
		ti.Kind = model.KindIdent
		// Is it a basic type?
		if isGoBasicType(e.Name) {
			ti.IsBasic = true
			ti.Kind = model.KindBasic
			ti.FullName = e.Name
		} else {
			// Could be a type defined in the current package (struct, named type)
			// or a type from a dot import.
			// If it's a known named type/struct in current package:
			if knownStruct, ok := parsedInfo.Structs[e.Name]; ok {
				ti.FullName = currentPkgPath + "." + e.Name
				ti.StructInfo = knownStruct // Link to struct info if available
				ti.Kind = model.KindStruct
			} else if knownNamed, ok := parsedInfo.NamedTypes[e.Name]; ok {
				ti.FullName = currentPkgPath + "." + e.Name
				ti.Underlying = knownNamed.Underlying // Link to underlying type
				ti.Kind = model.KindNamed
			} else {
				// Assume it's in the current package if not found in imports via selector
				// This might be a forward declaration or a type from a dot import.
				ti.FullName = currentPkgPath + "." + e.Name
				// If there's a dot import, this type *could* be from there.
				// This part is tricky without full symbol resolution.
				if dotImportPath, ok := fileImports["."]; ok {
					// Heuristic: assume type is from the dot-imported package.
					// This is not perfectly accurate as multiple dot imports could exist,
					// or the type might genuinely be from the current package.
					// For now, we'll prefer current package path, but acknowledge dot import.
					// A better system would check all dot-imported packages if `go/types` were used.
					fmt.Printf("Info: Type '%s' might be from dot import '%s' or current package '%s'\n", e.Name, dotImportPath, currentPkgPath)
				}
			}
		}
	case *ast.SelectorExpr: // pkg.Type
		ti.Kind = model.KindIdent // Can be struct, named type, etc.
		if pkgIdent, ok := e.X.(*ast.Ident); ok {
			ti.PackageName = pkgIdent.Name // This is the alias/package name used in THIS file
			ti.Name = e.Sel.Name
			if importPath, ok := fileImports[pkgIdent.Name]; ok {
				ti.PackagePath = importPath
				ti.FullName = importPath + "." + e.Sel.Name
			} else {
				// Package alias not found in imports, this is an issue or an unexpected AST structure.
				ti.FullName = pkgIdent.Name + "." + e.Sel.Name // Fallback
				fmt.Printf("Warning: Package alias '%s' for type '%s' not found in file imports. Using as raw selector.\n", pkgIdent.Name, ti.FullName)
			}
		} else {
			// Complex selector, e.g. (pkg.SubStruct).FieldType - not typical for type names
			ti.Name = model.AstExprToString(e, currentPkgName) // Fallback
			ti.FullName = ti.Name
			fmt.Printf("Warning: Unexpected selector expression for type: %s\n", ti.Name)
		}
	case *ast.StarExpr: // *Type
		ti.IsPointer = true
		ti.Kind = model.KindPointer
		ti.Elem = resolveTypeExpr(e.X, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if ti.Elem != nil {
			ti.FullName = "*" + ti.Elem.FullName
			ti.Name = "*" + ti.Elem.Name
		}
	case *ast.ArrayType: // []Type or [N]Type
		if e.Len == nil {
			ti.IsSlice = true
			ti.Kind = model.KindSlice
		} else {
			ti.IsArray = true
			ti.Kind = model.KindArray
			// TODO: store array length if needed: model.AstExprToString(e.Len, currentPkgName)
		}
		ti.Elem = resolveTypeExpr(e.Elt, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if ti.Elem != nil {
			ti.FullName = "[]" + ti.Elem.FullName // Simplified for now, doesn't show array length
			if ti.IsArray {
				ti.FullName = fmt.Sprintf("[%s]%s", model.AstExprToString(e.Len,currentPkgName) ,ti.Elem.FullName)
			}
			ti.Name = "[]" + ti.Elem.Name // Simplified
		}
	case *ast.MapType: // map[KeyType]ValueType
		ti.IsMap = true
		ti.Kind = model.KindMap
		ti.Key = resolveTypeExpr(e.Key, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		ti.Value = resolveTypeExpr(e.Value, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if ti.Key != nil && ti.Value != nil {
			ti.FullName = fmt.Sprintf("map[%s]%s", ti.Key.FullName, ti.Value.FullName)
			ti.Name = fmt.Sprintf("map[%s]%s", ti.Key.Name, ti.Value.Name)
		}
	case *ast.InterfaceType:
		ti.IsInterface = true
		ti.Kind = model.KindInterface
		ti.Name = "interface{}"
		if e.Methods != nil && len(e.Methods.List) > 0 {
			ti.Name = "interface{...}" // Simplified
		}
		ti.FullName = ti.Name // Interfaces are often anonymous or from stdlib like `error`
	case *ast.FuncType:
		ti.IsFunc = true
		ti.Kind = model.KindFunc
		ti.Name = "func(...)" // Simplified
		ti.FullName = ti.Name
	default:
		ti.Kind = model.KindUnknown
		rawTypeName := model.AstExprToString(expr, currentPkgName)
		ti.Name = rawTypeName
		ti.FullName = rawTypeName // Fallback
		fmt.Printf("Warning: Unknown AST expression type for TypeInfo: %T, raw: %s\n", expr, rawTypeName)
	}
	return ti
}

func isGoBasicType(name string) bool {
	switch name {
	case "bool", "byte", "complex128", "complex64", "error", "float32", "float64",
		"int", "int16", "int32", "int64", "int8", "rune", "string", "uint", "uint16",
		"uint32", "uint64", "uint8", "uintptr":
		return true
	}
	return false
}

// parsePackageComment parses a single package-level comment line.
func parsePackageComment(commentText string, info *model.ParsedInfo, fileImports map[string]string, currentPkgName, currentPkgPath string) {
	commentText = strings.TrimSpace(strings.TrimPrefix(commentText, "//"))
	parts := strings.Fields(commentText)

	if len(parts) < 2 || parts[0] != "convert:pair" && parts[0] != "convert:rule" {
		return
	}
	directive := parts[0]
	args := parts[1:]

	if directive == "convert:pair" {
		if len(args) < 3 || args[1] != "->" {
			fmt.Printf("Skipping malformed convert:pair: %s\n", commentText)
			return
		}
		srcTypeNameStr := args[0]
		dstTypeNameStr := args[2]
		maxErrors := 0

		optionsStr := ""
		if len(args) > 3 {
			optionsStr = strings.Join(args[3:], " ")
			if strings.HasPrefix(optionsStr, ",") {
				optionsStr = strings.TrimSpace(optionsStr[1:])
			}
		}
		parsedOptions := parseOptions(optionsStr)
		if val, ok := parsedOptions["max_errors"]; ok {
			fmt.Sscanf(val, "%d", &maxErrors)
		}

		// For resolving types in annotations, we need an AST expression.
		// This is tricky as comments don't directly give AST nodes for the types mentioned.
		// We'd have to parse these type strings into ast.Expr, which is non-trivial.
		// A simplification: Assume type names are resolvable as identifiers or selector exprs
		// by a mini-parser or by searching available types.
		// For now, store names and resolve later or assume simple names.
		// Let's try to parse them as if they were code snippets.
		srcTypeInfo := resolveTypeFromString(srcTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)
		dstTypeInfo := resolveTypeFromString(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)


		pair := model.ConversionPair{
			SrcTypeName: srcTypeNameStr,
			DstTypeName: dstTypeNameStr,
			SrcTypeInfo: srcTypeInfo,
			DstTypeInfo: dstTypeInfo,
			MaxErrors:   maxErrors,
		}
		info.ConversionPairs = append(info.ConversionPairs, pair)

	} else if directive == "convert:rule" {
		ruleText := strings.Join(args, " ")
		usingParts := strings.Split(ruleText, "using=")
		validatorParts := strings.Split(ruleText, "validator=")

		if len(usingParts) == 2 { // Conversion rule
			typePartsStr := strings.TrimSpace(strings.TrimSuffix(usingParts[0], ","))
			typeParts := strings.Fields(typePartsStr)
			if len(typeParts) == 3 && typeParts[1] == "->" {
				srcTypeNameStr := strings.Trim(typeParts[0], "\"")
				dstTypeNameStr := strings.Trim(typeParts[2], "\"")
				usingFunc := strings.TrimSpace(usingParts[1])

				srcTypeInfo := resolveTypeFromString(srcTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)
				dstTypeInfo := resolveTypeFromString(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)

				rule := model.TypeRule{
					SrcTypeName: srcTypeNameStr,
					DstTypeName: dstTypeNameStr,
					SrcTypeInfo: srcTypeInfo,
					DstTypeInfo: dstTypeInfo,
					UsingFunc:   usingFunc,
				}
				info.GlobalRules = append(info.GlobalRules, rule)
				return
			}
		} else if len(validatorParts) == 2 { // Validator rule
			dstTypeStr := strings.TrimSpace(strings.TrimSuffix(validatorParts[0], ","))
			dstTypeNameStr := strings.Trim(dstTypeStr, "\"")
			validatorFunc := strings.TrimSpace(validatorParts[1])

			dstTypeInfo := resolveTypeFromString(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)

			rule := model.TypeRule{
				DstTypeName:   dstTypeNameStr,
				DstTypeInfo:   dstTypeInfo,
				ValidatorFunc: validatorFunc,
			}
			info.GlobalRules = append(info.GlobalRules, rule)
			return
		}
		fmt.Printf("Skipping malformed convert:rule: %s\n", commentText)
	}
}

// resolveTypeFromString parses a type string (e.g., "MyType", "pkg.Type", "*pkg.Type")
// and resolves it to TypeInfo. This is a helper for parsing types from annotations.
func resolveTypeFromString(typeStr, currentPkgName, currentPkgPath string, fileImports map[string]string, parsedInfo *model.ParsedInfo) *model.TypeInfo {
    // This is a simplified parser for type strings.
    // A proper solution would use go/parser.ParseExpr to get an ast.Expr,
    // then call resolveTypeExpr.
    expr, err := parser.ParseExpr(typeStr)
    if err != nil {
        fmt.Printf("Warning: Could not parse type string '%s' from annotation: %v. Treating as simple identifier.\n", typeStr, err)
        // Fallback: treat as a simple identifier in the current package
        ti := &model.TypeInfo{
            Name: typeStr,
            FullName: currentPkgPath + "." + typeStr, // Assumption
            PackageName: currentPkgName,
            PackagePath: currentPkgPath,
            Kind: model.KindIdent, // Or KindUnknown
        }
        // Check if it's a known struct or named type in the current package as a fallback
		if stInfo, ok := parsedInfo.Structs[typeStr]; ok && stInfo.Type != nil {
			return stInfo.Type
		}
		if ntInfo, ok := parsedInfo.NamedTypes[typeStr]; ok {
			return ntInfo
		}
        return ti
    }
    return resolveTypeExpr(expr, currentPkgName, currentPkgPath, fileImports, parsedInfo)
}


func parseTypeSpec(typeSpec *ast.TypeSpec, info *model.ParsedInfo, fileImports map[string]string, currentPkgName, currentPkgPath string) {
	typeName := typeSpec.Name.Name

	// Pre-create a TypeInfo for this type definition itself
	// This helps if other types (fields, other definitions) refer to it.
	selfTypeInfo := &model.TypeInfo{
		Name:        typeName,
		FullName:    currentPkgPath + "." + typeName,
		PackageName: currentPkgName,
		PackagePath: currentPkgPath,
		AstExpr:     typeSpec.Name, // Or typeSpec.Type for more detail?
	}


	switch underlyingAstType := typeSpec.Type.(type) {
	case *ast.StructType:
		selfTypeInfo.Kind = model.KindStruct
		sInfo := &model.StructInfo{
			Name: typeName,
			Type: selfTypeInfo, // Link to its own TypeInfo
			Node: underlyingAstType,
		}
		info.Structs[typeName] = sInfo
		selfTypeInfo.StructInfo = sInfo // Circular link for easy access

		for _, field := range underlyingAstType.Fields.List {
			if len(field.Names) > 0 {
				for _, fieldNameIdent := range field.Names {
					fieldName := fieldNameIdent.Name
					fieldTypeInfo := resolveTypeExpr(field.Type, currentPkgName, currentPkgPath, fileImports, info)

					var convertTag model.ConvertTag
					if field.Tag != nil {
						tagValue := strings.Trim(field.Tag.Value, "`")
						if strings.HasPrefix(tagValue, "convert:\"") {
							rawTag := strings.TrimSuffix(strings.TrimPrefix(tagValue, "convert:\""), "\"")
							convertTag = parseFieldTag(rawTag)
						}
					}
					fInfo := model.FieldInfo{
						Name:         fieldName,
						OriginalName: fieldName,
						TypeInfo:     fieldTypeInfo,
						Tag:          convertTag,
						ParentStruct: sInfo,
						AstField:     field,
					}
					sInfo.Fields = append(sInfo.Fields, fInfo)
				}
			} else { // Embedded field
				fieldTypeInfo := resolveTypeExpr(field.Type, currentPkgName, currentPkgPath, fileImports, info)
				embeddedName := fieldTypeInfo.Name // Use resolved name from TypeInfo
				if fieldTypeInfo.PackageName != "" && fieldTypeInfo.PackageName != currentPkgName {
					// If it's like `pkg.Type`, the name in FieldInfo should probably be `Type`
					// and TypeInfo would carry `pkg.Type`.
					// For now, embeddedName will be `pkg.Type` if selector, or `Type` if ident.
					// This needs careful handling for how embedded fields are accessed in Go.
				}

				var convertTag model.ConvertTag // Tags on embedded fields are less common for `convert`
				if field.Tag != nil {
					tagValue := strings.Trim(field.Tag.Value, "`")
					if strings.HasPrefix(tagValue, "convert:\"") {
						rawTag := strings.TrimSuffix(strings.TrimPrefix(tagValue, "convert:\""), "\"")
						convertTag = parseFieldTag(rawTag)
					}
				}
				fInfo := model.FieldInfo{
					Name:         embeddedName, // This might need to be just the type name part for true embedding
					OriginalName: embeddedName, // e.g. if `foo.Bar` is embedded, OriginalName could be `Bar`
					TypeInfo:     fieldTypeInfo,
					Tag:          convertTag,
					ParentStruct: sInfo,
					AstField:     field,
				}
				sInfo.Fields = append(sInfo.Fields, fInfo)
			}
		}

	case *ast.Ident, *ast.SelectorExpr, *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.InterfaceType:
		// This is a `type NewType ExistingType` or `type MySlice []int` definition
		selfTypeInfo.Kind = model.KindNamed
		underlyingTypeInfo := resolveTypeExpr(underlyingAstType, currentPkgName, currentPkgPath, fileImports, info)
		selfTypeInfo.Underlying = underlyingTypeInfo
		info.NamedTypes[typeName] = selfTypeInfo

		// If the underlying type is a struct identifier (e.g. type T1 T2 where T2 is a struct)
		// we might want to treat T1 as an alias of that struct.
		if underlyingTypeInfo.Kind == model.KindIdent || underlyingTypeInfo.Kind == model.KindStruct { // KindIdent if T2 is defined elsewhere
			if baseStructInfo, isStruct := info.Structs[underlyingTypeInfo.Name]; isStruct && underlyingTypeInfo.PackagePath == currentPkgPath {
				// This is like `type MyStructAlias OtherLocalStruct`
				sInfo := &model.StructInfo{
					Name:            typeName,
					Type:            selfTypeInfo,
					IsAlias:         true,
					UnderlyingAlias: baseStructInfo.Type, // Point to OtherLocalStruct's TypeInfo
					Node:            baseStructInfo.Node, // Copy node for field access? Or handle via UnderlyingAlias.
					Fields:          baseStructInfo.Fields, // Share fields for alias
				}
				info.Structs[typeName] = sInfo // Register MyStructAlias as a struct too
				selfTypeInfo.StructInfo = sInfo // Link its TypeInfo to this new StructInfo
			} else if underlyingTypeInfo.StructInfo != nil { // Underlying is already a resolved struct
				sInfo := &model.StructInfo{
					Name:            typeName,
					Type:            selfTypeInfo,
					IsAlias:         true,
					UnderlyingAlias: underlyingTypeInfo,
					Node:            underlyingTypeInfo.StructInfo.Node,
					Fields:          underlyingTypeInfo.StructInfo.Fields,
				}
				info.Structs[typeName] = sInfo
				selfTypeInfo.StructInfo = sInfo
			}
		}


	default:
		fmt.Printf("Warning: Unhandled TypeSpec kind for '%s': %T\n", typeName, typeSpec.Type)
		selfTypeInfo.Kind = model.KindUnknown
		info.NamedTypes[typeName] = selfTypeInfo // Store it anyway
	}
}


// parseFieldTag parses the content of a `convert` struct tag.
func parseFieldTag(tagContent string) model.ConvertTag {
	ct := model.ConvertTag{RawValue: tagContent}
	if tagContent == "-" {
		ct.DstFieldName = "-"
		return ct
	}
	parts := strings.Split(tagContent, ",")
	if len(parts) > 0 && !strings.Contains(parts[0], "=") && parts[0] != "" {
		ct.DstFieldName = strings.TrimSpace(parts[0])
		parts = parts[1:]
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "required" {
			ct.Required = true
		} else if strings.HasPrefix(part, "using=") {
			ct.UsingFunc = strings.TrimPrefix(part, "using=")
		}
	}
	return ct
}

// parseOptions parses a comma-separated key=value string.
func parseOptions(optionsStr string) map[string]string {
	options := make(map[string]string)
	if optionsStr == "" {
		return options
	}
	pairs := strings.Split(optionsStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			options[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return options
}
