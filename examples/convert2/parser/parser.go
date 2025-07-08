package parser

import (
	"context" // Added for go-scan
	"fmt"
	"go/ast"
	// "go/parser" // No longer needed
	// "go/token" // No longer needed as direct usage is removed
	"os"
	"path/filepath"
	"strings"

	"log/slog" // Added for logging as per AGENTS.md

	"example.com/convert2/internal/model"
	goscan "github.com/podhmo/go-scan"               // Added for go-scan
	scannermodel "github.com/podhmo/go-scan/scanner" // Added for go-scan scanner models
)

// ParseDirectory scans the given directory for Go files, parses them using go-scan,
// and extracts conversion rules and type information.
func ParseDirectory(dirPath string) (*model.ParsedInfo, error) {
	ctx := context.Background() // Create a new context for go-scan operations

	// Initialize go-scan Scanner
	// dirPath is assumed to be the target package directory.
	// For go-scan's New, we ideally need a path within the module to help it find the module root.
	// If dirPath is like "./examples/convert2/testdata/simple", it should work.
	gs, err := goscan.New(dirPath)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to initialize go-scan scanner", slog.Any("error", err), slog.String("dirPath", dirPath))
		return nil, fmt.Errorf("failed to initialize go-scan scanner for path %s: %w", dirPath, err)
	}

	// TODO: Configure ExternalTypeOverrides if necessary.
	// For example, to ensure "time.Time" is recognized correctly if go-scan's default resolution isn't sufficient.
	// overrides := scannermodel.ExternalTypeOverride{
	//  "time.Time": "time.Time", // This tells go-scan to treat "time.Time" as a known type string.
	// }
	// gs.SetExternalTypeOverrides(ctx, overrides)

	// Scan the package using go-scan
	// ScanPackage is suitable when you have the direct file path to the package.
	scanInfo, err := gs.ScanPackage(ctx, dirPath)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to scan package with go-scan", slog.Any("error", err), slog.String("dirPath", dirPath))
		return nil, fmt.Errorf("failed to scan package at %s using go-scan: %w", dirPath, err)
	}

	if scanInfo == nil {
		slog.ErrorContext(ctx, "go-scan returned nil PackageInfo", slog.String("dirPath", dirPath))
		return nil, fmt.Errorf("go-scan returned nil PackageInfo for %s", dirPath)
	}
	if len(scanInfo.Files) == 0 && len(scanInfo.Types) == 0 && len(scanInfo.Functions) == 0 {
		// This might happen if the directory is empty or contains no Go files go-scan considers.
		// Or if all files were filtered out by go-scan's internal logic (e.g. _test.go, _gen.go by default in some contexts)
		// The original parser.ParseDir also filters _test.go and _gen.go, so this behavior might be consistent.
		slog.InfoContext(ctx, "go-scan found no relevant Go files or symbols in directory", slog.String("dirPath", dirPath), slog.String("packageName", scanInfo.Name))
		// The original code returns an error if no packages are found.
		// Let's consider if scanInfo indicates "no Go packages found" vs "package found but empty of relevant items".
		// goscan.Scanner.ScanPackage itself doesn't seem to error out for an empty dir, but returns minimal PackageInfo.
		// The original check was `if len(pkgs) == 0`.
		// A more direct check would be if `scanInfo.Name` is empty or if `scanInfo.Files` is empty after a successful scan.
		// If `scanInfo.Name` is populated, it means a package was identified.
		if scanInfo.Name == "" { // No package name could be determined
			return nil, fmt.Errorf("no Go packages found in directory %s (go-scan could not determine package name)", dirPath)
		}
		// If package name exists, but no types/files, it's an empty package, proceed to return empty ParsedInfo.
	}

	// TODO: The rest of this function needs to be refactored to use scanInfo
	// instead of the old fset and pkgs from go/parser.ParseDir.
	// The 2-pass system will be replaced by iterating over scanInfo.Types,
	// scanInfo.Functions, etc.

	// Placeholder for currentPkgName and currentPkgPath from scanInfo
	currentPkgName := scanInfo.Name
	currentPkgImportPath := scanInfo.ImportPath // Standardized variable name
	if currentPkgImportPath == "" {
		// Fallback similar to original, though scanInfo.ImportPath should be more reliable.
		slog.WarnContext(ctx, "go-scan PackageInfo has empty ImportPath. Attempting fallback.", slog.String("dirPath", dirPath), slog.String("packageName", currentPkgName))
		pathFromDerive := derivePackagePath(dirPath)
		if pathFromDerive != "" {
			currentPkgImportPath = pathFromDerive
		} else {
			slog.WarnContext(ctx, "Fallback derivePackagePath also failed. Using package name as import path placeholder.", slog.String("dirPath", dirPath), slog.String("packageName", currentPkgName))
			currentPkgImportPath = currentPkgName // Last resort
		}
	}

	parsedInfo := model.NewParsedInfo(currentPkgName, currentPkgImportPath)
	// The FileImports map might need to be populated differently, or its usage re-evaluated.
	// go-scan's scanner.FieldType has PkgName and fullImportPath, which should reduce reliance on per-file import maps.
	// For now, let's clear the old way of populating FileImports.
	// parsedInfo.FileImports = make(map[string]map[string]string) // Initialize if needed later

	// --- The following is the OLD logic and needs to be replaced ---
	// fset variable removed as it's unused. go-scan uses its own FileSet.
	// We should use gs.Fset() if AST nodes from scanInfo.AstFiles are used.
	// Or, if we re-parse for comments, ensure consistency.
	// For now, this part of the code is effectively dead or will be replaced.
	// pkgs variable and associated ParseDir call removed as it's unused.

	// This old logic for pkgs, typeSpecsToProcess, PASS 1, PASS 2 needs to be entirely replaced
	// by processing data from `scanInfo (*scannermodel.PackageInfo)`.

	// Example of how one might start processing types from scanInfo:
	// for _, typeDef := range scanInfo.Types {
	//    // Convert typeDef (*scannermodel.TypeInfo) to model.StructInfo or model.TypeInfo
	//    // This will involve calling the new `convertFieldTypeToModelTypeInfo` for fields/underlying types.
	// }

	// For now, returning a partially filled ParsedInfo based on what go-scan provided for package level details.
	// The detailed type parsing and directive parsing is the next step.
	// This means the function will not yet work correctly.

	// --- END OF OLD LOGIC TO BE REPLACED ---

	// TODO: Implement PASS 2 equivalent using scanInfo.
	// This involves iterating scanInfo.Types and scanInfo.AstFiles (if needed for comments).
	// For each type in scanInfo.Types, populate parsedInfo.Structs or parsedInfo.NamedTypes.
	// Then, parse directives from comments using scanInfo.AstFiles[filePath].Doc and .Comments.

	// Placeholder for the old PASS 1 and PASS 2 logic.
	// This will be replaced in the next step of the plan.
	// For now, to make it compile, let's keep the old parsing logic but acknowledge it's wrong.
	// Populate Structs and NamedTypes from scanInfo.Types
	for _, stypeInfo := range scanInfo.Types { // stypeInfo is *scannermodel.TypeInfo
		modelType := &model.TypeInfo{
			Name:        stypeInfo.Name,
			FullName:    fmt.Sprintf("%s.%s", currentPkgImportPath, stypeInfo.Name),
			PackageName: currentPkgName, // Assuming types defined are in the current package
			PackagePath: currentPkgImportPath,
			// AstExpr: is tricky. For a TypeSpec "type Foo Bar", Foo is Name, Bar is Type.
			// stypeInfo.Node is ast.Spec (*ast.TypeSpec). We need an ast.Expr.
			// For model.TypeInfo representing "Foo", AstExpr should be the *ast.Ident "Foo".
		}
		if ts, ok := stypeInfo.Node.(*ast.TypeSpec); ok {
			modelType.AstExpr = ts.Name // ts.Name is *ast.Ident, which is an ast.Expr
		} else {
			slog.WarnContext(ctx, "go-scan TypeInfo.Node was not *ast.TypeSpec as expected for type definition", slog.String("typeName", stypeInfo.Name), slog.Any("nodeType", fmt.Sprintf("%T", stypeInfo.Node)))
		}

		switch stypeInfo.Kind {
		case scannermodel.StructKind:
			modelType.Kind = model.KindStruct
			if stypeInfo.Struct == nil {
				slog.ErrorContext(ctx, "StructKind TypeInfo has nil Struct detail", slog.String("typeName", stypeInfo.Name))
				continue
			}
			// var astStructType *ast.StructType // Not needed as model.StructInfo has no Node field
			// if typeSpec, ok := stypeInfo.Node.(*ast.TypeSpec); ok {
			// 	if st, ok2 := typeSpec.Type.(*ast.StructType); ok2 {
			// 		astStructType = st
			// 	}
			// }
			// if astStructType == nil {
			// 	slog.ErrorContext(ctx, "Could not obtain *ast.StructType from scannermodel.TypeInfo", slog.String("typeName", stypeInfo.Name))
			// 	continue
			// }

			sinfo := &model.StructInfo{
				Name: stypeInfo.Name,
				// Node: astStructType, // model.StructInfo no longer has Node
				Type: modelType,
			}
			for _, sfield := range stypeInfo.Struct.Fields { // sfield is *scannermodel.FieldInfo
				mfield := model.FieldInfo{
					Name:         sfield.Name,
					OriginalName: sfield.Name, // Assuming Name is original name
					TypeInfo:     convertScannerTypeToModelType(ctx, sfield.Type, currentPkgImportPath),
					// AstField: // TODO: scannermodel.FieldInfo doesn't expose original *ast.Field.
					// This might be an issue if more than just tag string is needed.
					// For now, we hope sfield.Tag (string) is enough.
					ParentStruct: sinfo,
				}

				if sfield.Tag != "" {
					// parseFieldTag expects the content of a `convert:"..."` tag.
					// sfield.Tag is the full tag string, e.g., `json:"foo" convert:"bar"`.
					// We need to extract the `convert` part first.
					convertTagContent := extractConvertTag(sfield.Tag)
					if convertTagContent != "" {
						mfield.Tag = parseFieldTag(convertTagContent)
					}
				}
				sinfo.Fields = append(sinfo.Fields, mfield)
			}
			modelType.StructInfo = sinfo
			parsedInfo.Structs[sinfo.Name] = sinfo

		case scannermodel.AliasKind:
			modelType.Kind = model.KindNamed
			if stypeInfo.Underlying == nil {
				slog.ErrorContext(ctx, "AliasKind TypeInfo has nil Underlying detail", slog.String("typeName", stypeInfo.Name))
				continue
			}
			modelType.Underlying = convertScannerTypeToModelType(ctx, stypeInfo.Underlying, currentPkgImportPath)
			parsedInfo.NamedTypes[modelType.Name] = modelType

			// Handle aliases to structs (e.g., type MyStructAlias AnotherStruct)
			// This makes MyStructAlias discoverable as a struct-like type.
			effectiveUnderlying := modelType.Underlying
			if effectiveUnderlying != nil && effectiveUnderlying.IsPointer && effectiveUnderlying.Elem != nil {
				effectiveUnderlying = effectiveUnderlying.Elem
			}
			if effectiveUnderlying != nil && (effectiveUnderlying.Kind == model.KindStruct || effectiveUnderlying.Kind == model.KindIdent) {
				// Try to find the base StructInfo.
				// If effectiveUnderlying.FullName is "pkg.ActualStruct", look for "ActualStruct" in parsedInfo.Structs
				// coming from the correct package.
				var baseStructInfo *model.StructInfo
				if si, ok := parsedInfo.Structs[effectiveUnderlying.Name]; ok && si.Type.PackagePath == effectiveUnderlying.PackagePath {
					baseStructInfo = si
				}

				if baseStructInfo != nil {
					aliasStructInfo := &model.StructInfo{
						Name:            stypeInfo.Name, // Name of the alias
						Type:            modelType,      // TypeInfo of the alias itself
						IsAlias:         true,
						UnderlyingAlias: baseStructInfo.Type,   // Points to TypeInfo of ActualStruct
						// Node:            baseStructInfo.Node, // model.StructInfo no longer has Node
						Fields:          baseStructInfo.Fields, // Inherit fields
					}
					if _, exists := parsedInfo.Structs[stypeInfo.Name]; !exists {
						parsedInfo.Structs[stypeInfo.Name] = aliasStructInfo
					}
					modelType.StructInfo = aliasStructInfo // Link the alias's TypeInfo to this wrapper
				}
			}

		case scannermodel.InterfaceKind:
			modelType.Kind = model.KindInterface
			parsedInfo.NamedTypes[modelType.Name] = modelType

		case scannermodel.FuncKind: // Example: type MyFunc func(int) string
			modelType.Kind = model.KindFunc
			// Details of func signature (params/results) are in stypeInfo.Func if needed.
			// model.TypeInfo currently just marks it as KindFunc.
			// If stypeInfo.Func exists, we could try to populate Elem/Key/Value or a new field.
			// For now, this is consistent with how old parser treated func type aliases.
			parsedInfo.NamedTypes[modelType.Name] = modelType

		default:
			slog.WarnContext(ctx, "Unhandled scannermodel.TypeKind in parser type loop",
				slog.Any("kind", stypeInfo.Kind),
				slog.String("typeName", stypeInfo.Name))
		}
	}

	// Parse directives from comments using ASTs from go-scan
	for filePath, fileAst := range scanInfo.AstFiles {
		currentFileImports := make(map[string]string)
		for _, importSpec := range fileAst.Imports {
			path := strings.Trim(importSpec.Path.Value, `"`)
			name := ""
			if importSpec.Name != nil {
				name = importSpec.Name.Name
			} else {
				parts := strings.Split(path, "/")
				name = parts[len(parts)-1]
			}
			currentFileImports[name] = path // Handles dot imports if name is "."
		}
		parsedInfo.FileImports[filePath] = currentFileImports // Store for reference, though might not be widely used

		if fileAst.Doc != nil {
			for _, comment := range fileAst.Doc.List {
				parseGlobalCommentDirective(comment.Text, parsedInfo, currentFileImports, currentPkgName, currentPkgImportPath, ctx)
			}
		}
		for _, commentGroup := range fileAst.Comments {
			for _, comment := range commentGroup.List {
				parseGlobalCommentDirective(comment.Text, parsedInfo, currentFileImports, currentPkgName, currentPkgImportPath, ctx)
			}
		}
	}

	// The old parser logic (parser.ParseDir, typeSpecsToProcess, PASS1, PASS2 loops) is now fully removed.
	return parsedInfo, nil
}

