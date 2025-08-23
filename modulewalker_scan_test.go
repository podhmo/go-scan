package goscan_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
)

func TestModuleWalker_Scan(t *testing.T) {
	files := map[string]string{
		"go.mod":      "module example.com/mw-scan",
		"pkg/a/a.go":  `package a; import "example.com/mw-scan/pkg/b"`,
		"pkg/b/b.go":  `package b`,
		"pkg/c/c.txt": `not a go package`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	w := s.Walker

	pkgs, err := w.Scan("./...")
	if err != nil {
		t.Fatalf("Walker.Scan(\"./...\") failed: %v", err)
	}

	want := []*scanner.PackageImports{
		{
			ImportPath: "example.com/mw-scan/pkg/a",
			Name:       "a",
			Imports:    []string{"example.com/mw-scan/pkg/b"},
		},
		{
			ImportPath: "example.com/mw-scan/pkg/b",
			Name:       "b",
			Imports:    []string{},
		},
	}

	// We don't care about the order of packages, and Imports should be sorted by the scanner.
	sorter := cmpopts.SortSlices(func(a, b *scanner.PackageImports) bool { return a.ImportPath < b.ImportPath })
	ignore := cmpopts.IgnoreFields(scanner.PackageImports{}, "FileImports")
	if diff := cmp.Diff(want, pkgs, sorter, ignore); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
