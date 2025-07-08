package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"example.com/convert2/internal/model"
)

// ParseDirectory scans the given directory for Go files, parses them,
// and extracts conversion rules and type information.
func ParseDirectory(dirPath string) (*model.ParsedInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dirPath, func(fi os.FileInfo) bool {
		return !fi.IsDir() && strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go") && !strings.HasSuffix(fi.Name(), "_gen.go")
	}, parser.ParseComments)

	if err != nil {
		return nil, fmt.Errorf("failed to parse directory %s: %w", dirPath, err)
	}

	// Assuming a single package in the directory for simplicity,
	// or that all files contribute to one logical package for generation.
	for pkgName, pkg := range pkgs {
		parsedInfo := model.NewParsedInfo(pkgName)

		for fileName, file := range pkg.Files {
			fmt.Printf("Parsing file: %s\n", fileName) // For debugging

			// Parse package-level annotations
			if file.Doc != nil {
				for _, comment := range file.Doc.List {
					parsePackageComment(comment.Text, parsedInfo)
				}
			}
			// If package comments are associated with the package clause itself
			if file.Name != nil && file.Comments != nil {
				for _, commentGroup := range file.Comments {
					// Check comments directly above package clause or on the same line
					// This logic might need refinement based on typical comment placement.
					// For now, we'll iterate through all comments in the file's comment map
					// and check if they are package-level comments.
					// A more robust way is to check file.Doc for package comments.
					// The current parser.ParseComments mode should put package comments in file.Doc.
				}
			}


			ast.Inspect(file, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.GenDecl:
					if decl.Tok == token.TYPE {
						for _, spec := range decl.Specs {
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
								if structType, ok := typeSpec.Type.(*ast.StructType); ok {
									parseStruct(typeSpec, structType, parsedInfo, file)
								}
								// Could handle `type NewType BaseType` here later if needed
							}
						}
					}
				case *ast.Package:
					// This case might be redundant if file.Doc is correctly populated
					if decl.Doc != nil {
						for _, comment := range decl.Doc.List {
							parsePackageComment(comment.Text, parsedInfo)
						}
					}
				}
				return true
			})
		}
		// TODO: Further processing and validation of parsedInfo if needed.
		return parsedInfo, nil // Return after processing the first package found.
	}

	return nil, fmt.Errorf("no Go packages found in directory %s", dirPath)
}

// parsePackageComment parses a single package-level comment line.
// Example: // convert:pair SrcUser -> DstUser, max_errors=10
// Example: // convert:rule "string" -> "time.Time", using=parseTimeRFC3339
// Example: // convert:rule "openapi.Status", validator=ValidateStatus
func parsePackageComment(commentText string, info *model.ParsedInfo) {
	commentText = strings.TrimSpace(strings.TrimPrefix(commentText, "//"))
	parts := strings.Fields(commentText)

	if len(parts) < 2 || parts[0] != "convert:pair" && parts[0] != "convert:rule" {
		return // Not a recognized convert annotation
	}

	directive := parts[0]
	args := parts[1:]

	fmt.Printf("Found package directive: %s with args: %v\n", directive, args) // For debugging

	if directive == "convert:pair" {
		// Expected: <SrcType> "->" <DstType> [options...]
		if len(args) < 3 || args[1] != "->" {
			fmt.Printf("Skipping malformed convert:pair: %s\n", commentText)
			return
		}
		srcType := args[0]
		dstType := args[2]
		maxErrors := 0 // Default

		optionsStr := ""
		if len(args) > 3 {
			optionsStr = strings.Join(args[3:], " ") // Re-join options part
			if strings.HasPrefix(optionsStr, ",") {
				optionsStr = strings.TrimSpace(optionsStr[1:])
			}
		}

		parsedOptions := parseOptions(optionsStr)
		if val, ok := parsedOptions["max_errors"]; ok {
			fmt.Sscanf(val, "%d", &maxErrors) // Basic parsing, error handling can be added
		}

		pair := model.ConversionPair{
			SrcType:   srcType,
			DstType:   dstType,
			MaxErrors: maxErrors,
			// SrcTypeExpr and DstTypeExpr will be resolved later or need more context here
		}
		info.ConversionPairs = append(info.ConversionPairs, pair)
		fmt.Printf("Added ConversionPair: %+v\n", pair)

	} else if directive == "convert:rule" {
		// Expected: "<SrcType>" "->" "<DstType>" "using=<funcName>" OR "<DstType>" "validator=<funcName>"
		ruleText := strings.Join(args, " ")

		// Try parsing as: "<SrcType>" -> "<DstType>", using=<funcName>
		// Example: "string" -> "time.Time", using=parseTimeRFC3339
		usingParts := strings.Split(ruleText, "using=")
		if len(usingParts) == 2 {
			typePartsStr := strings.TrimSpace(strings.TrimSuffix(usingParts[0], ","))
			typeParts := strings.Fields(typePartsStr)
			if len(typeParts) == 3 && typeParts[1] == "->" {
				srcType := strings.Trim(typeParts[0], "\"")
				dstType := strings.Trim(typeParts[2], "\"")
				usingFunc := strings.TrimSpace(usingParts[1])

				rule := model.TypeRule{
					SrcType:   srcType,
					DstType:   dstType,
					UsingFunc: usingFunc,
				}
				info.GlobalRules = append(info.GlobalRules, rule)
				fmt.Printf("Added TypeRule (conversion): %+v\n", rule)
				return
			}
		}

		// Try parsing as: "<DstType>", validator=<funcName>
		// Example: "openapi.Status", validator=ValidateStatus
		validatorParts := strings.Split(ruleText, "validator=")
		if len(validatorParts) == 2 {
			dstTypeStr := strings.TrimSpace(strings.TrimSuffix(validatorParts[0], ","))
			dstType := strings.Trim(dstTypeStr, "\"")
			validatorFunc := strings.TrimSpace(validatorParts[1])

			rule := model.TypeRule{
				DstType:       dstType,
				ValidatorFunc: validatorFunc,
			}
			info.GlobalRules = append(info.GlobalRules, rule)
			fmt.Printf("Added TypeRule (validator): %+v\n", rule)
			return
		}
		fmt.Printf("Skipping malformed convert:rule: %s\n", commentText)
	}
}

