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

	"go/ast"
	"go/parser"
	"go/token"
	// "go/types" // May be needed for more complex type resolution if struct fields involve external types not easily stringified

	"github.com/podhmo/go-scan/astwalk"
	goscan "github.com/podhmo/go-scan"
	// "github.com/podhmo/go-scan/scanner" // No longer directly used in GenerateFiles
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
		filesByPackageDir[dir] = append(filesByPackageDir[dir], absPath)
	}

	if len(filesByPackageDir) == 0 {
		slog.ErrorContext(ctx, "No valid files to process.")
		os.Exit(1)
	}

	// gscn is no longer needed here as GenerateFiles will use go/parser
	// gscn, err := goscan.New(".")
	// if err != nil {
	// 	slog.ErrorContext(ctx, "Failed to create go-scan scanner", slog.Any("error", err))
	// 	os.Exit(1)
	// }

	overallSuccess := true
	for pkgDir, filePathsInDir := range filesByPackageDir { // filePathsInDir are now absolute paths
		slog.InfoContext(ctx, "Generating Bind method for package", slog.String("package_dir", pkgDir), slog.Any("files", filePathsInDir))

		// Pass absFilePaths directly to GenerateFiles. gscn is removed.
		if err := GenerateFiles(ctx, pkgDir, filePathsInDir); err != nil {
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
	FieldType    string // Base type for parser lookup (e.g., "string", "int")
	BindFrom     string
	BindName     string
	IsPointer    bool
	IsRequired   bool
	IsBody       bool
	BodyJSONName string // Not used by bind.tmpl directly for individual fields, but good to keep if body handling evolves

	IsSlice                 bool
	SliceElementType        string // Base type of slice elements (e.g., "string", "int")
	OriginalFieldTypeString string // Full original type string from AST (e.g., "*string", "[]*int", "mypkg.MyType")
	ParserFunc              string
	IsSliceElementPointer   bool
}

// Helper to get type string from ast.Expr, similar to derivingjson
func getTypeStringFromAST(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return getTypeStringFromAST(t.X) + "." + t.Sel.Name // Keep pkg selector for OriginalFieldTypeString
	case *ast.StarExpr:
		return "*" + getTypeStringFromAST(t.X)
	case *ast.ArrayType:
		return "[]" + getTypeStringFromAST(t.Elt)
	default:
		// This path should ideally not be hit for well-formed Go code struct fields.
		// If it is, it means there's a type construct not handled above.
		// For robustness, one might use go/printer to format the expression.
		// However, for typical struct fields, the cases above cover most scenarios.
		slog.Warn("Unhandled ast.Expr type in getTypeStringFromAST", "type", fmt.Sprintf("%T", expr))
		return "unknownType" // Fallback
	}
}


