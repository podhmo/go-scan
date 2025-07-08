package parser

import (
	"context" // Added for go-scan
	"fmt"
	"go/ast"
	"reflect" // Added for reflect.StructTag
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

// typePreParseInfo stores basic info about types collected in the first pass.
// TODO: This struct will likely be removed or heavily modified after go-scan integration.
type typePreParseInfo struct {
	name     string
	typeSpec *ast.TypeSpec
	file     *ast.File // File where the type is defined
	pkgName  string    // Package name from the file
	pkgPath  string    // Package path for this type
}

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
	// PASS 1: Collect all type definitions (structs, aliases) with initial TypeInfo
	// During this pass, TypeInfo for fields or alias underlying types are created
	// using convertScannerTypeToModelType but are not yet fully resolved against other definitions.
	for _, stypeInfo := range scanInfo.Types { // stypeInfo is *scannermodel.TypeInfo
		modelType := &model.TypeInfo{
			Name:        stypeInfo.Name,
			FullName:    fmt.Sprintf("%s.%s", currentPkgImportPath, stypeInfo.Name),
			PackageName: currentPkgName, // This is the current package's name. For external types, PkgName in TypeInfo will be different.
			PackagePath: currentPkgImportPath,
		}
		if ts, ok := stypeInfo.Node.(*ast.TypeSpec); ok {
			modelType.AstExpr = ts.Name
		} else {
			slog.WarnContext(ctx, "go-scan TypeInfo.Node was not *ast.TypeSpec for top-level type", slog.String("typeName", stypeInfo.Name))
		}

		switch stypeInfo.Kind {
		case scannermodel.StructKind:
			modelType.Kind = model.KindStruct
			if stypeInfo.Struct == nil {
				slog.ErrorContext(ctx, "StructKind TypeInfo has nil Struct detail in Pass 1", slog.String("typeName", stypeInfo.Name))
				continue
			}
			sinfo := &model.StructInfo{
				Name: stypeInfo.Name,
				Type: modelType,
			}
			for _, sfield := range stypeInfo.Struct.Fields {
				mfield := model.FieldInfo{
					Name:         sfield.Name,
					OriginalName: sfield.Name,
					TypeInfo:     convertScannerTypeToModelType(ctx, sfield.Type, currentPkgImportPath), // Initial, unresolved
					ParentStruct: sinfo,
				}
				if sfield.Tag != "" {
					tag := reflect.StructTag(sfield.Tag)
					convertTagValue := tag.Get("convert")
					mfield.Tag = parseFieldTag(convertTagValue)
				}
				sinfo.Fields = append(sinfo.Fields, mfield)
			}
			modelType.StructInfo = sinfo
			parsedInfo.Structs[sinfo.Name] = sinfo

		case scannermodel.AliasKind:
			modelType.Kind = model.KindNamed
			if stypeInfo.Underlying == nil {
				slog.ErrorContext(ctx, "AliasKind TypeInfo has nil Underlying detail in Pass 1", slog.String("typeName", stypeInfo.Name))
				// Still add it to NamedTypes so it can be potentially resolved if other types refer to it by name.
			} else {
				modelType.Underlying = convertScannerTypeToModelType(ctx, stypeInfo.Underlying, currentPkgImportPath) // Initial, unresolved
			}
			parsedInfo.NamedTypes[modelType.Name] = modelType

		case scannermodel.InterfaceKind:
			modelType.Kind = model.KindInterface
			parsedInfo.NamedTypes[modelType.Name] = modelType // Store interfaces also as NamedTypes

		case scannermodel.FuncKind:
			modelType.Kind = model.KindFunc
			// TODO: Populate func signature details if needed in model.TypeInfo
			parsedInfo.NamedTypes[modelType.Name] = modelType // Store func types also as NamedTypes

		default:
			slog.WarnContext(ctx, "Unhandled scannermodel.TypeKind in Pass 1", slog.Any("kind", stypeInfo.Kind), slog.String("typeName", stypeInfo.Name))
		}
	}

	// PASS 2: Resolve field types and alias underlying types using the fully collected parsedInfo.
	// This allows handling of forward declarations or inter-dependencies.
	for _, sinfo := range parsedInfo.Structs {
		if sinfo.Type.StructInfo == nil { // Ensure StructInfo is linked from the Type as well
			sinfo.Type.StructInfo = sinfo
		}
		for i := range sinfo.Fields {
			mfield := &sinfo.Fields[i]
			if mfield.TypeInfo != nil {
				mfield.TypeInfo = resolveTypeAgainstParsedInfo(mfield.TypeInfo, parsedInfo, currentPkgImportPath)
			}
		}
	}

	for _, ntInfo := range parsedInfo.NamedTypes {
		if ntInfo.Kind == model.KindNamed && ntInfo.Underlying != nil {
			ntInfo.Underlying = resolveTypeAgainstParsedInfo(ntInfo.Underlying, parsedInfo, currentPkgImportPath)
		}

		// Special handling for aliases that point to structs, to populate their StructInfo field
		// This makes them behave more like the struct they alias for generator logic.
		if ntInfo.Kind == model.KindNamed && ntInfo.Underlying != nil {
			// Resolve the underlying type fully to see if it's a struct
			// Use a temporary variable for resolved underlying to avoid modifying ntInfo.Underlying directly in this check phase
			fullyResolvedUnderlying := resolveTypeAgainstParsedInfo(ntInfo.Underlying, parsedInfo, currentPkgImportPath)

			// Check if the fully resolved underlying type is a struct
			var baseStructInfo *model.StructInfo
			if fullyResolvedUnderlying != nil {
				if fullyResolvedUnderlying.Kind == model.KindStruct && fullyResolvedUnderlying.StructInfo != nil {
					baseStructInfo = fullyResolvedUnderlying.StructInfo
				} else if fullyResolvedUnderlying.Kind == model.KindIdent || fullyResolvedUnderlying.Kind == model.KindNamed {
					// It might be an ident/named type that itself is a struct defined in parsedInfo.Structs
					if si, ok := parsedInfo.Structs[fullyResolvedUnderlying.Name]; ok && si.Type.PackagePath == fullyResolvedUnderlying.PackagePath {
						baseStructInfo = si
					}
				}
			}

			if baseStructInfo != nil {
				// This alias (ntInfo) effectively acts as a struct.
				// We create a StructInfo for it that mirrors the base struct.
				aliasStructInfo := &model.StructInfo{
					Name:            ntInfo.Name,         // The name of the alias
					Type:            ntInfo,              // The TypeInfo of the alias itself
					IsAlias:         true,
					UnderlyingAlias: baseStructInfo.Type, // Points to the TypeInfo of the actual struct definition
					Fields:          baseStructInfo.Fields, // Inherit fields
					// No Node for StructInfo
				}
				ntInfo.StructInfo = aliasStructInfo // Link it to the alias's TypeInfo

				// Optionally, also add this alias to parsedInfo.Structs if we want aliases-to-structs
				// to be discoverable via parsedInfo.Structs. This was previous behavior.
				// if _, exists := parsedInfo.Structs[ntInfo.Name]; !exists {
				// 	parsedInfo.Structs[ntInfo.Name] = aliasStructInfo
				// }
			}
		}
	}

	// Parse global comment directives (convert:pair, convert:rule)
	// This should ideally happen after all types are known, but resolveTypeFromString
	// has its own resolution logic that might need parsedInfo to be partially filled.
	// For now, keeping it here. If issues arise with resolving types in directives,
	// this might need to be a Pass 3 or resolveTypeFromString needs to be more robust
	// or use resolveTypeAgainstParsedInfo.
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
				parseGlobalCommentDirective(comment.Text, parsedInfo, currentFileImports, currentPkgName, currentPkgImportPath)
			}
		}
		for _, commentGroup := range fileAst.Comments {
			for _, comment := range commentGroup.List {
				parseGlobalCommentDirective(comment.Text, parsedInfo, currentFileImports, currentPkgName, currentPkgImportPath)
			}
		}
	}

	// The old parser logic (parser.ParseDir, typeSpecsToProcess, PASS1, PASS2 loops) is now fully removed.
	return parsedInfo, nil
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
			IsBasic:     stype.IsBuiltin,         // If *int, Elem is int (basic)
			IsInterface: (stype.Name == "error"), // If *error, Elem is error (interface)
			// Other IsX flags (IsPointer, IsSlice, IsMap) are false for the base element T
		}

		if elemModelType.IsBasic {
			elemModelType.Kind = model.KindBasic
			elemModelType.Name = stype.Name // e.g., "string" for *string. stype.Name is the element name for pointers.
			// Basic types don't have PkgName like "int.int", so no prefix trimming needed here based on PkgName.
			elemModelType.FullName = elemModelType.Name // FullName is same as Name for basic types
			elemModelType.PackagePath = ""              // Basic types have no package path
			elemModelType.PackageName = ""
			if stype.Name == "error" { // Special handling for error interface
				elemModelType.Kind = model.KindInterface
				// elemModelType.IsBasic is already true from stype.IsBuiltin
			}
		} else { // Identifier for the element type T
			elemModelType.Kind = model.KindIdent // Default to Ident, could be Struct, Named later if resolved
			elemModelType.Name = stype.Name      // For *foo.Bar, stype.Name is "Bar", stype.PkgName is "foo"
			elemModelType.PackageName = stype.PkgName
			elemModelType.PackagePath = stype.FullImportPath()

			if elemModelType.PackagePath != "" && elemModelType.PackagePath != currentPkgImportPath { // External package
				elemModelType.FullName = elemModelType.PackagePath + "." + elemModelType.Name
			} else if elemModelType.PackagePath == currentPkgImportPath { // Current package
				elemModelType.FullName = elemModelType.PackagePath + "." + elemModelType.Name
				elemModelType.PackageName = "" // No PkgName prefix needed for types in the same package for FullName construction
			} else { // TypeParam (PackagePath is often empty) or unresolvable
				elemModelType.FullName = elemModelType.Name // e.g. "T"
				// PackagePath and PackageName might be empty for type parameters
			}
		}
		mtype.Elem = elemModelType

		// For mtype (*T) itself:
		mtype.Name = elemModelType.Name           // The "name" of *T is often considered T's name in contexts
		mtype.FullName = "*" + elemModelType.FullName // FullName is *T_FullName
		mtype.PackageName = elemModelType.PackageName // Inherits package info from element
		mtype.PackagePath = elemModelType.PackagePath

	} else if mtype.IsSlice {
		mtype.Kind = model.KindSlice
		mtype.Elem = convertScannerTypeToModelType(ctx, stype.Elem, currentPkgImportPath)
		if mtype.Elem != nil {
			mtype.Name = mtype.Elem.Name // Name of []T is T's name
			mtype.FullName = "[]" + mtype.Elem.FullName
			mtype.PackageName = mtype.Elem.PackageName // Inherit from element
			mtype.PackagePath = mtype.Elem.PackagePath
		} else {
			slog.WarnContext(ctx, "Slice FieldType has nil Elem", slog.String("stypeName", stype.Name), slog.String("stypeFullName", stype.FullImportPath()+"."+stype.Name))
			// Fallback if Elem is nil (should not happen for valid types)
			mtype.Name = stype.Name
			mtype.FullName = "[]" + stype.FullImportPath() + "." + stype.Name // Best guess
		}
	} else if mtype.IsMap {
		mtype.Kind = model.KindMap
		mtype.Key = convertScannerTypeToModelType(ctx, stype.MapKey, currentPkgImportPath)
		mtype.Value = convertScannerTypeToModelType(ctx, stype.Elem, currentPkgImportPath) // Elem is Value for maps in scanner.FieldType
		if mtype.Key != nil && mtype.Value != nil {
			mtype.Name = fmt.Sprintf("map[%s]%s", mtype.Key.Name, mtype.Value.Name) // Simplified name
			mtype.FullName = fmt.Sprintf("map[%s]%s", mtype.Key.FullName, mtype.Value.FullName)
			// Package for map itself is not typical; depends on key/value packages.
		} else {
			slog.WarnContext(ctx, "Map FieldType has nil Key or Value", slog.String("stypeName", stype.Name))
			mtype.Name = stype.Name // Fallback
			mtype.FullName = stype.FullImportPath() + "." + stype.Name // Best guess
		}
	} else if mtype.IsBasic {
		mtype.Kind = model.KindBasic
		mtype.Name = stype.Name
		mtype.FullName = stype.Name // Basic types' FullName is their name
		mtype.PackageName = ""
		mtype.PackagePath = ""
		if stype.Name == "error" { // error is a builtin interface type
			mtype.IsInterface = true // Already set by stype.IsInterface probably
			mtype.Kind = model.KindInterface
		}
	} else { // Identifier (could be struct, named type, interface not caught by IsInterface, or type param)
		mtype.Kind = model.KindIdent // Default, might be refined by ParseDirectory's main loop for top-level types
		mtype.Name = stype.Name      // For foo.Bar, stype.Name is "Bar"

		if stype.PkgName != "" && stype.FullImportPath() != "" && stype.FullImportPath() != currentPkgImportPath {
			// External package
			mtype.PackageName = stype.PkgName // This is the alias/name used in source, e.g. "time"
			mtype.PackagePath = stype.FullImportPath() // e.g. "time"
			mtype.FullName = mtype.PackagePath + "." + mtype.Name // e.g. "time.Time"
		} else if stype.FullImportPath() == currentPkgImportPath {
			// Current package
			mtype.PackageName = "" // No package prefix for types in the current package for FullName construction
			mtype.PackagePath = currentPkgImportPath
			mtype.FullName = mtype.PackagePath + "." + mtype.Name // e.g. "example.com/mypkg.MyStruct"
		} else if stype.IsTypeParam {
			slog.DebugContext(ctx, "Encountered type parameter in convertScannerTypeToModelType", slog.String("name", stype.Name))
			mtype.PackageName = ""
			mtype.PackagePath = ""      // Type params don't belong to a package
			mtype.FullName = mtype.Name // e.g. "T"
		} else if stype.FullImportPath() == "" && stype.PkgName == "" && !stype.IsBuiltin {
			// Likely an unresolved type or a type from a package without a clear import path (e.g. vendored, or complex setup)
			// or a type defined in a file that isn't part of the main module (e.g. test files for external packages)
			// For model types (like MyInt -> int), the TypeInfo for "MyInt" (KindNamed) would have its Underlying as "int" (KindBasic).
			// If this 'else' block is hit for something like "MyInt", it implies 'MyInt' wasn't a top-level definition resolved yet.
			// This function primarily deals with field types.
			slog.WarnContext(ctx, "Type with empty PkgName and FullImportPath, not builtin or type param",
				slog.String("stype.Name", stype.Name),
				slog.String("currentPkgImportPath", currentPkgImportPath))
			mtype.PackageName = ""
			mtype.PackagePath = currentPkgImportPath // Assume current package if import path is empty and not type param
			mtype.FullName = mtype.PackagePath + "." + mtype.Name
		} else {
			// Default catch-all, should ideally not be hit if above cases are comprehensive
			slog.WarnContext(ctx, "Unhandled identifier case in convertScannerTypeToModelType",
				slog.String("stype.Name", stype.Name),
				slog.String("stype.PkgName", stype.PkgName),
				slog.String("stype.FullImportPath", stype.FullImportPath()),
				slog.Bool("stype.IsTypeParam", stype.IsTypeParam))
			mtype.PackageName = stype.PkgName
			mtype.PackagePath = stype.FullImportPath()
			if mtype.PackagePath != "" {
				mtype.FullName = mtype.PackagePath + "." + mtype.Name
			} else {
				mtype.FullName = mtype.Name // Fallback if package path is empty
			}
		}

		// Re-check for 'error' interface if it's an identifier "error" from no specific package (should be caught by IsBasic)
		if mtype.Name == "error" && mtype.PackagePath == "" && mtype.Kind != model.KindInterface {
			mtype.IsInterface = true
			mtype.Kind = model.KindInterface
			mtype.IsBasic = true // Treat 'error' as a basic type for some classification purposes
		}
	}
	return mtype
}

