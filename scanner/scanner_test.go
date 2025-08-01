package scanner

import (
	"context"
	"fmt"
	"go/token"
	"path/filepath"
	"testing"
)

// MockResolver is a mock implementation of PackageResolver for tests.
type MockResolver struct {
	ScanPackageByImportFunc func(ctx context.Context, importPath string) (*PackageInfo, error)
}

func (m *MockResolver) ScanPackageByImport(ctx context.Context, importPath string) (*PackageInfo, error) {
	if m.ScanPackageByImportFunc != nil {
		return m.ScanPackageByImportFunc(ctx, importPath)
	}
	return nil, nil // Default mock behavior
}

func newTestScanner(t *testing.T, modulePath, rootDir string) *Scanner {
	t.Helper()
	fset := token.NewFileSet()
	s, err := New(fset, nil, nil, modulePath, rootDir, &MockResolver{})
	if err != nil {
		t.Fatalf("scanner.New failed: %v", err)
	}
	return s
}

func TestNewScanner(t *testing.T) {
	modulePath := "example.com/test"
	rootDir, _ := filepath.Abs("../") // A plausible root for test purposes
	resolver := &MockResolver{}

	t.Run("nil_fset", func(t *testing.T) {
		_, err := New(nil, nil, nil, modulePath, rootDir, resolver)
		if err == nil {
			t.Error("Expected error when creating scanner with nil fset, got nil")
		}
	})

	t.Run("valid_fset", func(t *testing.T) {
		fset := token.NewFileSet()
		s, err := New(fset, nil, nil, modulePath, rootDir, resolver)
		if err != nil {
			t.Errorf("Expected no error when creating scanner with valid fset, got %v", err)
		}
		if s == nil {
			t.Error("Scanner should not be nil with valid fset")
		} else if s.fset != fset {
			t.Error("Scanner fset not set correctly")
		}
	})

	t.Run("nil_resolver", func(t *testing.T) {
		fset := token.NewFileSet()
		_, err := New(fset, nil, nil, modulePath, rootDir, nil)
		if err == nil {
			t.Error("Expected error when creating scanner with nil resolver, got nil")
		}
	})
}

func TestScanPackageFeatures(t *testing.T) {
	testDir, _ := filepath.Abs("../testdata/features")
	s := newTestScanner(t, "example.com/test", filepath.Dir(testDir))

	filesToScan := []string{
		filepath.Join(testDir, "features.go"),
		filepath.Join(testDir, "another.go"),
		filepath.Join(testDir, "variadic.go"),
	}

	pkgInfo, err := s.ScanFiles(context.Background(), filesToScan, testDir)
	if err != nil {
		t.Fatalf("ScanFiles failed for %v: %v", filesToScan, err)
	}

	types := make(map[string]*TypeInfo)
	for _, ti := range pkgInfo.Types {
		types[ti.Name] = ti
	}

	if _, ok := types["Item"]; !ok {
		t.Fatal("Type 'Item' not found")
	}
}

func TestScanFiles(t *testing.T) {
	testdataDir, _ := filepath.Abs("../testdata/features")
	s := newTestScanner(t, "example.com/test", filepath.Dir(testdataDir))

	t.Run("scan_single_file", func(t *testing.T) {
		filePath := filepath.Join(testdataDir, "features.go")
		pkgInfo, err := s.ScanFiles(context.Background(), []string{filePath}, testdataDir)
		if err != nil {
			t.Fatalf("ScanFiles single file failed: %v", err)
		}
		if pkgInfo.Name != "features" {
			t.Errorf("Expected package name 'features', got '%s'", pkgInfo.Name)
		}
	})

	t.Run("scan_files_different_packages", func(t *testing.T) {
		filePaths := []string{
			filepath.Join(testdataDir, "features.go"),
			filepath.Join(testdataDir, "differentpkg.go"),
		}
		pkgInfo, err := s.ScanFiles(context.Background(), filePaths, testdataDir)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(pkgInfo.Files) != 1 {
			t.Errorf("Expected 1 file, got %d", len(pkgInfo.Files))
		}
	})
}

func TestResolve_MutualRecursion(t *testing.T) {
	fset := token.NewFileSet()
	rootDir, _ := filepath.Abs("../testdata/recursion/mutual")

	s, err := New(fset, nil, nil, "example.com/recursion/mutual", rootDir, &MockResolver{})
	if err != nil {
		t.Fatalf("scanner.New failed: %v", err)
	}

	pkgCache := make(map[string]*PackageInfo)
	mockResolver := &MockResolver{
		ScanPackageByImportFunc: func(ctx context.Context, importPath string) (*PackageInfo, error) {
			if pkg, found := pkgCache[importPath]; found {
				return pkg, nil
			}
			var pkgDir string
			switch importPath {
			case "example.com/recursion/mutual/pkg_a":
				pkgDir = filepath.Join(rootDir, "pkg_a")
			case "example.com/recursion/mutual/pkg_b":
				pkgDir = filepath.Join(rootDir, "pkg_b")
			default:
				return nil, fmt.Errorf("unexpected import path: %s", importPath)
			}
			pkg, err := s.ScanFiles(ctx, []string{filepath.Join(pkgDir, filepath.Base(pkgDir)+".go")}, pkgDir)
			if err == nil && pkg != nil {
				pkgCache[importPath] = pkg
			}
			return pkg, err
		},
	}
	s.resolver = mockResolver

	pkgAInfo, err := s.ScanPackageByImport(context.Background(), "example.com/recursion/mutual/pkg_a")
	if err != nil {
		t.Fatalf("ScanPackageByImport for pkg_a failed: %v", err)
	}

	typeA := pkgAInfo.Lookup("A")
	if typeA == nil {
		t.Fatal("Type 'A' not found in pkg_a")
	}
}
