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
	"reflect"
	"strings"
	"text/template"

	"go/ast"
	"go/parser"
	"go/token"
	// "go/types" // Will be needed for type resolution

	"github.com/podhmo/go-scan/astwalk"
	goscan "github.com/podhmo/go-scan"
	// "github.com/podhmo/go-scan/scanner" // No longer directly used in Generate
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
	// Group files by package directory
	filesByPkgDir := make(map[string][]string)
	for _, filePath := range targetFiles {
		stat, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				slog.ErrorContext(ctx, "File path does not exist", slog.String("file_path", filePath))
			} else {
				slog.ErrorContext(ctx, "Error accessing file path", slog.String("file_path", filePath), slog.Any("error", err))
			}
			// Do not exit, just skip this file and count as error later if needed
			continue
		}
		if stat.IsDir() {
			slog.ErrorContext(ctx, "File path is a directory, please provide individual .go files", slog.String("file_path", filePath))
			continue
		}
		if !strings.HasSuffix(filePath, ".go") {
			slog.ErrorContext(ctx, "File path is not a .go file", slog.String("file_path", filePath))
			continue
		}
		pkgDir := filepath.Dir(filePath)
		filesByPkgDir[pkgDir] = append(filesByPkgDir[pkgDir], filePath)
	}


	var successCount int
	var errorCount int

	for pkgPath, filesInPkg := range filesByPkgDir {
		slog.InfoContext(ctx, "Generating UnmarshalJSON for package", slog.String("package_path", pkgPath), slog.Any("files", filesInPkg))
		// Pass all files in the package to Generate
		if err := Generate(ctx, pkgPath, filesInPkg); err != nil {
			slog.ErrorContext(ctx, "Error generating code for package", slog.String("package_path", pkgPath), slog.Any("error", err))
			errorCount++
		} else {
			slog.InfoContext(ctx, "Successfully generated UnmarshalJSON methods for package", slog.String("package_path", pkgPath))
			successCount++
		}
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

// Helper to get type string from ast.Expr
func getTypeString(expr ast.Expr) string {
	// This is a simplified version. A full version would handle
	// *ast.StarExpr, *ast.SelectorExpr, *ast.ArrayType, *ast.MapType, etc.
	// and potentially need to look up package aliases from imports.
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr: // For types like pkg.Type
		return fmt.Sprintf("%s.%s", getTypeString(t.X), t.Sel.Name)
	case *ast.StarExpr: // For pointer types like *MyType
		return "*" + getTypeString(t.X)
	case *ast.InterfaceType:
		// For anonymous interfaces, this might be complex.
		// For now, let's assume named interfaces or simple cases.
		// A real implementation might return "interface{}" or a more detailed representation.
		// For this refactoring, we are more interested in named interfaces.
		return "interface{}" // Placeholder
	default:
		return fmt.Sprintf("%T", expr) // Fallback, not ideal for code gen
	}
}


