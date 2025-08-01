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
	Overlay               Overlay
	modulePath            string
	moduleRootDir         string
	currentPkg            *PackageInfo
}

// New creates a new Scanner.
// The fset must be provided and is used for all parsing operations by this scanner instance.
func New(fset *token.FileSet, overrides ExternalTypeOverride, overlay Overlay, modulePath string, moduleRootDir string, resolver PackageResolver) (*Scanner, error) {
	if fset == nil {
		return nil, fmt.Errorf("fset cannot be nil")
	}
	if overrides == nil {
		overrides = make(ExternalTypeOverride)
	}
	if overlay == nil {
		overlay = make(Overlay)
	}
	if modulePath == "" || moduleRootDir == "" {
		return nil, fmt.Errorf("modulePath and moduleRootDir must be provided")
	}
	if resolver == nil {
		return nil, fmt.Errorf("resolver cannot be nil")
	}

	return &Scanner{
		fset:                  fset,
		ExternalTypeOverrides: overrides,
		Overlay:               overlay,
		modulePath:            modulePath,
		moduleRootDir:         moduleRootDir,
		resolver:              resolver,
	}, nil
}

// ResolveType starts the type resolution process for a given field type.
// It handles circular dependencies by tracking the resolution path.
// It's the public entry point for resolving types, initializing a new resolution tracker.
func (s *Scanner) ResolveType(ctx context.Context, fieldType *FieldType) (*TypeInfo, error) {
	// The internal Resolve method is called with a new, empty map for tracking.
	return fieldType.Resolve(ctx, make(map[string]struct{}))
}

// ScanPackageByImport makes scanner.Scanner implement the PackageResolver interface.
// It delegates the call to the configured resolver, which is typically the top-level
// goscan.Scanner instance.
func (s *Scanner) ScanPackageByImport(ctx context.Context, importPath string) (*PackageInfo, error) {
	if s.resolver == s {
		if s.currentPkg != nil && s.currentPkg.ImportPath == importPath {
			return s.currentPkg, nil
		}
		// This might indicate a logic error where the scanner is trying to resolve a package
		// it's already in the process of scanning, but the import path doesn't match.
		return nil, fmt.Errorf("internal resolver loop detected for import path %q, current package is %q", importPath, s.currentPkg.ImportPath)
	}
	if s.resolver == nil {
		return nil, fmt.Errorf("scanner's internal resolver is not set, cannot scan by import path %q", importPath)
	}
	return s.resolver.ScanPackageByImport(ctx, importPath)
}

// ScanPackage parses all .go files in a given directory and returns PackageInfo.
// It now uses ScanFiles internally.
func (s *Scanner) ScanPackage(ctx context.Context, dirPath string) (*PackageInfo, error) {
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
	return s.ScanFiles(ctx, filePaths, dirPath, "")
}

