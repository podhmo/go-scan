package scanner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath" // Added for filepath.Join
	"strings"
)

// Scanner parses Go source files within a package.
type Scanner struct {
	fset                  *token.FileSet // FileSet to use for parsing. Must be provided.
	resolver              PackageResolver
	importLookup          map[string]string // Maps import alias/name to full import path for the current file.
	ExternalTypeOverrides ExternalTypeOverride
}

// New creates a new Scanner.
// The fset must be provided and is used for all parsing operations by this scanner instance.
func New(fset *token.FileSet, overrides ExternalTypeOverride) (*Scanner, error) {
	if fset == nil {
		return nil, fmt.Errorf("fset cannot be nil")
	}
	if overrides == nil {
		overrides = make(ExternalTypeOverride)
	}
	return &Scanner{
		fset:                  fset,
		ExternalTypeOverrides: overrides,
	}, nil
}

// ScanPackage parses all .go files in a given directory and returns PackageInfo.
// It now uses ScanFiles internally.
func (s *Scanner) ScanPackage(ctx context.Context, dirPath string, resolver PackageResolver) (*PackageInfo, error) {
	s.resolver = resolver // Store resolver for use by parseTypeExpr etc.

	// List all .go files in the directory, excluding _test.go files.
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	var filePaths []string
	for _, entry := range dirEntries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			filePaths = append(filePaths, filepath.Join(dirPath, entry.Name()))
		}
	}

	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no buildable Go source files in %s", dirPath)
	}

	// Delegate to ScanFiles.
	// pkgImportPath for ScanFiles could be derived or passed in.
	// For now, let ScanFiles derive it or handle it based on its needs.
	// dirPath itself serves as the package's unique path identifier for PackageInfo.Path.
	return s.ScanFiles(ctx, filePaths, dirPath, resolver)
}

// ScanFiles parses a specific list of .go files and returns PackageInfo.
// pkgDirPath is the absolute directory path for this package, used for PackageInfo.Path.
func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string, pkgDirPath string, resolver PackageResolver) (*PackageInfo, error) {
	s.resolver = resolver // Ensure resolver is set for this scanning operation.

	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided to scan for package at %s", pkgDirPath)
	}

	info := &PackageInfo{
		Path:     pkgDirPath, // Physical directory path
		Fset:     s.fset,     // Use the shared FileSet
		Files:    make([]string, 0, len(filePaths)),
		AstFiles: make(map[string]*ast.File), // Initialize AstFiles
	}
	var firstPackageName string

	for _, filePath := range filePaths {
		// filePath here is absolute.
		fileAst, err := parser.ParseFile(s.fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
		}

		if info.Name == "" {
			info.Name = fileAst.Name.Name
			firstPackageName = fileAst.Name.Name
		} else if fileAst.Name.Name != firstPackageName {
			return nil, fmt.Errorf("mismatched package names: %s and %s in directory %s",
				firstPackageName, fileAst.Name.Name, pkgDirPath)
		}

		info.Files = append(info.Files, filePath) // Store absolute file path
		info.AstFiles[filePath] = fileAst         // Store AST
		slog.DebugContext(ctx, "Processing file for package", slog.String("filePath", filePath), slog.String("packageName", info.Name))
		s.buildImportLookup(fileAst)
		slog.DebugContext(ctx, "Built import lookup", slog.String("filePath", filePath), slog.Any("imports", s.importLookup))
		for declIndex, decl := range fileAst.Decls {
			slog.DebugContext(ctx, "Processing declaration", slog.String("filePath", filePath), slog.Int("declIndex", declIndex), slog.String("type", fmt.Sprintf("%T", decl)))
			switch d := decl.(type) {
			case *ast.GenDecl:
				slog.DebugContext(ctx, "Processing GenDecl", slog.String("token", d.Tok.String()), slog.String("filePath", filePath), slog.Int("specs", len(d.Specs)))
				s.parseGenDecl(ctx, d, info, filePath) // Pass context
			case *ast.FuncDecl:
				slog.DebugContext(ctx, "Processing FuncDecl", slog.String("name", d.Name.Name), slog.String("filePath", filePath))
				info.Functions = append(info.Functions, s.parseFuncDecl(d, filePath))
			}
		}
	}
	if info.Name == "" && len(filePaths) > 0 {
		// This case should ideally not be reached if ParseFile succeeds and files are valid Go files.
		return nil, fmt.Errorf("could not determine package name from scanned files in %s", pkgDirPath)
	}
	if len(info.Files) == 0 { // Should be redundant given the initial check, but as a safeguard.
		return nil, fmt.Errorf("no buildable Go source files processed in %s", pkgDirPath)
	}

	return info, nil
}