// resolveDirectiveType resolves a type string from a directive (e.g., in a comment)
// against the types already parsed and stored in parsedInfo.
// It uses fileImports to resolve package aliases for qualified type names.
func resolveDirectiveType(originalTypeStr string, currentPkgName string, currentPkgPath string, fileImports map[string]string, parsedInfo *model.ParsedInfo, ctx context.Context) *model.TypeInfo {
	if originalTypeStr == "" {
		slog.WarnContext(ctx, "Empty type string passed to resolveDirectiveType.")
		return nil
	}

	typeStr := strings.TrimSpace(originalTypeStr)

	// 1. Handle prefixes: *, [], map[]
	if strings.HasPrefix(typeStr, "*") {
		elemTypeStr := strings.TrimPrefix(typeStr, "*")
		elemTypeInfo := resolveDirectiveType(elemTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo, ctx)
		if elemTypeInfo == nil {
			slog.WarnContext(ctx, "Could not resolve element type for pointer in directive", slog.String("typeStr", originalTypeStr))
			return &model.TypeInfo{Name: originalTypeStr, FullName: originalTypeStr, Kind: model.KindPointer, IsPointer: true, Elem: &model.TypeInfo{Name: elemTypeStr, FullName: elemTypeStr, Kind: model.KindUnknown}}
		}
		return &model.TypeInfo{
			Name:        elemTypeInfo.Name,
			FullName:    "*" + elemTypeInfo.FullName,
			Kind:        model.KindPointer,
			IsPointer:   true,
			Elem:        elemTypeInfo,
			PackageName: elemTypeInfo.PackageName,
			PackagePath: elemTypeInfo.PackagePath,
		}
	}

	if strings.HasPrefix(typeStr, "[]") {
		elemTypeStr := strings.TrimPrefix(typeStr, "[]")
		elemTypeInfo := resolveDirectiveType(elemTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo, ctx)
		if elemTypeInfo == nil {
			slog.WarnContext(ctx, "Could not resolve element type for slice in directive", slog.String("typeStr", originalTypeStr))
			return &model.TypeInfo{Name: originalTypeStr, FullName: originalTypeStr, Kind: model.KindSlice, IsSlice: true, Elem: &model.TypeInfo{Name: elemTypeStr, FullName: elemTypeStr, Kind: model.KindUnknown}}
		}
		return &model.TypeInfo{
			Name:        elemTypeInfo.Name,
			FullName:    "[]" + elemTypeInfo.FullName,
			Kind:        model.KindSlice,
			IsSlice:     true,
			Elem:        elemTypeInfo,
			PackageName: elemTypeInfo.PackageName,
			PackagePath: elemTypeInfo.PackagePath,
		}
	}

	if strings.HasPrefix(typeStr, "map[") && strings.HasSuffix(typeStr, "]") {
		inner := strings.TrimPrefix(typeStr, "map[")
		inner = strings.TrimSuffix(inner, "]")
		parts := strings.SplitN(inner, "]", 2)
		if len(parts) != 2 {
			slog.WarnContext(ctx, "Could not parse map type string in directive", slog.String("typeStr", originalTypeStr))
			return &model.TypeInfo{Name: originalTypeStr, FullName: originalTypeStr, Kind: model.KindUnknown}
		}
		keyTypeStr := parts[0]
		valTypeStr := parts[1]

		keyTypeInfo := resolveDirectiveType(keyTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo, ctx)
		valTypeInfo := resolveDirectiveType(valTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo, ctx)

		if keyTypeInfo == nil || valTypeInfo == nil {
			slog.WarnContext(ctx, "Could not resolve key or value type for map in directive", slog.String("typeStr", originalTypeStr))
			unknownKey := keyTypeInfo
			if unknownKey == nil {	unknownKey = &model.TypeInfo{Name: keyTypeStr, FullName: keyTypeStr, Kind: model.KindUnknown} }
			unknownVal := valTypeInfo
			if unknownVal == nil { unknownVal = &model.TypeInfo{Name: valTypeStr, FullName: valTypeStr, Kind: model.KindUnknown} }
			return &model.TypeInfo{Name: originalTypeStr, FullName: originalTypeStr, Kind: model.KindMap, IsMap: true, Key: unknownKey, Value: unknownVal}
		}
		return &model.TypeInfo{
			Name:     fmt.Sprintf("map[%s]%s", keyTypeInfo.Name, valTypeInfo.Name),
			FullName: fmt.Sprintf("map[%s]%s", keyTypeInfo.FullName, valTypeInfo.FullName),
			Kind:     model.KindMap, IsMap: true, Key: keyTypeInfo, Value: valTypeInfo,
		}
	}

	// If not a prefix type, then it's an identifier (possibly quoted) or basic type.
	// Trim quotes now for the base name.
	baseTypeName := strings.Trim(typeStr, "\"")

	// 2. Handle basic Go types
	basicTypes := map[string]bool{
		"bool": true, "byte": true, "complex128": true, "complex64": true, "error": true,
		"float32": true, "float64": true, "int": true, "int16": true, "int32": true, "int64": true,
		"int8": true, "rune": true, "string": true, "uint": true, "uint16": true, "uint32": true,
		"uint64": true, "uint8": true, "uintptr": true,
	}
	if basicTypes[baseTypeName] {
		kind := model.KindBasic
		isInterface := false
		if baseTypeName == "error" {
			kind = model.KindInterface
			isInterface = true
		}
		return &model.TypeInfo{
			Name: baseTypeName, FullName: baseTypeName, Kind: kind, IsBasic: true, IsInterface: isInterface,
		}
	}

	// 3. Handle identifiers (local or qualified) by looking them up in parsedInfo
	parts := strings.SplitN(baseTypeName, ".", 2) // Use baseTypeName (quotes trimmed)
	var lookupName, pkgAliasLookup string
	var lookupPkgPath string

	if len(parts) == 1 { // Unqualified identifier (e.g., "MyType")
		lookupName = parts[0]
		lookupPkgPath = currentPkgPath // Assumed to be in the current package
		pkgAliasLookup = ""            // No alias for current package types
	} else { // Qualified identifier (e.g., "pkg.Type")
		pkgAliasLookup = parts[0]
		lookupName = parts[1]
		var ok bool
		lookupPkgPath, ok = fileImports[pkgAliasLookup]
		if !ok {
			// If alias not in fileImports, it could be a dot import scenario where pkgAliasLookup is actually the type name,
			// or it's an unknown package.
			// Check if `pkgAliasLookup` itself is a type in the current package (e.g. `MyType.Field` - not a type).
			// Or, it could be that the "alias" is actually the full import path if user wrote ` "example.com/mod/pkg".MyType `
			// This simple split is insufficient for full import paths with dots.
			// For now, assume if not in fileImports, the alias *is* the package path, or it's an error.
			// A more robust approach for ` "path".Type ` would require smarter parsing of typeStr.
			// Let's assume for directives, users use defined aliases or unqualified names for current pkg.
			slog.WarnContext(ctx, "Package alias not found in file imports for directive type resolution.",
				slog.String("alias", pkgAliasLookup), slog.String("typeStr", typeStr), slog.String("currentPkgPath", currentPkgPath))

			// Attempt to see if the pkgAliasLookup is actually a known package path from parsedInfo
			// (e.g. user wrote full path instead of alias)
			foundByPath := false
			for _, s := range parsedInfo.Structs {
				if s.Type.PackagePath == pkgAliasLookup {
					lookupPkgPath = pkgAliasLookup
					foundByPath = true
					break
				}
			}
			if !foundByPath {
				for _, nt := range parsedInfo.NamedTypes {
					if nt.PackagePath == pkgAliasLookup {
						lookupPkgPath = pkgAliasLookup
						foundByPath = true
						break
					}
				}
			}
			if !foundByPath {
				slog.WarnContext(ctx, "Assuming alias is the full package path due to no match in fileImports.", slog.String("alias", pkgAliasLookup), slog.String("typeStr", typeStr))
				lookupPkgPath = pkgAliasLookup // Fallback: assume alias is the full path
			}
		}
	}

	// Search in Structs
	for _, sinfo := range parsedInfo.Structs {
		if sinfo.Name == lookupName && sinfo.Type.PackagePath == lookupPkgPath {
			// If we looked up via an alias, ensure the TypeInfo's PackageName reflects that for consistency.
			// However, sinfo.Type.PackageName should already be correctly set by convertScannerTypeToModelType.
			// For local types, PackageName in TypeInfo is often empty.
			return sinfo.Type
		}
	}
	// Search in NamedTypes
	for _, ntInfo := range parsedInfo.NamedTypes {
		if ntInfo.Name == lookupName && ntInfo.PackagePath == lookupPkgPath {
			return ntInfo
		}
	}

	slog.WarnContext(ctx, "Type not found in parsedInfo for directive.",
		slog.String("typeStr", typeStr),
		slog.String("lookupName", lookupName),
		slog.String("lookupPkgPath", lookupPkgPath),
		slog.String("currentPkgPath", currentPkgPath))

	// Fallback: return a TypeInfo that represents an unresolved identifier
	// Its FullName will be constructed based on assumptions.
	fullName := lookupName
	if lookupPkgPath != "" {
		fullName = lookupPkgPath + "." + lookupName
	}

	return &model.TypeInfo{
		Name:        lookupName,
		FullName:    fullName,
		PackageName: pkgAliasLookup, // This is the alias used in the directive string
		PackagePath: lookupPkgPath,  // This is the resolved or assumed package path
		Kind:        model.KindUnknown,
	}
}

