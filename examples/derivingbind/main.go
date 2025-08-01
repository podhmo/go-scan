package main

import (
	"bytes"
	"context"
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

//go:embed bind_method.tmpl
var bindMethodTemplateFS embed.FS

//go:embed bind_method.tmpl
var bindMethodTemplateString string

func main() {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug)
	opts := slog.HandlerOptions{Level: logLevel}
	handler := slog.NewTextHandler(os.Stderr, &opts)
	slog.SetDefault(slog.New(handler))

	var cwd string
	flag.StringVar(&cwd, "cwd", ".", "current working directory")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: derivingbind [options] <file_or_dir_path_1> [file_or_dir_path_2 ...]\n")
		fmt.Fprintf(os.Stderr, "Example (file): derivingbind examples/derivingbind/testdata/simple/models.go\n")
		fmt.Fprintf(os.Stderr, "Example (dir):  derivingbind examples/derivingbind/testdata/simple/\n")
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
			slog.InfoContext(ctx, "Successfully generated Bind method for package", "path", dirPath)
			successCount++
		}
	}

	// Process file groups
	for pkgDir, filePaths := range filesByPackage {
		slog.InfoContext(ctx, "Scanning files in package", "package", pkgDir, "files", filePaths)
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
			slog.InfoContext(ctx, "Successfully generated Bind method for package", "package", pkgDir)
			successCount++
		}
	}

	slog.InfoContext(ctx, "Generation summary", slog.Int("successful_packages", successCount), slog.Int("failed_packages/files", errorCount))
	if errorCount > 0 {
		os.Exit(1)
	}
}

const bindingAnnotation = "@derivng:binding"

type TemplateData struct {
	StructName                 string
	Fields                     []FieldBindingInfo
	NeedsBody                  bool
	HasSpecificBodyFieldTarget bool
	ErrNoCookie                error
}

type FieldBindingInfo struct {
	FieldName    string
	FieldType    string // This will store the base type name for parser lookup
	BindFrom     string
	BindName     string
	IsPointer    bool
	IsRequired   bool
	IsBody       bool
	BodyJSONName string

	IsSlice                 bool
	SliceElementType        string // This will store the element's base type name
	OriginalFieldTypeString string // The original full type string (e.g., "[]*models.Item", "string")
	ParserFunc              string
	IsSliceElementPointer   bool
}