// buildImportLookup populates the importLookup map for the current file.
func (s *Scanner) buildImportLookup(file *ast.File) {
	s.importLookup = make(map[string]string)
	for _, i := range file.Imports {
		path := strings.Trim(i.Path.Value, `"`)
		if i.Name != nil {
			s.importLookup[i.Name.Name] = path
		} else {
			parts := strings.Split(path, "/")
			s.importLookup[parts[len(parts)-1]] = path
		}
	}
}

// parseGenDecl parses a general declaration (types, constants, variables).
func (s *Scanner) parseGenDecl(ctx context.Context, decl *ast.GenDecl, info *PackageInfo, absFilePath string) {
	for _, spec := range decl.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			typeInfo := s.parseTypeSpec(ctx, sp, absFilePath)
			if typeInfo.Doc == "" && decl.Doc != nil {
				typeInfo.Doc = commentText(decl.Doc)
			}
			slog.DebugContext(ctx, "Parsed TypeSpec, adding to PackageInfo", slog.String("name", typeInfo.Name), slog.Any("kind", typeInfo.Kind), slog.String("filePath", typeInfo.FilePath), slog.Int("currentTypesCount", len(info.Types)))
			info.Types = append(info.Types, typeInfo)
		case *ast.ValueSpec:
			if decl.Tok == token.CONST {
				doc := commentText(sp.Doc)
				if doc == "" && sp.Comment != nil {
					doc = commentText(sp.Comment)
				}
				if doc == "" && decl.Doc != nil {
					doc = commentText(decl.Doc)
				}
				for i, name := range sp.Names {
					var val string
					var inferredFieldType *FieldType // For type inference

					if i < len(sp.Values) {
						valueExpr := sp.Values[i]
						if lit, ok := valueExpr.(*ast.BasicLit); ok {
							val = lit.Value
							// Infer type from value if sp.Type is nil
							switch lit.Kind {
							case token.STRING:
								inferredFieldType = &FieldType{Name: "string", IsBuiltin: true}
							case token.INT:
								inferredFieldType = &FieldType{Name: "int", IsBuiltin: true}
							case token.FLOAT:
								inferredFieldType = &FieldType{Name: "float64", IsBuiltin: true}
							case token.CHAR:
								inferredFieldType = &FieldType{Name: "rune", IsBuiltin: true}
							default:
								slog.WarnContext(ctx, "Unhandled BasicLit kind for constant type inference", slog.String("kind", lit.Kind.String()), slog.String("const_name", name.Name), slog.String("filePath", absFilePath))
							}
						} else {
							slog.InfoContext(ctx, "Constant value is not a BasicLit, type inference might be limited", slog.String("const_name", name.Name), slog.String("value_type", fmt.Sprintf("%T", valueExpr)), slog.String("filePath", absFilePath))
						}
					}

					var finalFieldType *FieldType
					if sp.Type != nil { // Explicit type is present
						finalFieldType = s.parseTypeExpr(sp.Type)
					} else { // No explicit type, use inferred type
						finalFieldType = inferredFieldType
					}

					info.Constants = append(info.Constants, &ConstantInfo{
						Name:       name.Name,
						FilePath:   absFilePath,
						Doc:        doc,
						Value:      val,
						Type:       finalFieldType, // Use the determined field type
						IsExported: name.IsExported(),
						Node:       name,
					})
				}
			}
		}
	}
}

