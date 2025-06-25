package scanner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Scanner parses Go source files within a package.
type Scanner struct {
	resolver     PackageResolver
	importLookup map[string]string // Maps import alias/name to full import path for the current file.
}

// New creates a new Scanner.
func New() *Scanner {
	return &Scanner{}
}

// ScanPackage parses all .go files in a given directory and returns PackageInfo.
func (s *Scanner) ScanPackage(dirPath string, resolver PackageResolver) (*PackageInfo, error) {
	s.resolver = resolver
	pkgs, err := parser.ParseDir(token.NewFileSet(), dirPath, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse directory %s: %w", dirPath, err)
	}

	if len(pkgs) > 1 {
		return nil, fmt.Errorf("multiple packages found in directory %s", dirPath)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no buildable Go source files in %s", dirPath)
	}

	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
		break
	}

	info := &PackageInfo{
		Name: pkg.Name,
		Path: dirPath,
	}

	for fileName, file := range pkg.Files {
		info.Files = append(info.Files, fileName)
		s.buildImportLookup(file)
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				s.parseGenDecl(d, info)
			case *ast.FuncDecl:
				info.Functions = append(info.Functions, s.parseFuncDecl(d))
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

func (s *Scanner) parseGenDecl(decl *ast.GenDecl, info *PackageInfo) {
	for _, spec := range decl.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			info.Types = append(info.Types, s.parseTypeSpec(sp))

		case *ast.ValueSpec:
			if decl.Tok == token.CONST {
				doc := commentText(sp.Doc)
				if doc == "" {
					doc = commentText(sp.Comment)
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
						Name:  name.Name,
						Doc:   doc,
						Value: val,
						Type:  typeName,
					})
				}
			}
		}
	}
}

func (s *Scanner) parseTypeSpec(sp *ast.TypeSpec) *TypeInfo {
	typeInfo := &TypeInfo{
		Name: sp.Name.Name,
		Doc:  commentText(sp.Doc),
		Node: sp,
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

func (s *Scanner) parseFuncDecl(f *ast.FuncDecl) *FunctionInfo {
	funcInfo := s.parseFuncType(f.Type)
	funcInfo.Name = f.Name.Name
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
		ft.Name = fmt.Sprintf("%s.%s", pkgIdent.Name, t.Sel.Name)
		ft.PkgName = pkgIdent.Name
		ft.typeName = t.Sel.Name
		ft.fullImportPath = s.importLookup[pkgIdent.Name]
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