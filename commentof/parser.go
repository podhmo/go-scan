// Package commentof provides functions to parse Go source files and extract
// comments for various declarations like functions, types, and constants.
package commentof

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"strings"
)

// FromFile parses a Go source file from the given path and extracts documentation.
func FromFile(filepath string) ([]interface{}, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	return FromReader(f, filepath)
}

// FromReader parses Go source from an io.Reader and extracts documentation.
func FromReader(src io.Reader, filename string) ([]interface{}, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}
	return fromDecls(file.Decls, fset, file)
}

// FromDecls processes a slice of AST declarations. Note: For best results with
// comment association, use FromFile or FromReader which provide full file context.
func FromDecls(decls []ast.Decl) ([]interface{}, error) {
	return fromDecls(decls, token.NewFileSet(), nil)
}

// fromDecls is the internal engine that requires full file context to work correctly.
func fromDecls(decls []ast.Decl, fset *token.FileSet, file *ast.File) ([]interface{}, error) {
	var results []interface{}
	for _, decl := range decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if doc := fromFuncDecl(d, fset, file); doc != nil {
				results = append(results, doc)
			}
		case *ast.GenDecl:
			docs, err := fromGenDecl(d, fset, file)
			if err != nil {
				return nil, fmt.Errorf("failed to process generic declaration: %w", err)
			}
			results = append(results, docs...)
		}
	}
	return results, nil
}

// FromFuncDecl extracts documentation from a function declaration.
func FromFuncDecl(d *ast.FuncDecl) *Function {
	return fromFuncDecl(d, token.NewFileSet(), nil)
}

func fromFuncDecl(d *ast.FuncDecl, fset *token.FileSet, file *ast.File) *Function {
	if d == nil {
		return nil
	}
	return &Function{
		Name:    d.Name.Name,
		Doc:     cleanComment(d.Doc),
		Params:  extractFields(d.Type.Params, fset, file),
		Results: extractFields(d.Type.Results, fset, file),
	}
}

// FromGenDecl extracts documentation from a generic declaration.
func FromGenDecl(d *ast.GenDecl) ([]interface{}, error) {
	return fromGenDecl(d, token.NewFileSet(), nil)
}

func fromGenDecl(d *ast.GenDecl, fset *token.FileSet, file *ast.File) ([]interface{}, error) {
	if d == nil {
		return nil, nil
	}
	var results []interface{}
	genDoc := cleanComment(d.Doc)

	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			ts, err := fromTypeSpec(s, fset, file)
			if err != nil {
				return nil, err
			}
			// The GenDecl doc applies to the TypeSpec.
			if genDoc != "" {
				if ts.Doc != "" {
					ts.Doc = genDoc + "\n" + ts.Doc
				} else {
					ts.Doc = genDoc
				}
			}
			results = append(results, ts)

		case *ast.ValueSpec:
			vs := fromValueSpec(s)
			// The GenDecl doc applies to each ValueSpec.
			if genDoc != "" {
				if vs.Doc != "" {
					vs.Doc = genDoc + "\n" + vs.Doc
				} else {
					vs.Doc = genDoc
				}
			}
			vs.Kind = d.Tok
			results = append(results, vs)
		}
	}
	return results, nil
}

func fromTypeSpec(s *ast.TypeSpec, fset *token.FileSet, file *ast.File) (*TypeSpec, error) {
	ts := &TypeSpec{
		Name: s.Name.Name,
		Doc:  cleanComment(s.Doc, s.Comment), // Doc for the spec itself.
	}

	if st, ok := s.Type.(*ast.StructType); ok {
		ts.Definition = &Struct{
			Fields: extractFields(st.Fields, fset, file),
		}
	}
	return ts, nil
}

func fromValueSpec(s *ast.ValueSpec) *ValueSpec {
	names := make([]string, len(s.Names))
	for i, name := range s.Names {
		names[i] = name.Name
	}
	return &ValueSpec{
		Names: names,
		Doc:   cleanComment(s.Doc, s.Comment),
	}
}

func extractFields(fieldList *ast.FieldList, fset *token.FileSet, file *ast.File) []*Field {
	if fieldList == nil || len(fieldList.List) == 0 {
		return nil
	}
	var fields []*Field
	for i, f := range fieldList.List {
		// Get docs that are automatically associated by the parser.
		doc := cleanComment(f.Doc, f.Comment)

		// Manually associate comments if context is available.
		if file != nil && fset != nil {
			var associatedComments []*ast.CommentGroup

			// Define the search boundary. It ends where the next field begins,
			// or at the closing parenthesis of the list.
			endPos := fieldList.Closing
			if i+1 < len(fieldList.List) {
				endPos = fieldList.List[i+1].Pos()
			}

			for _, cgroup := range file.Comments {
				// The comment must start after the field's own declaration starts,
				// and before the search boundary.
				if cgroup.Pos() > f.Pos() && cgroup.End() < endPos {
					// For multi-line parameter lists, ensure comment is on the same line.
					// This prevents grabbing comments from subsequent lines.
					if fset.Position(f.Pos()).Line == fset.Position(cgroup.Pos()).Line {
						associatedComments = append(associatedComments, cgroup)
					} else if fset.Position(f.End()).Line == fset.Position(cgroup.Pos()).Line {
						// Also handle comments on the same line as the end of a multi-line field type.
						associatedComments = append(associatedComments, cgroup)
					}
				}
			}

			// Add manually found comments to any that were already there.
			if len(associatedComments) > 0 {
				manualDoc := cleanComment(associatedComments...)
				if doc != "" {
					doc += "\n" + manualDoc
				} else {
					doc = manualDoc
				}
			}
		}

		var names []string
		for _, name := range f.Names {
			names = append(names, name.Name)
		}
		if len(names) == 0 && f.Type != nil {
			names = append(names, typeToString(f.Type))
		}

		field := &Field{
			Names: names,
			Type:  typeToString(f.Type),
			Doc:   doc,
		}
		fields = append(fields, field)
	}
	return fields
}

func typeToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.SelectorExpr:
		return typeToString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeToString(t.Elt)
	case *ast.Ellipsis:
		return "..." + typeToString(t.Elt)
	case *ast.InterfaceType:
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	case *ast.StructType:
		return "struct{...}"
	default:
		return "unknown"
	}
}

func cleanComment(groups ...*ast.CommentGroup) string {
	var parts []string
	for _, group := range groups {
		if group != nil {
			text := strings.TrimSpace(group.Text())
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}