// GenerateFiles processes a list of absolute file paths within a specific package directory.
// It no longer takes goscan.Scanner as it uses go/parser directly.
func GenerateFiles(ctx context.Context, packageDir string, absFilePaths []string) error {
	fset := token.NewFileSet()
	var files []*ast.File
	var firstFilePkgName string

	for _, filePath := range absFilePaths {
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
		firstFilePkgName = filepath.Base(packageDir)
		slog.WarnContext(ctx, "Could not determine package name from parsed files, using directory name", "package_path", packageDir, "fallback_pkg_name", firstFilePkgName)
	}

	var generatedCodeForAllStructs bytes.Buffer
	collectedImports := make(map[string]string)

	collectedImports["github.com/podhmo/go-scan/examples/derivingbind/binding"] = ""
	collectedImports["github.com/podhmo/go-scan/examples/derivingbind/parser"] = ""

	for _, fileAST := range files {
		if fileAST == nil {
			continue
		}
		// Collect imports from this file
		for _, importSpec := range fileAST.Imports {
			path := strings.Trim(importSpec.Path.Value, `"`)
			var alias string
			if importSpec.Name != nil {
				alias = importSpec.Name.Name
			}
			collectedImports[path] = alias // Simple overwrite/add
		}

		for typeSpec := range astwalk.ToplevelStructs(fset, fileAST) {
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			hasBindingAnnotationOnStruct := false
			structLevelInTag := ""
			if typeSpec.Doc != nil {
				docText := typeSpec.Doc.Text()
				hasBindingAnnotationOnStruct = strings.Contains(docText, bindingAnnotation)
				if hasBindingAnnotationOnStruct {
					docLines := strings.Split(docText, "\n")
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
			}

			if !hasBindingAnnotationOnStruct {
				continue
			}
			slog.Debug("Processing struct for binding", "struct_name", typeSpec.Name.Name)


			data := TemplateData{
				StructName:                 typeSpec.Name.Name,
				Fields:                     []FieldBindingInfo{},
				NeedsBody:                  (structLevelInTag == "body"),
				HasSpecificBodyFieldTarget: false, // Will be set if any field has 'in:"body"'
				ErrNoCookie:                http.ErrNoCookie,
			}
			collectedImports["net/http"] = ""

			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 { // Skip embedded fields for now
					continue
				}
				fieldName := field.Names[0].Name
				var fieldTag reflect.StructTag
				if field.Tag != nil {
					tagVal := strings.Trim(field.Tag.Value, "`")
					fieldTag = reflect.StructTag(tagVal)
				}

				inTagVal := fieldTag.Get("in")
				bindFrom := ""
				bindName := ""

				if inTagVal != "" {
					bindFrom = strings.ToLower(strings.TrimSpace(inTagVal))
					switch bindFrom {
					case "path", "query", "header", "cookie":
						bindName = fieldTag.Get(bindFrom)
					case "body":
						data.NeedsBody = true // Mark that the request needs body parsing
						// Individual field from body, specific logic in template
					default:
						slog.Warn("Skipping field due to unknown 'in' tag", "field", fieldName, "in_tag", inTagVal)
						continue
					}
					if bindFrom != "body" && bindName == "" {
						slog.Warn("Skipping field: 'in' tag requires corresponding name tag", "field", fieldName, "bind_from", bindFrom)
						continue
					}
				} else if data.NeedsBody && structLevelInTag == "body" { // Field is part of a struct-level body binding
					// This field will be populated by the top-level JSON unmarshal of the struct.
					// No specific FieldBindingInfo needed for it unless it also had its own `in:"body"` (which would be unusual).
					// The template needs to know not to generate individual binding for these.
					// For now, we can just skip adding it to Fields if it's covered by struct-level body.
					// Or, the template can iterate data.Fields and only act on those with BindFrom set.
					// Let's assume the template handles fields that are part of a struct-level body binding implicitly.
					// If a field *within* a struct-level body target needs *special* handling (e.g. different JSON name),
					// that's a more complex scenario. For now, struct-level `in:"body"` means all fields are from body by default.
					continue
				} else {
					continue // No binding directive for this field, and not part of struct-level body
				}

				originalFieldTypeStr := getTypeStringFromAST(field.Type)
				fInfo := FieldBindingInfo{
					FieldName:               fieldName,
					BindFrom:                bindFrom,
					BindName:                bindName,
					IsRequired:              (fieldTag.Get("required") == "true"),
					OriginalFieldTypeString: originalFieldTypeStr,
				}

				// Determine base type, pointer, slice info from AST node (field.Type)
				var baseTypeForConversion string
				var currentAstType = field.Type

				if starExpr, ok := currentAstType.(*ast.StarExpr); ok {
					fInfo.IsPointer = true
					currentAstType = starExpr.X
				}

				if arrayType, ok := currentAstType.(*ast.ArrayType); ok {
					fInfo.IsSlice = true
					sliceEltType := arrayType.Elt
					if starElt, ok := sliceEltType.(*ast.StarExpr); ok {
						fInfo.IsSliceElementPointer = true
						sliceEltType = starElt.X
					}
					// Now, get the base type string of the element
					// For simplicity, assuming element is *ast.Ident or *ast.SelectorExpr (after ptr deref)
					if ident, ok := sliceEltType.(*ast.Ident); ok {
						fInfo.SliceElementType = ident.Name // This is the base type for parser lookup
						baseTypeForConversion = ident.Name
					} else if selExpr, ok := sliceEltType.(*ast.SelectorExpr); ok {
						// For pkg.Type, we might need to handle imports if parser needs qualified type
						// For now, using just the selector name for parser mapping
						fInfo.SliceElementType = selExpr.Sel.Name
						baseTypeForConversion = selExpr.Sel.Name
						// We might need to add import for selExpr.X if it's an external package
						// For now, parser functions are simple and don't depend on fully qualified types.
					} else {
						slog.Warn("Unhandled slice element type AST", "field", fieldName, "type", fmt.Sprintf("%T", sliceEltType))
						continue
					}
				} else if ident, ok := currentAstType.(*ast.Ident); ok {
					baseTypeForConversion = ident.Name
				} else if selExpr, ok := currentAstType.(*ast.SelectorExpr); ok {
					baseTypeForConversion = selExpr.Sel.Name // Use Name for parser, OriginalString has full
					// Add import for selExpr.X if external and if parser funcs were type-specific beyond builtins
				} else {
					if bindFrom != "body" { // Only error if not body, body allows complex types
						slog.Warn("Unhandled field type AST for non-body binding", "field", fieldName, "type", fmt.Sprintf("%T", currentAstType))
						continue
					}
					// For 'in:"body"', baseTypeForConversion might not be used if it's a complex struct/custom type
				}
				fInfo.FieldType = baseTypeForConversion // For parser lookup

				// Assign parser function based on baseTypeForConversion
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
						slog.Warn("Unhandled base type for non-body binding", "field", fieldName, "base_type", baseTypeForConversion, "bind_from", bindFrom)
						continue
					}
					// If bindFrom is "body", no specific parser func is needed here; JSON unmarshaling handles it.
				}

				if bindFrom != "body" {
					collectedImports["errors"] = "" // For errors.Join
					if fInfo.ParserFunc == "" {
						slog.Warn("No parser func for non-body binding", "field", fieldName)
						continue
					}
				} else { // bindFrom == "body"
					fInfo.IsBody = true
					data.NeedsBody = true // Ensure this is true
					data.HasSpecificBodyFieldTarget = true // This struct has at least one field explicitly from body
					collectedImports["encoding/json"] = ""
					collectedImports["io"] = ""
					collectedImports["fmt"] = ""    // For fmt.Errorf in template
					collectedImports["errors"] = "" // For errors.Join in template
				}
				data.Fields = append(data.Fields, fInfo)
			}

			if len(data.Fields) == 0 && !data.NeedsBody { // If no fields to bind and not a struct-level body target
				slog.Debug("Skipping struct: no bindable fields or global body target", "struct_name", typeSpec.Name.Name)
				continue
			}

			// If it's a struct-level body target but no specific fields were marked `in:"body"`
			if data.NeedsBody && !data.HasSpecificBodyFieldTarget {
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
				return fmt.Errorf("failed to execute template for struct %s: %w", typeSpec.Name.Name, err)
			}
			generatedCodeForAllStructs.Write(currentGeneratedCode.Bytes())
			generatedCodeForAllStructs.WriteString("\n\n")
		}
	}


	if generatedCodeForAllStructs.Len() == 0 {
		slog.InfoContext(ctx, "No structs found requiring Bind method generation in package files.", "package_dir", packageDir, "files", absFilePaths)
		return nil
	}

	// Determine actual package name (firstFilePkgName should be set)
	actualPackageName := firstFilePkgName
	// Fallback already handled if firstFilePkgName was empty after parsing.

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
