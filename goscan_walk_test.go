package goscan

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestScanPackageImports(t *testing.T) {
	s, err := New(WithWorkDir("./testdata/walk"))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	pkg, err := s.Walker.ScanPackageImports(context.Background(), "github.com/podhmo/go-scan/testdata/walk/a")
	if err != nil {
		t.Fatalf("ScanPackageImports failed: %v", err)
	}

	if pkg.Name != "a" {
		t.Errorf("expected package name 'a', got %q", pkg.Name)
	}

	expectedImports := []string{
		"github.com/podhmo/go-scan/testdata/walk/b",
		"github.com/podhmo/go-scan/testdata/walk/c",
		"github.com/podhmo/go-scan/testdata/walk/d",
	}
	sort.Strings(pkg.Imports) // sort for stable comparison
	if diff := cmp.Diff(expectedImports, pkg.Imports); diff != "" {
		t.Errorf("mismatch imports (-want +got):\n%s", diff)
	}
}

type collectingVisitor struct {
	visited []string
}

func (v *collectingVisitor) Visit(pkg *PackageImports) (importsToFollow []string, err error) {
	v.visited = append(v.visited, pkg.ImportPath)
	return pkg.Imports, nil // follow all imports
}

func TestWalk(t *testing.T) {
	s, err := New(WithWorkDir("./testdata/walk"))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	visitor := &collectingVisitor{}
	err = s.Walker.Walk(context.Background(), "github.com/podhmo/go-scan/testdata/walk/a", visitor)
	if err != nil {
		t.Fatalf("Walk() failed: %v", err)
	}

	expectedVisited := []string{
		"github.com/podhmo/go-scan/testdata/walk/a",
		"github.com/podhmo/go-scan/testdata/walk/b",
		"github.com/podhmo/go-scan/testdata/walk/c",
		"github.com/podhmo/go-scan/testdata/walk/d",
	}

	sort.Strings(visitor.visited)
	sort.Strings(expectedVisited)

	if diff := cmp.Diff(expectedVisited, visitor.visited); diff != "" {
		t.Errorf("mismatch visited packages (-want +got):\n%s", diff)
	}
}
