package locator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestModuleWithContent creates a temporary module structure for testing.
// It writes the given goModContent to a go.mod file in the root of the temporary module.
// It also creates any specified subdirectories within the module.
// Returns the root directory of the module, a path to start lookup from (a sub-directory or root), and a cleanup function.
func setupTestModuleWithContent(t *testing.T, goModContent string, subDirsToCreate []string) (string, string, func()) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "locator-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	goModPath := filepath.Join(rootDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		os.RemoveAll(rootDir) // cleanup before fatal
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	for _, p := range subDirsToCreate {
		fullPath := filepath.Join(rootDir, p)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			os.RemoveAll(rootDir) // cleanup before fatal
			t.Fatalf("Failed to create sub dir %s: %v", fullPath, err)
		}
	}

	var startLookupPath string
	if len(subDirsToCreate) > 0 {
		// Use the first created subdirectory for lookup tests, good for testing upward search for go.mod
		startLookupPath = filepath.Join(rootDir, subDirsToCreate[0])
	} else {
		startLookupPath = rootDir
	}

	return rootDir, startLookupPath, func() {
		os.RemoveAll(rootDir)
	}
}

// setupTestModule is a simplified version for compatibility with old tests.
func setupTestModule(t *testing.T, modulePath string) (string, func()) {
	t.Helper()
	// This setup implies "internal/api" is created and used as the start path for New()
	_, startPath, cleanup := setupTestModuleWithContent(t, "module "+modulePath, []string{filepath.Join("internal", "api")})
	return startPath, cleanup // startPath is rootDir/internal/api
}

func TestNew(t *testing.T) {
	moduleName := "example.com/myproject"
	// startLookupPath will be .../locator-test-XYZ/internal/api
	startLookupPath, cleanup := setupTestModule(t, moduleName)
	defer cleanup()

	l, err := New(startLookupPath)
	if err != nil {
		t.Fatalf("New() returned an error: %v", err)
	}

	// Expected rootDir is where go.mod is, which is two levels above startLookupPath
	expectedRootDir, _ := filepath.Abs(filepath.Dir(filepath.Dir(startLookupPath)))
	actualRootDir, _ := filepath.Abs(l.RootDir())

	if actualRootDir != expectedRootDir {
		t.Errorf("Expected root dir %q, got %q", expectedRootDir, actualRootDir)
	}

	if l.ModulePath() != moduleName {
		t.Errorf("Expected module path %q, got %q", moduleName, l.ModulePath())
	}
}

func TestFindPackageDir(t *testing.T) {
	moduleName := "example.com/myproject"
	// startLookupPath will be .../locator-test-XYZ/internal/api
	startLookupPath, cleanup := setupTestModule(t, moduleName)
	defer cleanup()

	// moduleActualRootDir is where go.mod is.
	moduleActualRootDir := filepath.Dir(filepath.Dir(startLookupPath))

	l, err := New(startLookupPath)
	if err != nil {
		t.Fatalf("New() returned an error: %v", err)
	}

	tests := []struct {
		name        string
		importPath  string
		expectedRel string // Relative to moduleActualRootDir
		expectErr   bool
	}{
		{"standard_subpackage", "example.com/myproject/internal/api", filepath.Join("internal", "api"), false},
		{"standard_root", "example.com/myproject", "", false},
		{"other_project", "example.com/otherproject/api", "", true}, // Cannot find other project
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := l.FindPackageDir(tt.importPath)

			if (err != nil) != tt.expectErr {
				t.Fatalf("FindPackageDir() error = %v, expectErr %v. Import path: %s", err, tt.expectErr, tt.importPath)
			}

			if !tt.expectErr {
				expectedPath := filepath.Join(moduleActualRootDir, tt.expectedRel)
				absExpected, _ := filepath.Abs(expectedPath)
				absGot, _ := filepath.Abs(dir)
				if absGot != absExpected {
					t.Errorf("Expected path %q, got %q for import %q", absExpected, absGot, tt.importPath)
				}
			}
		})
	}
}

