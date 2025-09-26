package goscan

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestObjectIdentity(t *testing.T) {
	ctx := context.Background()
	testdataDir := "testdata/identity"

	// Setup scanner
	s, err := New(WithWorkDir(testdataDir))
	if err != nil {
		t.Fatalf("failed to create scanner: %+v", err)
	}

	// --- First Scan ---
	pkg1, err := s.ScanPackageFromFilePath(ctx, filepath.Join(testdataDir, "foo"))
	if err != nil {
		t.Fatalf("first scan failed: %+v", err)
	}

	var func1 *FunctionInfo
	for _, f := range pkg1.Functions {
		if f.Name == "Bar" {
			func1 = f
			break
		}
	}
	if func1 == nil {
		t.Fatalf("function Bar not found in first scan")
	}
	if func1.AstDecl == nil {
		t.Fatalf("AstDecl for function Bar is nil in first scan")
	}

	// --- Second Scan of the same package ---
	// This should hit the package cache, but the test is about the underlying object identity
	// which is now managed by the identityCache.
	pkg2, err := s.ScanPackageFromFilePath(ctx, filepath.Join(testdataDir, "foo"))
	if err != nil {
		t.Fatalf("second scan failed: %+v", err)
	}

	var func2 *FunctionInfo
	for _, f := range pkg2.Functions {
		if f.Name == "Bar" {
			func2 = f
			break
		}
	}
	if func2 == nil {
		t.Fatalf("function Bar not found in second scan")
	}

	// --- Verification ---
	// 1. The main package objects should be the same instance because of the package cache.
	if pkg1 != pkg2 {
		t.Errorf("package objects are not the same instance, pkg1_ptr=%p, pkg2_ptr=%p", pkg1, pkg2)
	}

	// 2. The core test: The FunctionInfo objects must be the same instance due to the identity cache.
	if func1 != func2 {
		t.Errorf("FunctionInfo objects for 'Bar' are not the same instance, func1_ptr=%p, func2_ptr=%p", func1, func2)
	}

	// 3. Just to be certain, check the underlying AST declaration pointers.
	if diff := cmp.Diff(func1.AstDecl, func2.AstDecl); diff != "" {
		t.Errorf("AstDecl pointers mismatch (-want +got):\n%s", diff)
	}
}