func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string, pkgDirPath string, importPathOverride string) (*PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided to scan for package at %s", pkgDirPath)
	}

	var importPath string
	if importPathOverride != "" {
		importPath = importPathOverride
	} else {
		relPath, err := filepath.Rel(s.moduleRootDir, pkgDirPath)
		if err != nil {
			slog.WarnContext(ctx, "Could not determine relative path for import path derivation", "dirPath", pkgDirPath, "moduleRootDir", s.moduleRootDir)
			relPath = "."
		}
		importPath = filepath.ToSlash(filepath.Join(s.modulePath, relPath))
		if strings.HasSuffix(importPath, "/.") {
			importPath = importPath[:len(importPath)-2]
		}
	}

	info := &PackageInfo{
		Path:       pkgDirPath,
		ImportPath: importPath,
		Fset:       s.fset,
		Files:      make([]string, 0, len(filePaths)),
		AstFiles:   make(map[string]*ast.File),
	}
	s.currentPkg = info
	var firstPackageName string

	for _, filePath := range filePaths {
		var content any
		if s.Overlay != nil {
			relPath, err := filepath.Rel(s.moduleRootDir, filePath)
			if err == nil {
				if overlayContent, ok := s.Overlay[relPath]; ok {
					content = overlayContent
				}
			}
		}

		fileAst, err := parser.ParseFile(s.fset, filePath, content, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
		}

		if firstPackageName == "" {
			firstPackageName = fileAst.Name.Name
			info.Name = firstPackageName
		} else if fileAst.Name.Name != firstPackageName {
			slog.WarnContext(ctx, "Skipping file with mismatched package name in directory",
				"directory", pkgDirPath,
				"expected_package", firstPackageName,
				"found_package", fileAst.Name.Name,
				"file_path", filePath)
			continue
		}

		info.Files = append(info.Files, filePath)
		info.AstFiles[filePath] = fileAst
		slog.DebugContext(ctx, "Processing file for package", slog.String("filePath", filePath), slog.String("packageName", info.Name))
		s.buildImportLookup(fileAst)
		slog.DebugContext(ctx, "Built import lookup", slog.String("filePath", filePath), slog.Any("imports", s.importLookup))
		for declIndex, decl := range fileAst.Decls {
			slog.DebugContext(ctx, "Processing declaration", slog.String("filePath", filePath), slog.Int("declIndex", declIndex), slog.String("type", fmt.Sprintf("%T", decl)))
			switch d := decl.(type) {
			case *ast.GenDecl:
				slog.DebugContext(ctx, "Processing GenDecl", slog.String("token", d.Tok.String()), slog.String("filePath", filePath), slog.Int("specs", len(d.Specs)))
				s.parseGenDecl(ctx, d, info, filePath)
			case *ast.FuncDecl:
				slog.DebugContext(ctx, "Processing FuncDecl", slog.String("name", d.Name.Name), slog.String("filePath", filePath))
				info.Functions = append(info.Functions, s.parseFuncDecl(ctx, d, filePath, info))
			}
		}
	}
	if info.Name == "" && len(filePaths) > 0 {
		return nil, fmt.Errorf("could not determine package name from scanned files in %s", pkgDirPath)
	}
	if len(info.Files) == 0 {
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
						finalFieldType = s.parseTypeExpr(ctx, sp.Type, nil) // Pass ctx and nil for currentTypeParams
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
		PkgPath:  s.currentPkg.ImportPath,
		FilePath: absFilePath,
		Doc:      commentText(sp.Doc),
		Node:     sp,
	}

	// Parse type parameters if they exist (Go 1.18+)
	if sp.TypeParams != nil {
		typeInfo.TypeParams = s.parseTypeParamList(ctx, sp.TypeParams.List)
	}

	switch t := sp.Type.(type) {
	case *ast.StructType:
		typeInfo.Kind = StructKind
		typeInfo.Struct = s.parseStructType(ctx, t, typeInfo.TypeParams) // Pass ctx and type params for context
	case *ast.InterfaceType:
		typeInfo.Kind = InterfaceKind
		typeInfo.Interface = s.parseInterfaceType(ctx, t, typeInfo.TypeParams) // Pass type params for context
	case *ast.FuncType:
		typeInfo.Kind = FuncKind
		typeInfo.Func = s.parseFuncType(ctx, t, typeInfo.TypeParams) // Pass ctx and type params for context
	default:
		typeInfo.Kind = AliasKind
		typeInfo.Underlying = s.parseTypeExpr(ctx, sp.Type, typeInfo.TypeParams) // Pass ctx and type params
	}
	return typeInfo
}

// parseTypeParamList parses a list of ast.Field representing type parameters.
func (s *Scanner) parseTypeParamList(ctx context.Context, typeParamFields []*ast.Field) []*TypeParamInfo {
	var params []*TypeParamInfo
	if typeParamFields == nil {
		return nil
	}
	for _, typeParamField := range typeParamFields { // Each ast.Field in TypeParams.List
		constraintExpr := typeParamField.Type
		var constraintFieldType *FieldType
		if constraintExpr != nil {
			constraintFieldType = s.parseTypeExpr(ctx, constraintExpr, nil) // Pass ctx, No currentTypeParams for the constraint itself
			if constraintFieldType != nil {
				constraintFieldType.IsConstraint = true
			}
		}
		for _, nameIdent := range typeParamField.Names {
			params = append(params, &TypeParamInfo{
				Name:       nameIdent.Name,
				Constraint: constraintFieldType,
			})
		}
	}
	return params
}

