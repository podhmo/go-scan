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

// FromFile parses a Go source file from the given path and extracts documentation
// for all top-level declarations.
func FromFile(filepath string) ([]interface{}, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	return FromReader(f, filepath)
}

// FromReader parses Go source from an io.Reader and extracts documentation
// for all top-level declarations.
func FromReader(src io.Reader, filename string) ([]interface{}, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	return FromDecls(file.Decls)
}

// FromDecls processes a slice of AST declarations and extracts documentation
// from each supported declaration type.
func FromDecls(decls []ast.Decl) ([]interface{}, error) {
	var results []interface{}
	for _, decl := range decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if doc := FromFuncDecl(d); doc != nil {
				results = append(results, doc)
			}
		case *ast.GenDecl:
			docs, err := FromGenDecl(d)
			if err != nil {
				return nil, fmt.Errorf("failed to process generic declaration: %w", err)
			}
			results = append(results, docs...)
		}
	}
	return results, nil
}

// FromFuncDecl extracts documentation from a function declaration node.
func FromFuncDecl(d *ast.FuncDecl) *Function {
	if d == nil {
		return nil
	}
	fn := &Function{
		Name:    d.Name.Name,
		Doc:     cleanComment(d.Doc),
		Params:  extractFields(d.Type.Params),
		Results: extractFields(d.Type.Results),
	}
	return fn
}

// FromGenDecl extracts documentation from a generic declaration node (const, type, var).
func FromGenDecl(d *ast.GenDecl) ([]interface{}, error) {
	if d == nil {
		return nil, nil
	}
	var results []interface{}
	doc := cleanComment(d.Doc)

	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			ts, err := fromTypeSpec(s)
			if err != nil {
				return nil, err
			}
			// Prepend the GenDecl's doc if the TypeSpec has no doc of its own.
			if ts.Doc == "" {
				ts.Doc = doc
			}
			results = append(results, ts)

		case *ast.ValueSpec:
			vs := fromValueSpec(s)
			// Prepend the GenDecl's doc if the ValueSpec has no doc of its own.
			if vs.Doc == "" {
				vs.Doc = doc
			}
			vs.Kind = d.Tok
			results = append(results, vs)
		}
	}
	return results, nil
}

// fromTypeSpec extracts documentation from a type specification.
func fromTypeSpec(s *ast.TypeSpec) (*TypeSpec, error) {
	ts := &TypeSpec{
		Name: s.Name.Name,
		Doc:  cleanComment(s.Doc),
	}

	switch t := s.Type.(type) {
	case *ast.StructType:
		ts.Definition = &Struct{
			Fields: extractFields(t.Fields),
		}
	case *ast.Ident, *ast.SelectorExpr:
		// This handles type definitions and aliases, e.g., `type S2 S` or `type S3 = S`.
		// A more complex type resolver would be needed to get the fully qualified name.
		// For now, we just store it as a simple definition.
		ts.Definition = nil // Or a more descriptive structure for aliases.
	}
	return ts, nil
}

// fromValueSpec extracts documentation from a value specification (const or var).
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

// extractFields processes a field list (e.g., params, results, struct fields)
// and extracts the documentation for each field.
func extractFields(fieldList *ast.FieldList) []*Field {
	if fieldList == nil {
		return nil
	}
	var fields []*Field
	for _, f := range fieldList.List {
		var names []string
		for _, name := range f.Names {
			names = append(names, name.Name)
		}

		// If there are no explicit names (e.g., anonymous parameter),
		// we might leave the names slice empty or use the type as a placeholder.
		if len(names) == 0 && f.Type != nil {
			// Handle unnamed parameters like `context.Context` or `string`.
			names = append(names, typeToString(f.Type))
		}

		field := &Field{
			Names: names,
			Type:  typeToString(f.Type),
			Doc:   cleanComment(f.Doc, f.Comment),
		}
		fields = append(fields, field)
	}
	return fields
}

// typeToString converts an AST expression for a type into a string representation.
// This is a simplified version.
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
		// A simple representation for interface{}.
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	case *ast.StructType:
		return "struct{...}" // Simplified representation
	default:
		return "unknown"
	}
}

// cleanComment combines multiple comment groups into a single, clean string.
func cleanComment(groups ...*ast.CommentGroup) string {
	var parts []string
	for _, group := range groups {
		if group != nil {
			parts = append(parts, group.Text())
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}