// TestFindPackageDirWithReplace tests the FindPackageDir method with various replace directives.
func TestFindPackageDirWithReplace(t *testing.T) {
	mainModulePath := "example.com/mainmodule"

	// Create a master temporary directory for tests that might use absolute paths for replacement
	masterTmpDir, err := os.MkdirTemp("", "locator-master-test-*")
	if err != nil {
		t.Fatalf("Failed to create master temp dir: %v", err)
	}
	defer os.RemoveAll(masterTmpDir)

	// Prepare an absolute path target for one of the tests
	absReplaceDir := filepath.Join(masterTmpDir, "absreplacement", "another")
	absReplaceSubPkgDir := filepath.Join(absReplaceDir, "pkg")
	if err := os.MkdirAll(absReplaceSubPkgDir, 0755); err != nil {
		t.Fatalf("Failed to create absolute replacement target dir %s: %v", absReplaceSubPkgDir, err)
	}

	testCases := []struct {
		name              string
		goModContent      string
		subDirsToCreate   []string // Relative to module root for the primary module being set up
		importPath        string
		expectedFoundPath string // Expected absolute path, or relative to the setup module's root
		expectErr         bool
	}{
		{
			name: "replace_with_local_relative_path",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/replacedmodule => ./local/replacedmodule
`,
			subDirsToCreate:   []string{filepath.Join("local", "replacedmodule", "pkg")},
			importPath:        "example.com/replacedmodule/pkg",
			expectedFoundPath: filepath.Join("local", "replacedmodule", "pkg"), // Relative to module root
			expectErr:         false,
		},
		{
			name: "replace_with_local_path_root",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/replacedmodule => ./local/replacedmodule
`,
			subDirsToCreate:   []string{filepath.Join("local", "replacedmodule")},
			importPath:        "example.com/replacedmodule",
			expectedFoundPath: filepath.Join("local", "replacedmodule"),
			expectErr:         false,
		},
		{
			name: "replace_with_local_path_version_on_old",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/replacedmodule v1.0.0 => ./local/versionedreplacedmodule
`,
			subDirsToCreate:   []string{filepath.Join("local", "versionedreplacedmodule", "subpkg")},
			importPath:        "example.com/replacedmodule/subpkg",
			expectedFoundPath: filepath.Join("local", "versionedreplacedmodule", "subpkg"),
			expectErr:         false,
		},
		{
			name: "replace_with_local_absolute_path",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/another => ` + absReplaceDir + `
`,
			subDirsToCreate:   []string{}, // No subdirs needed in the main module for this
			importPath:        "example.com/another/pkg",
			expectedFoundPath: absReplaceSubPkgDir, // This is an absolute path
			expectErr:         false,
		},
		{
			name: "replace_module_with_another_module_path_within_same_project",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/oldinternal => example.com/mainmodule/newinternal v1.0.0
`,
			subDirsToCreate:   []string{filepath.Join("newinternal", "api")},
			importPath:        "example.com/oldinternal/api",
			expectedFoundPath: filepath.Join("newinternal", "api"), // Relative to module root
			expectErr:         false,
		},
		{
			name: "replace_target_local_path_does_not_exist",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/nonexistent => ./does/not/exist
`,
			subDirsToCreate:   []string{},
			importPath:        "example.com/nonexistent/pkg",
			expectedFoundPath: "",
			// Current behavior: if a replace rule matches, but target is invalid, FindPackageDir continues
			// and then fails at the end if no other rule or default logic finds the path.
			// This might need to be stricter: if a replace rule applies, its target *must* be valid.
			// For now, this will result in an error because ./does/not/exist/pkg won't be found.
			expectErr: true,
		},
		{
			name: "no_matching_replace_directive_falls_back_to_module_path",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/foo => ./bar
`,
			subDirsToCreate:   []string{"actualpkg"}, // Subdir of mainmodule
			importPath:        "example.com/mainmodule/actualpkg",
			expectedFoundPath: "actualpkg", // Relative to module root
			expectErr:         false,
		},
		{
			name: "replace_in_block_form_finds_alpha",
			goModContent: `
module example.com/mainmodule
go 1.16
replace (
	example.com/alpha => ./local/alpha
	example.com/beta v1.0.0 => ./local/betaversioned
)
`,
			subDirsToCreate:   []string{filepath.Join("local", "alpha"), filepath.Join("local", "betaversioned", "sub")},
			importPath:        "example.com/alpha",
			expectedFoundPath: filepath.Join("local", "alpha"),
			expectErr:         false,
		},
		{
			name: "replace_in_block_form_finds_beta_subpackage",
			goModContent: `
module example.com/mainmodule
go 1.16
replace (
	example.com/alpha => ./local/alpha
	example.com/beta v1.0.0 => ./local/betaversioned
)
`,
			subDirsToCreate:   []string{filepath.Join("local", "alpha"), filepath.Join("local", "betaversioned", "sub")},
			importPath:        "example.com/beta/sub",
			expectedFoundPath: filepath.Join("local", "betaversioned", "sub"),
			expectErr:         false,
		},
		{
			name: "replace_to_external_module_not_in_current_locator_scope_fails_gracefully",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/currenttomodule => example.com/someothermodule v1.0.0
`,
			subDirsToCreate:   []string{},
			importPath:        "example.com/currenttomodule/pkg",
			expectedFoundPath: "",
			expectErr:         true, // Because someothermodule is not mainmodule and locator is scoped
		},
		{
			name: "replace_old_path_is_prefix_of_import_path",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/prefixmod => ./local/prefixmod
`,
			subDirsToCreate:   []string{filepath.Join("local", "prefixmod", "sub", "pkg")},
			importPath:        "example.com/prefixmod/sub/pkg",
			expectedFoundPath: filepath.Join("local", "prefixmod", "sub", "pkg"),
			expectErr:         false,
		},
		{
			name: "replace_old_path_is_prefix_of_import_path_targetting_root_of_replacement",
			goModContent: `
module example.com/mainmodule
go 1.16
replace example.com/prefixmod => ./local/prefixmod
`,
			subDirsToCreate:   []string{filepath.Join("local", "prefixmod")}, // only root of replacement exists
			importPath:        "example.com/prefixmod",
			expectedFoundPath: filepath.Join("local", "prefixmod"),
			expectErr:         false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure module path in goModContent is consistent for tests assuming mainModulePath
			var currentGoModContent string
			if strings.HasPrefix(strings.TrimSpace(tt.goModContent), "module ") {
				currentGoModContent = tt.goModContent
			} else {
				currentGoModContent = "module " + mainModulePath + "\n" + tt.goModContent
			}

			moduleRootDir, startLookupPath, cleanup := setupTestModuleWithContent(t, currentGoModContent, tt.subDirsToCreate)
			defer cleanup()

			l, err := New(startLookupPath)
			if err != nil {
				// This might indicate an issue with go.mod parsing in New() if the go.mod was intended to be valid.
				t.Fatalf("Test %q: New() returned an error: %v. go.mod content:\n%s", tt.name, err, currentGoModContent)
			}

			dir, err := l.FindPackageDir(tt.importPath)

			if (err != nil) != tt.expectErr {
				t.Errorf("Test %q: FindPackageDir() error = %v, expectErr %v. Import path: %s", tt.name, err, tt.expectErr, tt.importPath)
			}

			if !tt.expectErr && err == nil { // Only check path if no error was expected and no error occurred
				var expectedPathAbs string
				if filepath.IsAbs(tt.expectedFoundPath) {
					expectedPathAbs = tt.expectedFoundPath
				} else {
					expectedPathAbs = filepath.Join(moduleRootDir, tt.expectedFoundPath)
				}
				// Normalize paths for comparison
				absExpected, _ := filepath.Abs(expectedPathAbs)
				absGot, _ := filepath.Abs(dir)

				if absGot != absExpected {
					t.Errorf("Test %q: For import %q, expected path %q, got %q", tt.name, tt.importPath, absExpected, absGot)
				}
			} else if !tt.expectErr && err != nil {
				t.Errorf("Test %q: For import %q, expected success, but got error: %v", tt.name, tt.importPath, err)
			} else if tt.expectErr && err == nil {
				t.Errorf("Test %q: For import %q, expected error, but got path: %s", tt.name, tt.importPath, dir)
			}
		})
	}
}

// TODO: Add tests for getReplaceDirectives specifically if complex parsing logic needs unit testing.
// For now, its behavior is indirectly tested via TestFindPackageDirWithReplace.
