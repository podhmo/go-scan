package symgo_test

import (
	"go/ast"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

// common test helper for this package
func findFile(t *testing.T, pkg *goscan.Package, filename string) *ast.File {
	t.Helper()
	for path, f := range pkg.AstFiles {
		if strings.HasSuffix(path, filename) {
			return f
		}
	}
	t.Fatalf("file %q not found in package %q", filename, pkg.Name)
	return nil
}

// findFunc is a test helper to find a function by its name in a package.
func findFunc(t *testing.T, pkg *goscan.Package, name string) *scanner.FunctionInfo {
	t.Helper()
	for _, f := range pkg.Functions {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("function %q not found in package %q", name, pkg.ImportPath)
	return nil
}
