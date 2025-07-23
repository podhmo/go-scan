package main

import (
	"bytes"
	"context"
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

//go:embed unmarshal.tmpl
var templateFile embed.FS

func main() {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug)
	opts := slog.HandlerOptions{Level: logLevel}
	handler := slog.NewTextHandler(os.Stderr, &opts)
	slog.SetDefault(slog.New(handler))

	var cwd string
	flag.StringVar(&cwd, "cwd", ".", "current working directory")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: derivingjson [options] <file_or_dir_path_1> [file_or_dir_path_2 ...]\n")
		fmt.Fprintf(os.Stderr, "Example (file): derivingjson examples/derivingjson/testdata/simple/models.go\n")
		fmt.Fprintf(os.Stderr, "Example (dir):  derivingjson examples/derivingjson/testdata/simple/\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	ctx := context.Background()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	gscn, err := goscan.New(goscan.WithWorkDir(cwd))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create go-scan scanner", slog.Any("error", err))
		os.Exit(1)
	}

	filesByPackage := make(map[string][]string)
	dirsToScan := []string{}

	for _, path := range flag.Args() {
		stat, err := os.Stat(path)
		if err != nil {
			slog.ErrorContext(ctx, "Error accessing path", slog.String("path", path), slog.Any("error", err))
			continue
		}
		if stat.IsDir() {
			dirsToScan = append(dirsToScan, path)
		} else if strings.HasSuffix(path, ".go") {
			pkgDir := filepath.Dir(path)
			filesByPackage[pkgDir] = append(filesByPackage[pkgDir], path)
		} else {
			slog.WarnContext(ctx, "Argument is not a .go file or directory, skipping", slog.String("path", path))
		}
	}

	var successCount, errorCount int

	// Process directories
	for _, dirPath := range dirsToScan {
		slog.InfoContext(ctx, "Scanning directory", "path", dirPath)
		pkgInfo, err := gscn.ScanPackage(ctx, dirPath)
		if err != nil {
			slog.ErrorContext(ctx, "Error scanning package", "path", dirPath, slog.Any("error", err))
			errorCount++
			continue
		}
		if err := Generate(ctx, gscn, pkgInfo); err != nil {
			slog.ErrorContext(ctx, "Error generating code for package", "path", dirPath, slog.Any("error", err))
			errorCount++
		} else {
			slog.InfoContext(ctx, "Successfully generated UnmarshalJSON for package", "path", dirPath)
			successCount++
		}
	}

	// Process file groups
	for pkgDir, filePaths := range filesByPackage {
		slog.InfoContext(ctx, "Scanning files in package", "package", pkgDir, "files", filePaths)
		// Note: ScanFiles requires the package directory to be passed explicitly.
		pkgInfo, err := gscn.ScanFiles(ctx, filePaths)
		if err != nil {
			slog.ErrorContext(ctx, "Error scanning files", "package", pkgDir, slog.Any("error", err))
			errorCount++
			continue
		}
		if err := Generate(ctx, gscn, pkgInfo); err != nil {
			slog.ErrorContext(ctx, "Error generating code for files", "package", pkgDir, slog.Any("error", err))
			errorCount++
		} else {
			slog.InfoContext(ctx, "Successfully generated UnmarshalJSON for package", "package", pkgDir)
			successCount++
		}
	}

	slog.InfoContext(ctx, "Generation summary", slog.Int("successful_packages", successCount), slog.Int("failed_packages/files", errorCount))
	if errorCount > 0 {
		os.Exit(1)
	}
}

const unmarshalAnnotation = "deriving:unmarshal"

type TemplateData struct {
	StructName                 string
	OtherFields                []FieldInfo
	OneOfFields                []OneOfFieldDetail
	DiscriminatorFieldJSONName string
}

type FieldInfo struct {
	Name    string
	Type    string
	JSONTag string
}

type OneOfFieldDetail struct {
	FieldName    string
	FieldType    string
	JSONTag      string
	Implementers []OneOfTypeMapping
}

type OneOfTypeMapping struct {
	JSONValue string
	GoType    string
}

func findTypeInPackage(pkgInfo *scanner.PackageInfo, typeName string) *scanner.TypeInfo {
	for _, t := range pkgInfo.Types {
		if t.Name == typeName {
			return t
		}
	}
	return nil
}

