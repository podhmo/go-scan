package evaluator

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan"
)

func TestResolver_ResolvePackage(t *testing.T) {
	files := map[string]string{
		"go.mod":             "module example.com/me",
		"main.go":            "package main",
		"lib/lib.go":         "package lib",
		"foreign/foreign.go": "package foreign",
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Policy: allow scanning local packages, but not "foreign"
	policy := func(path string) bool {
		return !strings.Contains(path, "foreign")
	}

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		resolver := NewResolver(s, policy)

		// Case 1: Resolve an in-policy package
		libPkg, err := resolver.ResolvePackage(ctx, "example.com/me/lib")
		if err != nil {
			t.Errorf("expected no error for in-policy package, got %v", err)
		}
		if libPkg == nil {
			t.Fatal("in-policy package is nil")
		}
		if libPkg.ScannedInfo == nil {
			t.Error("in-policy package should have ScannedInfo, but it's nil")
		}
		if libPkg.Name != "lib" {
			t.Errorf("expected package name 'lib', got %q", libPkg.Name)
		}

		// Case 2: Resolve an out-of-policy package
		foreignPkg, err := resolver.ResolvePackage(ctx, "example.com/me/foreign")
		if err != nil {
			t.Errorf("expected no error for out-of-policy package, got %v", err)
		}
		if foreignPkg == nil {
			t.Fatal("out-of-policy package is nil")
		}
		if foreignPkg.ScannedInfo != nil {
			t.Error("out-of-policy package should NOT have ScannedInfo, but it's not nil")
		}

		// Case 3: Check caching - resolve the same packages again
		libPkg2, _ := resolver.ResolvePackage(ctx, "example.com/me/lib")
		if libPkg != libPkg2 {
			t.Error("expected cached package object for in-policy package, but got a different one")
		}

		foreignPkg2, _ := resolver.ResolvePackage(ctx, "example.com/me/foreign")
		if foreignPkg != foreignPkg2 {
			t.Error("expected cached package object for out-of-policy package, but got a different one")
		}

		// Case 4: Non-existent package
		_, err = resolver.ResolvePackage(ctx, "example.com/me/nonexistent")
		if err == nil {
			t.Error("expected an error for non-existent package, but got nil")
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