// parseStruct parses a struct type spec and its fields.
func parseStruct(typeSpec *ast.TypeSpec, structType *ast.StructType, info *model.ParsedInfo, file *ast.File) {
	structName := typeSpec.Name.Name
	fmt.Printf("Found struct: %s\n", structName) // For debugging

	sInfo := &model.StructInfo{
		Name:   structName,
		Fields: []model.FieldInfo{},
		Node:   structType,
	}

	for _, field := range structType.Fields.List {
		fieldName := ""
		if len(field.Names) > 0 {
			fieldName = field.Names[0].Name
		} else if ident, ok := field.Type.(*ast.Ident); ok {
			// Embedded struct (simple case, e.g. `OtherStruct`)
			// More complex embedded like `pkg.OtherStruct` needs resolver
			fieldName = ident.Name
		} else if se, ok := field.Type.(*ast.SelectorExpr); ok {
			// Embedded struct (e.g. `pkg.OtherStruct`)
			// fieldName here could be just `OtherStruct` or `pkg.OtherStruct` depending on desired representation
			fieldName = se.Sel.Name
		}


		if fieldName == "" {
			fmt.Printf("Skipping unnamed field in struct %s\n", structName)
			continue
		}

		var convertTag model.ConvertTag
		if field.Tag != nil {
			tagValue := strings.Trim(field.Tag.Value, "`")
			// Basic parsing for `convert:"..."`
			// Example: `convert:"dstFieldName,required,using=MyFunc"`
			if strings.HasPrefix(tagValue, "convert:\"") {
				rawTag := strings.TrimSuffix(strings.TrimPrefix(tagValue, "convert:\""), "\"")
				convertTag = parseFieldTag(rawTag)
			}
		}

		fInfo := model.FieldInfo{
			Name:         fieldName,
			OriginalName: fieldName, // May differ if DstFieldName is set
			Type:         field.Type,
			Tag:          convertTag,
			ParentStruct: sInfo,
		}
		sInfo.Fields = append(sInfo.Fields, fInfo)
		fmt.Printf("  Field: %s, Type: %s, Tag: %+v\n", fieldName, astTypeToString(field.Type), convertTag) // For debugging
	}
	info.Structs[structName] = sInfo
}

// parseFieldTag parses the content of a `convert` struct tag.
// Example: "dstFieldName,required,using=MyFunc" or "-"
func parseFieldTag(tagContent string) model.ConvertTag {
	ct := model.ConvertTag{RawValue: tagContent}
	if tagContent == "-" {
		ct.DstFieldName = "-"
		return ct
	}

	parts := strings.Split(tagContent, ",")
	if len(parts) > 0 && !strings.Contains(parts[0], "=") { // First part could be dstFieldName
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
		// Add more option parsing here if needed
	}
	return ct
}

// parseOptions parses a comma-separated key=value string.
// Example: "max_errors=10, other_opt=true"
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


// astTypeToString converts an ast.Expr (representing a type) to its string representation.
// This is a simplified version and may not cover all complex types or qualified identifiers perfectly.
func astTypeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr: // For types like pkg.Type
		return astTypeToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr: // For pointer types like *Type
		return "*" + astTypeToString(t.X)
	case *ast.ArrayType: // For slice or array types like []Type or [N]Type
		lenStr := ""
		if t.Len != nil {
			lenStr = astTypeToString(t.Len) // For arrays, might be a number or const
		}
		return "[" + lenStr + "]" + astTypeToString(t.Elt)
	// Add more cases here for map, func, interface, chan types if needed
	default:
		// Fallback for unknown types - try to use a temporary fileset to format the node
		// This is more robust but requires a fileset. For simplicity, we'll return a placeholder.
		// Or, one could use format.Node from "go/format" but that also needs a fileset.
		return fmt.Sprintf("%T", expr) // Returns the Go type of the AST node itself
	}
}

// Helper function to get package path for an import spec
// (Not used in current parser stub but can be useful later)
func getImportPath(spec *ast.ImportSpec) string {
	if spec.Path != nil {
		return strings.Trim(spec.Path.Value, `"`)
	}
	return ""
}