// parseInterfaceType parses an interface type.
func (s *Scanner) parseInterfaceType(ctx context.Context, it *ast.InterfaceType, currentTypeParams []*TypeParamInfo) *InterfaceInfo {
	if it.Methods == nil || len(it.Methods.List) == 0 {
		return &InterfaceInfo{Methods: []*MethodInfo{}} // Empty interface
	}
	interfaceInfo := &InterfaceInfo{
		Methods: make([]*MethodInfo, 0, len(it.Methods.List)),
	}
	for _, field := range it.Methods.List {
		if len(field.Names) > 0 { // Method signature
			methodName := field.Names[0].Name
			funcType, ok := field.Type.(*ast.FuncType)
			if !ok {
				slog.WarnContext(ctx, "Expected FuncType for interface method, skipping", slog.String("method_name", methodName), slog.String("got_type", fmt.Sprintf("%T", field.Type)))
				continue
			}
			methodInfo := &MethodInfo{
				Name: methodName,
			}
			parsedFuncDetails := s.parseFuncType(ctx, funcType, currentTypeParams) // Pass ctx and currentTypeParams
			methodInfo.Parameters = parsedFuncDetails.Parameters
			methodInfo.Results = parsedFuncDetails.Results

			interfaceInfo.Methods = append(interfaceInfo.Methods, methodInfo)
		} else {
			embeddedType := s.parseTypeExpr(ctx, field.Type, currentTypeParams) // Pass ctx and currentTypeParams
			interfaceInfo.Methods = append(interfaceInfo.Methods, &MethodInfo{
				Name:       fmt.Sprintf("embedded_%s", embeddedType.String()), // Placeholder name
				Parameters: nil,                                               // Not a real method signature in the same way
				Results:    []*FieldInfo{{Type: embeddedType}},                // Store the type here
			})
			slog.InfoContext(ctx, "Embedded interface/constraint in interface definition", slog.String("type", embeddedType.String()))
		}
	}
	return interfaceInfo
}

// parseStructType parses a struct type.
func (s *Scanner) parseStructType(ctx context.Context, st *ast.StructType, currentTypeParams []*TypeParamInfo) *StructInfo {
	structInfo := &StructInfo{}
	for _, field := range st.Fields.List {
		fieldType := s.parseTypeExpr(ctx, field.Type, currentTypeParams) // Pass ctx and currentTypeParams
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
func (s *Scanner) parseFuncDecl(ctx context.Context, f *ast.FuncDecl, absFilePath string, pkgInfo *PackageInfo) *FunctionInfo {
	var funcOwnTypeParams []*TypeParamInfo // Renamed from currentTypeParams to avoid confusion
	if f.Type.TypeParams != nil {
		funcOwnTypeParams = s.parseTypeParamList(ctx, f.Type.TypeParams.List) // Pass ctx
	}

	funcInfo := s.parseFuncType(ctx, f.Type, funcOwnTypeParams) // Use funcOwnTypeParams

	funcInfo.Name = f.Name.Name
	funcInfo.FilePath = absFilePath
	funcInfo.Doc = commentText(f.Doc)
	funcInfo.AstDecl = f
	funcInfo.TypeParams = funcOwnTypeParams // Assign parsed type parameters of the function itself

	if f.Recv != nil && len(f.Recv.List) > 0 { // This is a method
		recvField := f.Recv.List[0]
		var recvName string
		if len(recvField.Names) > 0 {
			recvName = recvField.Names[0].Name
		}

		var receiverBaseTypeParams []*TypeParamInfo
		parsedRecvFieldType := s.parseTypeExpr(ctx, recvField.Type, funcOwnTypeParams)

		if parsedRecvFieldType != nil {
			baseRecvTypeName := parsedRecvFieldType.Name
			if parsedRecvFieldType.IsPointer && parsedRecvFieldType.Elem != nil {
				baseRecvTypeName = parsedRecvFieldType.Elem.Name
			}
			if parts := strings.Split(baseRecvTypeName, "."); len(parts) > 1 {
				baseRecvTypeName = parts[len(parts)-1]
			}

			if pkgInfo != nil { // pkgInfo is passed to parseFuncDecl
				for _, ti := range pkgInfo.Types {
					if ti.Name == baseRecvTypeName {
						receiverBaseTypeParams = ti.TypeParams // These are the TPs of the struct (e.g. T from List[T])
						parsedRecvFieldType = s.parseTypeExpr(ctx, recvField.Type, receiverBaseTypeParams)
						break
					}
				}
			}
		}

		funcInfo.Receiver = &FieldInfo{
			Name: recvName,
			Type: parsedRecvFieldType,
		}

		methodScopeTypeParams := append([]*TypeParamInfo{}, receiverBaseTypeParams...)
		methodScopeTypeParams = append(methodScopeTypeParams, funcOwnTypeParams...)

		reparsedFuncSignature := s.parseFuncType(ctx, f.Type, methodScopeTypeParams)
		funcInfo.Parameters = reparsedFuncSignature.Parameters
		funcInfo.Results = reparsedFuncSignature.Results
	}
	return funcInfo
}

// parseFuncType parses a function type (signature).
func (s *Scanner) parseFuncType(ctx context.Context, ft *ast.FuncType, currentTypeParams []*TypeParamInfo) *FunctionInfo {
	funcInfo := &FunctionInfo{}
	if ft.Params != nil {
		funcInfo.Parameters = s.parseFieldList(ctx, ft.Params.List, currentTypeParams)
		if len(ft.Params.List) > 0 {
			lastParam := ft.Params.List[len(ft.Params.List)-1]
			if _, ok := lastParam.Type.(*ast.Ellipsis); ok {
				funcInfo.IsVariadic = true
			}
		}
	}
	if ft.Results != nil {
		funcInfo.Results = s.parseFieldList(ctx, ft.Results.List, currentTypeParams)
	}
	return funcInfo
}

// parseFieldList parses a list of fields (parameters or results).
func (s *Scanner) parseFieldList(ctx context.Context, fields []*ast.Field, currentTypeParams []*TypeParamInfo) []*FieldInfo {
	var result []*FieldInfo
	for _, field := range fields {
		fieldType := s.parseTypeExpr(ctx, field.Type, currentTypeParams) // Pass ctx and currentTypeParams
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				result = append(result, &FieldInfo{Name: name.Name, Type: fieldType, Doc: commentText(field.Doc)})
			}
		} else {
			result = append(result, &FieldInfo{Type: fieldType, Doc: commentText(field.Doc)})
		}
	}
	return result
}

