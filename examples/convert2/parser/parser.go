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

// typePreParseInfo stores basic info about types collected in the first pass.
type typePreParseInfo struct {
	name     string
	typeSpec *ast.TypeSpec
	file     *ast.File // File where the type is defined
	pkgName  string    // Package name from the file
	pkgPath  string    // Package path for this type
}

// ParseDirectory scans the given directory for Go files, parses them,
// and extracts conversion rules and type information using a 2-pass approach.
func ParseDirectory(dirPath string) (*model.ParsedInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dirPath, func(fi os.FileInfo) bool {
		return !fi.IsDir() && strings.HasSuffix(fi.Name(), ".go") &&
			!strings.HasSuffix(fi.Name(), "_test.go") &&
			!strings.HasSuffix(fi.Name(), "_gen.go")
	}, parser.ParseComments)

	if err != nil {
		return nil, fmt.Errorf("failed to parse directory %s: %w", dirPath, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go packages found in directory %s", dirPath)
	}
	if len(pkgs) > 1 {
		// For simplicity, this parser currently handles one package per directory.
		// Multiple packages in the same directory (e.g., using build tags for different package names)
		// is an advanced scenario not covered.
		var pkgNames []string
		for name := range pkgs {
			pkgNames = append(pkgNames, name)
		}
		return nil, fmt.Errorf("multiple packages (%s) found in directory %s; this parser handles one package per directory", strings.Join(pkgNames, ", "), dirPath)
	}

	var parsedInfo *model.ParsedInfo
	var currentPkgName string
	var currentPkgPath string
	typeSpecsToProcess := make(map[string]typePreParseInfo) // map[typeName]typePreParseInfo

	// PASS 1: Collect all type declarations and file-level info (imports, package name/path)
	for pkgName, pkgAst := range pkgs {
		currentPkgName = pkgName
		currentPkgPath = derivePackagePath(dirPath)
		if currentPkgPath == "" {
			fmt.Printf("Warning: Could not reliably determine import path for package %s in dir %s. Using package name as placeholder, which might lead to incorrect type resolution for external references.\n", pkgName, dirPath)
			currentPkgPath = pkgName // Fallback, less reliable
		}
		parsedInfo = model.NewParsedInfo(currentPkgName, currentPkgPath)

		for filePath, fileAst := range pkgAst.Files {
			fileImports := collectFileImports(fileAst)
			parsedInfo.FileImports[filePath] = fileImports

			ast.Inspect(fileAst, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.GenDecl:
					if decl.Tok == token.TYPE {
						for _, spec := range decl.Specs {
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
								typeName := typeSpec.Name.Name
								if _, exists := typeSpecsToProcess[typeName]; exists {
									fmt.Printf("Warning: Type '%s' redefined or encountered multiple times. Using the first definition.\n", typeName)
								} else {
									typeSpecsToProcess[typeName] = typePreParseInfo{
										name:     typeName,
										typeSpec: typeSpec,
										file:     fileAst,
										pkgName:  currentPkgName, // Should be same as currentPkgName for this pass
										pkgPath:  currentPkgPath, // Should be same as currentPkgPath
									}
								}
							}
						}
					}
				}
				return true
			})
		}
	}

	if parsedInfo == nil {
		return nil, fmt.Errorf("failed to initialize parsed info, likely no packages processed in pass 1")
	}

	// PASS 2: Resolve types and parse directives
	// First, parse all type specs to populate Structs and NamedTypes in parsedInfo
	for _, preInfo := range typeSpecsToProcess { // Changed typeName to _
		fileImports := parsedInfo.FileImports[fset.File(preInfo.file.Pos()).Name()]
		parseTypeSpec(preInfo.typeSpec, parsedInfo, fileImports, preInfo.pkgName, preInfo.pkgPath)
	}

	// Then, parse directives from comments (package and file level)
	for pkgName, pkgAst := range pkgs { // Should be only one pkg due to earlier check
		_ = pkgName // Use currentPkgName, currentPkgPath
		for filePath, fileAst := range pkgAst.Files {
			fileImports := parsedInfo.FileImports[filePath]
			// Parse package-level comments (// convert: ... at the top of the file, associated with package decl)
			if fileAst.Doc != nil {
				for _, comment := range fileAst.Doc.List {
					parseGlobalCommentDirective(comment.Text, parsedInfo, fileImports, currentPkgName, currentPkgPath)
				}
			}
			// Parse other top-level comments in the file (not associated with a specific AST node)
			for _, commentGroup := range fileAst.Comments {
				for _, comment := range commentGroup.List {
					parseGlobalCommentDirective(comment.Text, parsedInfo, fileImports, currentPkgName, currentPkgPath)
				}
			}
		}
	}
	return parsedInfo, nil
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
// This is a key function that needs to handle all relevant type expressions.
// - currentPkgName: The name of the package where this type expression is encountered.
// - currentPkgPath: The import path of the package.
// - fileImports: A map of import alias/name to full import path for the current file.
// - parsedInfo: The global parsed information, used to look up already defined structs/named types.
func resolveTypeExpr(expr ast.Expr, currentPkgName, currentPkgPath string, fileImports map[string]string, parsedInfo *model.ParsedInfo) *model.TypeInfo {
	ti := &model.TypeInfo{AstExpr: expr}

	switch e := expr.(type) {
	case *ast.Ident:
		ti.Name = e.Name
		if isGoBasicType(e.Name) {
			ti.Kind = model.KindBasic
			ti.IsBasic = true
			ti.FullName = e.Name
			ti.PackageName = "" // Basic types don't have a package
			ti.PackagePath = ""
		} else {
			// Could be a type defined in the current package or from a dot import.
			// Check if it's a known named type or struct in the current package.
			// This lookup relies on parseTypeSpec having run for these types.
			if namedType, ok := parsedInfo.NamedTypes[e.Name]; ok && namedType.PackagePath == currentPkgPath {
				// It's a named type defined in the current package (e.g. type MyInt int)
				// We return a copy or a pointer to the existing TypeInfo to avoid cycles if it's complex.
				// For simplicity, let's return the direct namedType. The generator must handle recursion.
				return namedType
			} else if structInfo, ok := parsedInfo.Structs[e.Name]; ok && structInfo.Type.PackagePath == currentPkgPath {
				// It's a struct defined in the current package
				return structInfo.Type
			} else {
				// Not a basic type, not a known named type or struct in current package.
				// It could be:
				// 1. A type from a dot import (e.g. import . "other/pkg", then use "OtherType")
				// 2. A forward declaration within the current package (less common for this to be hit before its spec is parsed in pass 1)
				// 3. An unresolved type (error case)
				ti.Kind = model.KindIdent // Assume it's an identifier for now
				ti.PackageName = currentPkgName
				ti.PackagePath = currentPkgPath
				ti.FullName = currentPkgPath + "." + e.Name

				if dotImportPath, ok := fileImports["."]; ok {
					// If there's a dot import, the type *might* be from there.
					// This is hard to confirm without full type checking.
					// We could assume it's from the dot-imported package for FullName resolution.
					// However, multiple dot imports or conflicts with local types make this ambiguous.
					// For now, we'll primarily assume it's a local package type if not explicitly qualified.
					// The generator will need to handle this (e.g. by not prefixing if from dot import).
					// Let's keep its PackagePath as current, but acknowledge.
					fmt.Printf("Info: Type '%s' in package '%s' might be from a dot import ('%s') or a forward/local declaration.\n", e.Name, currentPkgPath, dotImportPath)
					// A more aggressive approach would be to check if `e.Name` exists in the dot-imported package.
					// But that requires parsing that package too, which is beyond current scope.
				}
			}
		}
	case *ast.SelectorExpr: // pkg.Type
		ti.Kind = model.KindIdent // Could resolve to struct, named, etc.
		if pkgIdent, ok := e.X.(*ast.Ident); ok {
			ti.PackageName = pkgIdent.Name // This is the alias/package name used in THIS file
			ti.Name = e.Sel.Name
			if importPath, ok := fileImports[pkgIdent.Name]; ok {
				ti.PackagePath = importPath
				ti.FullName = importPath + "." + e.Sel.Name
				// Now, check if this refers to a known struct/named type from an *imported* package
				// This requires `parsedInfo` to potentially hold info from other packages if we were parsing dependencies.
				// For now, we assume types from other packages are not further resolved into `StructInfo` etc.
				// unless they were part of the same parsing batch (not typical for dependencies).
			} else {
				ti.PackagePath = "" // Unknown package path
				ti.FullName = pkgIdent.Name + "." + e.Sel.Name // Fallback
				fmt.Printf("Warning: Package alias '%s' for type '%s' not found in file imports. FullName may be incorrect.\n", pkgIdent.Name, ti.FullName)
			}
		} else {
			// Complex selector, e.g. (pkg.SubStruct).FieldType - not typical for top-level type names
			// but could appear in field types.
			rawName := model.AstExprToString(e, currentPkgName)
			ti.Name = rawName
			ti.FullName = rawName // Best guess
			ti.PackageName = ""   // Hard to determine package context
			ti.PackagePath = ""
			fmt.Printf("Warning: Unexpected selector expression X type for type: %s (X is %T)\n", rawName, e.X)
		}
	case *ast.StarExpr: // *Type
		ti.IsPointer = true
		ti.Kind = model.KindPointer
		ti.Elem = resolveTypeExpr(e.X, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if ti.Elem != nil {
			ti.FullName = "*" + ti.Elem.FullName
			ti.Name = "*" + ti.Elem.Name // Simple name is also prefixed for clarity
			// PackageName and PackagePath are implicitly those of the Elem
			ti.PackageName = ti.Elem.PackageName
			ti.PackagePath = ti.Elem.PackagePath
		} else {
			ti.FullName = "*" + model.AstExprToString(e.X, currentPkgName)
			ti.Name = "*" + model.AstExprToString(e.X, currentPkgName)
		}
	case *ast.ArrayType: // []Type or [N]Type
		if e.Len == nil {
			ti.IsSlice = true
			ti.Kind = model.KindSlice
		} else {
			ti.IsArray = true
			ti.Kind = model.KindArray
			// Length is e.Len (ast.Expr)
		}
		ti.Elem = resolveTypeExpr(e.Elt, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if ti.Elem != nil {
			prefix := "[]"
			if ti.IsArray {
				lenStr := model.AstExprToString(e.Len, currentPkgName)
				prefix = "[" + lenStr + "]"
			}
			ti.FullName = prefix + ti.Elem.FullName
			ti.Name = prefix + ti.Elem.Name
			ti.PackageName = ti.Elem.PackageName // Inherit from element
			ti.PackagePath = ti.Elem.PackagePath
		} else {
			ti.FullName = model.AstExprToString(e, currentPkgName) // Fallback
			ti.Name = model.AstExprToString(e, currentPkgName)
		}
	case *ast.MapType: // map[KeyType]ValueType
		ti.IsMap = true
		ti.Kind = model.KindMap
		ti.Key = resolveTypeExpr(e.Key, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		ti.Value = resolveTypeExpr(e.Value, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if ti.Key != nil && ti.Value != nil {
			ti.FullName = fmt.Sprintf("map[%s]%s", ti.Key.FullName, ti.Value.FullName)
			ti.Name = fmt.Sprintf("map[%s]%s", ti.Key.Name, ti.Value.Name) // Simple name for map is its full structure
			// Maps don't have a single PackageName/Path; it's implicit from Key/Value types.
		} else {
			ti.FullName = model.AstExprToString(e, currentPkgName) // Fallback
			ti.Name = model.AstExprToString(e, currentPkgName)
		}
	case *ast.InterfaceType:
		ti.IsInterface = true
		ti.Kind = model.KindInterface
		if e.Methods == nil || len(e.Methods.List) == 0 {
			ti.Name = "interface{}"
		} else {
			ti.Name = "interface{...}" // Simplified
		}
		ti.FullName = ti.Name // Interfaces are often anonymous or built-in like `error`
		// Package context is typically not applicable or refers to "builtin"
	case *ast.FuncType:
		ti.IsFunc = true
		ti.Kind = model.KindFunc
		ti.Name = model.AstExprToString(e, currentPkgName) // Use AstExprToString for a representation
		ti.FullName = ti.Name
		// Package context not directly applicable for func type structure itself
	default:
		ti.Kind = model.KindUnknown
		rawTypeName := model.AstExprToString(expr, currentPkgName)
		ti.Name = rawTypeName
		ti.FullName = rawTypeName // Fallback
		fmt.Printf("Warning: Unknown AST expression type (%T) for TypeInfo processing: %s. Raw: %s\n", expr, rawTypeName, model.AstExprToString(expr, ""))
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

// parseGlobalCommentDirective parses a single package-level or file-level comment line.
func parseGlobalCommentDirective(commentText string, info *model.ParsedInfo, fileImports map[string]string, currentPkgName, currentPkgPath string) {
	trimmedComment := strings.TrimSpace(strings.TrimPrefix(commentText, "//"))
	trimmedComment = strings.TrimSpace(strings.TrimPrefix(trimmedComment, "convert:"))
	parts := strings.Fields(trimmedComment)

	if len(parts) == 0 {
		return
	}
	directive := parts[0] // "pair" or "rule"
	args := parts[1:]

	if directive == "pair" {
		if len(args) < 3 || args[1] != "->" {
			fmt.Printf("Skipping malformed convert:pair: %s (original: %s)\n", trimmedComment, commentText)
			return
		}
		srcTypeNameStr := args[0]
		dstTypeNameStr := args[2]
		maxErrors := 0 // Default

		optionsStr := ""
		if len(args) > 3 {
			// Join the rest, then split by comma for key=value
			optionsStr = strings.Join(args[3:], " ")
		}
		parsedOptions := parseOptions(optionsStr)
		if val, ok := parsedOptions["max_errors"]; ok {
			if _, err := fmt.Sscanf(val, "%d", &maxErrors); err != nil {
				fmt.Printf("Warning: Could not parse max_errors value '%s' for pair %s -> %s. Error: %v\n", val, srcTypeNameStr, dstTypeNameStr, err)
			}
		}

		srcTypeInfo := resolveTypeFromString(srcTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)
		dstTypeInfo := resolveTypeFromString(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)

		if srcTypeInfo == nil || dstTypeInfo == nil {
			fmt.Printf("Warning: Could not resolve types for convert:pair %s -> %s. Skipping.\n", srcTypeNameStr, dstTypeNameStr)
			return
		}

		pair := model.ConversionPair{
			SrcTypeName: srcTypeNameStr,
			DstTypeName: dstTypeNameStr,
			SrcTypeInfo: srcTypeInfo,
			DstTypeInfo: dstTypeInfo,
			MaxErrors:   maxErrors,
		}
		info.ConversionPairs = append(info.ConversionPairs, pair)

	} else if directive == "rule" {
		ruleText := strings.Join(args, " ") // Reconstruct rule string after "convert:rule "
		usingParts := strings.Split(ruleText, "using=")
		validatorParts := strings.Split(ruleText, "validator=")

		if len(usingParts) == 2 { // Conversion rule: "<SrcT>" -> "<DstT>", using=<func>
			typePartsStr := strings.TrimSpace(strings.TrimSuffix(usingParts[0], ","))
			// Need to parse this carefully, e.g. "\"time.Time\" -> \"string\""
			arrowIndex := strings.Index(typePartsStr, "->")
			if arrowIndex == -1 || len(strings.Fields(typePartsStr)) < 3 { // Basic check
				fmt.Printf("Skipping malformed convert:rule (using): %s (original: %s)\n", ruleText, commentText)
				return
			}

			srcTypeRaw := strings.TrimSpace(typePartsStr[:arrowIndex])
			dstTypeRaw := strings.TrimSpace(typePartsStr[arrowIndex+2:])

			srcTypeNameStr := strings.Trim(srcTypeRaw, "\"")
			dstTypeNameStr := strings.Trim(dstTypeRaw, "\"")
			usingFunc := strings.TrimSpace(usingParts[1])

			srcTypeInfo := resolveTypeFromString(srcTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)
			dstTypeInfo := resolveTypeFromString(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)

			if srcTypeInfo == nil || dstTypeInfo == nil {
				fmt.Printf("Warning: Could not resolve types for convert:rule (using) %s -> %s. Skipping.\n", srcTypeNameStr, dstTypeNameStr)
				return
			}
			if usingFunc == "" {
				fmt.Printf("Warning: convert:rule (using) %s -> %s has empty using function. Skipping.\n", srcTypeNameStr, dstTypeNameStr)
				return
			}


			rule := model.TypeRule{
				SrcTypeName: srcTypeNameStr,
				DstTypeName: dstTypeNameStr,
				SrcTypeInfo: srcTypeInfo,
				DstTypeInfo: dstTypeInfo,
				UsingFunc:   usingFunc,
			}
			info.GlobalRules = append(info.GlobalRules, rule)

		} else if len(validatorParts) == 2 { // Validator rule: "<DstT>", validator=<func>
			dstTypeStr := strings.TrimSpace(strings.TrimSuffix(validatorParts[0], ","))
			dstTypeNameStr := strings.Trim(dstTypeStr, "\"")
			validatorFunc := strings.TrimSpace(validatorParts[1])

			dstTypeInfo := resolveTypeFromString(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info)
			if dstTypeInfo == nil {
				fmt.Printf("Warning: Could not resolve type for convert:rule (validator) %s. Skipping.\n", dstTypeNameStr)
				return
			}
			if validatorFunc == "" {
				fmt.Printf("Warning: convert:rule (validator) %s has empty validator function. Skipping.\n", dstTypeNameStr)
				return
			}


			rule := model.TypeRule{
				DstTypeName:   dstTypeNameStr,
				DstTypeInfo:   dstTypeInfo,
				ValidatorFunc: validatorFunc,
			}
			info.GlobalRules = append(info.GlobalRules, rule)
		} else {
			fmt.Printf("Skipping malformed or unsupported convert:rule: %s (original: %s)\n", ruleText, commentText)
		}
	} else {
		// Not a "pair" or "rule" directive that we understand at this level.
		// Could be field-specific if not starting with "convert:" prefix after trimming comment chars.
		// Or just an unrelated comment.
	}
}

// resolveTypeFromString parses a type string (e.g., "MyType", "pkg.Type", "*pkg.Type")
// from an annotation and resolves it to TypeInfo.
func resolveTypeFromString(typeStr, currentPkgName, currentPkgPath string, fileImports map[string]string, parsedInfo *model.ParsedInfo) *model.TypeInfo {
	if typeStr == "" {
		fmt.Println("Warning: Empty type string passed to resolveTypeFromString.")
		return nil
	}
	// Use go/parser.ParseExpr to convert the type string into an AST expression.
	expr, err := parser.ParseExpr(typeStr)
	if err != nil {
		fmt.Printf("Warning: Could not parse type string '%s' from annotation into AST expression: %v. Treating as potentially unresolved.\n", typeStr, err)
		// Fallback: create a simple TypeInfo that is marked as unresolved or basic KindIdent.
		// This allows processing to continue but generation will likely fail or produce placeholder for this type.
		ti := &model.TypeInfo{
			Name:        typeStr, // Use the raw string as name
			FullName:    typeStr, // FullName might be pkg.Type or just Type
			PackageName: "",      // Unknown at this stage without successful parsing
			PackagePath: "",
			Kind:        model.KindUnknown, // Mark as unknown due to parse failure
			AstExpr:     nil,               // No valid AST expression
		}
		// Attempt to qualify if it looks like a local type.
		if !strings.Contains(typeStr, ".") { // Does not contain "." so might be local to current package
			ti.FullName = currentPkgPath + "." + typeStr
			ti.PackagePath = currentPkgPath
			ti.PackageName = currentPkgName
		}
		return ti
	}
	// If parsing to AST expression is successful, resolve it using resolveTypeExpr.
	return resolveTypeExpr(expr, currentPkgName, currentPkgPath, fileImports, parsedInfo)
}

// parseTypeSpec is called during Pass 2. It fully parses an ast.TypeSpec
// to create model.StructInfo or model.TypeInfo for named types.
func parseTypeSpec(typeSpec *ast.TypeSpec, parsedInfo *model.ParsedInfo, fileImports map[string]string, currentPkgName, currentPkgPath string) {
	typeName := typeSpec.Name.Name

	// Create the primary TypeInfo for this type definition.
	// This selfTypeInfo will be stored in parsedInfo.NamedTypes or associated with a StructInfo.
	selfTypeInfo := &model.TypeInfo{
		Name:        typeName,
		FullName:    currentPkgPath + "." + typeName,
		PackageName: currentPkgName,
		PackagePath: currentPkgPath,
		AstExpr:     typeSpec.Name, // Reference to its own declaration identifier
	}

	switch underlyingAstType := typeSpec.Type.(type) {
	case *ast.StructType:
		selfTypeInfo.Kind = model.KindStruct
		sInfo := &model.StructInfo{
			Name: typeName,
			Type: selfTypeInfo, // Link to its own TypeInfo
			Node: underlyingAstType,
		}
		parsedInfo.Structs[typeName] = sInfo
		selfTypeInfo.StructInfo = sInfo // Make the TypeInfo point to its StructInfo

		for _, field := range underlyingAstType.Fields.List {
			fieldTypeInfo := resolveTypeExpr(field.Type, currentPkgName, currentPkgPath, fileImports, parsedInfo)
			var fieldTag model.ConvertTag
			if field.Tag != nil {
				tagValue := strings.Trim(field.Tag.Value, "`")
				// Assuming format `convert:"name,opt1,opt2" other:"..."`
				if tagIdx := strings.Index(tagValue, "convert:\""); tagIdx != -1 {
					rawTag := tagValue[tagIdx+len("convert:\""):]
					if endIdx := strings.Index(rawTag, "\""); endIdx != -1 {
						rawTag = rawTag[:endIdx]
						fieldTag = parseFieldTag(rawTag)
					}
				}
			}

			if len(field.Names) > 0 { // Regular field
				for _, fieldNameIdent := range field.Names {
					fInfo := model.FieldInfo{
						Name:         fieldNameIdent.Name,
						OriginalName: fieldNameIdent.Name,
						TypeInfo:     fieldTypeInfo,
						Tag:          fieldTag,
						ParentStruct: sInfo,
						AstField:     field,
					}
					sInfo.Fields = append(sInfo.Fields, fInfo)
				}
			} else { // Embedded field
				// Name of embedded field is usually the type name itself.
				// If fieldTypeInfo.Name is "pkg.Type", embedded name is "Type".
				// If fieldTypeInfo.Name is "MyType" (local), embedded name is "MyType".
				embeddedName := fieldTypeInfo.Name
				if fieldTypeInfo.PackagePath != "" && fieldTypeInfo.PackagePath != currentPkgPath {
					// It's an external type, e.g. time.Time. Name should be "Time".
					// The TypeInfo.Name for pkg.Type is already "Type" if resolveTypeExpr handles selectors correctly.
				}

				fInfo := model.FieldInfo{
					Name:         embeddedName, // For embedded, Name is usually the type name.
					OriginalName: embeddedName, // This might need to be model.AstExprToString(field.Type,...) for full name
					TypeInfo:     fieldTypeInfo,
					Tag:          fieldTag, // Tags on embedded fields are parsed if present
					ParentStruct: sInfo,
					AstField:     field,
				}
				sInfo.Fields = append(sInfo.Fields, fInfo)
			}
		}
		// After processing all fields, this struct definition is complete.
		// selfTypeInfo (and sInfo.Type) is already set up.

	case *ast.Ident, *ast.SelectorExpr, *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.InterfaceType, *ast.FuncType:
		// This is a named type definition, e.g., `type MyInt int`, `type MySlice []string`, `type Point *image.Point`
		selfTypeInfo.Kind = model.KindNamed
		underlyingType := resolveTypeExpr(underlyingAstType, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		selfTypeInfo.Underlying = underlyingType
		parsedInfo.NamedTypes[typeName] = selfTypeInfo

		// Special handling: if this named type is an alias to a struct
		// e.g., `type MyStructAlias AnotherStruct` or `type MyStructPtrAlias *AnotherStruct`
		// We want MyStructAlias to also be "discoverable" as a struct-like type for pairings.
		effectiveUnderlyingType := underlyingType
		if underlyingType.IsPointer && underlyingType.Elem != nil {
			effectiveUnderlyingType = underlyingType.Elem // Look at what the pointer points to
		}

		if effectiveUnderlyingType.Kind == model.KindStruct || (effectiveUnderlyingType.Kind == model.KindIdent && effectiveUnderlyingType.StructInfo != nil) {
			// The named type (or what it points to) is effectively a struct.
			// Create a StructInfo for this alias to make it behave like a struct.
			baseStructInfo := effectiveUnderlyingType.StructInfo
			if baseStructInfo == nil && effectiveUnderlyingType.Kind == model.KindIdent {
				// It might be an identifier that resolved to a type that *is* a struct, look it up
				if si, ok := parsedInfo.Structs[effectiveUnderlyingType.Name]; ok && si.Type.PackagePath == effectiveUnderlyingType.PackagePath {
					baseStructInfo = si
				}
			}

			if baseStructInfo != nil {
				aliasStructInfo := &model.StructInfo{
					Name:            typeName, // The name of the alias, e.g., MyStructAlias
					Type:            selfTypeInfo, // The TypeInfo of MyStructAlias itself
					IsAlias:         true,
					UnderlyingAlias: baseStructInfo.Type, // Points to TypeInfo of AnotherStruct
					Node:            baseStructInfo.Node,  // "Inherit" AST node of the actual struct
					Fields:          baseStructInfo.Fields,// "Inherit" fields
				}
				// Add this alias to the list of known structs so it can be used in `convert:pair`
				if _, exists := parsedInfo.Structs[typeName]; !exists {
					parsedInfo.Structs[typeName] = aliasStructInfo
				}
				// Also link the selfTypeInfo of the alias to this new StructInfo wrapper
				selfTypeInfo.StructInfo = aliasStructInfo
			}
		}

	default:
		fmt.Printf("Warning: Unhandled TypeSpec kind for type '%s': %T. Storing as KindUnknown.\n", typeName, typeSpec.Type)
		selfTypeInfo.Kind = model.KindUnknown
		selfTypeInfo.Underlying = resolveTypeExpr(underlyingAstType, currentPkgName, currentPkgPath, fileImports, parsedInfo) // Attempt to resolve underlying anyway
		parsedInfo.NamedTypes[typeName] = selfTypeInfo // Store it so it's not completely lost
	}
}

// parseFieldTag parses the content of a `convert:"..."` struct tag.
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
