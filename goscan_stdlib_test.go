package goscan

import (
	"context"
	"testing"
)

func TestScanStdlib_SucceedsWithoutOverride(t *testing.T) {
	// This test verifies that the scanner can successfully scan a stdlib package
	// like "time" from within a test binary, without needing an ExternalTypeOverride.
	// This is made possible by the two-pass scanning logic in `scanGoFiles` which
	// correctly identifies the dominant package name and ignores the "main" package
	// injected by the `go test` environment.
	ctx := context.Background()

	s, err := New(WithGoModuleResolver())
	if err != nil {
		t.Fatalf("New() failed: %+v", err)
	}

	pkg, err := s.ScanPackageByImport(ctx, "time")
	if err != nil {
		t.Fatalf("expected no error when scanning stdlib package 'time', but got: %v", err)
	}

	if pkg == nil {
		t.Fatal("scanner returned a nil package")
	}
	if pkg.Name != "time" {
		t.Errorf("expected package name to be 'time', but got %q", pkg.Name)
	}

	// With the new "block scan" fix, external packages (including stdlib) are
	// intentionally not scanned for their contents. We expect to get a minimal
	// PackageInfo object, but it should not contain any types.
	if len(pkg.Types) > 0 {
		t.Errorf("expected no types in minimally scanned 'time' package, but got %d", len(pkg.Types))
	}
}
