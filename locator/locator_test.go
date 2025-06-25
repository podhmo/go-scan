package locator

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestModule(t *testing.T, modulePath string) (string, func()) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "locator-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	goModContent := "module " + modulePath
	goModPath := filepath.Join(rootDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	// Create a subdirectory to test lookup from a nested path
	subDir := filepath.Join(rootDir, "internal", "api")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create sub dir: %v", err)
	}

	return subDir, func() {
		os.RemoveAll(rootDir)
	}
}

func TestNew(t *testing.T) {
	modulePath := "example.com/myproject"
	startPath, cleanup := setupTestModule(t, modulePath)
	defer cleanup()

	l, err := New(startPath)
	if err != nil {
		t.Fatalf("New() returned an error: %v", err)
	}

	expectedRootDir := filepath.Dir(filepath.Dir(startPath))
	if l.RootDir() != expectedRootDir {
		t.Errorf("Expected root dir %q, got %q", expectedRootDir, l.RootDir())
	}

	if l.ModulePath() != modulePath {
		t.Errorf("Expected module path %q, got %q", modulePath, l.ModulePath())
	}
}

func TestFindPackageDir(t *testing.T) {
	modulePath := "example.com/myproject"
	startPath, cleanup := setupTestModule(t, modulePath)
	defer cleanup()

	l, err := New(startPath)
	if err != nil {
		t.Fatalf("New() returned an error: %v", err)
	}

	tests := []struct {
		importPath    string
		expectedRel   string
		expectErr     bool
	}{
		{"example.com/myproject/internal/api", "internal/api", false},
		{"example.com/myproject", "", false},
		{"example.com/otherproject/api", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.importPath, func(t *testing.T) {
			dir, err := l.FindPackageDir(tt.importPath)

			if (err != nil) != tt.expectErr {
				t.Fatalf("FindPackageDir() error = %v, expectErr %v", err, tt.expectErr)
			}

			if !tt.expectErr {
				expectedPath := filepath.Join(l.RootDir(), tt.expectedRel)
				if dir != expectedPath {
					t.Errorf("Expected path %q, got %q", expectedPath, dir)
				}
			}
		})
	}
}