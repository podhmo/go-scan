package symgo_test

import (
	"go/ast"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
)

// FindFile is a test helper to find a specific AST file in a scanned package.
func FindFile(t *testing.T, pkg *goscan.Package, filename string) *ast.File {
	t.Helper()
	for path, f := range pkg.AstFiles {
		if strings.HasSuffix(path, filename) {
			return f
		}
	}
	t.Fatalf("file %q not found in package %q", filename, pkg.Name)
	return nil
}