// parseTypeExpr parses an expression representing a type.
func (s *Scanner) parseTypeExpr(ctx context.Context, expr ast.Expr, currentTypeParams []*TypeParamInfo) *FieldType {
	ft := &FieldType{Resolver: s.resolver}
	switch t := expr.(type) {
	case *ast.Ident:
		ft.Name = t.Name
		isTypeParam := false
		if currentTypeParams != nil {
			for _, tp := range currentTypeParams {
				if tp.Name == t.Name {
					isTypeParam = true
					break
				}
			}
		}
		if isTypeParam {
			ft.IsTypeParam = true
		} else {
			isBuiltin := false
			switch t.Name {
			case "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
				"int", "int8", "int16", "int32", "int64", "rune", "string",
				"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
				"any", "comparable":
				isBuiltin = true
				ft.IsBuiltin = true
				if t.Name == "any" || t.Name == "comparable" {
					ft.IsConstraint = true
				}
			}
			if !isBuiltin && s.currentPkg != nil {
				ft.FullImportPath = s.currentPkg.ImportPath
				ft.TypeName = t.Name
			}
		}
	case *ast.StarExpr:
		underlyingType := s.parseTypeExpr(ctx, t.X, currentTypeParams) // Pass ctx and currentTypeParams
		underlyingType.IsPointer = true
		return underlyingType
	case *ast.SelectorExpr:
		pkgIdent, ok := t.X.(*ast.Ident)
		if !ok {
			slog.Warn("Unhandled SelectorExpr with non-Ident X part", slog.Any("selector_x_type", fmt.Sprintf("%T", t.X)))
			ft.Name = fmt.Sprintf("unsupported_selector_expr.%s", t.Sel.Name) // Fallback name
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
		ft.TypeName = t.Sel.Name
		ft.FullImportPath = pkgImportPath
	case *ast.IndexExpr:
		genericType := s.parseTypeExpr(ctx, t.X, currentTypeParams)
		if genericType.IsTypeParam {
			slog.WarnContext(ctx, "IndexExpr on a type parameter, this might not be fully supported or implies an error", slog.String("type_param_name", genericType.Name))
		}
		typeArg := s.parseTypeExpr(ctx, t.Index, currentTypeParams)
		genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		return genericType
	case *ast.IndexListExpr:
		genericType := s.parseTypeExpr(ctx, t.X, currentTypeParams)
		if genericType.IsTypeParam {
			slog.WarnContext(ctx, "IndexListExpr on a type parameter, this might not be fully supported or implies an error", slog.String("type_param_name", genericType.Name))
		}
		for _, indexExpr := range t.Indices {
			typeArg := s.parseTypeExpr(ctx, indexExpr, currentTypeParams)
			genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		}
		return genericType
	case *ast.ArrayType:
		ft.IsSlice = true
		ft.Name = "slice"
		ft.Elem = s.parseTypeExpr(ctx, t.Elt, currentTypeParams)
	case *ast.MapType:
		ft.IsMap = true
		ft.Name = "map"
		ft.MapKey = s.parseTypeExpr(ctx, t.Key, currentTypeParams)
		ft.Elem = s.parseTypeExpr(ctx, t.Value, currentTypeParams)
	case *ast.InterfaceType:
		ft.Name = "interface{}"
	case *ast.Ellipsis:
		ft.IsSlice = true
		ft.Name = "slice"
		ft.Elem = s.parseTypeExpr(ctx, t.Elt, currentTypeParams)
	default:
		slog.Warn("Unhandled type expression", slog.String("type", fmt.Sprintf("%T", t)))
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