// convertScannerTypeToModelType converts scannermodel.FieldType to model.TypeInfo
func convertScannerTypeToModelType(
	ctx context.Context,
	stype *scannermodel.FieldType,
	currentPkgImportPath string,
) *model.TypeInfo {
	if stype == nil {
		return nil
	}

	mtype := &model.TypeInfo{
		// AstExpr: // scannermodel.FieldType doesn't directly hold the ast.Expr it came from.
		// model.TypeInfo.AstExpr might be less critical now.
	}

	mtype.IsBasic = stype.IsBuiltin
	mtype.IsPointer = stype.IsPointer
	mtype.IsMap = stype.IsMap
	mtype.IsInterface = strings.HasPrefix(stype.Name, "interface{") || stype.Name == "error"
	mtype.IsFunc = strings.HasPrefix(stype.Name, "func(")

	// Simplified: go-scan's FieldType doesn't easily expose array vs slice distinction
	// in a way that maps directly to model.TypeInfo's IsArray and ArrayLengthExpr.
	// For now, treat array-like things primarily as slices if stype.IsSlice is true.
	// Fixed-size array distinction is lost for now.
	mtype.IsArray = false // Assume not an array unless specifically determined otherwise.
	mtype.IsSlice = stype.IsSlice

	if mtype.IsPointer {
		mtype.Kind = model.KindPointer

		// Create Elem TypeInfo based on current stype's non-pointer attributes
		// Effectively, we are describing the type that stype (*T) points to (T).
		elemModelType := &model.TypeInfo{
			// Name, FullName, PkgName, PkgPath for T will be derived from stype (which represents *T but holds T's name)
			IsBasic:     stype.IsBuiltin, // If *int, Elem is int (basic)
			IsInterface: (stype.Name == "error"), // If *error, Elem is error (interface)
			// Other IsX flags (IsPointer, IsSlice, IsMap) are false for the base element T
		}

		if elemModelType.IsBasic {
			elemModelType.Kind = model.KindBasic
			elemModelType.Name = stype.Name // e.g., "string" for *string
			if stype.PkgName != "" && strings.HasPrefix(elemModelType.Name, stype.PkgName+".") { // e.g. *pkg.MyBasicAlias
				elemModelType.Name = strings.TrimPrefix(elemModelType.Name, stype.PkgName+".")
			}
			elemModelType.FullName = elemModelType.Name
			elemModelType.PackagePath = "" // Basic types have no package path
			elemModelType.PackageName = ""
			if stype.Name == "error" { // Special handling for error interface
				elemModelType.Kind = model.KindInterface
			}
		} else { // Identifier for the element type T
			elemModelType.Kind = model.KindIdent
			if stype.PkgName != "" && strings.HasPrefix(stype.Name, stype.PkgName+".") {
				elemModelType.Name = strings.TrimPrefix(stype.Name, stype.PkgName+".")
			} else {
				elemModelType.Name = stype.Name
			}
			elemModelType.PackageName = stype.PkgName
			elemModelType.PackagePath = stype.FullImportPath()

			if elemModelType.PackagePath != "" && elemModelType.PackagePath != currentPkgImportPath { // External
				elemModelType.FullName = elemModelType.PackagePath + "." + elemModelType.Name
			} else { // Current package or type param
				elemModelType.PackagePath = currentPkgImportPath
				elemModelType.FullName = elemModelType.PackagePath + "." + elemModelType.Name
				if stype.IsTypeParam && stype.PkgName == "" && stype.FullImportPath() == "" {
					elemModelType.FullName = elemModelType.Name
					elemModelType.PackagePath = ""
				}
			}
		}
		mtype.Elem = elemModelType

		mtype.Name = elemModelType.Name
		mtype.FullName = "*" + elemModelType.FullName
		mtype.PackageName = elemModelType.PackageName
		mtype.PackagePath = elemModelType.PackagePath

	} else if mtype.IsSlice {
		mtype.Kind = model.KindSlice
		mtype.Elem = convertScannerTypeToModelType(ctx, stype.Elem, currentPkgImportPath)
		if mtype.Elem != nil {
			mtype.Name = mtype.Elem.Name
			mtype.FullName = "[]" + mtype.Elem.FullName
			mtype.PackageName = mtype.Elem.PackageName
			mtype.PackagePath = mtype.Elem.PackagePath
		} else {
			slog.WarnContext(ctx, "Slice FieldType has nil Elem", slog.String("stypeName", stype.Name))
			name := stype.Name // stype.Name could be "pkg.Type" or "Type"
			simpleName := name
			if stype.PkgName != "" && strings.HasPrefix(name, stype.PkgName+".") {
				simpleName = strings.TrimPrefix(name, stype.PkgName+".")
			}
			mtype.Name = simpleName
			mtype.FullName = "[]" + stype.FullImportPath() + "." + simpleName
		}
	} else if mtype.IsMap {
		mtype.Kind = model.KindMap
		mtype.Key = convertScannerTypeToModelType(ctx, stype.MapKey, currentPkgImportPath)
		mtype.Value = convertScannerTypeToModelType(ctx, stype.Elem, currentPkgImportPath) // Elem is Value for maps
		if mtype.Key != nil && mtype.Value != nil {
			mtype.Name = fmt.Sprintf("map[%s]%s", mtype.Key.Name, mtype.Value.Name)
			mtype.FullName = fmt.Sprintf("map[%s]%s", mtype.Key.FullName, mtype.Value.FullName)
		} else {
			slog.WarnContext(ctx, "Map FieldType has nil Key or Value", slog.String("stypeName", stype.Name))
			keyName := "any"
			if stype.MapKey != nil {
				name := stype.MapKey.Name
				simpleName := name
				if stype.MapKey.PkgName != "" && strings.HasPrefix(name, stype.MapKey.PkgName+".") { simpleName = strings.TrimPrefix(name, stype.MapKey.PkgName+".") }
				keyName = simpleName
			}
			valName := "any"
			if stype.Elem != nil {
				name := stype.Elem.Name
				simpleName := name
				if stype.Elem.PkgName != "" && strings.HasPrefix(name, stype.Elem.PkgName+".") { simpleName = strings.TrimPrefix(name, stype.Elem.PkgName+".") }
				valName = simpleName
			}
			mtype.Name = fmt.Sprintf("map[%s]%s", keyName, valName)
			mtype.FullName = mtype.Name
		}
	} else if mtype.IsBasic {
		mtype.Kind = model.KindBasic
		mtype.Name = stype.Name
		mtype.FullName = stype.Name
		mtype.PackageName = ""
		mtype.PackagePath = ""
		if stype.Name == "error" {
			mtype.IsInterface = true
			mtype.Kind = model.KindInterface
		}
	} else {
		mtype.Kind = model.KindIdent
		// Determine simple name for mtype.Name
		if stype.PkgName != "" && strings.HasPrefix(stype.Name, stype.PkgName+".") {
			mtype.Name = strings.TrimPrefix(stype.Name, stype.PkgName+".")
		} else {
			mtype.Name = stype.Name // Assumed to be simple name already (local type, type param, or PkgName is empty)
		}

		if stype.PkgName != "" && stype.FullImportPath() != "" && stype.FullImportPath() != currentPkgImportPath {
			// External package
			mtype.PackageName = stype.PkgName // This is the alias used in source, or derived by go-scan
			mtype.PackagePath = stype.FullImportPath()
			mtype.FullName = mtype.PackagePath + "." + mtype.Name
		} else {
			// Current package, or a type without explicit package path (e.g. could be unresolved type parameter if not caught)
			mtype.PackageName = "" // Local types don't need package name in model.TypeInfo.PackageName
			mtype.PackagePath = currentPkgImportPath
			mtype.FullName = mtype.PackagePath + "." + mtype.Name
		}
		if stype.IsTypeParam {
			slog.DebugContext(ctx, "Encountered type parameter in convertScannerTypeToModelType", slog.String("name", stype.Name))
			// model.TypeInfo doesn't have IsTypeParam. Treat as KindIdent. Its FullName might be just its name.
			// This could be an issue if it needs special handling later.
			// For now, its FullName might be "currentPkg.T" if T is defined in currentPkg.
			// If it's a generic parameter like `T` from `func foo[T any]()`, its PkgName/Path might be empty from go-scan.
			if stype.PkgName == "" && stype.FullImportPath() == "" {
				mtype.FullName = mtype.Name // e.g. "T"
				mtype.PackagePath = ""      // Type params don't belong to a package in the same way
			}
		}
		// If it's 'error' and wasn't caught by IsBasic
		if mtype.Name == "error" && mtype.PackagePath == "" {
			mtype.IsInterface = true
			mtype.Kind = model.KindInterface
			mtype.IsBasic = true // Treat 'error' as a basic type for some classification purposes
		}

	}
	return mtype
}

