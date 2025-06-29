package astwalk

import (
	"go/ast"
	"go/token"
)

// ToplevelStructs returns an iterator function for top-level struct type definitions
// in the given AST file.
// This function is designed to be used with Go 1.23's range-over-function feature.
// Example:
//
//	for typeSpec := range ToplevelStructs(fset, file) {
//		// use typeSpec
//	}
func ToplevelStructs(fset *token.FileSet, file *ast.File) func(yield func(*ast.TypeSpec) bool) {
	return func(yield func(*ast.TypeSpec) bool) {
		if file == nil {
			return
		}

		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			if genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
					if !yield(typeSpec) {
						return // Stop iteration if yield returns false
					}
				}
			}
		}
	}
}
