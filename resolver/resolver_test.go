package resolver

import (
	"context"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

// spyScanner is a wrapper around goscan.Scanner to track method calls.
// It implements the scannerService interface.
type spyScanner struct {
	scanner                  *goscan.Scanner
	ScanPackageByImportCount int
}

func (s *spyScanner) ScanPackageByImport(ctx context.Context, path string) (*goscan.Package, error) {
	s.ScanPackageByImportCount++
	return s.scanner.ScanPackageByImport(ctx, path)
}

func TestResolver_Resolve_Caching(t *testing.T) {
	ctx := context.Background()

	// 1. Set up the test files in a temporary directory.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module my/app",
		"foo/foo.go": `
package foo

import "fmt"

func Greet() {
	fmt.Println("Hello")
}
`,
	})
	defer cleanup()

	// 2. Create the real scanner and wrap it in our spy.
	// We need a real scanner so that the first call can actually succeed.
	realScanner, err := goscan.New(goscan.WithWorkDir(dir))
	if err != nil {
		t.Fatalf("failed to create real scanner: %v", err)
	}
	spy := &spyScanner{
		scanner: realScanner,
	}

	// 3. Create the resolver with the spy scanner.
	resolver := New(spy)

	// --- First call: should trigger a scan ---
	pkgInfo1, err := resolver.Resolve(ctx, "my/app/foo")
	if err != nil {
		t.Fatalf("first Resolve() failed: %v", err)
	}
	if pkgInfo1 == nil {
		t.Fatal("first Resolve() returned nil package info")
	}
	if !strings.HasSuffix(pkgInfo1.ImportPath, "my/app/foo") {
		t.Errorf("expected import path to be 'my/app/foo', got %q", pkgInfo1.ImportPath)
	}

	// Check if the scan was called.
	if spy.ScanPackageByImportCount != 1 {
		t.Errorf("expected ScanPackageByImport to be called 1 time, but was called %d times", spy.ScanPackageByImportCount)
	}

	// --- Second call: should use the cache ---
	pkgInfo2, err := resolver.Resolve(ctx, "my/app/foo")
	if err != nil {
		t.Fatalf("second Resolve() failed: %v", err)
	}
	if pkgInfo2 == nil {
		t.Fatal("second Resolve() returned nil package info")
	}

	// Check that the scan was NOT called again.
	if spy.ScanPackageByImportCount != 1 {
		t.Errorf("expected ScanPackageByImport to be called 1 time after second resolve, but was called %d times", spy.ScanPackageByImportCount)
	}

	// Check if the same instance is returned.
	if pkgInfo1 != pkgInfo2 {
		t.Error("expected the same Package instance to be returned from cache")
	}
}
