package typescanner

import (
	"encoding/json" // Added for manual cache file creation in tests
	"os"            // Added for os.MkdirTemp, os.ReadFile, os.Stat
	"path/filepath" // Added for filepath.Join, filepath.Abs
	"strings"       // Added for strings.Contains
	"testing"

	"github.com/podhmo/go-scan/scanner"
	// No need to import cache directly unless we are type-asserting SymbolCache internals
)

// Helper to create a temporary directory for testing scanner cache
func tempScannerDir(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "scanner_cache_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir for scanner test: %v", err)
	}
	return dir, func() { os.RemoveAll(dir) }
}

// TestNew_Integration tests the creation of a new Scanner and its underlying locator.
func TestNew_Integration(t *testing.T) {
	s, err := New("./scanner")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if s.locator == nil {
		t.Fatal("Scanner locator should not be nil")
	}
	if s.scanner == nil {
		t.Fatal("Scanner scanner should not be nil")
	}
	if s.locator.ModulePath() != "github.com/podhmo/go-scan" {
		t.Errorf("Expected module path 'github.com/podhmo/go-scan', got %q", s.locator.ModulePath())
	}
}

// TestLazyResolution_Integration tests the full scanning and lazy resolution process.
func TestLazyResolution_Integration(t *testing.T) {
	s, err := New(".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Scan the 'api' package, which depends on the 'models' package.
	apiImportPath := "example.com/multipkg-test/api"
	pkgInfo, err := s.ScanPackageByImport(apiImportPath)
	if err != nil {
		t.Fatalf("ScanPackageByImport() failed: %v", err)
	}

	// Find the Handler struct
	var handlerStruct *scanner.TypeInfo
	for _, ti := range pkgInfo.Types {
		if ti.Name == "Handler" {
			handlerStruct = ti
			break
		}
	}
	if handlerStruct == nil {
		t.Fatal("Could not find 'Handler' struct in api package")
	}
	if handlerStruct.Struct == nil || len(handlerStruct.Struct.Fields) == 0 {
		t.Fatal("Handler struct has no fields")
	}

	// Find the User field
	userField := handlerStruct.Struct.Fields[0]
	if userField.Name != "User" {
		t.Fatalf("Expected first field to be 'User', got %s", userField.Name)
	}

	// At this point, the 'models' package should not have been scanned yet.
	s.mu.RLock()
	_, found := s.packageCache["example.com/multipkg-test/models"]
	s.mu.RUnlock()
	if found {
		t.Fatal("'models' package should not be in cache before resolving")
	}

	// Trigger lazy resolution
	userDef, err := userField.Type.Resolve()
	if err != nil {
		t.Fatalf("Failed to resolve User field type: %v", err)
	}

	// Now the 'models' package should be in the cache.
	s.mu.RLock()
	_, found = s.packageCache["example.com/multipkg-test/models"]
	s.mu.RUnlock()
	if !found {
		t.Fatal("'models' package should be in cache after resolving")
	}

	// Check the resolved definition
	if userDef.Name != "User" {
		t.Errorf("Expected resolved type name to be 'User', got %q", userDef.Name)
	}
	if userDef.Kind != scanner.StructKind {
		t.Errorf("Expected resolved type kind to be StructKind")
	}
	if len(userDef.Struct.Fields) != 2 {
		t.Errorf("Expected resolved User struct to have 2 fields, got %d", len(userDef.Struct.Fields))
	}
	if userDef.Struct.Fields[0].Name != "ID" || userDef.Struct.Fields[1].Name != "Name" {
		t.Error("Resolved User struct fields are incorrect")
	}
}

func TestScanner_WithSymbolCache(t *testing.T) {
	// Base directory for creating temporary project structures for cache tests
	baseTestDir, cleanupBaseTestDir := tempScannerDir(t)
	defer cleanupBaseTestDir()

	// Define import paths from testdata
	apiImportPath := "example.com/multipkg-test/api"    // Contains Handler type
	modelsImportPath := "example.com/multipkg-test/models" // Contains User type

	sRoot, err := New(".") // Assuming this test runs from module root.
	if err != nil {
		t.Fatalf("Failed to create scanner for module root: %v", err)
	}
	moduleRootDir := sRoot.locator.RootDir()

	expectedHandlerFilePath, _ := filepath.Abs(filepath.Join(moduleRootDir, "testdata/multipkg/api/handler.go"))
	expectedUserFilePath, _ := filepath.Abs(filepath.Join(moduleRootDir, "testdata/multipkg/models/user.go"))


	t.Run("ScanAndUpdateCache_FindSymbol_CacheHit", func(t *testing.T) {
		testCacheDir, cleanupTestCacheDir := tempScannerDir(t)
		defer cleanupTestCacheDir()
		cacheFilePath := filepath.Join(testCacheDir, "symbols.json")

		s, err := New(".")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		s.UseCache = true
		s.CachePath = cacheFilePath

		defer func() {
			if err := s.SaveSymbolCache(); err != nil {
				t.Errorf("Failed to save symbol cache: %v", err)
			}
		}()

		_, err = s.ScanPackageByImport(apiImportPath)
		if err != nil {
			t.Fatalf("ScanPackageByImport(%s) failed: %v", apiImportPath, err)
		}

		handlerSymbolFullName := apiImportPath + ".Handler"
		loc, err := s.FindSymbolDefinitionLocation(handlerSymbolFullName)
		if err != nil {
			t.Fatalf("FindSymbolDefinitionLocation(%s) after scan failed: %v", handlerSymbolFullName, err)
		}
		if !pathsEqual(loc, expectedHandlerFilePath) {
			t.Errorf("Expected Handler path %s, got %s", expectedHandlerFilePath, loc)
		}

		if err := s.SaveSymbolCache(); err != nil { t.Fatalf("Explicit save failed: %v", err) }

		data, err := os.ReadFile(cacheFilePath)
		if err != nil { t.Fatalf("Failed to read cache file: %v", err) }
		if !strings.Contains(string(data), handlerSymbolFullName) {
			t.Errorf("Cache file content does not seem to contain %s. Content: %s", handlerSymbolFullName, string(data))
		}

		_, err = s.ScanPackageByImport(modelsImportPath)
		if err != nil {
			t.Fatalf("ScanPackageByImport(%s) failed: %v", modelsImportPath, err)
		}

		userSymbolFullName := modelsImportPath + ".User"
		locUser, errUser := s.FindSymbolDefinitionLocation(userSymbolFullName)
		if errUser != nil {
			t.Fatalf("FindSymbolDefinitionLocation(%s) after scan failed: %v", userSymbolFullName, errUser)
		}
		if !pathsEqual(locUser, expectedUserFilePath) {
			t.Errorf("Expected User path %s, got %s", expectedUserFilePath, locUser)
		}
	})

	t.Run("FindSymbol_CacheMiss_FallbackScanSuccess", func(t *testing.T) {
		testCacheDir, cleanupTestCacheDir := tempScannerDir(t)
		defer cleanupTestCacheDir()
		cacheFilePath := filepath.Join(testCacheDir, "symbols_fallback.json")

		s, err := New(".")
		if err != nil { t.Fatalf("New() failed: %v", err) }
		s.UseCache = true
		s.CachePath = cacheFilePath
		defer func() { s.SaveSymbolCache() }()

		userSymbolFullName := modelsImportPath + ".User"
		loc, err := s.FindSymbolDefinitionLocation(userSymbolFullName)
		if err != nil {
			t.Fatalf("FindSymbolDefinitionLocation(%s) with empty cache failed: %v", userSymbolFullName, err)
		}
		if !pathsEqual(loc, expectedUserFilePath) {
			t.Errorf("Expected User path %s, got %s after fallback scan", expectedUserFilePath, loc)
		}

		locHit, errHit := s.FindSymbolDefinitionLocation(userSymbolFullName)
		if errHit != nil {
			t.Fatalf("FindSymbolDefinitionLocation(%s) second time (expect cache hit) failed: %v", userSymbolFullName, errHit)
		}
		if !pathsEqual(locHit, expectedUserFilePath) {
			t.Errorf("Expected User path %s on cache hit, got %s", expectedUserFilePath, locHit)
		}
	})

	t.Run("FindSymbol_CacheStale_FallbackScanSuccess", func(t *testing.T) {
		testCacheDir, cleanupTestCacheDir := tempScannerDir(t)
		defer cleanupTestCacheDir()
		cacheFilePath := filepath.Join(testCacheDir, "symbols_stale.json")

		s, err := New(".")
		if err != nil { t.Fatalf("New() failed: %v", err) }
		s.UseCache = true
		s.CachePath = cacheFilePath
		defer func() { s.SaveSymbolCache() }()

		staleUserSymbol := modelsImportPath + ".User"
		// Construct path relative to moduleRootDir for the prefilled cache.
		// SymbolCache stores paths relative to its rootDir, which for Scanner is moduleRootDir.
		staleFileRelativePath := "testdata/multipkg/models/non_existent_user.go"

		prefilledCacheData := map[string]string{
			staleUserSymbol: staleFileRelativePath, // Stored as relative path with forward slashes
		}
		jsonData, _ := json.Marshal(prefilledCacheData)
		os.MkdirAll(filepath.Dir(cacheFilePath), 0755)
		os.WriteFile(cacheFilePath, jsonData, 0644)

		loc, err := s.FindSymbolDefinitionLocation(staleUserSymbol)
		if err != nil {
			t.Fatalf("FindSymbolDefinitionLocation for stale entry failed: %v", err)
		}
		if !pathsEqual(loc, expectedUserFilePath) {
			t.Errorf("Expected User path %s after stale cache fallback, got %s", expectedUserFilePath, loc)
		}

		s.SaveSymbolCache()

		sVerify, _ := New(".")
		sVerify.UseCache = true
		sVerify.CachePath = cacheFilePath

		locVerify, errVerify := sVerify.FindSymbolDefinitionLocation(staleUserSymbol)
		if errVerify != nil {
			t.Fatalf("FindSymbolDefinitionLocation after stale fix failed: %v", errVerify)
		}
		if !pathsEqual(locVerify, expectedUserFilePath) {
			t.Errorf("Cache not updated correctly. Expected %s, got %s", expectedUserFilePath, locVerify)
		}
	})

	t.Run("FindSymbol_NonExistentSymbol_FallbackScanFail", func(t *testing.T) {
		testCacheDir, cleanupTestCacheDir := tempScannerDir(t)
		defer cleanupTestCacheDir()
		cacheFilePath := filepath.Join(testCacheDir, "symbols_nonexist.json")

		s, err := New(".")
		if err != nil { t.Fatalf("New() failed: %v", err) }
		s.UseCache = true
		s.CachePath = cacheFilePath
		defer func() { s.SaveSymbolCache() }()

		nonExistentSymbol := modelsImportPath + ".NonExistentType"
		_, err = s.FindSymbolDefinitionLocation(nonExistentSymbol)
		if err == nil {
			t.Fatalf("FindSymbolDefinitionLocation for non-existent symbol %s should have failed", nonExistentSymbol)
		}
		expectedErrorSubString := "not found in package"
		if !strings.Contains(err.Error(), expectedErrorSubString) {
			t.Errorf("Expected error for non-existent symbol to contain %q, got: %v", expectedErrorSubString, err)
		}
	})

	t.Run("CacheDisabled_NoCacheFileCreated", func(t *testing.T) {
		testCacheDir, cleanupTestCacheDir := tempScannerDir(t)
		defer cleanupTestCacheDir()
		cacheFilePath := filepath.Join(testCacheDir, "symbols_disabled.json")

		s, err := New(".")
		if err != nil { t.Fatalf("New() failed: %v", err) }
		s.UseCache = false
		s.CachePath = cacheFilePath
		defer func() { s.SaveSymbolCache() }()

		_, err = s.ScanPackageByImport(apiImportPath)
		if err != nil {
			t.Fatalf("ScanPackageByImport failed: %v", err)
		}

		if errSave := s.SaveSymbolCache(); errSave != nil {
			t.Errorf("SaveSymbolCache() with disabled cache errored: %v", errSave)
		}

		if _, err := os.Stat(cacheFilePath); !os.IsNotExist(err) {
			t.Errorf("Cache file %s was created even when UseCache is false", cacheFilePath)
		}
	})
}

func pathsEqual(p1, p2 string) bool {
	abs1, err1 := filepath.Abs(p1)
	if err1 != nil {
		return false
	}
	abs2, err2 := filepath.Abs(p2)
	if err2 != nil {
		return false
	}
	// On Windows, file paths are case-insensitive.
	// On other systems, they are case-sensitive.
	// For robust testing, especially if developing on one OS and CI on another:
	if strings.EqualFold(abs1, abs2) { // Use EqualFold for case-insensitivity
        return true
    }
	return abs1 == abs2 // Fallback for systems where case matters and paths differ only by case
}
