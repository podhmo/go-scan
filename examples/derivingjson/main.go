package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	// "go/format" // No longer needed here, handled by SaveGoFile
	"log/slog"
	"os"
	"path/filepath"
	// "reflect" // No longer needed
	"strings"
	"text/template"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

//go:embed unmarshal.tmpl
var templateFile embed.FS

func main() {
	// Add this block to enable debug logging
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug)
	opts := slog.HandlerOptions{
		Level: logLevel,
	}
	handler := slog.NewTextHandler(os.Stderr, &opts)
	slog.SetDefault(slog.New(handler))
	// End of debug logging setup

	ctx := context.Background() // Or your application's context
	if len(os.Args) <= 1 {
		slog.ErrorContext(ctx, "Usage: derivingjson <file_path_1> [file_path_2 ...]")
		slog.ErrorContext(ctx, "Example: derivingjson examples/derivingjson/testdata/simple/models.go examples/derivingjson/testdata/separated/models/models.go")
		os.Exit(1)
	}

	targetFiles := os.Args[1:]
	processedDirs := make(map[string]bool)
	var successCount int
	var errorCount int

	for _, filePath := range targetFiles {
		// Ensure the file path exists and is a file
		stat, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				slog.ErrorContext(ctx, "File path does not exist", slog.String("file_path", filePath))
			} else {
				slog.ErrorContext(ctx, "Error accessing file path", slog.String("file_path", filePath), slog.Any("error", err))
			}
			errorCount++
			continue
		}
		if stat.IsDir() {
			slog.ErrorContext(ctx, "File path is a directory, please provide individual .go files", slog.String("file_path", filePath))
			errorCount++
			continue
		}
		if !strings.HasSuffix(filePath, ".go") {
			slog.ErrorContext(ctx, "File path is not a .go file", slog.String("file_path", filePath))
			errorCount++
			continue
		}

		pkgPath := filepath.Dir(filePath)
		if _, processed := processedDirs[pkgPath]; processed {
			slog.InfoContext(ctx, "Package already processed, skipping generation for this file's package", slog.String("package_path", pkgPath), slog.String("file_path", filePath))
			continue
		}

		// Ensure the derived package path (directory) exists
		dirStat, err := os.Stat(pkgPath)
		if err != nil {
			slog.ErrorContext(ctx, "Error accessing package directory", slog.String("package_path", pkgPath), slog.String("derived_from_file", filePath), slog.Any("error", err))
			errorCount++
			continue
		}
		if !dirStat.IsDir() {
			// This case should ideally not be reached if filePath was a valid file.
			slog.ErrorContext(ctx, "Derived package path is not a directory", slog.String("package_path", pkgPath), slog.String("derived_from_file", filePath))
			errorCount++
			continue
		}

		slog.InfoContext(ctx, "Generating UnmarshalJSON for package", slog.String("package_path", pkgPath), slog.String("triggered_by_file", filePath))
		if err := Generate(ctx, pkgPath); err != nil {
			slog.ErrorContext(ctx, "Error generating code for package", slog.String("package_path", pkgPath), slog.Any("error", err))
			errorCount++
		} else {
			slog.InfoContext(ctx, "Successfully generated UnmarshalJSON methods for package", slog.String("package_path", pkgPath))
			successCount++
		}
		processedDirs[pkgPath] = true
	}

	slog.InfoContext(ctx, "Generation summary", slog.Int("successful_packages", successCount), slog.Int("failed_packages/files", errorCount))
	if errorCount > 0 {
		os.Exit(1)
	}
}

const unmarshalAnnotation = "@deriving:unmarshall"

type TemplateData struct {
	// PackageName string // Will be set in GoFile
	StructName                 string
	OtherFields                []FieldInfo
	OneOfFields                []OneOfFieldDetail
	Imports                    map[string]string // This will be collected and passed to GoFile
	DiscriminatorFieldJSONName string            // Assuming this is global for the struct for now
}

type FieldInfo struct {
	Name    string
	Type    string
	JSONTag string
}

// OneOfFieldDetail holds information for a single oneOf field
type OneOfFieldDetail struct {
	FieldName    string             // Name of the field in the struct (e.g., "ShapeData", "EventPayload")
	FieldType    string             // Go type of the interface (e.g., "shapes.Shape", "events.Event")
	JSONTag      string             // JSON tag name (e.g., "shape_data", "payload")
	Implementers []OneOfTypeMapping // Mappings of JSON discriminator value to concrete Go type
}

type OneOfTypeMapping struct {
	JSONValue string // The value in the discriminator field (e.g., "circle", "user_created")
	GoType    string // The concrete Go type (e.g., "*shapes.Circle", "*events.UserCreatedEvent")
}

func findTypeInPackage(pkgInfo *scanner.PackageInfo, typeName string) *scanner.TypeInfo {
	for _, t := range pkgInfo.Types {
		if t.Name == typeName {
			return t
		}
	}
	return nil
}

