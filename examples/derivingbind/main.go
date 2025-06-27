package main

import (
	"bytes"
	"context"
	"embed" // Added
	"fmt"
	// "go/format" // No longer needed here
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	// "sort" // No longer needed here for imports
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
	ctx := context.Background() // Or your application's context

	if len(os.Args) <= 1 {
		slog.ErrorContext(ctx, "Usage: derivingbind <file1.go> [file2.go ...]")
		slog.ErrorContext(ctx, "Example: derivingbind examples/derivingbind/testdata/simple/models.go")
		os.Exit(1)
	}
	targetFiles := os.Args[1:]

	filesByPackageDir := make(map[string][]string)
	for _, filePath := range targetFiles {
		stat, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				slog.ErrorContext(ctx, "File does not exist", slog.String("file_path", filePath))
			} else {
				slog.ErrorContext(ctx, "Error accessing file", slog.String("file_path", filePath), slog.Any("error", err))
			}
			continue
		}
		if stat.IsDir() {
			slog.ErrorContext(ctx, "Argument is a directory, expected a file", slog.String("path", filePath))
			continue
		}

		absPath, err := filepath.Abs(filePath)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get absolute path", slog.String("file_path", filePath), slog.Any("error", err))
			continue
		}
		dir := filepath.Dir(absPath)
		filesByPackageDir[dir] = append(filesByPackageDir[dir], absPath) // Store absPath for ScanFiles
	}

	if len(filesByPackageDir) == 0 {
		slog.ErrorContext(ctx, "No valid files to process.")
		os.Exit(1)
	}

	gscn, err := goscan.New(".")
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create go-scan scanner", slog.Any("error", err))
		os.Exit(1)
	}

	overallSuccess := true
	for pkgDir, filePathsInDir := range filesByPackageDir { // filePathsInDir are now absolute paths
		slog.InfoContext(ctx, "Generating Bind method for package", slog.String("package_dir", pkgDir), slog.Any("files", filePathsInDir))

		// GenerateFiles now takes absolute file paths directly for ScanFiles
		if err := GenerateFiles(ctx, pkgDir, filePathsInDir, gscn); err != nil {
			slog.ErrorContext(ctx, "Error generating code for package", slog.String("package_dir", pkgDir), slog.Any("error", err))
			overallSuccess = false
			continue
		}
		slog.InfoContext(ctx, "Successfully generated Bind methods for package", slog.String("package_dir", pkgDir))
	}

	if !overallSuccess {
		slog.ErrorContext(ctx, "One or more packages failed to generate.")
		os.Exit(1)
	}
	slog.InfoContext(ctx, "All specified files processed.")
}

const bindingAnnotation = "@derivng:binding"

type TemplateData struct {
	// PackageName string // Will be set in GoFile
	StructName                 string
	Fields                     []FieldBindingInfo
	// Imports                    map[string]string // alias -> path. This will be collected and passed to GoFile
	NeedsBody                  bool
	HasSpecificBodyFieldTarget bool
	ErrNoCookie                error // For template: http.ErrNoCookie
}

type FieldBindingInfo struct {
	FieldName    string
	FieldType    string
	BindFrom     string
	BindName     string
	IsPointer    bool
	IsRequired   bool
	IsBody       bool
	BodyJSONName string

	IsSlice                 bool
	SliceElementType        string
	OriginalFieldTypeString string
	ParserFunc              string
	IsSliceElementPointer   bool
}