func Generate(ctx context.Context, gscn *goscan.Scanner, pkgInfo *scanner.PackageInfo) error {
	if pkgInfo == nil {
		return fmt.Errorf("cannot generate code for a nil package")
	}
	pkgPath := pkgInfo.Path
	importManager := goscan.NewImportManager(pkgInfo)
	var generatedCodeForAllStructs bytes.Buffer
	anyCodeGenerated := false

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}
		if _, ok := typeInfo.Annotation(unmarshalAnnotation); !ok {
			continue
		}

		data := TemplateData{
			StructName:                 typeInfo.Name,
			OneOfFields:                []OneOfFieldDetail{},
			OtherFields:                []FieldInfo{},
			DiscriminatorFieldJSONName: "type", // Default discriminator
		}

		for _, field := range typeInfo.Struct.Fields {
			jsonTag := field.TagValue("json")
			var resolvedFieldType *scanner.TypeInfo
			if field.Type.FullImportPath() == "" {
				resolvedFieldType = findTypeInPackage(pkgInfo, field.Type.Name)
			} else {
				resolvedFieldType, _ = gscn.ResolveType(ctx, field.Type)
			}

			isInterfaceField := false
			if resolvedFieldType != nil && resolvedFieldType.Kind == scanner.InterfaceKind {
				isInterfaceField = true
			} else if resolvedFieldType == nil && strings.Contains(field.Type.Name, "interface{") { // Heuristic for anonymous interfaces
				isInterfaceField = true
				// For anonymous interfaces, we might not have a resolvedFieldType.
				// We'll use field.Type.Name directly, assuming it's defined in the current package or is a standard type.
				// This part might need more robust handling for anonymous interfaces from other packages.
			}

			if isInterfaceField {
				oneOfDetail := OneOfFieldDetail{
					FieldName:    field.Name,
					JSONTag:      jsonTag,
					Implementers: []OneOfTypeMapping{},
				}

				var interfaceDef *scanner.TypeInfo = resolvedFieldType
				var interfaceDefiningPkgImportPath string

				if interfaceDef != nil { // Resolved interface
					if field.Type.FullImportPath() != "" && field.Type.FullImportPath() != pkgInfo.ImportPath {
						interfaceDefiningPkgImportPath = field.Type.FullImportPath()
					} else if interfaceDef.FilePath != "" {
						interfaceDir := filepath.Dir(interfaceDef.FilePath)
						scannedPkgForInterfaceFile, errPkgScan := gscn.ScanPackage(ctx, interfaceDir)
						if errPkgScan == nil && scannedPkgForInterfaceFile != nil && scannedPkgForInterfaceFile.ImportPath != "" {
							interfaceDefiningPkgImportPath = scannedPkgForInterfaceFile.ImportPath
						} else {
							interfaceDefiningPkgImportPath = pkgInfo.ImportPath // Fallback
							if errPkgScan != nil {
								slog.WarnContext(ctx, "Could not determine import path for interface's defining package, falling back.", "interfaceName", interfaceDef.Name, "filePath", interfaceDef.FilePath, "error", errPkgScan)
							}
						}
					} else {
						interfaceDefiningPkgImportPath = pkgInfo.ImportPath
					}
					oneOfDetail.FieldType = importManager.Qualify(interfaceDefiningPkgImportPath, interfaceDef.Name)
				} else { // Likely an anonymous interface string like "interface{...}"
					oneOfDetail.FieldType = field.Type.Name             // Use as is, assuming current package or built-in.
					interfaceDefiningPkgImportPath = pkgInfo.ImportPath // Assume current package for searching implementers initially
					// Attempt to parse the anonymous interface definition to find its methods (complex, not done here)
					// For now, searching for implementers of anonymous interfaces might be limited.
					// A proper solution would involve parsing the anonymous interface string.
					// Let's assume for now if resolvedFieldType is nil for an interface, it's an anonymous one from current pkg.
					// This is a simplification.
					slog.DebugContext(ctx, "Handling field as anonymous interface", "fieldName", field.Name, "fieldType", field.Type.Name)
					// Create a temporary TypeInfo for the anonymous interface for Implements check if possible
					// This is non-trivial. For now, implementer search might not work well for these.
					// Let's try to proceed but acknowledge limitations.
					// We need a valid interfaceDef for goscan.Implements.
					// If we can't get one, we can't find implementers.
					// For this example, we'll assume if interfaceDef is nil here, we cannot find implementers.
					if interfaceDef == nil {
						slog.WarnContext(ctx, "Cannot find implementers for anonymous interface field without a resolved TypeInfo", "fieldName", field.Name, "fieldType", field.Type.Name)
						data.OneOfFields = append(data.OneOfFields, oneOfDetail) // Add with potentially empty implementers
						continue                                                 // Skip implementer search for this field
					}
				}

				searchPkgs := []*scanner.PackageInfo{pkgInfo}
				if interfaceDefiningPkgImportPath != "" && interfaceDefiningPkgImportPath != pkgInfo.ImportPath {
					scannedInterfacePkg, errScan := gscn.ScanPackageByImport(ctx, interfaceDefiningPkgImportPath)
					if errScan == nil && scannedInterfacePkg != nil {
						if !isPackageInSlice(searchPkgs, scannedInterfacePkg.ImportPath) {
							searchPkgs = append(searchPkgs, scannedInterfacePkg)
						}
					} else if errScan != nil {
						slog.WarnContext(ctx, "Failed to scan interface's defining package", "importPath", interfaceDefiningPkgImportPath, "error", errScan)
					}
				}

				processedImplementerKeys := make(map[string]bool)
				for _, currentSearchPkg := range searchPkgs {
					if currentSearchPkg == nil {
						continue
					}
					for _, candidateType := range currentSearchPkg.Types {
						if candidateType.Kind != scanner.StructKind || candidateType.Struct == nil {
							continue
						}
						// Use currentSearchPkg.ImportPath for the package part of the key
						implementerKey := currentSearchPkg.ImportPath + "." + candidateType.Name
						if processedImplementerKeys[implementerKey] {
							continue
						}
						if goscan.Implements(candidateType, interfaceDef, currentSearchPkg) {
							processedImplementerKeys[implementerKey] = true
							var goTypeString string
							if currentSearchPkg.ImportPath != "" && currentSearchPkg.ImportPath != pkgInfo.ImportPath {
								goTypeString = importManager.Qualify(currentSearchPkg.ImportPath, candidateType.Name)
							} else {
								goTypeString = candidateType.Name
							}
							if !strings.HasPrefix(goTypeString, "*") {
								goTypeString = "*" + goTypeString
							}
							// Simplified discriminator value
							discriminatorValue := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(candidateType.Name, "*"), "Event"))

							oneOfDetail.Implementers = append(oneOfDetail.Implementers, OneOfTypeMapping{
								JSONValue: discriminatorValue,
								GoType:    goTypeString,
							})
						}
					}
				}
				data.OneOfFields = append(data.OneOfFields, oneOfDetail)

			} else { // Other fields
				var typeName string
				if resolvedFieldType != nil && resolvedFieldType.Name != "" { // resolvedFieldType.Name can be empty for unnamed types like `struct{}`
					definingPkgPath := pkgInfo.ImportPath // Default to current package
					if resolvedFieldType.FilePath != "" { // If file path is known, try to get its package
						dir := filepath.Dir(resolvedFieldType.FilePath)
						definingActualPkg, errScan := gscn.ScanPackage(ctx, dir) // Might hit cache
						if errScan == nil && definingActualPkg != nil && definingActualPkg.ImportPath != "" {
							definingPkgPath = definingActualPkg.ImportPath
						} else if errScan != nil {
							slog.DebugContext(ctx, "Could not scan package for resolved field type, using current package for qualification.", "field", field.Name, "resolvedTypeName", resolvedFieldType.Name, "error", errScan)
						}
					} else if field.Type.FullImportPath() != "" { // Fallback to FieldType's import path if FilePath on resolved is empty
						definingPkgPath = field.Type.FullImportPath()
					}
					typeName = importManager.Qualify(definingPkgPath, resolvedFieldType.Name)
				} else { // Fallback to original FieldType info if resolution failed or name is empty
					typeName = importManager.Qualify(field.Type.FullImportPath(), field.Type.Name)
				}
				data.OtherFields = append(data.OtherFields, FieldInfo{Name: field.Name, Type: typeName, JSONTag: jsonTag})
			}
		}

		if len(data.OneOfFields) == 0 {
			continue
		}
		anyCodeGenerated = true // Mark that at least one struct will generate code

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
	}

	if !anyCodeGenerated { // If no structs produced any code (e.g. no OneOfFields)
		slog.InfoContext(ctx, "No structs found requiring UnmarshalJSON generation in package", slog.String("package_path", pkgPath))
		return nil
	}

	// Add common imports only if code was generated
	importManager.Add("encoding/json", "")
	importManager.Add("fmt", "")

	outputDir := goscan.NewPackageDirectory(pkgPath, pkgInfo.Name)
	goFile := goscan.GoFile{
		PackageName: pkgInfo.Name,
		Imports:     importManager.Imports(),
		CodeSet:     generatedCodeForAllStructs.String(),
	}

	outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(pkgInfo.Name))
	if err := outputDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
		return fmt.Errorf("failed to save generated file for package %s: %w", pkgInfo.Name, err)
	}
	return nil
}

func isPackageInSlice(slice []*scanner.PackageInfo, importPath string) bool {
	for _, p := range slice {
		if p.ImportPath == importPath {
			return true
		}
	}
	return false
}
