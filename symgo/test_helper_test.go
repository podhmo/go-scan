package symgo_test

import (
	"go/ast"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
)

// Helper to find a specific file from a scanned package, failing the test if not found.
func FindFile(t *testing.T, pkg *goscan.Package, filename string) *ast.File {
	t.Helper()
	for path, astFile := range pkg.AstFiles {
		if filepath.Base(path) == filename {
			return astFile
		}
	}
	t.Fatalf("file %q not found in package %q", filename, pkg.ImportPath)
	return nil
}