func Generate(ctx context.Context, pkgPath string, filePaths []string) error {
	fset := token.NewFileSet()
	var files []*ast.File
	var firstFilePkgName string // To store the package name from the first parsed file

	for _, filePath := range filePaths {
		file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("failed to parse file %s: %w", filePath, err)
		}
		files = append(files, file)
		if firstFilePkgName == "" && file.Name != nil {
			firstFilePkgName = file.Name.Name
		}
	}

	if firstFilePkgName == "" {
		// Fallback to directory name if no package name could be determined
		// This could happen if all files are empty or only contain comments without package decl
		firstFilePkgName = filepath.Base(pkgPath)
		slog.WarnContext(ctx, "Could not determine package name from parsed files, using directory name", "package_path", pkgPath, "fallback_pkg_name", firstFilePkgName)
	}


	var generatedCodeForAllStructs bytes.Buffer
	collectedImports := make(map[string]string) // path -> alias. Used to populate GoFile.Imports

	// TODO: Initialize types.Importer for resolving types across packages
	// conf := types.Config{Importer: importer.Default()}
	// pkgScope := types.NewPackage(pkgPath, firstFilePkgName) // This might need adjustment based on how types.Checker works

	for _, file := range files { // Iterate over each parsed file AST
		if file == nil { // Should not happen if parser.ParseFile succeeded
			continue
		}
		// Update collectedImports with imports from the current file
		for _, importSpec := range file.Imports {
			path := strings.Trim(importSpec.Path.Value, `"`)
			var alias string
			if importSpec.Name != nil {
				alias = importSpec.Name.Name
			}
			// Simple merge: if an alias exists and is different, log or decide strategy.
			// For now, just overwrite or add.
			collectedImports[path] = alias
		}


		for typeSpec := range astwalk.ToplevelStructs(fset, file) {
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok { // Should not happen due to astwalk.ToplevelStructs logic
				continue
			}

			if typeSpec.Doc == nil || !strings.Contains(typeSpec.Doc.Text(), unmarshalAnnotation) {
				continue
			}

			structSpecificImports := make(map[string]string) // For this struct

			data := TemplateData{
				StructName:                 typeSpec.Name.Name,
				Imports:                    structSpecificImports,
				OneOfFields:                []OneOfFieldDetail{},
				OtherFields:                []FieldInfo{},
				DiscriminatorFieldJSONName: "type", // Hardcoded for now
			}

			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 { // Embedded struct or interface, handle if necessary
					continue
				}
				fieldName := field.Names[0].Name
				jsonTag := ""
				if field.Tag != nil {
					tagVal := strings.Trim(field.Tag.Value, "`")
					tag := reflect.StructTag(tagVal)
					jsonTagVal := tag.Get("json")
					if commaIdx := strings.Index(jsonTagVal, ","); commaIdx != -1 {
						jsonTag = jsonTagVal[:commaIdx]
					} else {
						jsonTag = jsonTagVal
					}
				}

				// TODO: Replace scanner.TypeInfo based logic with AST based type analysis
				// This is the most complex part: resolving field types, especially interfaces
				// and finding their implementers. This will likely require using go/types.

				// Placeholder for field type resolution and interface checking
				fieldTypeStr := getTypeString(field.Type) // Simplified
				isInterfaceField := false                 // Placeholder

				// Example of checking if a field is an interface (very simplified)
				// A robust solution needs type checking with go/types.
				if _, ok := field.Type.(*ast.InterfaceType); ok {
					isInterfaceField = true
				} else if ident, ok := field.Type.(*ast.Ident); ok {
					// If it's an identifier, it could be a named interface.
					// Need to look up its definition.
					// For now, let's assume a placeholder logic.
					// This is where go/types would be essential.
					// if ident.Obj != nil && ident.Obj.Kind == ast.Typ {
					// 	if ts, ok := ident.Obj.Decl.(*ast.TypeSpec); ok {
					// 		if _, ok := ts.Type.(*ast.InterfaceType); ok {
					// 			isInterfaceField = true
					// 		}
					//	}
					// }
					// The above commented code is a naive intra-file check.
					// Cross-package and more complex scenarios need go/types.
					slog.Debug("Field type is Ident", "name", ident.Name)
				}


				if isInterfaceField {
					// TODO: Re-implement OneOfFieldDetail population using AST and type information
					// This involves:
					// 1. Identifying the interface type correctly (potentially cross-package).
					// 2. Finding all types in the scanned scope (and their dependencies)
					//    that implement this interface. This is a major task for go/types.
					slog.Warn("Interface field found, but implementer resolution is not yet fully ported to AST", "field", fieldName, "type", fieldTypeStr)

					// Dummy data for now to allow template execution
					oneOfDetail := OneOfFieldDetail{
						FieldName: fieldName,
						FieldType: fieldTypeStr, // This needs to be the fully qualified type name
						JSONTag:   jsonTag,
						Implementers: []OneOfTypeMapping{
							// {JSONValue: "example", GoType: "*ExampleType"}, // Placeholder
						},
					}
					data.OneOfFields = append(data.OneOfFields, oneOfDetail)


				} else { // Other fields
					data.OtherFields = append(data.OtherFields, FieldInfo{Name: fieldName, Type: fieldTypeStr, JSONTag: jsonTag})
				}
			}

			if len(data.OneOfFields) == 0 && len(data.OtherFields) > 0 { // If only other fields, but no oneOf, skip for now if original logic skipped
				// The original logic was: if len(data.OneOfFields) == 0 { continue }
				// We need to ensure we only generate for structs that would have had OneOfFields.
				// For now, if @deriving:unmarshall is present, we attempt to generate.
				// If the template relies on OneOfFields, it might generate empty methods.
				// Let's stick to the original skip condition for now.
				hasOneOf := false
				for _, f := range structType.Fields.List {
					if _, ok := f.Type.(*ast.InterfaceType); ok { // Simplified check
						hasOneOf = true
						break
					}
				}
				if !hasOneOf {
					slog.Debug("Skipping struct as no interface fields were identified (simplified check)", "struct", typeSpec.Name.Name)
					continue
				}
				// If we reach here, it means we found an interface field (or thought we did)
				// but the OneOfFields list might be empty if implementer lookup failed.
				// This part of the logic needs to be robust.
				if len(data.OneOfFields) == 0 {
					slog.Warn("Struct has @deriving:unmarshall and potentially interface fields, but no OneOfFields were populated. Generation might be incomplete.", "struct", typeSpec.Name.Name)
					// continue // Potentially skip if no oneOf details could be gathered.
				}
			}


			tmpl, err := template.ParseFS(templateFile, "unmarshal.tmpl")
			if err != nil {
				return fmt.Errorf("failed to parse template: %w", err)
			}
			var currentGeneratedCode bytes.Buffer
			if err := tmpl.Execute(&currentGeneratedCode, data); err != nil {
				return fmt.Errorf("failed to execute template for struct %s: %w", typeSpec.Name.Name, err)
			}
			generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
			generatedCodeForAllStructs.WriteString("\n\n")

			// Merge struct-specific imports into collectedImports (already done globally from file.Imports)
			// If template execution generates needs for specific imports not in file.Imports,
			// those would need to be added here or managed by the template data.
			// For now, global file imports are collected. Specific new imports by template are not handled yet.
		}
	}


	if generatedCodeForAllStructs.Len() == 0 {
		slog.InfoContext(ctx, "No structs found requiring UnmarshalJSON generation in package", "package_path", pkgPath)
		return nil
	}

	// Ensure "encoding/json" and "fmt" are added if any code was generated
	// These are typically needed by the unmarshal.tmpl template.
	if generatedCodeForAllStructs.Len() > 0 {
		if _, exists := collectedImports["encoding/json"]; !exists {
			collectedImports["encoding/json"] = ""
		}
		if _, exists := collectedImports["fmt"]; !exists {
			collectedImports["fmt"] = ""
		}
	}


	// Use PackageDirectory to save the file
	// Ensure firstFilePkgName is used if pkgInfo.Name was used before
	outputDir := goscan.NewPackageDirectory(pkgPath, firstFilePkgName)
	goFile := goscan.GoFile{
		PackageName: firstFilePkgName, // Use the determined package name
		Imports:     collectedImports,
		CodeSet:     generatedCodeForAllStructs.String(),
	}

	outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(firstFilePkgName))

	if err := outputDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
		return fmt.Errorf("failed to save generated file for package %s: %w", firstFilePkgName, err)
	}
	return nil
}
