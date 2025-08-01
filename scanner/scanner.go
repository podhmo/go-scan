package scanner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"path/filepath"
	"strings"
)

// Scanner parses Go source files within a package.
type Scanner struct {
	fset                  *token.FileSet
	resolver              PackageResolver
	ExternalTypeOverrides ExternalTypeOverride
	Overlay               Overlay
	modulePath            string
	moduleRootDir         string
}

// New creates a new Scanner.
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
func (s *Scanner) ResolveType(ctx context.Context, fieldType *FieldType) (*TypeInfo, error) {
	return fieldType.Resolve(ctx, make(map[string]struct{}))
}

// ScanPackageByImport makes scanner.Scanner implement the PackageResolver interface.
func (s *Scanner) ScanPackageByImport(ctx context.Context, importPath string) (*PackageInfo, error) {
	if s.resolver == nil {
		return nil, fmt.Errorf("scanner's internal resolver is not set, cannot scan by import path %q", importPath)
	}
	return s.resolver.ScanPackageByImport(ctx, importPath)
}

// ScanFiles parses a specific list of .go files and returns PackageInfo.
func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string, pkgDirPath string) (*PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided to scan for package at %s", pkgDirPath)
	}

	relPath, err := filepath.Rel(s.moduleRootDir, pkgDirPath)
	if err != nil {
		slog.WarnContext(ctx, "Could not determine relative path for import path derivation", "dirPath", pkgDirPath, "moduleRootDir", s.moduleRootDir)
		relPath = "."
	}
	importPath := filepath.ToSlash(filepath.Join(s.modulePath, relPath))
	if strings.HasSuffix(importPath, "/.") {
		importPath = importPath[:len(importPath)-2]
	}

	return s.scanGoFiles(ctx, filePaths, pkgDirPath, importPath)
}

// ScanFilesWithKnownImportPath parses files with a predefined import path.
func (s *Scanner) ScanFilesWithKnownImportPath(ctx context.Context, filePaths []string, pkgDirPath string, canonicalImportPath string) (*PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided to scan for package at %s", pkgDirPath)
	}
	return s.scanGoFiles(ctx, filePaths, pkgDirPath, canonicalImportPath)
}

func (s *Scanner) scanGoFiles(ctx context.Context, filePaths []string, pkgDirPath string, canonicalImportPath string) (*PackageInfo, error) {
	info := &PackageInfo{
		Path:       pkgDirPath,
		ImportPath: canonicalImportPath,
		Fset:       s.fset,
		Files:      make([]string, 0, len(filePaths)),
		AstFiles:   make(map[string]*ast.File),
	}
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

		if info.Name == "" {
			info.Name = fileAst.Name.Name
			firstPackageName = fileAst.Name.Name
		} else if fileAst.Name.Name != firstPackageName {
			return nil, fmt.Errorf("mismatched package names: %s and %s in directory %s", firstPackageName, fileAst.Name.Name, pkgDirPath)
		}

		info.Files = append(info.Files, filePath)
		info.AstFiles[filePath] = fileAst

		importLookup := s.buildImportLookup(fileAst)

		for _, decl := range fileAst.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				s.parseGenDecl(ctx, d, info, filePath, importLookup)
			case *ast.FuncDecl:
				info.Functions = append(info.Functions, s.parseFuncDecl(ctx, d, filePath, info, importLookup))
			}
		}
	}
	if info.Name == "" && len(filePaths) > 0 {
		return nil, fmt.Errorf("could not determine package name from scanned files in %s", pkgDirPath)
	}
	return info, nil
}

func (s *Scanner) buildImportLookup(file *ast.File) map[string]string {
	importLookup := make(map[string]string)
	for _, i := range file.Imports {
		path := strings.Trim(i.Path.Value, `"`)
		if i.Name != nil {
			importLookup[i.Name.Name] = path
		} else {
			parts := strings.Split(path, "/")
			importLookup[parts[len(parts)-1]] = path
		}
	}
	return importLookup
}