// derivePackagePath tries to derive an import path from a directory path.
// This is a simplified heuristic and might not work for all project layouts.
// A robust solution would involve parsing go.mod.
// TODO: Evaluate if this is still needed after go-scan integration, as scanInfo.ImportPath should be more reliable.
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

// isGoBasicType checks if a type name is a basic Go type.
// TODO: This will likely be removed as scannermodel.FieldType.IsBuiltin should be used.
func isGoBasicType(name string) bool {
	switch name {
	case "bool", "byte", "complex128", "complex64", "error", "float32", "float64", // "error" is not strictly basic, but often treated so
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
		slog.Warn("Empty type string passed to resolveTypeFromString.")
		return nil
	}

	// 1. Handle prefixes: *, [], map[]
	if strings.HasPrefix(typeStr, "*") {
		elemTypeStr := strings.TrimPrefix(typeStr, "*")
		elemTypeInfo := resolveTypeFromString(elemTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if elemTypeInfo == nil {
			slog.Warn("Could not resolve element type for pointer", slog.String("typeStr", typeStr))
			return &model.TypeInfo{Name: typeStr, FullName: typeStr, Kind: model.KindUnknown}
		}
		return &model.TypeInfo{
			Name:        elemTypeInfo.Name, // Simplified name
			FullName:    "*" + elemTypeInfo.FullName,
			Kind:        model.KindPointer,
			IsPointer:   true,
			Elem:        elemTypeInfo,
			PackageName: elemTypeInfo.PackageName, // Inherit from element
			PackagePath: elemTypeInfo.PackagePath,
		}
	}

	if strings.HasPrefix(typeStr, "[]") {
		elemTypeStr := strings.TrimPrefix(typeStr, "[]")
		elemTypeInfo := resolveTypeFromString(elemTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		if elemTypeInfo == nil {
			slog.Warn("Could not resolve element type for slice", slog.String("typeStr", typeStr))
			return &model.TypeInfo{Name: typeStr, FullName: typeStr, Kind: model.KindUnknown}
		}
		return &model.TypeInfo{
			Name:        elemTypeInfo.Name, // Simplified name
			FullName:    "[]" + elemTypeInfo.FullName,
			Kind:        model.KindSlice,
			IsSlice:     true,
			Elem:        elemTypeInfo,
			PackageName: elemTypeInfo.PackageName, // Inherit from element
			PackagePath: elemTypeInfo.PackagePath,
		}
	}

	if strings.HasPrefix(typeStr, "map[") && strings.HasSuffix(typeStr, "]") {
		// map[KeyType]ValueType
		// This parsing is simplistic and assumes no nested maps or complex types in key/value strings directly.
		inner := strings.TrimPrefix(typeStr, "map[")
		inner = strings.TrimSuffix(inner, "]")

		// Find the first ']' that correctly closes the key type, considering nested types.
		// This is hard with simple string splitting if keys can be complex (e.g. map[*pkg.Key]Value).
		// For now, assume simple key types that don't contain ']'.
		// A more robust solution would require a proper parser for type strings.
		var keyTypeStr, valTypeStr string
		bracketDepth := 0
		splitIndex := -1
		for i, char := range inner {
			if char == '[' {
				bracketDepth++
			} else if char == ']' {
				bracketDepth--
				if bracketDepth == -1 { // Found the closing bracket for the key type
					keyTypeStr = inner[:i]
					valTypeStr = inner[i+1:]
					splitIndex = i
					break
				}
			}
		}
		if splitIndex == -1 && bracketDepth == 0 { // No brackets in key, simple split e.g. "string]int"
			parts := strings.SplitN(inner, "]", 2)
			if len(parts) == 2 {
				keyTypeStr = parts[0]
				valTypeStr = parts[1]
			} else {
				slog.Warn("Could not parse map type string", slog.String("typeStr", typeStr))
				return &model.TypeInfo{Name: typeStr, FullName: typeStr, Kind: model.KindUnknown}
			}
		} else if splitIndex == -1 { // Malformed or complex key type not handled
			slog.Warn("Could not accurately parse complex key or malformed map type string", slog.String("typeStr", typeStr))
			return &model.TypeInfo{Name: typeStr, FullName: typeStr, Kind: model.KindUnknown}
		}

		keyTypeInfo := resolveTypeFromString(keyTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo)
		valTypeInfo := resolveTypeFromString(valTypeStr, currentPkgName, currentPkgPath, fileImports, parsedInfo)

		if keyTypeInfo == nil || valTypeInfo == nil {
			slog.Warn("Could not resolve key or value type for map", slog.String("typeStr", typeStr))
			return &model.TypeInfo{Name: typeStr, FullName: typeStr, Kind: model.KindUnknown}
		}
		return &model.TypeInfo{
			Name:     fmt.Sprintf("map[%s]%s", keyTypeInfo.Name, valTypeInfo.Name),
			FullName: fmt.Sprintf("map[%s]%s", keyTypeInfo.FullName, valTypeInfo.FullName),
			Kind:     model.KindMap,
			IsMap:    true,
			Key:      keyTypeInfo,
			Value:    valTypeInfo,
			// PackageName/Path for map itself isn't standard.
		}
	}

	// 2. Handle basic Go types
	// isGoBasicType is removed, so check directly
	basicTypes := map[string]bool{
		"bool": true, "byte": true, "complex128": true, "complex64": true, "error": true,
		"float32": true, "float64": true, "int": true, "int16": true, "int32": true, "int64": true,
		"int8": true, "rune": true, "string": true, "uint": true, "uint16": true, "uint32": true,
		"uint64": true, "uint8": true, "uintptr": true,
	}
	if basicTypes[typeStr] {
		kind := model.KindBasic
		isInterface := false
		if typeStr == "error" {
			kind = model.KindInterface
			isInterface = true
		}
		return &model.TypeInfo{
			Name:        typeStr,
			FullName:    typeStr,
			Kind:        kind,
			IsBasic:     true, // Even 'error' can be considered basic for this tool's purposes
			IsInterface: isInterface,
		}
	}

	// 3. Handle identifiers (local or qualified)
	parts := strings.SplitN(typeStr, ".", 2)
	if len(parts) == 1 { // Unqualified identifier (e.g., "MyType")
		name := parts[0]
		// Check if it's a known struct in the current package
		if sinfo, ok := parsedInfo.Structs[name]; ok && sinfo.Type.PackagePath == currentPkgPath {
			return sinfo.Type // Return the existing TypeInfo for the struct
		}
		// Check if it's a known named type in the current package
		if ntInfo, ok := parsedInfo.NamedTypes[name]; ok && ntInfo.PackagePath == currentPkgPath {
			return ntInfo // Return the existing TypeInfo for the named type
		}
		// If not found, assume it's a type in the current package that might be defined elsewhere or is a forward declaration.
		// This is a common case for types used in annotations before their full definition is processed by the main loop.
		slog.Debug("Unqualified type not found in parsedInfo, assuming local/unresolved", slog.String("name", name), slog.String("currentPkg", currentPkgPath))
		return &model.TypeInfo{
			Name:        name,
			FullName:    currentPkgPath + "." + name,
			PackageName: currentPkgName, // This might be empty if currentPkgName is the package name, not an alias.
			PackagePath: currentPkgPath,
			Kind:        model.KindIdent, // Could be a struct, interface, or alias not yet fully processed
		}
	} else { // Qualified identifier (e.g., "pkg.Type")
		pkgAlias := parts[0]
		typeName := parts[1]

		if pkgAlias == currentPkgName { // Check if it's referring to the current package
			// Treat as local type: look up typeName in parsedInfo
			if sinfo, ok := parsedInfo.Structs[typeName]; ok && sinfo.Type.PackagePath == currentPkgPath {
				return sinfo.Type
			}
			if ntInfo, ok := parsedInfo.NamedTypes[typeName]; ok && ntInfo.PackagePath == currentPkgPath {
				return ntInfo
			}
			slog.Debug("Qualified type referring to current package not found in parsedInfo, assuming local/unresolved", slog.String("typeName", typeName), slog.String("currentPkg", currentPkgPath))
			return &model.TypeInfo{ // Fallback for local type not yet fully processed in parsedInfo
				Name:        typeName,
				FullName:    currentPkgPath + "." + typeName,
				PackageName: "", // No package alias for current package types
				PackagePath: currentPkgPath,
				Kind:        model.KindIdent,
			}
		}

		// External package
		importedPkgPath, ok := fileImports[pkgAlias]
		if !ok {
			slog.Warn("External package alias not found in file imports", slog.String("alias", pkgAlias), slog.String("typeStr", typeStr))
			// Fallback: assume pkgAlias is the full package path if not in imports
			return &model.TypeInfo{
				Name:        typeName,
				FullName:    typeStr,
				PackageName: pkgAlias,
				PackagePath: pkgAlias, // Assuming alias is path if not found
				Kind:        model.KindIdent,
			}
		}
		return &model.TypeInfo{
			Name:        typeName,
			FullName:    importedPkgPath + "." + typeName,
			PackageName: pkgAlias,
			PackagePath: importedPkgPath,
			Kind:        model.KindIdent,
		}
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

// resolveTypeAgainstParsedInfo attempts to refine a TypeInfo object by cross-referencing
// it with globally parsed named types and structs. This is crucial for populating
// fields like 'Underlying' or 'StructInfo' for types used as fields or alias targets.
// currentPkgImportPath is passed to correctly identify types belonging to the package being parsed.
func resolveTypeAgainstParsedInfo(ti *model.TypeInfo, parsedInfo *model.ParsedInfo, currentPkgImportPath string) *model.TypeInfo {
	if ti == nil {
		return nil
	}

	// Handle pointer, slice, and map types recursively for their element/key/value types.
	if ti.IsPointer {
		resolvedElem := resolveTypeAgainstParsedInfo(ti.Elem, parsedInfo, currentPkgImportPath)
		// If the element type was resolved to a more specific one (e.g., different instance or more fields populated),
		// create a new pointer TypeInfo wrapper around it.
		if resolvedElem != nil && resolvedElem != ti.Elem { // Check for actual change to avoid infinite recursion on non-change
			newPtrType := *ti // Shallow copy
			newPtrType.Elem = resolvedElem
			// Update other fields of newPtrType if necessary based on resolvedElem (e.g., FullName)
			newPtrType.FullName = "*" + resolvedElem.FullName
			// Ensure PackageName and PackagePath are consistent with the element
			newPtrType.PackageName = resolvedElem.PackageName
			newPtrType.PackagePath = resolvedElem.PackagePath
			return &newPtrType
		}
		return ti // Return original if element didn't change or couldn't be resolved further
	}

	if ti.IsSlice {
		resolvedElem := resolveTypeAgainstParsedInfo(ti.Elem, parsedInfo, currentPkgImportPath)
		if resolvedElem != nil && resolvedElem != ti.Elem {
			newSliceType := *ti // Shallow copy
			newSliceType.Elem = resolvedElem
			newSliceType.FullName = "[]" + resolvedElem.FullName
			newSliceType.PackageName = resolvedElem.PackageName
			newSliceType.PackagePath = resolvedElem.PackagePath
			return &newSliceType
		}
		return ti
	}

	if ti.IsMap {
		resolvedKey := resolveTypeAgainstParsedInfo(ti.Key, parsedInfo, currentPkgImportPath)
		resolvedValue := resolveTypeAgainstParsedInfo(ti.Value, parsedInfo, currentPkgImportPath)
		// Check if either key or value type was meaningfully changed
		keyChanged := resolvedKey != nil && resolvedKey != ti.Key
		valueChanged := resolvedValue != nil && resolvedValue != ti.Value

		if keyChanged || valueChanged {
			newMapType := *ti // Shallow copy
			if keyChanged {
				newMapType.Key = resolvedKey
			}
			if valueChanged {
				newMapType.Value = resolvedValue
			}
			newMapType.FullName = fmt.Sprintf("map[%s]%s", newMapType.Key.FullName, newMapType.Value.FullName)
			// Package info for maps is tricky; usually determined by key/value types.
			return &newMapType
		}
		return ti
	}

	// For identifiers (KindIdent) or already somewhat resolved named types (KindNamed),
	// try to find a more complete definition from parsedInfo.
	// This is where we link up with top-level type definitions (structs, aliases).
	// We should only do this if the type 'ti' is from the current package,
	// because parsedInfo.NamedTypes and parsedInfo.Structs are keyed by simple name
	// and store definitions for the current package.
	if ti.Kind == model.KindIdent || ti.Kind == model.KindNamed {
		isLocalType := ti.PackagePath == currentPkgImportPath
		// Heuristic for types parsed without full path info but are local (and not basic types)
		if ti.PackagePath == "" && !ti.IsBasic {
			// This condition might also be true for type parameters,
			// but type parameters wouldn't be found in parsedInfo.NamedTypes or parsedInfo.Structs anyway.
			isLocalType = true
		}

		if isLocalType {
			// Check NamedTypes first (aliases, other named types like enums if they were KindNamed)
			if nt, ok := parsedInfo.NamedTypes[ti.Name]; ok {
				// Return the fully resolved one from NamedTypes, which includes .Underlying
				return nt
			}
			// Then check Structs (in case it's a struct name not an alias)
			if st, ok := parsedInfo.Structs[ti.Name]; ok {
				// Return the TypeInfo associated with the StructInfo
				return st.Type
			}
		}
	}

	return ti // Return original if no specific resolution found or type is not local ident/named
}
