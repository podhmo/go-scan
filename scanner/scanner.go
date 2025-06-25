package scanner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// Scanner parses Go source files within a package.
type Scanner struct {
	resolver              PackageResolver
	importLookup          map[string]string // Maps import alias/name to full import path for the current file.
	ExternalTypeOverrides ExternalTypeOverride
}

// New creates a new Scanner.
func New(overrides ExternalTypeOverride) *Scanner {
	if overrides == nil {
		overrides = make(ExternalTypeOverride)
	}
	return &Scanner{
		ExternalTypeOverrides: overrides,
	}
}

// ScanPackage parses all .go files in a given directory and returns PackageInfo.
func (s *Scanner) ScanPackage(dirPath string, resolver PackageResolver) (*PackageInfo, error) {
	s.resolver = resolver
	fset := token.NewFileSet()                                              // Initialized fset
	pkgs, err := parser.ParseDir(fset, dirPath, func(fi os.FileInfo) bool { // Used fset
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse directory %s: %w", dirPath, err)
	}

	if len(pkgs) > 1 {
		// Note: ParseDir can return multiple packages if files in a directory
		// have different package declarations. This typically indicates an error
		// or a special setup. For simplicity, we handle only one package per dir.
		// GOROOT/src often has this structure (e.g. multiple packages in net/http for tests or examples)
		// but for typical user projects, it's one package per directory.
		pkgNames := []string{}
		for name := range pkgs {
			pkgNames = append(pkgNames, name)
		}
		return nil, fmt.Errorf("multiple packages (%s) found in directory %s", strings.Join(pkgNames, ", "), dirPath)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no buildable Go source files in %s", dirPath)
	}

	var pkg *ast.Package
	var pkgNameFromAst string
	for name, p := range pkgs {
		pkg = p
		pkgNameFromAst = name // Actual package name from AST
		break
	}

	info := &PackageInfo{
		Name: pkgNameFromAst, // Use package name from AST
		Path: dirPath,
		Fset: fset, // Stored fset
	}

	// file.Name in pkg.Files is the absolute path to the file.
	for absFilePath, fileAst := range pkg.Files {
		info.Files = append(info.Files, absFilePath)
		s.buildImportLookup(fileAst) // fileAst is *ast.File
		for _, decl := range fileAst.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				s.parseGenDecl(d, info, absFilePath) // Pass absFilePath
			case *ast.FuncDecl:
				info.Functions = append(info.Functions, s.parseFuncDecl(d, absFilePath)) // Pass absFilePath
			}
		}
	}
	return info, nil
}

func (s *Scanner) buildImportLookup(file *ast.File) {
	s.importLookup = make(map[string]string)
	for _, i := range file.Imports {
		path := strings.Trim(i.Path.Value, `"`)
		if i.Name != nil {
			// Explicit alias, e.g., `m "example.com/models"`
			s.importLookup[i.Name.Name] = path
		} else {
			// Default name, e.g., `import "example.com/models"` -> name is "models"
			parts := strings.Split(path, "/")
			s.importLookup[parts[len(parts)-1]] = path
		}
	}
}

func (s *Scanner) parseGenDecl(decl *ast.GenDecl, info *PackageInfo, absFilePath string) { // Added absFilePath
	for _, spec := range decl.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			typeInfo := s.parseTypeSpec(sp, absFilePath) // Pass absFilePath
			if typeInfo.Doc == "" && decl.Doc != nil {   // Check decl.Doc != nil
				typeInfo.Doc = commentText(decl.Doc)
			}
			info.Types = append(info.Types, typeInfo)

		case *ast.ValueSpec:
			if decl.Tok == token.CONST {
				doc := commentText(sp.Doc)
				if doc == "" && sp.Comment != nil { // Check sp.Comment != nil
					doc = commentText(sp.Comment)
				}
				// If still no doc, and the GenDecl has a doc, it might apply to all consts in the block.
				if doc == "" && decl.Doc != nil {
					// This is less common for individual consts if they are grouped, but possible.
					// Usually, a doc on GenDecl for consts applies if it's a single const.
					// For a group, each ValueSpec or its names often get specific docs.
					// Let's be conservative: only use decl.Doc if sp.Doc and sp.Comment are empty.
					doc = commentText(decl.Doc)
				}

				for i, name := range sp.Names {
					var val string
					if i < len(sp.Values) {
						if lit, ok := sp.Values[i].(*ast.BasicLit); ok {
							val = lit.Value
						}
					}
					var typeName string
					if sp.Type != nil {
						typeName = s.parseTypeExpr(sp.Type).Name
					}

					info.Constants = append(info.Constants, &ConstantInfo{
						Name:     name.Name,
						FilePath: absFilePath, // Set FilePath
						Doc:      doc,
						Value:    val,
						Type:     typeName,
						Node:     name, // Use the ast.Ident node for the constant name
					})
				}
			}
		}
	}
}

func (s *Scanner) parseTypeSpec(sp *ast.TypeSpec, absFilePath string) *TypeInfo { // Added absFilePath
	typeInfo := &TypeInfo{
		Name:     sp.Name.Name,
		FilePath: absFilePath, // Set FilePath
		Doc:      commentText(sp.Doc),
		Node:     sp,
	}

	switch t := sp.Type.(type) {
	case *ast.StructType:
		typeInfo.Kind = StructKind
		typeInfo.Struct = s.parseStructType(t)
	case *ast.FuncType:
		typeInfo.Kind = FuncKind
		typeInfo.Func = s.parseFuncType(t)
	default:
		typeInfo.Kind = AliasKind
		typeInfo.Underlying = s.parseTypeExpr(sp.Type)
	}

	return typeInfo
}

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

func (s *Scanner) parseFuncDecl(f *ast.FuncDecl, absFilePath string) *FunctionInfo { // Added absFilePath
	funcInfo := s.parseFuncType(f.Type) // This populates Parameters and Results
	funcInfo.Name = f.Name.Name
	funcInfo.FilePath = absFilePath // Set FilePath
	funcInfo.Doc = commentText(f.Doc)
	// Note: FunctionInfo in models.go doesn't have a generic Node field yet for the *ast.FuncDecl itself.
	// If needed for other purposes, it could be added. For FilePath, this is sufficient.

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

func (s *Scanner) parseTypeExpr(expr ast.Expr) *FieldType {
	ft := &FieldType{resolver: s.resolver}
	switch t := expr.(type) {
	case *ast.Ident:
		ft.Name = t.Name
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
			// PkgName, typeName, fullImportPath might not be relevant if overridden to a primitive.
			// However, if it's overridden to another known complex type, they might be.
			// For now, we assume override means it's treated as a simple type.
			return ft
		}

		ft.Name = fmt.Sprintf("%s.%s", pkgIdent.Name, t.Sel.Name)
		ft.PkgName = pkgIdent.Name
		ft.typeName = t.Sel.Name
		ft.fullImportPath = pkgImportPath
	case *ast.ArrayType:
		ft.IsSlice = true
		ft.Name = "slice"
		ft.Elem = s.parseTypeExpr(t.Elt)
	case *ast.MapType:
		ft.IsMap = true
		ft.Name = "map"
		ft.MapKey = s.parseTypeExpr(t.Key)
		ft.Elem = s.parseTypeExpr(t.Value)
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
