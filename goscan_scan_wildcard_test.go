package goscan_test

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestScan_WildcardPatterns(t *testing.T) {
	files := map[string]string{
		"go.mod":              `module example.com/wildcard`,
		"main.go":             `package main`,
		"pkg/a/a.go":          `package a`,
		"pkg/a/sub/sub.go":    `package sub`,
		"pkg/b/b.go":          `package b`,
		"vendor/v/v.go":       `package v`,
		".hidden/h/h.go":      `package h`,
		"_ignore/i/i.go":      `package i`,
		"pkg/a/testdata/d.go": `package d`,
		"empty/README.md":     `empty dir`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	tests := []struct {
		name     string
		pattern  string
		wantPkgs []string
		wantErr  bool
	}{
		{
			name:    "relative path",
			pattern: "./...",
			wantPkgs: []string{
				"example.com/wildcard",
				"example.com/wildcard/pkg/a",
				"example.com/wildcard/pkg/a/sub",
				"example.com/wildcard/pkg/b",
				"example.com/wildcard/vendor/v",
			},
		},
		{
			name:    "import path",
			pattern: "example.com/wildcard/pkg/a/...",
			wantPkgs: []string{
				"example.com/wildcard/pkg/a",
				"example.com/wildcard/pkg/a/sub",
			},
		},
		{
			name:    "all packages in module",
			pattern: "...",
			wantPkgs: []string{
				"example.com/wildcard",
				"example.com/wildcard/pkg/a",
				"example.com/wildcard/pkg/a/sub",
				"example.com/wildcard/pkg/b",
				"example.com/wildcard/vendor/v",
			},
		},
		{
			name:    "import path partial",
			pattern: "example.com/wildcard/pkg/...",
			wantPkgs: []string{
				"example.com/wildcard/pkg/a",
				"example.com/wildcard/pkg/a/sub",
				"example.com/wildcard/pkg/b",
			},
		},
		{
			name:     "non-existent file path",
			pattern:  "./nonexistent/...",
			wantPkgs: []string{},
		},
		{
			name:    "non-existent import path",
			pattern: "example.com/wildcard/nonexistent/...",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				// The scanner 's' is now correctly configured by scantest.Run.
				// We can call s.Scan which uses the logic we want to test.
				foundPkgs, err := s.Scan(tt.pattern)
				if err != nil {
					if tt.wantErr {
						return nil // expected error, so test passes
					}
					return err
				}
				if tt.wantErr {
					t.Fatalf("Scan(%q) expected to fail, but got no error", tt.pattern)
				}

				gotPkgs := make([]string, len(foundPkgs))
				for i, p := range foundPkgs {
					gotPkgs[i] = p.ImportPath
				}
				sort.Strings(gotPkgs)
				sort.Strings(tt.wantPkgs)

				if diff := cmp.Diff(tt.wantPkgs, gotPkgs); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
				return nil
			}

			if _, err := scantest.Run(t, dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}
		})
	}
}