// parseTypeSpec parses a type specification.
func (s *Scanner) parseTypeSpec(ctx context.Context, sp *ast.TypeSpec, absFilePath string) *TypeInfo {
	typeInfo := &TypeInfo{
		Name:     sp.Name.Name,
		FilePath: absFilePath,
		Doc:      commentText(sp.Doc),
		Node:     sp,
	}
	switch t := sp.Type.(type) {
	case *ast.StructType:
		typeInfo.Kind = StructKind
		typeInfo.Struct = s.parseStructType(t) // parseStructType does not log with context yet
	case *ast.InterfaceType: // Added case for interface types
		typeInfo.Kind = InterfaceKind
		typeInfo.Interface = s.parseInterfaceType(ctx, t)
	case *ast.FuncType:
		typeInfo.Kind = FuncKind
		typeInfo.Func = s.parseFuncType(t) // parseFuncType does not log with context
	default:
		typeInfo.Kind = AliasKind
		typeInfo.Underlying = s.parseTypeExpr(sp.Type) // sp.Type is correct here for alias
	}
	return typeInfo
}

// parseInterfaceType parses an interface type.
func (s *Scanner) parseInterfaceType(ctx context.Context, it *ast.InterfaceType) *InterfaceInfo {
	if it.Methods == nil || len(it.Methods.List) == 0 {
		return &InterfaceInfo{Methods: []*MethodInfo{}} // Empty interface
	}
	interfaceInfo := &InterfaceInfo{
		Methods: make([]*MethodInfo, 0, len(it.Methods.List)),
	}
	for _, field := range it.Methods.List {
		// Interface methods can be:
		// 1. Method signature: Name + Type (which is *ast.FuncType)
		// 2. Embedded interface: Type (which is an *ast.Ident or *ast.SelectorExpr referring to another interface)
		if len(field.Names) > 0 { // Method signature
			methodName := field.Names[0].Name
			funcType, ok := field.Type.(*ast.FuncType)
			if !ok {
				// This case should ideally not happen for a valid Go interface method.
				// Skip or log an error if it does.
				slog.WarnContext(ctx, "Expected FuncType for interface method, skipping", slog.String("method_name", methodName), slog.String("got_type", fmt.Sprintf("%T", field.Type)))
				continue
			}
			methodInfo := &MethodInfo{
				Name: methodName,
			}
			if funcType.Params != nil {
				methodInfo.Parameters = s.parseFieldList(funcType.Params.List)
			}
			if funcType.Results != nil {
				methodInfo.Results = s.parseFieldList(funcType.Results.List)
			}
			interfaceInfo.Methods = append(interfaceInfo.Methods, methodInfo)
		} else {
			// Embedded interface: field.Type is the name of the embedded interface
			// We need to resolve this type to get its methods and add them to the current interface.
			// This requires the resolver and potentially looking up the type.
			// For now, we will store the name of the embedded interface.
			// A more complete implementation would recursively resolve and inline methods.
			embeddedTypeName := s.parseTypeExpr(field.Type)
			// TODO: Handle embedded interfaces by resolving and merging their methods.
			// For now, we can represent it as a special kind of "method" or skip.
			// Let's skip direct handling of embedded interfaces for now to simplify,
			// as `goscan.Implements` will need to handle this when comparing.
			// Or, we can add a placeholder if needed for `Implements` to recognize it.
			// For `derivingjson`, direct method signatures are the primary concern.
			slog.InfoContext(ctx, "Embedded interface found, its methods are not recursively parsed in this version.", slog.String("interface_name", embeddedTypeName.Name))
		}
	}
	return interfaceInfo
}

// parseStructType parses a struct type.
func (s *Scanner) parseStructType(st *ast.StructType) *StructInfo {
	structInfo := &StructInfo{}
	for _, field := range st.Fields.List {
		fieldType := s.parseTypeExpr(field.Type)
		var tag string
		if field.Tag != nil {
			tag = strings.Trim(field.Tag.Value, "`")
		}
		doc := commentText(field.Doc)
		if doc == "" {
			doc = commentText(field.Comment)
		}
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				structInfo.Fields = append(structInfo.Fields, &FieldInfo{
					Name: name.Name,
					Doc:  doc,
					Type: fieldType,
					Tag:  tag,
				})
			}
		} else { // Embedded field
			structInfo.Fields = append(structInfo.Fields, &FieldInfo{
				Name:     fieldType.Name, // For embedded, field name is type name
				Doc:      doc,
				Type:     fieldType,
				Tag:      tag,
				Embedded: true,
			})
		}
	}
	return structInfo
}

