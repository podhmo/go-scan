package symgo_test

import (
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

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