// GenerateFiles processes a list of absolute file paths within a specific package directory.
func GenerateFiles(ctx context.Context, packageDir string, absFilePaths []string, gscn *goscan.Scanner) error {
	pkgInfo, err := gscn.ScanFiles(ctx, absFilePaths) // Use absolute paths directly
	if err != nil {
		return fmt.Errorf("go-scan failed to scan files in package %s: %w", packageDir, err)
	}
	if pkgInfo == nil {
		return fmt.Errorf("go-scan returned nil package info for %s (using files: %v)", packageDir, absFilePaths)
	}
	if pkgInfo.Name == "" {
		// If ScanFiles doesn't robustly determine package name from a subset of files,
		// use the directory name as a fallback.
		// This might happen if the parsed files don't contain a package declaration
		// (e.g. if they are all empty or only comments).
		// goscan.PackageDirectory's DefaultPackageName will handle this if pkgInfo.Name is empty.
		slog.WarnContext(ctx, "ScanFiles resulted in an empty package name.", slog.String("packageDir", packageDir), slog.Any("absFilePaths", absFilePaths))
	}


	var generatedCodeForAllStructs bytes.Buffer
	collectedImports := make(map[string]string) // path -> alias. For GoFile.Imports

	// Initialize with common imports that are almost always needed by the template
	collectedImports["github.com/podhmo/go-scan/examples/derivingbind/binding"] = ""
	collectedImports["github.com/podhmo/go-scan/examples/derivingbind/parser"] = ""
	// Other imports like net/http, fmt, errors, encoding/json, io will be added conditionally

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Kind != scanner.StructKind || typeInfo.Struct == nil {
			continue
		}
		hasBindingAnnotationOnStruct := strings.Contains(typeInfo.Doc, bindingAnnotation)
		structLevelInTag := ""
		if hasBindingAnnotationOnStruct {
			docLines := strings.Split(typeInfo.Doc, "\n")
			for _, line := range docLines {
				if strings.Contains(line, bindingAnnotation) {
					parts := strings.Fields(line)
					for _, part := range parts {
						if strings.HasPrefix(part, "in:") {
							structLevelInTag = strings.TrimSuffix(strings.SplitN(part, ":", 2)[1], `"`)
							structLevelInTag = strings.TrimPrefix(structLevelInTag, `"`)
							break
						}
					}
				}
				if structLevelInTag != "" {
					break
				}
			}
		}

		if !hasBindingAnnotationOnStruct {
			continue
		}
		fmt.Printf("  Processing struct: %s for %s\n", typeInfo.Name, bindingAnnotation)

		data := TemplateData{
			StructName: typeInfo.Name,
			Fields:                     []FieldBindingInfo{},
			NeedsBody:                  (structLevelInTag == "body"),
			HasSpecificBodyFieldTarget: false,
			ErrNoCookie:                http.ErrNoCookie,
		}
		collectedImports["net/http"] = "" // For http.ErrNoCookie and request object

		for _, field := range typeInfo.Struct.Fields {
			tag := reflect.StructTag(field.Tag)
			inTagVal := tag.Get("in")
			bindFrom := ""
			bindName := ""

			if inTagVal != "" {
				bindFrom = strings.ToLower(strings.TrimSpace(inTagVal))
				switch bindFrom {
				case "path", "query", "header", "cookie":
					bindName = tag.Get(bindFrom) // e.g., tag.Get("path")
				case "body":
					data.NeedsBody = true
				default:
					fmt.Printf("      Skipping field %s: unknown 'in' tag value '%s'\n", field.Name, inTagVal)
					continue
				}
				if bindFrom != "body" && bindName == "" {
					fmt.Printf("      Skipping field %s: 'in:\"%s\"' tag requires corresponding '%s:\"name\"' tag\n", field.Name, bindFrom, bindFrom)
					continue
				}
			} else if data.NeedsBody {
				continue // Part of struct-level body, handled by overall JSON decode
			} else {
				continue // No binding directive
			}

			fInfo := FieldBindingInfo{
				FieldName:               field.Name,
				BindFrom:                bindFrom,
				BindName:                bindName,
				IsRequired:              (tag.Get("required") == "true"),
				OriginalFieldTypeString: field.Type.String(),
				IsPointer:               field.Type.IsPointer,
			}

			currentScannerType := field.Type
			baseTypeForConversion := ""

			if currentScannerType.IsSlice {
				fInfo.IsSlice = true
				if currentScannerType.Elem != nil {
					fInfo.SliceElementType = currentScannerType.Elem.String()
					fInfo.IsSliceElementPointer = currentScannerType.Elem.IsPointer
					sliceElemScannerType := currentScannerType.Elem
					if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem != nil {
						baseTypeForConversion = sliceElemScannerType.Elem.Name
					} else if sliceElemScannerType.IsPointer && sliceElemScannerType.Elem == nil {
						baseTypeForConversion = sliceElemScannerType.Name
					} else {
						baseTypeForConversion = sliceElemScannerType.Name
					}
				} else {
					fmt.Printf("      Skipping field %s: slice with nil Elem type\n", field.Name)
					continue
				}
			} else if currentScannerType.IsPointer {
				if currentScannerType.Elem != nil {
					baseTypeForConversion = currentScannerType.Elem.Name
				} else {
					baseTypeForConversion = currentScannerType.Name
					if baseTypeForConversion == "" && bindFrom != "body" {
						fmt.Printf("      Warning: Pointer field %s (%s) - Name empty. Skipping non-body.\n", field.Name, fInfo.OriginalFieldTypeString)
						continue
					}
				}
			} else {
				baseTypeForConversion = currentScannerType.Name
				if baseTypeForConversion == "" && bindFrom != "body" {
					fmt.Printf("      Warning: Field %s (%s) - Name empty. Skipping non-body.\n", field.Name, fInfo.OriginalFieldTypeString)
					continue
				}
			}
			fInfo.FieldType = baseTypeForConversion

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
				if bindFrom != "body" {
					fmt.Printf("      Skipping field %s: unhandled base type '%s' for %s binding.\n", field.Name, baseTypeForConversion, bindFrom)
					continue
				}
			}

			if bindFrom != "body" {
				collectedImports["errors"] = "" // For errors.Join
				if fInfo.ParserFunc == "" {
					fmt.Printf("      Skipping field %s: No parser func for non-body binding.\n", field.Name)
					continue
				}
			} else {
				fInfo.IsBody = true
				data.NeedsBody = true
				data.HasSpecificBodyFieldTarget = true
				collectedImports["encoding/json"] = ""
				collectedImports["io"] = ""
				collectedImports["fmt"] = ""    // For fmt.Errorf
				collectedImports["errors"] = "" // For errors.Join
			}
			data.Fields = append(data.Fields, fInfo)
		}

		if len(data.Fields) == 0 && !data.NeedsBody {
			fmt.Printf("  Skipping struct %s: no bindable fields or global body target.\n", typeInfo.Name)
			continue
		}

		if data.NeedsBody && !data.HasSpecificBodyFieldTarget { // Struct itself is body target
			collectedImports["encoding/json"] = ""
			collectedImports["io"] = ""
			collectedImports["fmt"] = ""
			collectedImports["errors"] = ""
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

	if generatedCodeForAllStructs.Len() == 0 {
		fmt.Println("No structs found requiring Bind method generation.")
		return nil
	}

	actualPackageName := pkgInfo.Name
	if actualPackageName == "" {
		actualPackageName = filepath.Base(packageDir)
		slog.InfoContext(ctx, "Using directory name as package name for generated file", "package_name", actualPackageName, "package_dir", packageDir)
	}

	outputPkgDir := goscan.NewPackageDirectory(packageDir, actualPackageName)
	goFile := goscan.GoFile{
		PackageName: actualPackageName,
		Imports:     collectedImports,
		CodeSet:     generatedCodeForAllStructs.String(),
	}

	outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(actualPackageName))
	if err := outputPkgDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
		return fmt.Errorf("failed to save generated bind file for package %s: %w", actualPackageName, err)
	}
	return nil
}

// Helper function to check if a base type string is numeric or boolean
func isNumericOrBool(baseType string) bool {
	switch baseType {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128", "bool":
		return true
	default:
		return false
	}
}

// Helper function to check if a slice element type is one of the directly convertible primitives
func isWellKnownSliceElementType(sliceElementType string) bool {
	base := sliceElementType
	if strings.HasPrefix(base, "*") && len(base) > 1 {
		base = base[1:]
	}
	switch base {
	case "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128", "bool", "uintptr":
		return true
	default:
		return false
	}
}