func (s *Scanner) parseGenDecl(ctx context.Context, decl *ast.GenDecl, info *PackageInfo, absFilePath string, importLookup map[string]string) {
	for _, spec := range decl.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			typeInfo := s.parseTypeSpec(ctx, sp, info, absFilePath, importLookup)
			if typeInfo.Doc == "" && decl.Doc != nil {
				typeInfo.Doc = commentText(decl.Doc)
			}
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
					var inferredFieldType *FieldType

					if i < len(sp.Values) {
						valueExpr := sp.Values[i]
						if lit, ok := valueExpr.(*ast.BasicLit); ok {
							val = lit.Value
							switch lit.Kind {
							case token.STRING:
								inferredFieldType = &FieldType{Name: "string", IsBuiltin: true}
							case token.INT:
								inferredFieldType = &FieldType{Name: "int", IsBuiltin: true}
							case token.FLOAT:
								inferredFieldType = &FieldType{Name: "float64", IsBuiltin: true}
							case token.CHAR:
								inferredFieldType = &FieldType{Name: "rune", IsBuiltin: true}
							}
						}
					}

					var finalFieldType *FieldType
					if sp.Type != nil {
						finalFieldType = s.parseTypeExpr(ctx, sp.Type, nil, info, importLookup)
					} else {
						finalFieldType = inferredFieldType
					}

					info.Constants = append(info.Constants, &ConstantInfo{
						Name:       name.Name,
						FilePath:   absFilePath,
						Doc:        doc,
						Value:      val,
						Type:       finalFieldType,
						IsExported: name.IsExported(),
						Node:       name,
					})
				}
			}
		}
	}
}

func (s *Scanner) parseTypeSpec(ctx context.Context, sp *ast.TypeSpec, info *PackageInfo, absFilePath string, importLookup map[string]string) *TypeInfo {
	typeInfo := &TypeInfo{
		Name:     sp.Name.Name,
		PkgPath:  info.ImportPath,
		FilePath: absFilePath,
		Doc:      commentText(sp.Doc),
		Node:     sp,
	}

	if sp.TypeParams != nil {
		typeInfo.TypeParams = s.parseTypeParamList(ctx, sp.TypeParams.List, info, importLookup)
	}

	switch t := sp.Type.(type) {
	case *ast.StructType:
		typeInfo.Kind = StructKind
		typeInfo.Struct = s.parseStructType(ctx, t, typeInfo.TypeParams, info, importLookup)
	case *ast.InterfaceType:
		typeInfo.Kind = InterfaceKind
		typeInfo.Interface = s.parseInterfaceType(ctx, t, typeInfo.TypeParams, info, importLookup)
	case *ast.FuncType:
		typeInfo.Kind = FuncKind
		typeInfo.Func = s.parseFuncType(ctx, t, typeInfo.TypeParams, info, importLookup)
	default:
		typeInfo.Kind = AliasKind
		typeInfo.Underlying = s.parseTypeExpr(ctx, sp.Type, typeInfo.TypeParams, info, importLookup)
	}
	return typeInfo
}

