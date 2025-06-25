package goscan

import (
	"encoding/json" // Added for manual cache file creation in tests
	"os"            // Added for os.MkdirTemp, os.ReadFile, os.Stat
	"path/filepath" // Added for filepath.Join, filepath.Abs
	"strings"       // Added for strings.Contains
	"testing"
	// "time" // Removed: No longer used

	"github.com/podhmo/go-scan/cache" // Now needed for direct cache content manipulation
	"github.com/podhmo/go-scan/scanner"
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
	// Define import paths from testdata
	apiImportPath := "example.com/multipkg-test/api"       // Contains Handler type
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
		// s.UseCache = true // Removed
		s.CachePath = cacheFilePath // Cache enabled by setting a non-empty path

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

		if err := s.SaveSymbolCache(); err != nil {
			t.Fatalf("Explicit save failed: %v", err)
		}

		data, err := os.ReadFile(cacheFilePath)
		if err != nil {
			t.Fatalf("Failed to read cache file: %v", err)
		}

		var loadedCacheContent cache.CacheContent
		if err := json.Unmarshal(data, &loadedCacheContent); err != nil {
			t.Fatalf("Failed to unmarshal cache content: %v", err)
		}

		// Check if Handler symbol is in the Symbols map
		if _, ok := loadedCacheContent.Symbols[handlerSymbolFullName]; !ok {
			t.Errorf("Cache Symbols map does not contain %s. Content: %+v", handlerSymbolFullName, loadedCacheContent.Symbols)
		}

		// Check if the file containing Handler is in the Files map and has Handler in its symbol list
		// Get the relative path of the handler file as stored in the cache (from the Symbols map).
		relPathForHandlerFileFromCache, foundSymbol := loadedCacheContent.Symbols[handlerSymbolFullName]
		if !foundSymbol {
			t.Fatalf("Handler symbol %s not found in loaded cache Symbols map, cannot verify Files map.", handlerSymbolFullName)
		}

		if fileMeta, ok := loadedCacheContent.Files[relPathForHandlerFileFromCache]; !ok {
			t.Errorf("Cache Files map does not contain entry for handler file %s (path from Symbols map). Content: %+v", relPathForHandlerFileFromCache, loadedCacheContent.Files)
		} else {
			foundHandlerInFileMeta := false
			for _, symName := range fileMeta.Symbols {
				if symName == "Handler" { // Assuming symbol name is stored without package prefix in FileMetadata.Symbols
					foundHandlerInFileMeta = true
					break
				}
			}
			if !foundHandlerInFileMeta {
				t.Errorf("Handler symbol not found in FileMetadata for %s. Symbols: %v", relPathForHandlerFileFromCache, fileMeta.Symbols)
			}
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
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		// s.UseCache = true // Removed
		s.CachePath = cacheFilePath // Cache enabled by setting a non-empty path
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
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		// s.UseCache = true // Removed
		s.CachePath = cacheFilePath // Cache enabled by setting a non-empty path
		defer func() { s.SaveSymbolCache() }()

		staleUserSymbol := modelsImportPath + ".User"
		// Construct path relative to moduleRootDir for the prefilled cache.
		// SymbolCache stores paths relative to its rootDir, which for Scanner is moduleRootDir.
		staleFileRelativePath := "testdata/multipkg/models/non_existent_user.go" // This file doesn't actually exist

		// Prefill cache with a stale entry pointing to a non-existent file
		// for a specific symbol, and also include a FileMetadata entry for that file.
		prefilledCacheContent := cache.CacheContent{
			Symbols: map[string]string{
				staleUserSymbol: staleFileRelativePath, // staleUserSymbol -> non_existent_user.go
			},
			Files: map[string]cache.FileMetadata{
				staleFileRelativePath: { // Entry for non_existent_user.go
					Symbols: []string{"User"}, // Assumes "User" was in this non-existent file
					// ModTime would have been here if used
				},
			},
		}
		jsonData, _ := json.MarshalIndent(prefilledCacheContent, "", "  ")
		os.MkdirAll(filepath.Dir(cacheFilePath), 0755) // Ensure cache directory exists
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
		// sVerify.UseCache = true // Removed
		sVerify.CachePath = cacheFilePath // Cache enabled by setting path

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
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		// s.UseCache = true // Removed
		s.CachePath = cacheFilePath // Cache enabled by setting a non-empty path
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
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		// s.UseCache = false // Removed
		s.CachePath = "" // Cache explicitly disabled by empty path
		// We can still set cacheFilePath for os.Stat check, to ensure no file is created AT THAT specific path
		// even if some default path logic were to kick in (though it shouldn't with empty CachePath).
		// For this test, the check is that s.CachePath (being empty) prevents creation.
		// If we want to ensure no file is created at a *hypothetical* default location, that's a different test.
		// The current CachePath on Scanner is the single source of truth.
		// So, if s.CachePath is "", no file should be written by SaveSymbolCache.
		// The test needs to check for a file at `cacheFilePath` (the variable).
		// If CachePath is empty, SaveSymbolCache should do nothing.

		// Let's clarify the test's intent:
		// If CachePath is empty, SaveSymbolCache should not attempt to write *any* file.
		// We don't need `cacheFilePath` variable for s.CachePath here if it's meant to be disabled.
		// The check `os.Stat(cacheFilePath)` where `cacheFilePath` is `filepath.Join(testCacheDir, "symbols_disabled.json")`
		// is fine to ensure that specific file isn't created.
		// What `s.CachePath` is set to for the `os.Stat` check needs to be consistent.
		// If `s.CachePath` is `""`, then `s.symbolCache.FilePath()` would be `""`.
		// `SaveSymbolCache` checks `s.CachePath == ""`.

		// Revised logic for this test:
		// s.CachePath is kept as "" (or not set) to disable caching.
		// The check for file creation needs to consider that no path means no creation.
		// The test as written tries to Stat `cacheFilePath` which is a local var.
		// This is fine: we are checking that a file at a specific location is NOT created
		// when cache is disabled via empty s.CachePath.

		defer func() { s.SaveSymbolCache() }() // This will be called, SaveSymbolCache should do nothing if s.CachePath is ""

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

func TestScannerWithExternalTypeOverrides(t *testing.T) {
	// Setup: Create a scanner instance. Point to the new testdata module.
	// The module for externaltypes is example.com/externaltypes
	// The scanner needs to be initialized with a path *within* that module,
	// or any path from which that module can be found (e.g. "." if testdata/externaltypes/go.mod makes it part of a workspace).
	// For simplicity, let's assume we run tests from the project root, and locator can find testdata/externaltypes.
	// We need to ensure the locator can correctly find "example.com/externaltypes".
	// The New() function takes a startPath to find the main go.mod of the project being scanned.
	// If testdata/externaltypes is a self-contained module, we might need a way to point the locator to it.
	// Let's use "./testdata/externaltypes" as the start path for New(), assuming it has its own go.mod.
	s, err := New("./testdata/externaltypes")
	if err != nil {
		t.Fatalf("Failed to create Scanner for testdata/externaltypes: %v", err)
	}

	// Define overrides
	overrides := scanner.ExternalTypeOverride{
		"github.com/google/uuid.UUID": "string",      // uuid.UUID should be treated as string
		"example.com/somepkg.Time":    "mypkg.MyTime",  // a custom non-existent type to another custom string
	}
	s.SetExternalTypeOverrides(overrides)

	// Scan the package containing the types.
	// The import path for testdata/externaltypes module is example.com/externaltypes as defined in its go.mod
	pkgInfo, err := s.ScanPackageByImport("example.com/externaltypes")
	if err != nil {
		t.Fatalf("Failed to scan package 'example.com/externaltypes': %v", err)
	}

	foundObjectWithUUID := false
	foundObjectWithCustomTime := false

	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Name == "ObjectWithUUID" {
			foundObjectWithUUID = true
			if typeInfo.Struct == nil {
				t.Errorf("ObjectWithUUID should be a struct, but it's not")
				continue
			}
			for _, field := range typeInfo.Struct.Fields {
				if field.Name == "ID" { // This field is of type uuid.UUID
					if field.Type.Name != "string" {
						t.Errorf("Expected field ID of ObjectWithUUID to be overridden to 'string', got '%s'", field.Type.Name)
					}
					if !field.Type.IsResolvedByConfig {
						t.Errorf("Expected field ID of ObjectWithUUID to have IsResolvedByConfig=true")
					}
					// Try to resolve to see if it correctly does nothing for overridden types
					resolvedType, errResolve := field.Type.Resolve()
					if errResolve != nil {
						t.Errorf("field.Type.Resolve() for overridden type should not error, got %v", errResolve)
					}
					if resolvedType != nil {
						t.Errorf("field.Type.Resolve() for overridden type should return nil TypeInfo, got %v", resolvedType)
					}
				}
			}
		} else if typeInfo.Name == "ObjectWithCustomTime" {
			foundObjectWithCustomTime = true
			if typeInfo.Struct == nil {
				t.Errorf("ObjectWithCustomTime should be a struct, but it's not")
				continue
			}
			for _, field := range typeInfo.Struct.Fields {
				if field.Name == "Timestamp" { // This field is of type somepkg.Time
					if field.Type.Name != "mypkg.MyTime" {
						t.Errorf("Expected field Timestamp of ObjectWithCustomTime to be overridden to 'mypkg.MyTime', got '%s'", field.Type.Name)
					}
					if !field.Type.IsResolvedByConfig {
						t.Errorf("Expected field Timestamp of ObjectWithCustomTime to have IsResolvedByConfig=true")
					}
				}
			}
		}
	}

	if !foundObjectWithUUID {
		t.Errorf("Type 'ObjectWithUUID' not found in scanned package")
	}
	if !foundObjectWithCustomTime {
		t.Errorf("Type 'ObjectWithCustomTime' not found in scanned package")
	}

	// Test that non-overridden types are handled normally.
	// Re-scan the basic package with no overrides on this scanner instance.
	// To do this cleanly, create a new scanner or reset overrides.
	sBasic, err := New("./testdata/basic") // Assuming basic is another module or found from root
	if err != nil {
		t.Fatalf("Failed to create scanner for basic testdata: %v", err)
	}
	sBasic.SetExternalTypeOverrides(nil) // Ensure no overrides

	pkgBasic, err := sBasic.ScanPackageByImport("github.com/podhmo/go-scan/testdata/basic")
	if err != nil {
		t.Fatalf("Failed to scan basic package: %v", err)
	}
	foundUserStruct := false
	for _, typeInfo := range pkgBasic.Types {
		if typeInfo.Name == "User" { // Changed from MyStruct to User
			foundUserStruct = true
			if typeInfo.Struct == nil {
				t.Errorf("User type should be a struct but it's not.")
				continue
			}
			// Assuming User has a field like "ID int" or similar primitive
			if len(typeInfo.Struct.Fields) > 0 {
				idField := typeInfo.Struct.Fields[0] // Assuming ID is the first field
				if idField.Name == "ID" { // Check field "ID" specifically
					if idField.Type.IsResolvedByConfig {
						t.Errorf("Field ID in User should not have IsResolvedByConfig=true when no overrides are active for it")
					}
					if idField.Type.Name != "int" {
						t.Errorf("User.ID expected type 'int', got '%s'", idField.Type.Name)
					}
				}
			}
			break
		}
	}
	if !foundUserStruct { // Changed from foundMyStruct
		t.Errorf("User struct not found in basic package scan without overrides") // Changed message
	}
}
