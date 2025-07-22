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
	"sort"
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

		_, hasBindingAnnotationOnStruct := typeInfo.Annotation("derivng:binding")
		if !hasBindingAnnotationOnStruct {
			continue
		}
		slog.DebugContext(ctx, "Processing struct for binding", slog.String("struct", typeInfo.Name))

		data := TemplateData{
			StructName: typeInfo.Name,
			Fields:     []FieldBindingInfo{},
		}
		importManager.Add("net/http", "") // For http.ErrNoCookie and request object (r *http.Request)

		for _, field := range typeInfo.Struct.Fields {
			bindFrom := field.TagValue("in")
			if bindFrom == "" {
				continue
			}
			bindFrom = strings.ToLower(strings.TrimSpace(bindFrom))
			bindName := field.TagValue(bindFrom)

			if bindName == "" {
				continue
			}

			fInfo := FieldBindingInfo{
				FieldName: field.Name,
				BindFrom:  bindFrom,
				BindName:  bindName,
				IsPointer: field.Type.IsPointer,
			}

			currentScannerType := field.Type
			baseTypeForConversion := ""
			if currentScannerType.IsPointer {
				if currentScannerType.Elem != nil {
					baseTypeForConversion = currentScannerType.Elem.Name
				} else {
					baseTypeForConversion = currentScannerType.Name
				}
			} else {
				baseTypeForConversion = currentScannerType.Name
			}
			fInfo.FieldType = baseTypeForConversion

			switch baseTypeForConversion {
			case "string":
				fInfo.ParserFunc = "parser.String"
			default:
				continue
			}

			importManager.Add("errors", "") // For errors.Join
			data.Fields = append(data.Fields, fInfo)
			anyCodeGenerated = true
		}

		if !anyCodeGenerated {
			continue
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
		return nil
	}

	outputPkgDir := goscan.NewPackageDirectory(packageDir, pkgInfo.Name)
	importSpecs := importManager.Imports()
	paths := make([]string, 0, len(importSpecs))
	for path := range importSpecs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	goFile := goscan.GoFile{
		PackageName: pkgInfo.Name,
		Imports:     importManager.Imports(),
		CodeSet:     generatedCodeForAllStructs.String(),
	}

	outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(pkgInfo.Name))
	if err := outputPkgDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
		return fmt.Errorf("failed to save generated bind file for package %s: %w", pkgInfo.Name, err)
	}
	return nil
}
