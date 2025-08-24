package goscan_test

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestScan_Wildcard_FileSystemPath(t *testing.T) {
	files := map[string]string{
		"go.mod":           `module example.com/wildcard`,
		"main.go":          `package main`,
		"pkg/a/a.go":       `package a`,
		"pkg/a/sub/sub.go": `package sub`,
		"vendor/v/v.go":    `package v`, // Should be ignored
		"empty/README.md":  `empty dir`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	pkgs, err := s.Scan("./...")
	if err != nil {
		t.Fatalf("Scan(\"./...\") failed: %v", err)
	}

	gotPkgs := make([]string, len(pkgs))
	for i, p := range pkgs {
		gotPkgs[i] = p.ImportPath
	}
	sort.Strings(gotPkgs)

	wantPkgs := []string{
		"example.com/wildcard",
		"example.com/wildcard/pkg/a",
		"example.com/wildcard/pkg/a/sub",
	}
	if diff := cmp.Diff(wantPkgs, gotPkgs); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestScan_Wildcard_ImportPath(t *testing.T) {
	files := map[string]string{
		"go.mod":           `module example.com/wildcard`,
		"main.go":          `package main`,
		"pkg/a/a.go":       `package a`,
		"pkg/a/sub/sub.go": `package sub`,
		"pkg/b/b.go":       `package b`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Use an import path pattern
	pkgs, err := s.Scan("example.com/wildcard/pkg/a/...")
	if err != nil {
		t.Fatalf("Scan() with import path failed: %v", err)
	}

	gotPkgs := make([]string, len(pkgs))
	for i, p := range pkgs {
		gotPkgs[i] = p.ImportPath
	}
	sort.Strings(gotPkgs)

	wantPkgs := []string{
		"example.com/wildcard/pkg/a",
		"example.com/wildcard/pkg/a/sub",
	}
	if diff := cmp.Diff(wantPkgs, gotPkgs); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestScan_Wildcard_NoRootPackage(t *testing.T) {
	files := map[string]string{
		"go.mod":     `module example.com/wildcard`,
		"pkg/a/a.go": `package a`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	pkgs, err := s.Scan("./...")
	if err != nil {
		t.Fatalf("Scan(\"./...\") failed: %v", err)
	}

	gotPkgs := make([]string, len(pkgs))
	for i, p := range pkgs {
		gotPkgs[i] = p.ImportPath
	}
	sort.Strings(gotPkgs)

	// Should not include the root "example.com/wildcard" because it has no .go files
	wantPkgs := []string{
		"example.com/wildcard/pkg/a",
	}
	if diff := cmp.Diff(wantPkgs, gotPkgs); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
