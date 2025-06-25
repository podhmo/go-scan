package goscan

import (
	"encoding/json" // Added for manual cache file creation in tests
	"os"            // Added for os.MkdirTemp, os.ReadFile, os.Stat
	"path/filepath" // Added for filepath.Join, filepath.Abs
	"reflect"       // Added for reflect.DeepEqual in tests
	"sort"          // Added for sorting slices in tests
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
	s, err := New("./scanner") // Assuming this test runs from project root where ./scanner is a valid sub-package.
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if s.locator == nil {
		t.Fatal("Scanner locator should not be nil")
	}
	if s.scanner == nil {
		t.Fatal("Scanner scanner should not be nil")
	}
	// This assertion depends on the main go.mod of the project.
	// If running tests from within a nested module or if go.mod changes, this might fail.
	// For now, assume "github.com/podhmo/go-scan" is the main module.
	if s.locator.ModulePath() != "github.com/podhmo/go-scan" {
		t.Errorf("Expected module path 'github.com/podhmo/go-scan', got %q", s.locator.ModulePath())
	}
}

// TestLazyResolution_Integration tests the full scanning and lazy resolution process.
// This test uses testdata/multipkg which should have its own go.mod declaring "example.com/multipkg-test"
func TestLazyResolution_Integration(t *testing.T) {
	// Scanner is initialized relative to the "testdata/multipkg" module.
	s, err := New("./testdata/multipkg")
	if err != nil {
		t.Fatalf("New() for multipkg failed: %v", err)
	}
	if s.locator.ModulePath() != "example.com/multipkg-test" {
		t.Fatalf("Expected module path 'example.com/multipkg-test', got '%s'", s.locator.ModulePath())
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
	// Define import paths from testdata/multipkg
	apiImportPath := "example.com/multipkg-test/api"       // Contains Handler type
	modelsImportPath := "example.com/multipkg-test/models" // Contains User type

	// Initialize scanner relative to the multipkg module
	sRoot, err := New("./testdata/multipkg")
	if err != nil {
		t.Fatalf("Failed to create scanner for multipkg module: %v", err)
	}
	moduleRootDir := sRoot.locator.RootDir() // This will be testdata/multipkg

	expectedHandlerFilePath, _ := filepath.Abs(filepath.Join(moduleRootDir, "api/handler.go"))
	expectedUserFilePath, _ := filepath.Abs(filepath.Join(moduleRootDir, "models/user.go"))

	t.Run("ScanAndUpdateCache_FindSymbol_CacheHit", func(t *testing.T) {
		testCacheDir, cleanupTestCacheDir := tempScannerDir(t)
		defer cleanupTestCacheDir()
		cacheFilePath := filepath.Join(testCacheDir, "symbols.json")

		s, err := New("./testdata/multipkg") // Scanner for the test module
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
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

		// In the symbol cache, file paths are stored relative to the module root (s.locator.RootDir())
		// which is testdata/multipkg in this test case.
		// So, expected path in cache is "api/handler.go", not the full absolute path.
		expectedHandlerRelPath := "api/handler.go"

		if actualPathInCache, ok := loadedCacheContent.Symbols[handlerSymbolFullName]; !ok {
			t.Errorf("Cache Symbols map does not contain %s. Content: %+v", handlerSymbolFullName, loadedCacheContent.Symbols)
		} else if actualPathInCache != expectedHandlerRelPath {
			t.Errorf("Cache Symbols map for %s has path %s, expected %s", handlerSymbolFullName, actualPathInCache, expectedHandlerRelPath)
		}

		if fileMeta, ok := loadedCacheContent.Files[expectedHandlerRelPath]; !ok {
			t.Errorf("Cache Files map does not contain entry for handler file %s. Content: %+v", expectedHandlerRelPath, loadedCacheContent.Files)
		} else {
			foundHandlerInFileMeta := false
			for _, symName := range fileMeta.Symbols {
				if symName == "Handler" {
					foundHandlerInFileMeta = true
					break
				}
			}
			if !foundHandlerInFileMeta {
				t.Errorf("Handler symbol not found in FileMetadata for %s. Symbols: %v", expectedHandlerRelPath, fileMeta.Symbols)
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

		s, err := New("./testdata/multipkg")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
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

		s, err := New("./testdata/multipkg")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		s.CachePath = cacheFilePath
		defer func() { s.SaveSymbolCache() }()

		staleUserSymbol := modelsImportPath + ".User"
		staleFileRelativePath := "models/non_existent_user.go"

		prefilledCacheContent := cache.CacheContent{
			Symbols: map[string]string{
				staleUserSymbol: staleFileRelativePath,
			},
			Files: map[string]cache.FileMetadata{
				staleFileRelativePath: {
					Symbols: []string{"User"},
				},
			},
		}
		jsonData, _ := json.MarshalIndent(prefilledCacheContent, "", "  ")
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

		sVerify, _ := New("./testdata/multipkg")
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

		s, err := New("./testdata/multipkg")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
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

		s, err := New("./testdata/multipkg")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		s.CachePath = ""

		defer func() { s.SaveSymbolCache() }()

		_, err = s.ScanPackageByImport(apiImportPath)
		if err != nil {
			t.Fatalf("ScanPackageByImport failed: %v", err)
		}

		if errSave := s.SaveSymbolCache(); errSave != nil {
			t.Errorf("SaveSymbolCache() with disabled cache errored: %v", errSave)
		}

		if _, err := os.Stat(cacheFilePath); !os.IsNotExist(err) {
			t.Errorf("Cache file %s was created even when s.CachePath is empty", cacheFilePath)
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
	if strings.EqualFold(abs1, abs2) {
		return true
	}
	return abs1 == abs2
}

func TestScannerWithExternalTypeOverrides(t *testing.T) {
	s, err := New("./testdata/externaltypes")
	if err != nil {
		t.Fatalf("Failed to create Scanner for testdata/externaltypes: %v", err)
	}

	overrides := scanner.ExternalTypeOverride{
		"github.com/google/uuid.UUID": "string",
		"example.com/somepkg.Time":    "mypkg.MyTime",
	}
	s.SetExternalTypeOverrides(overrides)

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
				if field.Name == "ID" {
					if field.Type.Name != "string" {
						t.Errorf("Expected field ID of ObjectWithUUID to be overridden to 'string', got '%s'", field.Type.Name)
					}
					if !field.Type.IsResolvedByConfig {
						t.Errorf("Expected field ID of ObjectWithUUID to have IsResolvedByConfig=true")
					}
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
				if field.Name == "Timestamp" {
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

	sBasic, err := New("./testdata/basic")
	if err != nil {
		t.Fatalf("Failed to create scanner for basic testdata: %v", err)
	}
	sBasic.SetExternalTypeOverrides(nil)

	pkgBasic, err := sBasic.ScanPackageByImport("github.com/podhmo/go-scan/testdata/basic")
	if err != nil {
		t.Fatalf("Failed to scan basic package: %v", err)
	}
	foundUserStruct := false
	for _, typeInfo := range pkgBasic.Types {
		if typeInfo.Name == "User" {
			foundUserStruct = true
			if typeInfo.Struct == nil {
				t.Errorf("User type should be a struct but it's not.")
				continue
			}
			if len(typeInfo.Struct.Fields) > 0 {
				idField := typeInfo.Struct.Fields[0]
				if idField.Name == "ID" {
					if idField.Type.IsResolvedByConfig {
						t.Errorf("Field ID in User should not have IsResolvedByConfig=true when no overrides are active for it")
					}
					if idField.Type.Name != "int" { // Assuming basic.User.ID is int
						t.Errorf("User.ID expected type 'int', got '%s'", idField.Type.Name)
					}
				}
			}
			break
		}
	}
	if !foundUserStruct {
		t.Errorf("User struct not found in basic package scan without overrides")
	}
}

// Helper function to check if a slice of strings contains a specific string.
func containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

// Helper function to check if two slices of strings are equal regardless of order.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := make([]string, len(a))
	bCopy := make([]string, len(b))
	copy(aCopy, a)
	copy(bCopy, b)
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	return reflect.DeepEqual(aCopy, bCopy)
}

func TestScanFilesAndGetUnscanned(t *testing.T) {
	// Setup: Initialize scanner relative to the 'testdata/scanfiles' module
	s, err := New("./testdata/scanfiles")
	if err != nil {
		t.Fatalf("New() for scanfiles module failed: %v", err)
	}
	if s.locator.ModulePath() != "example.com/scanfiles" {
		t.Fatalf("Expected module path 'example.com/scanfiles', got '%s'", s.locator.ModulePath())
	}

	coreUserPathAbs, _ := filepath.Abs("testdata/scanfiles/core/user.go")
	coreItemPathAbs, _ := filepath.Abs("testdata/scanfiles/core/item.go")
	coreEmptyPathAbs, _ := filepath.Abs("testdata/scanfiles/core/empty.go")
	// handlersUserHandlerPathAbs, _ := filepath.Abs("testdata/scanfiles/handlers/user_handler.go")

	corePkgImportPath := "example.com/scanfiles/core"
	// handlersPkgImportPath := "example.com/scanfiles/handlers"

	t.Run("ScanFiles_RelativePath_CoreUser", func(t *testing.T) {
		sTest, _ := New("./testdata/scanfiles") // Fresh scanner for this subtest

		// Change CWD for this specific sub-test to test CWD-relative paths robustly
		originalCwd, _ := os.Getwd()
		os.Chdir("testdata/scanfiles/core") // Change to core directory
		defer os.Chdir(originalCwd)         // Change back

		pkgInfo, err := sTest.ScanFiles([]string{"user.go"}) // Relative to new CWD: testdata/scanfiles/core
		if err != nil {
			t.Fatalf("ScanFiles for core/user.go (relative) failed: %v", err)
		}
		if pkgInfo == nil {
			t.Fatal("ScanFiles returned nil pkgInfo")
		}
		if !containsString(pkgInfo.Files, coreUserPathAbs) || len(pkgInfo.Files) != 1 {
			t.Errorf("ScanFiles: expected Files to contain only %s, got %v", coreUserPathAbs, pkgInfo.Files)
		}
		if _, visited := sTest.visitedFiles[coreUserPathAbs]; !visited {
			t.Errorf("ScanFiles: %s should be marked as visited", coreUserPathAbs)
		}
		if typeUser := findType(pkgInfo.Types, "User"); typeUser == nil {
			t.Error("Type 'User' not found in pkgInfo from ScanFiles(user.go)")
		}
	})

	t.Run("ScanFiles_AbsolutePath_CoreItem", func(t *testing.T) {
		sTest, _ := New("./testdata/scanfiles")
		pkgInfo, err := sTest.ScanFiles([]string{coreItemPathAbs})
		if err != nil {
			t.Fatalf("ScanFiles for %s (absolute) failed: %v", coreItemPathAbs, err)
		}
		if !containsString(pkgInfo.Files, coreItemPathAbs) || len(pkgInfo.Files) != 1 {
			t.Errorf("ScanFiles: expected Files to contain only %s, got %v", coreItemPathAbs, pkgInfo.Files)
		}
		if _, visited := sTest.visitedFiles[coreItemPathAbs]; !visited {
			t.Errorf("ScanFiles: %s should be marked as visited", coreItemPathAbs)
		}
		if typeItem := findType(pkgInfo.Types, "Item"); typeItem == nil {
			t.Error("Type 'Item' not found in pkgInfo from ScanFiles(item.go)")
		}
	})

	t.Run("ScanFiles_ModuleQualifiedPath_CoreUser", func(t *testing.T) {
		sTest, _ := New("./testdata/scanfiles") // Scanner initialized at scanfiles module root
		moduleQualifiedPath := "example.com/scanfiles/core/user.go"
		pkgInfo, err := sTest.ScanFiles([]string{moduleQualifiedPath})
		if err != nil {
			t.Fatalf("ScanFiles for %s (module-qualified) failed: %v", moduleQualifiedPath, err)
		}
		if !containsString(pkgInfo.Files, coreUserPathAbs) || len(pkgInfo.Files) != 1 {
			t.Errorf("ScanFiles: expected Files to contain only %s for module-qualified path, got %v", coreUserPathAbs, pkgInfo.Files)
		}
		if _, visited := sTest.visitedFiles[coreUserPathAbs]; !visited {
			t.Errorf("ScanFiles: %s should be marked as visited via module-qualified path", coreUserPathAbs)
		}
	})

	t.Run("ScanFiles_MultipleCalls_VisitedSkipped", func(t *testing.T) {
		sTest, _ := New("./testdata/scanfiles")
		// First call: scan user.go
		_, err := sTest.ScanFiles([]string{coreUserPathAbs})
		if err != nil {
			t.Fatalf("First ScanFiles call failed: %v", err)
		}

		// Second call: scan user.go (already visited) and item.go (new)
		pkgInfo, err := sTest.ScanFiles([]string{coreUserPathAbs, coreItemPathAbs})
		if err != nil {
			t.Fatalf("Second ScanFiles call failed: %v", err)
		}

		if !containsString(pkgInfo.Files, coreItemPathAbs) || len(pkgInfo.Files) != 1 {
			t.Errorf("Second ScanFiles: expected Files to contain only newly scanned %s, got %v", coreItemPathAbs, pkgInfo.Files)
		}
		if typeItem := findType(pkgInfo.Types, "Item"); typeItem == nil {
			t.Error("Type 'Item' not found in second ScanFiles call (should be from item.go)")
		}
		if typeUser := findType(pkgInfo.Types, "User"); typeUser != nil {
			t.Error("Type 'User' (from user.go) should not be in second ScanFiles result as it was already visited")
		}
		if _, visited := sTest.visitedFiles[coreUserPathAbs]; !visited {
			t.Error("user.go not marked visited")
		}
		if _, visited := sTest.visitedFiles[coreItemPathAbs]; !visited {
			t.Error("item.go not marked visited")
		}
	})

	t.Run("ScanFiles_AllFilesVisited_EmptyResult", func(t *testing.T) {
		sTest, _ := New("./testdata/scanfiles")
		_, err := sTest.ScanFiles([]string{coreUserPathAbs}) // Visit user.go
		if err != nil {
			t.Fatalf("Failed to scan user.go: %v", err)
		}

		pkgInfo, err := sTest.ScanFiles([]string{coreUserPathAbs}) // Scan again
		if err != nil {
			t.Fatalf("Second scan of user.go failed: %v", err)
		}
		if len(pkgInfo.Files) != 0 {
			t.Errorf("Expected Files to be empty when scanning already visited file, got %v", pkgInfo.Files)
		}
		if len(pkgInfo.Types) != 0 || len(pkgInfo.Functions) != 0 || len(pkgInfo.Constants) != 0 {
			t.Error("Expected no symbols when scanning already visited file")
		}
	})

	t.Run("UnscannedGoFiles_CorePackage", func(t *testing.T) {
		sTest, _ := New("./testdata/scanfiles")

		// Initially, all files in core should be unscanned
		unscanned, err := sTest.UnscannedGoFiles(corePkgImportPath)
		if err != nil {
			t.Fatalf("UnscannedGoFiles initial failed: %v", err)
		}
		expectedUnscannedInitial := []string{coreEmptyPathAbs, coreItemPathAbs, coreUserPathAbs}
		if !equalStringSlices(unscanned, expectedUnscannedInitial) {
			t.Errorf("Initial unscanned files: expected %v, got %v", expectedUnscannedInitial, unscanned)
		}

		// Scan user.go
		_, err = sTest.ScanFiles([]string{coreUserPathAbs})
		if err != nil {
			t.Fatalf("ScanFiles(user.go) failed: %v", err)
		}

		unscannedAfterUser, err := sTest.UnscannedGoFiles(corePkgImportPath)
		if err != nil {
			t.Fatalf("UnscannedGoFiles after user.go scan failed: %v", err)
		}
		expectedUnscannedAfterUser := []string{coreEmptyPathAbs, coreItemPathAbs}
		if !equalStringSlices(unscannedAfterUser, expectedUnscannedAfterUser) {
			t.Errorf("Unscanned after user.go: expected %v, got %v", expectedUnscannedAfterUser, unscannedAfterUser)
		}

		// Scan item.go and empty.go
		_, err = sTest.ScanFiles([]string{coreItemPathAbs, coreEmptyPathAbs})
		if err != nil {
			t.Fatalf("ScanFiles(item.go, empty.go) failed: %v", err)
		}

		unscannedAllScanned, err := sTest.UnscannedGoFiles(corePkgImportPath)
		if err != nil {
			t.Fatalf("UnscannedGoFiles after all core files scanned failed: %v", err)
		}
		if len(unscannedAllScanned) != 0 {
			t.Errorf("Expected no unscanned files after all core files scanned, got %v", unscannedAllScanned)
		}
	})

	t.Run("ScanPackage_RespectsVisitedFiles", func(t *testing.T) {
		sTest, _ := New("./testdata/scanfiles")
		// Scan core/user.go via ScanFiles first
		_, err := sTest.ScanFiles([]string{coreUserPathAbs})
		if err != nil {
			t.Fatalf("ScanFiles(user.go) failed: %v", err)
		}

		// Now ScanPackage for the whole core package
		// It should only parse item.go and empty.go as user.go is visited
		pkgInfo, err := sTest.ScanPackage("./testdata/scanfiles/core")
		if err != nil {
			t.Fatalf("ScanPackage(core) failed: %v", err)
		}

		if pkgInfo == nil {
			t.Fatal("ScanPackage returned nil pkgInfo")
		}

		// pkgInfo.Files from ScanPackage should only contain newly parsed files (item.go, empty.go)
		expectedFiles := []string{coreItemPathAbs, coreEmptyPathAbs}
		if !equalStringSlices(pkgInfo.Files, expectedFiles) {
			t.Errorf("ScanPackage(core) after ScanFiles(user.go): expected Files %v, got %v", expectedFiles, pkgInfo.Files)
		}
		if findType(pkgInfo.Types, "Item") == nil {
			t.Error("Type 'Item' should be in ScanPackage result")
		}
		if findType(pkgInfo.Types, "User") != nil { // User was already visited
			t.Error("Type 'User' should NOT be in this ScanPackage result (already visited)")
		}
		if _, visited := sTest.visitedFiles[coreItemPathAbs]; !visited {
			t.Error("item.go not marked visited after ScanPackage")
		}
		if _, visited := sTest.visitedFiles[coreEmptyPathAbs]; !visited {
			t.Error("empty.go not marked visited after ScanPackage")
		}

		// Check package cache for the import path
		sTest.mu.RLock()
		cachedInfo, found := sTest.packageCache[corePkgImportPath]
		sTest.mu.RUnlock()
		if !found {
			t.Errorf("PackageInfo for %s not found in packageCache after ScanPackage", corePkgImportPath)
		} else {
			// The cached info should be the result of this ScanPackage call
			if !equalStringSlices(cachedInfo.Files, expectedFiles) {
				t.Errorf("Cached PackageInfo.Files: expected %v, got %v", expectedFiles, cachedInfo.Files)
			}
		}
	})
}

func findType(types []*scanner.TypeInfo, name string) *scanner.TypeInfo {
	for _, ti := range types {
		if ti.Name == name {
			return ti
		}
	}
	return nil
}

func TestImplements(t *testing.T) {
	// Setup: Initialize scanner relative to the project root,
	// as testdata/implements is not a separate Go module.
	// We'll use ScanPackage with a direct path.
	s, err := New(".") // Assuming tests are run from project root
	if err != nil {
		t.Fatalf("New(\".\") failed: %v", err)
	}

	// Scan the testdata/implements package
	// Note: ScanPackage's import path resolution might be tricky for non-module testdata.
	// We rely on the absolute path for scanning and then manually construct import path if needed by TypeInfo.
	// However, Implements itself doesn't directly use import paths from TypeInfo for resolution,
	// it uses PkgName and Name from FieldType, which are derived by the scanner.
	// The crucial part is that all types and functions are loaded into one PackageInfo.
	pkgPath := "./testdata/implements"
	pkgInfo, err := s.ScanPackage(pkgPath)
	if err != nil {
		t.Fatalf("ScanPackage(%q) failed: %v", pkgPath, err)
	}
	if pkgInfo == nil {
		t.Fatalf("ScanPackage(%q) returned nil PackageInfo", pkgPath)
	}
	// For types within the same package, PkgName in FieldType might be empty or the package name.
	// This detail affects how compareFieldTypes works if it were to compare PkgName.
	// Current compareFieldTypes is simple, so this is less of an issue.

	getType := func(name string) *scanner.TypeInfo {
		for _, ti := range pkgInfo.Types {
			if ti.Name == name {
				return ti
			}
		}
		t.Fatalf("Type %q not found in package %q", name, pkgInfo.Name)
		return nil
	}

	tests := []struct {
		name                string
		structName          string
		interfaceName       string
		expectedToImplement bool
	}{
		// Basic cases
		{"SimpleStruct_SimpleInterface", "SimpleStruct", "SimpleInterface", true},
		{"SimpleStruct_EmptyInterface", "SimpleStruct", "EmptyInterface", true},
		{"PointerReceiverStruct_PointerReceiverInterface", "PointerReceiverStruct", "PointerReceiverInterface", true},
		{"ValueReceiverStruct_ValueReceiverInterface", "ValueReceiverStruct", "ValueReceiverInterface", true},
		{"ComplexTypeStruct_ComplexTypeInterface", "ComplexTypeStruct", "ComplexTypeInterface", true},

		// Negative cases: Method mismatches
		{"MissingMethodStruct_SimpleInterface", "MissingMethodStruct", "SimpleInterface", false},
		{"WrongNameStruct_SimpleInterface", "WrongNameStruct", "SimpleInterface", false},
		{"WrongParamCountStruct_SimpleInterface", "WrongParamCountStruct", "SimpleInterface", false},
		{"WrongParamTypeStruct_SimpleInterface", "WrongParamTypeStruct", "SimpleInterface", false},
		{"WrongReturnCountStruct_SimpleInterface", "WrongReturnCountStruct", "SimpleInterface", false},
		{"WrongReturnTypeStruct_SimpleInterface", "WrongReturnTypeStruct", "SimpleInterface", false},

		// Negative cases: Interface not implemented
		{"SimpleStruct_UnimplementedInterface", "SimpleStruct", "UnimplementedInterface", false},

		// Edge cases: Empty and no methods
		{"NoMethodStruct_SimpleInterface", "NoMethodStruct", "SimpleInterface", false},
		{"NoMethodStruct_EmptyInterface", "NoMethodStruct", "EmptyInterface", true}, // Any type implements empty interface

		// Receiver type considerations (based on current Implements logic)
		// Implements gathers methods for both T and *T if structCandidate is T.
		{"StructValueReceiverMethodX_InterfaceRequiresMethodX", "StructValueReceiverMethodX", "InterfaceRequiresMethodX", true},
		// StructPointerReceiverMethodX only has (*S) MethodX().
		// Implements(S_TypeInfo, I_TypeInfo) should be true if I just needs MethodX()
		// because *S has MethodX() and Implements considers methods of *S for S_TypeInfo.
		{"StructPointerReceiverMethodX_InterfaceRequiresMethodX", "StructPointerReceiverMethodX", "InterfaceRequiresMethodX", true},

		// Test if a struct with pointer receiver method can satisfy an interface method (no receiver specified on interface method)
		// when the struct itself is passed as a value type candidate.
		// InterfaceValueRecMethod has DoIt(). StructPointerRecMethodForInterfaceValue has (*S) DoIt().
		// Implements(StructPointerRecMethodForInterfaceValue_TypeInfo, InterfaceValueRecMethod_TypeInfo) should be true.
		{"StructPointerRecMethodForInterfaceValue_InterfaceValueRecMethod", "StructPointerRecMethodForInterfaceValue", "InterfaceValueRecMethod", true},

		// Test if a struct with only value receiver can satisfy an interface that might "imply" pointer methods.
		// MyStructForReceiverTest has (t MyStructForReceiverTest) ValRec() and (t *MyStructForReceiverTest) PtrRec()
		// InterfaceForValRec has ValRec()
		// InterfaceForPtrRec has PtrRec()
		{"MyStructForReceiverTest_InterfaceForValRec", "MyStructForReceiverTest", "InterfaceForValRec", true},
		// MyStructForReceiverTest (value type) does not have PtrRec() in its value method set.
		// But Implements will find (*MyStructForReceiverTest) PtrRec() and associate it.
		{"MyStructForReceiverTest_InterfaceForPtrRec", "MyStructForReceiverTest", "InterfaceForPtrRec", true},

		// Nil and invalid inputs (tested directly, not via table for clarity on nil pkgInfo etc.)

		// Type comparison details (assuming compareFieldTypes will be/is correct for slices, maps, pointers)
		{"StructWithAnotherType_InterfaceWithAnotherType", "StructWithAnotherType", "InterfaceWithAnotherType", true},
		{"StructWithDifferentNamedType_InterfaceWithAnotherType", "StructWithDifferentNamedType", "InterfaceWithAnotherType", false},                           // Fails due to YetAnotherType vs AnotherType
		{"StructWithMismatchedPointerForAnotherType_InterfaceWithAnotherType", "StructWithMismatchedPointerForAnotherType", "InterfaceWithAnotherType", false}, // Fails due to pointer mismatch on param/return

		{"StructImplementingSliceMap_InterfaceWithSliceMap", "StructImplementingSliceMap", "InterfaceWithSliceMap", true},
		{"StructMismatchSlice_InterfaceWithSliceMap", "StructMismatchSlice", "InterfaceWithSliceMap", false},       // Assumes compareFieldTypes handles slice element type
		{"StructMismatchMapValue_InterfaceWithSliceMap", "StructMismatchMapValue", "InterfaceWithSliceMap", false}, // Assumes compareFieldTypes handles map value type
		{"StructMismatchMapKey_InterfaceWithSliceMap", "StructMismatchMapKey", "InterfaceWithSliceMap", false},     // Assumes compareFieldTypes handles map key type

		{"StructWithPointerInSlice_InterfaceWithPointerInSlice", "StructWithPointerInSlice", "InterfaceWithPointerInSlice", true},                    // []*int vs []*int
		{"StructWithPointerInSlice_InterfaceWithDifferentPointerInSlice", "StructWithPointerInSlice", "InterfaceWithDifferentPointerInSlice", false}, // []*int vs []*string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structCandidate := getType(tt.structName)
			interfaceDef := getType(tt.interfaceName)

			// Pre-checks for test validity
			if structCandidate == nil {
				t.Fatalf("Test setup error: Struct candidate %q not found", tt.structName)
			}
			if structCandidate.Kind != StructKind && tt.structName != "NotAStruct" { // Allow NotAStruct for specific test
				t.Fatalf("Test setup error: Candidate %q is not a struct but Kind=%v", tt.structName, structCandidate.Kind)
			}
			if interfaceDef == nil {
				t.Fatalf("Test setup error: Interface definition %q not found", tt.interfaceName)
			}
			if interfaceDef.Kind != InterfaceKind && tt.interfaceName != "NotAnInterface" { // Allow for specific test
				t.Fatalf("Test setup error: Interface def %q is not an interface but Kind=%v", tt.interfaceName, interfaceDef.Kind)
			}

			// Perform the Implements check
			// Ensure pkgInfo is correctly passed; it's derived from scanning "./testdata/implements"
			// and contains all types and functions from types.go
			actual := Implements(structCandidate, interfaceDef, pkgInfo)
			if actual != tt.expectedToImplement {
				t.Errorf("Implements(%s, %s): expected %v, got %v", tt.structName, tt.interfaceName, tt.expectedToImplement, actual)
			}
		})
	}

	// Direct tests for nil and invalid inputs
	t.Run("NilInputs", func(t *testing.T) {
		simpleStruct := getType("SimpleStruct")
		simpleInterface := getType("SimpleInterface")

		if Implements(nil, simpleInterface, pkgInfo) != false {
			t.Error("Implements(nil, interface, pkgInfo) should be false")
		}
		if Implements(simpleStruct, nil, pkgInfo) != false {
			t.Error("Implements(struct, nil, pkgInfo) should be false")
		}
		if Implements(simpleStruct, simpleInterface, nil) != false {
			t.Error("Implements(struct, interface, nil) should be false")
		}
	})

	t.Run("InvalidKindInputs", func(t *testing.T) {
		notAStruct := getType("NotAStruct")         // This is an alias to int
		notAnInterface := getType("NotAnInterface") // This is an alias to int
		simpleStruct := getType("SimpleStruct")
		simpleInterface := getType("SimpleInterface")

		if notAStruct.Kind == StructKind { // Defensive check on test data itself
			t.Fatal("Test data error: NotAStruct should not have StructKind")
		}
		if notAnInterface.Kind == InterfaceKind { // Defensive check
			t.Fatal("Test data error: NotAnInterface should not have InterfaceKind")
		}

		if Implements(notAStruct, simpleInterface, pkgInfo) != false {
			t.Errorf("Implements(NotAStruct, SimpleInterface, pkgInfo) expected false, got true. NotAStruct.Kind: %v", notAStruct.Kind)
		}
		if Implements(simpleStruct, notAnInterface, pkgInfo) != false {
			t.Errorf("Implements(SimpleStruct, NotAnInterface, pkgInfo) expected false, got true. NotAnInterface.Kind: %v, Interface field: %+v", notAnInterface.Kind, notAnInterface.Interface)
		}
		// Test case where interfaceDef.Interface is nil (e.g. type alias for interface)
		// The getType function resolves to TypeInfo. If NotAnInterface is an alias type,
		// its TypeInfo.Interface field should be nil.
		// The Implements function checks `interfaceDef.Interface == nil`.
		// `scanner.parseTypeSpec` for alias `type MyI = OtherInterface` would set Underlying.
		// For `type MyI int` where MyI is used as interface, `interfaceDef.Kind` wouldn't be `InterfaceKind`.
		// For `type MyI interface {}` then `interfaceDef.Kind` is `InterfaceKind`.
		// If `type MyI OtherInterface` (not an alias, but embedding), scanner would need to resolve.
		// The `NotAnInterface` type in `types.go` is `type NotAnInterface int`. Its kind is `AliasKind`.
		// So `interfaceDef.Kind != InterfaceKind` check in `Implements` handles this.
	})

	// Test for interfaceDef.Interface being nil (e.g. an alias that isn't an interface kind)
	// This is implicitly covered by InvalidKindInputs if NotAnInterface.Kind is not InterfaceKind.
	// If we had `type AliasToInterface = SimpleInterface`, then its Kind might be InterfaceKind (or AliasKind, depending on scanner).
	// If `AliasKind` and `Underlying` points to an interface, `Implements` would need to handle it.
	// Current `Implements` expects `interfaceDef.Kind == InterfaceKind`.
}