func Generate(ctx context.Context, gscn *goscan.Scanner, pkgInfo *scanner.PackageInfo) error {
	if pkgInfo == nil {
		return fmt.Errorf("cannot generate code for a nil package")
	}
	packageDir := pkgInfo.Path
	importManager := goscan.NewImportManager(pkgInfo)
	var generatedCodeForAllStructs bytes.Buffer
	anyCodeGenerated := false

	// Always add parser and binding, as they are fundamental to the template
	importManager.Add("github.com/podhmo/go-scan/examples/derivingbind/binding", "")
	importManager.Add("github.com/podhmo/go-scan/examples/derivingbind/parser", "")

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}

		annotationValue, hasBindingAnnotationOnStruct := typeInfo.Annotation("derivng:binding")
		structLevelInTag := ""
		if hasBindingAnnotationOnStruct {
			parts := strings.Fields(annotationValue)
			for _, part := range parts {
				if strings.HasPrefix(part, "in:") {
					structLevelInTag = strings.TrimSuffix(strings.SplitN(part, ":", 2)[1], `"`)
					structLevelInTag = strings.TrimPrefix(structLevelInTag, `"`)
					break
				}
			}
		}

		if !hasBindingAnnotationOnStruct {
			continue
		}
		slog.DebugContext(ctx, "Processing struct for binding", slog.String("struct", typeInfo.Name))

		data := TemplateData{
			StructName:                 typeInfo.Name,
			Fields:                     []FieldBindingInfo{},
			NeedsBody:                  (structLevelInTag == "body"),
			HasSpecificBodyFieldTarget: false,
			ErrNoCookie:                http.ErrNoCookie,
		}
		importManager.Add("net/http", "") // For http.ErrNoCookie and request object (r *http.Request)

		structHasBindableFields := false
		for _, field := range typeInfo.Struct.Fields {
			bindFrom := field.TagValue("in")
			if bindFrom == "" {
				if data.NeedsBody && structLevelInTag == "body" {
					// Field is part of struct-level body, handled by overall JSON decode.
				}
				continue
			}
			bindFrom = strings.ToLower(strings.TrimSpace(bindFrom))
			bindName := field.TagValue(bindFrom)

			switch bindFrom {
			case "path", "query", "header", "cookie":
				if bindName == "" {
					slog.DebugContext(ctx, "Skipping field: tag requires corresponding name tag", "struct", typeInfo.Name, "field", field.Name, "in_tag", bindFrom)
					continue
				}
			case "body":
				data.NeedsBody = true
			default:
				slog.DebugContext(ctx, "Skipping field: unknown 'in' tag value", "struct", typeInfo.Name, "field", field.Name, "in_tag", bindFrom)
				continue
			}

			// Use field.Type.String() for OriginalFieldTypeString as it gives the full type representation
			// including package alias if it's from an external package resolved by the core scanner.
			// ImportManager's role here is mainly to ensure the *package* is imported,
			// not to rewrite these type strings unless they are being constructed from parts.
			originalFieldTypeStr := field.Type.String()
			if field.Type.FullImportPath != "" && field.Type.FullImportPath != pkgInfo.ImportPath {
				// Ensure the package of this field type is registered for import
				originalFieldTypeStr = importManager.Qualify(field.Type.FullImportPath, field.Type.Name)
				if field.Type.IsSlice && field.Type.Elem != nil {
					sliceElemStr := importManager.Qualify(field.Type.Elem.FullImportPath, field.Type.Elem.Name)
					if field.Type.Elem.IsPointer {
						sliceElemStr = "*" + sliceElemStr
					}
					originalFieldTypeStr = "[]" + sliceElemStr
				} else if field.Type.IsPointer && field.Type.Elem != nil {
					originalFieldTypeStr = "*" + importManager.Qualify(field.Type.Elem.FullImportPath, field.Type.Elem.Name)
				}
			}

			fInfo := FieldBindingInfo{
				FieldName:               field.Name,
				BindFrom:                bindFrom,
				BindName:                bindName,
				IsRequired:              (field.TagValue("required") == "true"),
				OriginalFieldTypeString: originalFieldTypeStr,
				IsPointer:               field.Type.IsPointer,
			}

			currentScannerType := field.Type
			baseTypeForConversion := "" // This will be the simple, unqualified type name for parser lookup

			if currentScannerType.IsSlice {
				fInfo.IsSlice = true
				if currentScannerType.Elem != nil {
					// For SliceElementType, we also want the potentially qualified name for the template
					fInfo.SliceElementType = importManager.Qualify(currentScannerType.Elem.FullImportPath, currentScannerType.Elem.Name)
					if currentScannerType.Elem.IsPointer {
						fInfo.SliceElementType = "*" + fInfo.SliceElementType
					}
					fInfo.IsSliceElementPointer = currentScannerType.Elem.IsPointer

					// For baseTypeForConversion (parser lookup), use the unqualified name
					sliceElemForParser := currentScannerType.Elem
					if sliceElemForParser.IsPointer && sliceElemForParser.Elem != nil {
						baseTypeForConversion = sliceElemForParser.Elem.Name
					} else { // Non-pointer element or pointer to built-in
						baseTypeForConversion = sliceElemForParser.Name
					}
				} else {
					slog.DebugContext(ctx, "Skipping field: slice with nil Elem type", "struct", typeInfo.Name, "field", field.Name)
					continue
				}
			} else if currentScannerType.IsPointer {
				if currentScannerType.Elem != nil {
					baseTypeForConversion = currentScannerType.Elem.Name
				} else { // Pointer to a built-in or unresolved type
					baseTypeForConversion = currentScannerType.Name // e.g. *string, Name would be "string"
				}
			} else { // Not a slice, not a pointer
				baseTypeForConversion = currentScannerType.Name
			}
			fInfo.FieldType = baseTypeForConversion // Store the base (unqualified) type for parser lookup

			// Determine parser function based on the unqualified base type
			switch baseTypeForConversion {
			case "string":
				fInfo.ParserFunc = "parser.String"
			case "int", "int8", "int16", "int32", "int64":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			case "bool":
				fInfo.ParserFunc = "parser.Bool"
			case "float32", "float64":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			case "complex64", "complex128":
				fInfo.ParserFunc = "parser." + strings.Title(baseTypeForConversion)
			default:
				if bindFrom != "body" { // Custom types not supported for non-body binding directly by these parsers
					slog.DebugContext(ctx, "Skipping field: unhandled base type for non-body binding", "struct", typeInfo.Name, "field", field.Name, "baseType", baseTypeForConversion, "bindFrom", bindFrom)
					continue
				}
				// For 'body' binding, fInfo.ParserFunc is not used; it's direct unmarshaling.
			}

			if bindFrom != "body" {
				importManager.Add("errors", "") // For errors.Join
				if fInfo.ParserFunc == "" {
					slog.DebugContext(ctx, "Skipping field: No parser func for non-body binding", "struct", typeInfo.Name, "field", field.Name)
					continue
				}
			} else { // bindFrom == "body"
				fInfo.IsBody = true
				data.NeedsBody = true // Ensure this is true if any field is body-bound
				data.HasSpecificBodyFieldTarget = true
				importManager.Add("encoding/json", "")
				importManager.Add("io", "")
				importManager.Add("fmt", "")    // For fmt.Errorf
				importManager.Add("errors", "") // For errors.Join
			}
			data.Fields = append(data.Fields, fInfo)
			structHasBindableFields = true
		}

		if !structHasBindableFields && !data.NeedsBody { // If no fields were processed for binding and it's not a global body target
			slog.DebugContext(ctx, "Skipping struct: no bindable fields or global body target", "struct", typeInfo.Name)
			continue
		}
		anyCodeGenerated = true

		if data.NeedsBody && !data.HasSpecificBodyFieldTarget { // Struct itself is body target
			importManager.Add("encoding/json", "")
			importManager.Add("io", "")
			importManager.Add("fmt", "")
			importManager.Add("errors", "")
		}

		funcMap := template.FuncMap{"TitleCase": strings.Title}
		tmpl, err := template.New("bind").Funcs(funcMap).Parse(bindMethodTemplateString)
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

	if !anyCodeGenerated {
		slog.InfoContext(ctx, "No structs found requiring Bind method generation in package", slog.String("packageDir", packageDir))
		return nil
	}

	actualPackageName := pkgInfo.Name
	if actualPackageName == "" {
		actualPackageName = filepath.Base(packageDir) // Fallback to directory name
		slog.InfoContext(ctx, "Using directory name as package name for generated file", "package_name", actualPackageName, "package_dir", packageDir)
	}

	outputPkgDir := goscan.NewPackageDirectory(packageDir, actualPackageName)
	goFile := goscan.GoFile{
		PackageName: actualPackageName,
		Imports:     importManager.Imports(),
		CodeSet:     generatedCodeForAllStructs.String(),
	}

	outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(actualPackageName))
	if err := outputPkgDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
		return fmt.Errorf("failed to save generated bind file for package %s: %w", actualPackageName, err)
	}
	return nil
}