// parseFuncDecl parses a function declaration.
func (s *Scanner) parseFuncDecl(f *ast.FuncDecl, absFilePath string) *FunctionInfo {
	funcInfo := s.parseFuncType(f.Type)
	funcInfo.Name = f.Name.Name
	funcInfo.FilePath = absFilePath
	funcInfo.Doc = commentText(f.Doc)
	if f.Recv != nil && len(f.Recv.List) > 0 {
		recvField := f.Recv.List[0]
		var recvName string
		if len(recvField.Names) > 0 {
			recvName = recvField.Names[0].Name
		}
		funcInfo.Receiver = &FieldInfo{
			Name: recvName,
			Type: s.parseTypeExpr(recvField.Type),
		}
	}
	return funcInfo
}

// parseFuncType parses a function type (signature).
func (s *Scanner) parseFuncType(ft *ast.FuncType) *FunctionInfo {
	funcInfo := &FunctionInfo{}
	if ft.Params != nil {
		funcInfo.Parameters = s.parseFieldList(ft.Params.List)
	}
	if ft.Results != nil {
		funcInfo.Results = s.parseFieldList(ft.Results.List)
	}
	return funcInfo
}

// parseFieldList parses a list of fields (parameters or results).
func (s *Scanner) parseFieldList(fields []*ast.Field) []*FieldInfo {
	var result []*FieldInfo
	for _, field := range fields {
		fieldType := s.parseTypeExpr(field.Type)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				result = append(result, &FieldInfo{Name: name.Name, Type: fieldType})
			}
		} else {
			result = append(result, &FieldInfo{Type: fieldType})
		}
	}
	return result
}

// parseTypeExpr parses an expression representing a type.
func (s *Scanner) parseTypeExpr(expr ast.Expr) *FieldType {
	ft := &FieldType{resolver: s.resolver}
	switch t := expr.(type) {
	case *ast.Ident:
		ft.Name = t.Name
		// Check if it's a built-in type
		// https://golang.org/ref/spec#Predeclared_identifiers
		switch t.Name {
		case "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
			"int", "int8", "int16", "int32", "int64", "rune", "string",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
			ft.IsBuiltin = true
		}
	case *ast.StarExpr:
		underlyingType := s.parseTypeExpr(t.X)
		underlyingType.IsPointer = true
		return underlyingType
	case *ast.SelectorExpr:
		pkgIdent, ok := t.X.(*ast.Ident)
		if !ok {
			ft.Name = "unsupported_selector"
			return ft
		}
		pkgImportPath, _ := s.importLookup[pkgIdent.Name]
		qualifiedName := fmt.Sprintf("%s.%s", pkgImportPath, t.Sel.Name)
		if overrideType, ok := s.ExternalTypeOverrides[qualifiedName]; ok {
			ft.Name = overrideType
			ft.IsResolvedByConfig = true
			return ft
		}
		ft.Name = fmt.Sprintf("%s.%s", pkgIdent.Name, t.Sel.Name)
		ft.PkgName = pkgIdent.Name
		ft.typeName = t.Sel.Name
		ft.fullImportPath = pkgImportPath
	case *ast.ArrayType:
		ft.IsSlice = true
		ft.Name = "slice" // Or construct "[]ElemName"
		ft.Elem = s.parseTypeExpr(t.Elt)
	case *ast.MapType:
		ft.IsMap = true
		ft.Name = "map" // Or construct "map[KeyName]ValueName"
		ft.MapKey = s.parseTypeExpr(t.Key)
		ft.Elem = s.parseTypeExpr(t.Value)
	default:
		ft.Name = fmt.Sprintf("unhandled_type_%T", t)
	}
	return ft
}

// commentText extracts the text from a comment group.
func commentText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}

// (No trailing comments or code after the last function - ensure this is the true end of the file)