func (s *Scanner) parseTypeParamList(ctx context.Context, typeParamFields []*ast.Field, info *PackageInfo, importLookup map[string]string) []*TypeParamInfo {
	var params []*TypeParamInfo
	if typeParamFields == nil {
		return nil
	}
	for _, typeParamField := range typeParamFields {
		var constraintFieldType *FieldType
		if constraintExpr := typeParamField.Type; constraintExpr != nil {
			constraintFieldType = s.parseTypeExpr(ctx, constraintExpr, nil, info, importLookup)
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

func (s *Scanner) parseInterfaceType(ctx context.Context, it *ast.InterfaceType, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *InterfaceInfo {
	if it.Methods == nil || len(it.Methods.List) == 0 {
		return &InterfaceInfo{Methods: []*MethodInfo{}}
	}
	interfaceInfo := &InterfaceInfo{
		Methods: make([]*MethodInfo, 0, len(it.Methods.List)),
	}
	for _, field := range it.Methods.List {
		if len(field.Names) > 0 {
			methodName := field.Names[0].Name
			funcType, ok := field.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			methodInfo := &MethodInfo{Name: methodName}
			parsedFuncDetails := s.parseFuncType(ctx, funcType, currentTypeParams, info, importLookup)
			methodInfo.Parameters = parsedFuncDetails.Parameters
			methodInfo.Results = parsedFuncDetails.Results
			interfaceInfo.Methods = append(interfaceInfo.Methods, methodInfo)
		} else {
			embeddedType := s.parseTypeExpr(ctx, field.Type, currentTypeParams, info, importLookup)
			interfaceInfo.Methods = append(interfaceInfo.Methods, &MethodInfo{
				Name:    fmt.Sprintf("embedded_%s", embeddedType.String()),
				Results: []*FieldInfo{{Type: embeddedType}},
			})
		}
	}
	return interfaceInfo
}

func (s *Scanner) parseStructType(ctx context.Context, st *ast.StructType, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *StructInfo {
	structInfo := &StructInfo{}
	for _, field := range st.Fields.List {
		fieldType := s.parseTypeExpr(ctx, field.Type, currentTypeParams, info, importLookup)
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
		} else {
			structInfo.Fields = append(structInfo.Fields, &FieldInfo{
				Name:     fieldType.Name,
				Doc:      doc,
				Type:     fieldType,
				Tag:      tag,
				Embedded: true,
			})
		}
	}
	return structInfo
}

func (s *Scanner) parseFuncDecl(ctx context.Context, f *ast.FuncDecl, absFilePath string, pkgInfo *PackageInfo, importLookup map[string]string) *FunctionInfo {
	var funcOwnTypeParams []*TypeParamInfo
	if f.Type.TypeParams != nil {
		funcOwnTypeParams = s.parseTypeParamList(ctx, f.Type.TypeParams.List, pkgInfo, importLookup)
	}

	funcInfo := s.parseFuncType(ctx, f.Type, funcOwnTypeParams, pkgInfo, importLookup)
	funcInfo.Name = f.Name.Name
	funcInfo.FilePath = absFilePath
	funcInfo.Doc = commentText(f.Doc)
	funcInfo.AstDecl = f
	funcInfo.TypeParams = funcOwnTypeParams

	if f.Recv != nil && len(f.Recv.List) > 0 {
		recvField := f.Recv.List[0]
		var recvName string
		if len(recvField.Names) > 0 {
			recvName = recvField.Names[0].Name
		}

		var receiverBaseTypeParams []*TypeParamInfo
		parsedRecvFieldType := s.parseTypeExpr(ctx, recvField.Type, funcOwnTypeParams, pkgInfo, importLookup)

		if parsedRecvFieldType != nil {
			baseRecvTypeName := parsedRecvFieldType.Name
			if parsedRecvFieldType.IsPointer && parsedRecvFieldType.Elem != nil {
				baseRecvTypeName = parsedRecvFieldType.Elem.Name
			}
			if parts := strings.Split(baseRecvTypeName, "."); len(parts) > 1 {
				baseRecvTypeName = parts[len(parts)-1]
			}

			if pkgInfo != nil {
				for _, ti := range pkgInfo.Types {
					if ti.Name == baseRecvTypeName {
						receiverBaseTypeParams = ti.TypeParams
						parsedRecvFieldType = s.parseTypeExpr(ctx, recvField.Type, receiverBaseTypeParams, pkgInfo, importLookup)
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

		reparsedFuncSignature := s.parseFuncType(ctx, f.Type, methodScopeTypeParams, pkgInfo, importLookup)
		funcInfo.Parameters = reparsedFuncSignature.Parameters
		funcInfo.Results = reparsedFuncSignature.Results
	}
	return funcInfo
}

func (s *Scanner) parseFuncType(ctx context.Context, ft *ast.FuncType, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *FunctionInfo {
	funcInfo := &FunctionInfo{}
	if ft.Params != nil {
		funcInfo.Parameters = s.parseFieldList(ctx, ft.Params.List, currentTypeParams, info, importLookup)
		if len(ft.Params.List) > 0 {
			if _, ok := ft.Params.List[len(ft.Params.List)-1].Type.(*ast.Ellipsis); ok {
				funcInfo.IsVariadic = true
			}
		}
	}
	if ft.Results != nil {
		funcInfo.Results = s.parseFieldList(ctx, ft.Results.List, currentTypeParams, info, importLookup)
	}
	return funcInfo
}

func (s *Scanner) parseFieldList(ctx context.Context, fields []*ast.Field, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) []*FieldInfo {
	var result []*FieldInfo
	for _, field := range fields {
		fieldType := s.parseTypeExpr(ctx, field.Type, currentTypeParams, info, importLookup)
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

func (s *Scanner) parseTypeExpr(ctx context.Context, expr ast.Expr, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *FieldType {
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
			switch t.Name {
			case "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
				"int", "int8", "int16", "int32", "int64", "rune", "string",
				"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
				"any", "comparable":
				ft.IsBuiltin = true
				if t.Name == "any" || t.Name == "comparable" {
					ft.IsConstraint = true
				}
			default:
				if info != nil {
					ft.FullImportPath = info.ImportPath
					ft.TypeName = t.Name
				}
			}
		}
	case *ast.StarExpr:
		underlyingType := s.parseTypeExpr(ctx, t.X, currentTypeParams, info, importLookup)
		underlyingType.IsPointer = true
		return underlyingType
	case *ast.SelectorExpr:
		pkgIdent, ok := t.X.(*ast.Ident)
		if !ok {
			return &FieldType{Name: fmt.Sprintf("unsupported_selector_expr.%s", t.Sel.Name)}
		}
		pkgImportPath, _ := importLookup[pkgIdent.Name]
		qualifiedName := fmt.Sprintf("%s.%s", pkgImportPath, t.Sel.Name)

		// Check for external type overrides first.
		if overrideInfo, ok := s.ExternalTypeOverrides[qualifiedName]; ok {
			// If an override is found, create a FieldType from the synthetic TypeInfo.
			return &FieldType{
				Name:               overrideInfo.Name,
				PkgName:            overrideInfo.PkgPath,
				FullImportPath:     overrideInfo.PkgPath,
				TypeName:           overrideInfo.Name,
				IsResolvedByConfig: true,
				Definition:         overrideInfo, // Link to the synthetic definition
				Resolver:           s.resolver,
			}
		}

		// If no override, proceed with normal parsing.
		ft.PkgName = pkgIdent.Name
		ft.TypeName = t.Sel.Name
		ft.FullImportPath = pkgImportPath
		ft.Name = t.Sel.Name
	case *ast.IndexExpr:
		genericType := s.parseTypeExpr(ctx, t.X, currentTypeParams, info, importLookup)
		typeArg := s.parseTypeExpr(ctx, t.Index, currentTypeParams, info, importLookup)
		genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		return genericType
	case *ast.IndexListExpr:
		genericType := s.parseTypeExpr(ctx, t.X, currentTypeParams, info, importLookup)
		for _, indexExpr := range t.Indices {
			typeArg := s.parseTypeExpr(ctx, indexExpr, currentTypeParams, info, importLookup)
			genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		}
		return genericType
	case *ast.ArrayType:
		ft.IsSlice = true
		ft.Name = "slice"
		ft.Elem = s.parseTypeExpr(ctx, t.Elt, currentTypeParams, info, importLookup)
	case *ast.MapType:
		ft.IsMap = true
		ft.Name = "map"
		ft.MapKey = s.parseTypeExpr(ctx, t.Key, currentTypeParams, info, importLookup)
		ft.Elem = s.parseTypeExpr(ctx, t.Value, currentTypeParams, info, importLookup)
	case *ast.InterfaceType:
		ft.Name = "interface{}"
	case *ast.Ellipsis:
		ft.IsSlice = true
		ft.Name = "slice"
		ft.Elem = s.parseTypeExpr(ctx, t.Elt, currentTypeParams, info, importLookup)
	default:
		ft.Name = fmt.Sprintf("unhandled_type_%T", t)
	}
	return ft
}

func commentText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}