// extractConvertTag extracts the content of the `convert:"..."` part from a full struct tag string.
func extractConvertTag(fullTag string) string {
	// Example tag: `json:"name,omitempty" convert:"target_name,required" validate:"nonempty"`
	// We need to find `convert:"..."`
	const convertKey = `convert:"`
	idx := strings.Index(fullTag, convertKey)
	if idx == -1 {
		return ""
	}
	// Found the start of convert key
	valStart := idx + len(convertKey)
	// Find the closing quote for this value
	// Need to handle escaped quotes inside, though rare for this specific tag.
	// For simplicity, find the next quote.
	endIdx := -1
	searchStart := valStart
	for {
		nextQuote := strings.Index(fullTag[searchStart:], `"`)
		if nextQuote == -1 {
			return "" // Malformed, no closing quote
		}
		// Check if this quote is escaped
		if searchStart+nextQuote > 0 && fullTag[searchStart+nextQuote-1] == '\\' {
			// It's an escaped quote, continue search after it
			searchStart += nextQuote + 1
			continue
		}
		endIdx = searchStart + nextQuote
		break
	}

	if endIdx == -1 {
		return "" // Malformed
	}
	return fullTag[valStart:endIdx]
}

// derivePackagePath tries to derive an import path from a directory path.
// This is a simplified heuristic and might not work for all project layouts.
// A robust solution would involve parsing go.mod.
// derivePackagePath tries to derive an import path from a directory path.
// This is a simplified heuristic and might not work for all project layouts.
// A robust solution would involve parsing go.mod.
// scanInfo.ImportPath is now preferred. This function is a fallback.
func derivePackagePath(dirPath string) string {
	slog.Debug("Attempting to derive package path as fallback", slog.String("dirPath", dirPath))
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

// parseGlobalCommentDirective parses a single package-level or file-level comment line.
func parseGlobalCommentDirective(commentText string, info *model.ParsedInfo, fileImports map[string]string, currentPkgName, currentPkgPath string, ctx context.Context) {
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
			slog.WarnContext(ctx, "Skipping malformed convert:pair directive", slog.String("text", trimmedComment), slog.String("original", commentText))
			return
		}
		srcTypeNameStr := args[0]
		dstTypeNameStr := args[2]
		maxErrors := 0 // Default

		optionsStr := ""
		if len(args) > 3 {
			optionsStr = strings.Join(args[3:], " ")
		}
		parsedOptions := parseOptions(optionsStr)
		if val, ok := parsedOptions["max_errors"]; ok {
			if _, err := fmt.Sscanf(val, "%d", &maxErrors); err != nil {
				slog.WarnContext(ctx, "Could not parse max_errors value for pair",
					slog.String("value", val),
					slog.String("src", srcTypeNameStr),
					slog.String("dst", dstTypeNameStr),
					slog.Any("error", err))
			}
		}

		srcTypeInfo := resolveDirectiveType(srcTypeNameStr, currentPkgName, currentPkgPath, fileImports, info, ctx)
		dstTypeInfo := resolveDirectiveType(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info, ctx)

		if srcTypeInfo == nil || dstTypeInfo == nil || srcTypeInfo.Kind == model.KindUnknown || dstTypeInfo.Kind == model.KindUnknown {
			slog.WarnContext(ctx, "Could not resolve types for convert:pair, or types are unknown. Skipping.",
				slog.String("src", srcTypeNameStr),
				slog.String("dst", dstTypeNameStr),
				slog.Any("srcInfo", srcTypeInfo),
				slog.Any("dstInfo", dstTypeInfo))
			return
		}

		pair := model.ConversionPair{
			SrcTypeName: srcTypeNameStr, // Keep original string for reference/debugging
			DstTypeName: dstTypeNameStr, // Keep original string for reference/debugging
			SrcTypeInfo: srcTypeInfo,
			DstTypeInfo: dstTypeInfo,
			MaxErrors:   maxErrors,
		}
		info.ConversionPairs = append(info.ConversionPairs, pair)
		slog.DebugContext(ctx, "Successfully parsed convert:pair directive", slog.String("src", srcTypeInfo.FullName), slog.String("dst", dstTypeInfo.FullName))

	} else if directive == "rule" {
		ruleArgsText := strings.TrimSpace(strings.Join(args, " ")) // Full text after "convert:rule "

		idxVal := strings.Index(ruleArgsText, "validator=")
		idxUsing := strings.Index(ruleArgsText, "using=")

		if idxVal != -1 && (idxUsing == -1 || idxVal < idxUsing) { // Found validator= and it appears before any using= or no using=
			// Validator rule: "<DstT>", validator=<func>
			parts := strings.SplitN(ruleArgsText, "validator=", 2)
			dstTypeAndComma := strings.TrimSpace(parts[0])
			dstTypeStr := strings.TrimSpace(strings.TrimSuffix(dstTypeAndComma, ","))
			dstTypeNameStr := strings.Trim(dstTypeStr, "\"")
			validatorFunc := strings.TrimSpace(parts[1])
			slog.DebugContext(ctx, "Parsing convert:rule (validator)", "dstTypeStr", dstTypeStr, "dstTypeNameStr", dstTypeNameStr, "validatorFunc", validatorFunc)

			dstTypeInfo := resolveDirectiveType(dstTypeNameStr, currentPkgName, currentPkgPath, fileImports, info, ctx)
			if dstTypeInfo == nil || dstTypeInfo.Kind == model.KindUnknown {
				slog.WarnContext(ctx, "Could not resolve type for convert:rule (validator), or type is unknown. Skipping.",
					slog.String("dstRule", dstTypeNameStr), // Log the name used for lookup
					slog.Any("dstInfo", dstTypeInfo))
				return
			}
			if validatorFunc == "" {
				slog.WarnContext(ctx, "convert:rule (validator) has empty validator function. Skipping.", slog.String("dst", dstTypeNameStr))
				return
			}
			rule := model.TypeRule{
				DstTypeName:   dstTypeNameStr, // Store the cleaned name
				DstTypeInfo:   dstTypeInfo,
				ValidatorFunc: validatorFunc,
			}
			info.GlobalRules = append(info.GlobalRules, rule)
			slog.DebugContext(ctx, "Successfully parsed convert:rule (validator) directive",
				slog.String("dst", dstTypeInfo.FullName),
				slog.String("validator", validatorFunc))

		} else if idxUsing != -1 { // Found using=
			// Conversion rule: "<SrcT>" -> "<DstT>", using=<func>
			parts := strings.SplitN(ruleArgsText, "using=", 2)
			typesAndArrowPart := strings.TrimSpace(strings.TrimSuffix(parts[0], ","))
			usingFunc := strings.TrimSpace(parts[1])

			arrowIdx := strings.Index(typesAndArrowPart, "->")
			if arrowIdx == -1 {
				slog.WarnContext(ctx, "Malformed convert:rule (using), missing '->'", "text", ruleArgsText, "original", commentText)
				return
			}

			srcTypeRaw := strings.TrimSpace(typesAndArrowPart[:arrowIdx])
			dstTypeRaw := strings.TrimSpace(typesAndArrowPart[arrowIdx+2:]) // Comma was already trimmed from typesAndArrowPart

			srcTypeStr := strings.Trim(srcTypeRaw, "\"")
			dstTypeStr := strings.Trim(dstTypeRaw, "\"")

			slog.DebugContext(ctx, "Parsing convert:rule (using)", "srcTypeRaw", srcTypeRaw, "dstTypeRaw", dstTypeRaw, "srcTypeStr", srcTypeStr, "dstTypeStr", dstTypeStr, "usingFunc", usingFunc)

			srcTypeInfo := resolveDirectiveType(srcTypeStr, currentPkgName, currentPkgPath, fileImports, info, ctx)
			dstTypeInfo := resolveDirectiveType(dstTypeStr, currentPkgName, currentPkgPath, fileImports, info, ctx)

			if srcTypeInfo == nil || dstTypeInfo == nil || srcTypeInfo.Kind == model.KindUnknown || dstTypeInfo.Kind == model.KindUnknown {
				slog.WarnContext(ctx, "Could not resolve types for convert:rule (using), or types are unknown. Skipping.",
					slog.String("srcRule", srcTypeStr), "dstRule", dstTypeStr,
					slog.Any("srcInfo", srcTypeInfo), slog.Any("dstInfo", dstTypeInfo))
				return
			}
			if usingFunc == "" {
				slog.WarnContext(ctx, "convert:rule (using) has empty using function. Skipping.", slog.String("src", srcTypeStr), slog.String("dst", dstTypeStr))
				return
			}
			rule := model.TypeRule{
				SrcTypeName: srcTypeStr, // Store cleaned names
				DstTypeName: dstTypeStr,
				SrcTypeInfo: srcTypeInfo,
				DstTypeInfo: dstTypeInfo,
				UsingFunc:   usingFunc,
			}
			info.GlobalRules = append(info.GlobalRules, rule)
			slog.DebugContext(ctx, "Successfully parsed convert:rule (using) directive",
				slog.String("src", srcTypeInfo.FullName),
				slog.String("dst", dstTypeInfo.FullName),
				slog.String("using", usingFunc))
		} else {
			slog.WarnContext(ctx, "Skipping malformed or unsupported convert:rule directive", "text", ruleArgsText, "original", commentText)
		}
	} else {
		// Not a "pair" or "rule" directive that we understand at this level.
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
