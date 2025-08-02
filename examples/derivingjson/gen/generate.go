package gen

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"text/template"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

//go:embed unmarshal.tmpl marshal.tmpl
var templateFile embed.FS

const unmarshalAnnotation = "deriving:unmarshal"
const marshalAnnotation = "deriving:marshal"

type TemplateData struct {
	StructName                 string
	OtherFields                []FieldInfo
	OneOfFields                []OneOfFieldDetail
	DiscriminatorFieldJSONName string
}

type MarshalTemplateData struct {
	StructName                 string
	DiscriminatorFieldJSONName string
	DiscriminatorValue         string
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

func Generate(ctx context.Context, gscn *goscan.Scanner, pkgInfo *scanner.PackageInfo, importManager *goscan.ImportManager) ([]byte, error) {
	if pkgInfo == nil {
		return nil, fmt.Errorf("cannot generate code for a nil package")
	}
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
			if field.Type.FullImportPath == "" {
				resolvedFieldType = findTypeInPackage(pkgInfo, field.Type.Name)
			} else {
				resolvedFieldType, _ = gscn.ResolveType(ctx, field.Type)
			}

			isInterfaceField := false
			if resolvedFieldType != nil && resolvedFieldType.Kind == scanner.InterfaceKind {
				isInterfaceField = true
			} else if resolvedFieldType == nil && strings.Contains(field.Type.Name, "interface{") { // Heuristic for anonymous interfaces
				isInterfaceField = true
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
					if field.Type.FullImportPath != "" && field.Type.FullImportPath != pkgInfo.ImportPath {
						interfaceDefiningPkgImportPath = field.Type.FullImportPath
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
					oneOfDetail.FieldType = field.Type.Name
					interfaceDefiningPkgImportPath = pkgInfo.ImportPath
					slog.DebugContext(ctx, "Handling field as anonymous interface", "fieldName", field.Name, "fieldType", field.Type.Name)
					if interfaceDef == nil {
						slog.WarnContext(ctx, "Cannot find implementers for anonymous interface field without a resolved TypeInfo", "fieldName", field.Name, "fieldType", field.Type.Name)
						data.OneOfFields = append(data.OneOfFields, oneOfDetail)
						continue
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
				if resolvedFieldType != nil && resolvedFieldType.Name != "" {
					definingPkgPath := pkgInfo.ImportPath
					if resolvedFieldType.FilePath != "" {
						dir := filepath.Dir(resolvedFieldType.FilePath)
						definingActualPkg, errScan := gscn.ScanPackage(ctx, dir)
						if errScan == nil && definingActualPkg != nil && definingActualPkg.ImportPath != "" {
							definingPkgPath = definingActualPkg.ImportPath
						} else if errScan != nil {
							slog.DebugContext(ctx, "Could not scan package for resolved field type, using current package for qualification.", "field", field.Name, "resolvedTypeName", resolvedFieldType.Name, "error", errScan)
						}
					} else if field.Type.FullImportPath != "" {
						definingPkgPath = field.Type.FullImportPath
					}
					typeName = importManager.Qualify(definingPkgPath, resolvedFieldType.Name)
				} else {
					typeName = importManager.Qualify(field.Type.FullImportPath, field.Type.Name)
				}
				data.OtherFields = append(data.OtherFields, FieldInfo{Name: field.Name, Type: typeName, JSONTag: jsonTag})
			}
		}

		if len(data.OneOfFields) == 0 {
			continue
		}
		anyCodeGenerated = true

		tmpl, err := template.ParseFS(templateFile, "unmarshal.tmpl")
		if err != nil {
			return nil, fmt.Errorf("failed to parse template: %w", err)
		}
		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, data); err != nil {
			return nil, fmt.Errorf("failed to execute template for struct %s: %w", typeInfo.Name, err)
		}
		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")
	}

	// Scan for marshal annotation
	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}
		if _, ok := typeInfo.Annotation(marshalAnnotation); !ok {
			continue
		}

		// Prepare data for the marshaling template
		marshalData := MarshalTemplateData{
			StructName:                 typeInfo.Name,
			DiscriminatorFieldJSONName: "type", // Hardcoded for now
			DiscriminatorValue:         strings.ToLower(typeInfo.Name),
		}

		// Generate code using the marshal template
		tmpl, err := template.ParseFS(templateFile, "marshal.tmpl")
		if err != nil {
			return nil, fmt.Errorf("failed to parse marshal template: %w", err)
		}
		var currentGeneratedCode bytes.Buffer
		if err := tmpl.Execute(&currentGeneratedCode, marshalData); err != nil {
			return nil, fmt.Errorf("failed to execute marshal template for struct %s: %w", typeInfo.Name, err)
		}
		generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
		generatedCodeForAllStructs.WriteString("\n\n")
		anyCodeGenerated = true // Mark that we've generated some code
	}

	if !anyCodeGenerated {
		slog.InfoContext(ctx, "No structs found requiring UnmarshalJSON generation in package", slog.String("package_path", pkgInfo.Path))
		return nil, nil // No code generated, but not an error
	}

	importManager.Add("encoding/json", "")
	importManager.Add("fmt", "")

	return generatedCodeForAllStructs.Bytes(), nil
}

func isPackageInSlice(slice []*scanner.PackageInfo, importPath string) bool {
	for _, p := range slice {
		if p.ImportPath == importPath {
			return true
		}
	}
	return false
}