func Generate(ctx context.Context, pkgPath string) error {
	gscn, err := goscan.New(".")
	if err != nil {
		return fmt.Errorf("failed to create go-scan scanner: %w", err)
	}

	pkgInfo, err := gscn.ScanPackage(ctx, pkgPath)
	if err != nil {
		return fmt.Errorf("go-scan failed to scan package at %s: %w", pkgPath, err)
	}

	var generatedCodeForAllStructs bytes.Buffer
	collectedImports := make(map[string]string) // path -> alias. Used to populate GoFile.Imports

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}
		// Use the new Annotation method to check for the unmarshal annotation
		if _, ok := typeInfo.Annotation("deriving:unmarshall"); !ok {
			continue
		}

		// Imports for this specific struct's generation, will be merged into collectedImports
		structSpecificImports := make(map[string]string)

		data := TemplateData{
			// PackageName: pkgInfo.Name, // No longer set here
			StructName:                 typeInfo.Name,
			Imports:                    structSpecificImports, // Pass this map to collect imports for this struct
			OneOfFields:                []OneOfFieldDetail{},
			OtherFields:                []FieldInfo{},
			DiscriminatorFieldJSONName: "type", // Hardcoded for now
		}

		for _, field := range typeInfo.Struct.Fields {
			// Use the new TagValue method
			jsonTag := field.TagValue("json")

			resolvedFieldType, errResolve := field.Type.Resolve(ctx)
			if errResolve != nil {
				if field.Type.PkgName == "" || field.Type.PkgName == pkgInfo.Name {
					resolvedFieldType = findTypeInPackage(pkgInfo, field.Type.Name)
				}
				if resolvedFieldType == nil && !field.Type.IsBuiltin { // if still nil after local lookup or if external, and not a builtin
				}
			}

			isInterfaceField := false
			if resolvedFieldType != nil && resolvedFieldType.Kind == scanner.InterfaceKind {
				isInterfaceField = true
			} else if resolvedFieldType == nil && strings.Contains(field.Type.Name, "interface{") { // Heuristic for anonymous interfaces, though less robust
			}

			if isInterfaceField {
				oneOfDetail := OneOfFieldDetail{
					FieldName:    field.Name,
					JSONTag:      jsonTag,
					Implementers: []OneOfTypeMapping{},
				}

				interfaceDef := resolvedFieldType
				fieldTypeString := interfaceDef.Name
				var interfaceDefiningPkgImportPath string
				var interfaceDefiningPkgNameForImport string

				if field.Type.FullImportPath() != "" && field.Type.FullImportPath() != pkgInfo.ImportPath {
					interfaceDefiningPkgImportPath = field.Type.FullImportPath()
					interfaceDefiningPkgNameForImport = field.Type.PkgName
					fieldTypeString = interfaceDefiningPkgNameForImport + "." + interfaceDef.Name
					structSpecificImports[interfaceDefiningPkgImportPath] = interfaceDefiningPkgNameForImport // Use PkgName as alias

				} else if interfaceDef.FilePath != "" {
					interfaceDir := filepath.Dir(interfaceDef.FilePath)
					scannedPkgForInterfaceFile, errPkgScan := gscn.ScanPackage(ctx, interfaceDir)

					if errPkgScan == nil && scannedPkgForInterfaceFile != nil && scannedPkgForInterfaceFile.ImportPath != "" {
						if scannedPkgForInterfaceFile.ImportPath != pkgInfo.ImportPath {
							interfaceDefiningPkgImportPath = scannedPkgForInterfaceFile.ImportPath
							interfaceDefiningPkgNameForImport = scannedPkgForInterfaceFile.Name
							fieldTypeString = interfaceDefiningPkgNameForImport + "." + interfaceDef.Name
							structSpecificImports[interfaceDefiningPkgImportPath] = interfaceDefiningPkgNameForImport // Use actual package name as alias
						} else {
							interfaceDefiningPkgImportPath = pkgInfo.ImportPath
							interfaceDefiningPkgNameForImport = pkgInfo.Name
						}
					} else {
					}
				} else {
					interfaceDefiningPkgImportPath = pkgInfo.ImportPath
					interfaceDefiningPkgNameForImport = pkgInfo.Name
				}
				oneOfDetail.FieldType = fieldTypeString

				searchPkgs := []*scanner.PackageInfo{pkgInfo}
				if interfaceDefiningPkgImportPath != "" && interfaceDefiningPkgImportPath != pkgInfo.ImportPath {
					scannedInterfacePkg, errScan := gscn.ScanPackageByImport(ctx, interfaceDefiningPkgImportPath)
					if errScan == nil && scannedInterfacePkg != nil {
						alreadyAdded := false
						for _, sp := range searchPkgs {
							if sp.ImportPath == scannedInterfacePkg.ImportPath {
								alreadyAdded = true
								break
							}
						}
						if !alreadyAdded {
							searchPkgs = append(searchPkgs, scannedInterfacePkg)
						}
					} else {
					}
				}

				foundImplementersForThisInterface := false
				processedImplementerKeys := make(map[string]bool)

				for _, currentSearchPkg := range searchPkgs {
					if currentSearchPkg == nil {
						continue
					}
					for _, candidateType := range currentSearchPkg.Types {
						if candidateType.Kind != scanner.StructKind || candidateType.Struct == nil {
							continue
						}

						implementerKey := candidateType.FilePath + "::" + candidateType.Name
						if processedImplementerKeys[implementerKey] {
							continue
						}
						implementsResult := goscan.Implements(candidateType, interfaceDef, currentSearchPkg)

						if implementsResult {
							processedImplementerKeys[implementerKey] = true
							foundImplementersForThisInterface = true

							discriminatorValue := strings.ToLower(candidateType.Name)
							if candidateType.Name == "Circle" {
								discriminatorValue = "circle"
							} else if candidateType.Name == "Rectangle" {
								discriminatorValue = "rectangle"
							} else {
							}

							goTypeString := candidateType.Name
							if currentSearchPkg.ImportPath != "" && currentSearchPkg.ImportPath != pkgInfo.ImportPath {
								goTypeString = currentSearchPkg.Name + "." + candidateType.Name
								// Add import for the implementer's package
								structSpecificImports[currentSearchPkg.ImportPath] = currentSearchPkg.Name
							}
							if !strings.HasPrefix(goTypeString, "*") {
								goTypeString = "*" + goTypeString
							}
							oneOfDetail.Implementers = append(oneOfDetail.Implementers, OneOfTypeMapping{
								JSONValue: discriminatorValue,
								GoType:    goTypeString,
							})
						}
					}
				}
				if !foundImplementersForThisInterface {
					warnPath := interfaceDefiningPkgImportPath
					if warnPath == "" {
						warnPath = pkgInfo.ImportPath
					}
				}
				data.OneOfFields = append(data.OneOfFields, oneOfDetail)

			} else { // Other fields
				typeName := field.Type.String()
				if resolvedFieldType != nil && resolvedFieldType.FilePath != "" {
					fieldDir := filepath.Dir(resolvedFieldType.FilePath)
					// Avoid re-scanning current package or already known ones if possible.
					// For simplicity here, just scan. This might be optimized.
					fieldDefiningPkg, errPkgScan := gscn.ScanPackage(ctx, fieldDir)
					if errPkgScan == nil && fieldDefiningPkg != nil && fieldDefiningPkg.ImportPath != "" {
						if fieldDefiningPkg.ImportPath != pkgInfo.ImportPath {
							structSpecificImports[fieldDefiningPkg.ImportPath] = fieldDefiningPkg.Name
						}
					}
				} else if field.Type.PkgName != "" && field.Type.PkgName != pkgInfo.Name && field.Type.FullImportPath() != "" {
					// Fallback using FieldType's PkgName and FullImportPath if available
					structSpecificImports[field.Type.FullImportPath()] = field.Type.PkgName
				}
				data.OtherFields = append(data.OtherFields, FieldInfo{Name: field.Name, Type: typeName, JSONTag: jsonTag})
			}
		}

		if len(data.OneOfFields) == 0 {
			continue
		}

		tmpl, err := template.ParseFS(templateFile, "unmarshal.tmpl")
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}
		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, data); err != nil {
			return fmt.Errorf("failed to execute template for struct %s: %w", typeInfo.Name, err)
		}
		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")

		// Merge struct-specific imports into collectedImports
		for path, alias := range structSpecificImports {
			existingAlias, ok := collectedImports[path]
			if ok && existingAlias != alias && alias != "" {
				// Handle potential alias conflicts, e.g. log a warning or prefer one.
				// For now, let's overwrite if the new alias is not empty.
				slog.WarnContext(ctx, "Import alias conflict", slog.String("path", path), slog.String("existing_alias", existingAlias), slog.String("new_alias", alias))
			}
			// Add if new alias is non-empty, or if path not present, or if existing alias is different and new one is not empty
			if alias != "" || !ok {
				collectedImports[path] = alias
			} else if ok && existingAlias == "" && alias == "" { // both empty, ensure path is present
				collectedImports[path] = ""
			}
		}
		// Ensure "encoding/json" and "fmt" are added if any code was generated
		if generatedCodeForAllStructs.Len() > 0 {
			collectedImports["encoding/json"] = ""
			collectedImports["fmt"] = ""
		}
	}

	if generatedCodeForAllStructs.Len() == 0 {
		return nil
	}

	// Use PackageDirectory to save the file
	outputDir := goscan.NewPackageDirectory(pkgPath, pkgInfo.Name) // pkgInfo.Name is the default package name
	goFile := goscan.GoFile{
		PackageName: pkgInfo.Name,
		Imports:     collectedImports,
		CodeSet:     generatedCodeForAllStructs.String(),
	}

	outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(pkgInfo.Name))
	// Temporary: Print generated code for debugging
	// Ensure you have this block:
	// formattedCode, err := format.Source(finalOutput.Bytes())
	// if err != nil { ... }
	// Then print formattedCode before writing.
	// This requires moving format.Source back into this function or printing unformatted code from goFile.CodeSet.
	// For simplicity, let's print the unformatted codeset from goFile, assuming SaveGoFile handles formatting.

	if err := outputDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
		// SaveGoFile now handles formatting and logging, so we just return the error.
		return fmt.Errorf("failed to save generated file for package %s: %w", pkgInfo.Name, err)
	}
	return nil
}
