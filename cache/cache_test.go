package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime" // Added for GOOS
	"strings"
	"testing"
)

// Helper to create a temporary directory for testing
func tempDir(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cache_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir, func() { os.RemoveAll(dir) }
}

// Helper to create a dummy root directory for relative path testing
func tempRootDir(t *testing.T, baseDir string) string {
	t.Helper()
	rootDir := filepath.Join(baseDir, "project_root")
	err := os.MkdirAll(rootDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create temp root dir: %v", err)
	}
	return rootDir
}

func TestNewSymbolCache(t *testing.T) {
	projectRoot, cleanupProjectRoot := tempDir(t)
	defer cleanupProjectRoot()

	t.Run("CacheDisabled_WithEmptyPath", func(t *testing.T) {
		sc, err := NewSymbolCache(projectRoot, "") // Empty path
		if err != nil {
			t.Fatalf("NewSymbolCache with empty path failed: %v", err)
		}
		if sc.IsEnabled() {
			t.Errorf("Expected cache to be disabled when path is empty")
		}
		if sc.FilePath() != "" {
			t.Errorf("Expected empty file path for disabled cache, got %s", sc.FilePath())
		}
	})

	// Default path logic is removed from NewSymbolCache.
	// This test is now simplified to "CacheEnabled_WithNonEmptyPath".
	t.Run("CacheEnabled_WithNonEmptyPath", func(t *testing.T) {
		customPath := filepath.Join(projectRoot, "custom_cache.json")
		sc, err := NewSymbolCache(projectRoot, customPath) // Non-empty path
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !sc.IsEnabled() {
			t.Errorf("Expected cache to be enabled")
		}
		if sc.FilePath() != customPath {
			t.Errorf("Expected custom path %s, got %s", customPath, sc.FilePath())
		}
	})
}

func TestSymbolCache_Load_Save(t *testing.T) {
	cacheDir, cleanupCacheDir := tempDir(t)
	defer cleanupCacheDir()
	projectRoot := tempRootDir(t, cacheDir) // project_root inside cacheDir

	cacheFilePath := filepath.Join(cacheDir, "test_cache.json")

	t.Run("Load_file_not_exist", func(t *testing.T) {
		sc, _ := NewSymbolCache(projectRoot, cacheFilePath) // Path provided, cache enabled
		err := sc.Load(context.Background())
		if err != nil {
			t.Fatalf("Load() from non-existent file should not error, got %v", err)
		}
		if len(sc.content.Symbols) != 0 {
			t.Errorf("Expected empty Symbols cache, got %d items", len(sc.content.Symbols))
		}
		if len(sc.content.Files) != 0 {
			t.Errorf("Expected empty Files cache, got %d items", len(sc.content.Files))
		}
	})

	t.Run("Save_and_Load_data", func(t *testing.T) {
		scWrite, _ := NewSymbolCache(projectRoot, cacheFilePath)
		absPath1 := filepath.Join(projectRoot, "src/file1.go")
		absPath2 := filepath.Join(projectRoot, "pkg/file2.go")
		_ = os.MkdirAll(filepath.Dir(absPath1), 0755) // Ensure dir exists for makeRelative
		_ = os.MkdirAll(filepath.Dir(absPath2), 0755) // Ensure dir exists for makeRelative

		// Set symbols
		err := scWrite.SetSymbol("key1", absPath1)
		if err != nil {
			t.Fatalf("SetSymbol for key1 error: %v", err)
		}
		err = scWrite.SetSymbol("key2", absPath2)
		if err != nil {
			t.Fatalf("SetSymbol for key2 error: %v", err)
		}

		// Set file metadata
		meta1 := FileMetadata{Symbols: []string{"SymbolA", "SymbolB"}}
		// ModTime removed, no need to set it.
		err = scWrite.SetFileMetadata(absPath1, meta1)
		if err != nil {
			t.Fatalf("SetFileMetadata for absPath1 error: %v", err)
		}
		meta2 := FileMetadata{Symbols: []string{"SymbolC"}}
		err = scWrite.SetFileMetadata(absPath2, meta2)
		if err != nil {
			t.Fatalf("SetFileMetadata for absPath2 error: %v", err)
		}

		err = scWrite.Save()
		if err != nil {
			t.Fatalf("Save() error: %v", err)
		}

		// Verify file content
		data, _ := os.ReadFile(cacheFilePath)
		var loadedContent CacheContent
		json.Unmarshal(data, &loadedContent)

		expectedRelPath1 := "src/file1.go"
		if runtime.GOOS == "windows" {
			expectedRelPath1 = "src\\file1.go"
		}

		if loadedContent.Symbols["key1"] != filepath.ToSlash("src/file1.go") {
			t.Errorf("Expected key1 path 'src/file1.go', got '%s'", loadedContent.Symbols["key1"])
		}
		if loadedContent.Files[filepath.ToSlash(expectedRelPath1)].Symbols == nil || len(loadedContent.Files[filepath.ToSlash(expectedRelPath1)].Symbols) != 2 {
			t.Errorf("Expected file1 metadata to have 2 symbols, got %v", loadedContent.Files[filepath.ToSlash(expectedRelPath1)].Symbols)
		}

		scRead, _ := NewSymbolCache(projectRoot, cacheFilePath)
		err = scRead.Load(context.Background())
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if len(scRead.content.Symbols) != 2 {
			t.Fatalf("Expected 2 items in Symbols cache, got %d", len(scRead.content.Symbols))
		}
		if len(scRead.content.Files) != 2 {
			t.Fatalf("Expected 2 items in Files cache, got %d", len(scRead.content.Files))
		}

		val1, ok1 := scRead.Get("key1")
		if !ok1 || val1 != absPath1 {
			t.Errorf("Expected key1 to be %s, got %s (found: %v)", absPath1, val1, ok1)
		}
		// Check FileMetadata for absPath1
		relPath1, _ := scRead.makeRelative(absPath1)
		fileMeta1, metaOk1 := scRead.content.Files[relPath1]
		if !metaOk1 {
			t.Errorf("Expected FileMetadata for %s not found", relPath1)
		} else if len(fileMeta1.Symbols) != 2 || fileMeta1.Symbols[0] != "SymbolA" {
			t.Errorf("Expected FileMetadata for %s to contain [SymbolA, SymbolB], got %v", relPath1, fileMeta1.Symbols)
		}

		val2, ok2 := scRead.Get("key2")
		expectedPath2 := filepath.Join(projectRoot, filepath.FromSlash("pkg/file2.go"))
		if !ok2 || val2 != expectedPath2 {
			t.Errorf("Expected key2 to be %s, got %s (found: %v)", expectedPath2, val2, ok2)
		}
		relPath2, _ := scRead.makeRelative(absPath2)
		fileMeta2, metaOk2 := scRead.content.Files[relPath2]
		if !metaOk2 {
			t.Errorf("Expected FileMetadata for %s not found", relPath2)
		} else if len(fileMeta2.Symbols) != 1 || fileMeta2.Symbols[0] != "SymbolC" {
			t.Errorf("Expected FileMetadata for %s to contain [SymbolC], got %v", relPath2, fileMeta2.Symbols)
		}
	})

	t.Run("Load_corrupted_json", func(t *testing.T) {
		err := os.WriteFile(cacheFilePath, []byte("this is not json"), 0644)
		if err != nil {
			t.Fatalf("Failed to write corrupted file: %v", err)
		}

		sc, _ := NewSymbolCache(projectRoot, cacheFilePath)
		loadErr := sc.Load(context.Background())
		if loadErr != nil {
			t.Fatalf("Load() from corrupted file returned error %v, expected nil (and reset cache)", loadErr)
		}
		if len(sc.content.Symbols) != 0 { // Check new structure
			t.Errorf("Expected empty Symbols cache after loading corrupted file, got %d items", len(sc.content.Symbols))
		}
		if len(sc.content.Files) != 0 { // Check new structure
			t.Errorf("Expected empty Files cache after loading corrupted file, got %d items", len(sc.content.Files))
		}

		// Set symbol and file metadata after corrupted load
		absPathAfterCorrupt := filepath.Join(projectRoot, "file_after.go")
		_ = os.MkdirAll(filepath.Dir(absPathAfterCorrupt), 0755) // Ensure dir for makeRelative

		err = sc.SetSymbol("key_after_corrupt", absPathAfterCorrupt)
		if err != nil {
			t.Fatalf("SetSymbol after corrupt failed: %v", err)
		}

		metaAfterCorrupt := FileMetadata{Symbols: []string{"TestSymbol"}}
		err = sc.SetFileMetadata(absPathAfterCorrupt, metaAfterCorrupt)
		if err != nil {
			t.Fatalf("SetFileMetadata after corrupt failed: %v", err)
		}

		saveErr := sc.Save()
		if saveErr != nil {
			t.Fatalf("Save after corrupted load failed: %v", saveErr)
		}

		data, _ := os.ReadFile(cacheFilePath)
		var raw CacheContent // Check new structure
		json.Unmarshal(data, &raw)
		if _, ok := raw.Symbols["key_after_corrupt"]; !ok {
			t.Errorf("Cache (Symbols) not properly saved after loading corrupted file and setting new data.")
		}
		relPathAfterCorrupt, _ := sc.makeRelative(absPathAfterCorrupt)
		if _, ok := raw.Files[relPathAfterCorrupt]; !ok {
			t.Errorf("Cache (Files) not properly saved after loading corrupted file and setting new data.")
		}
	})

	t.Run("Save_empty_cache", func(t *testing.T) {
		if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
			// Ensure file exists with some old content to check if it's overwritten correctly
			oldContent := `{"symbols":{"oldkey":"oldvalue"},"files":{"oldfile":{"symbols":["OldSymbol"]}}}`
			os.WriteFile(cacheFilePath, []byte(oldContent), 0644)
		}

		sc, _ := NewSymbolCache(projectRoot, cacheFilePath)
		// Ensure cache is empty before saving
		sc.content.Symbols = make(map[string]string)
		sc.content.Files = make(map[string]FileMetadata)

		err := sc.Save()
		if err != nil {
			t.Fatalf("Save() empty cache error: %v", err)
		}
		content, err := os.ReadFile(cacheFilePath)
		if err != nil {
			t.Fatalf("Failed to read cache file after saving empty cache: %v", err)
		}
		// Expect '{"symbols":{},"files":{}}' or just '{}' if fields are omitempty and maps are nil.
		// Current SymbolCache.Save marshals even empty maps.
		// Normalize expected output based on actual MarshalIndent output for empty CacheContent
		emptyCacheContentForJSON := CacheContent{Symbols: make(map[string]string), Files: make(map[string]FileMetadata)} // Corrected: removed {} from make
		expectedBytes, err := json.MarshalIndent(emptyCacheContentForJSON, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal expected empty content: %v", err)
		}
		expectedJSON := string(expectedBytes)

		trimmedContent := strings.TrimSpace(string(content))

		if trimmedContent != expectedJSON {
			t.Errorf("Expected empty cache file to be '%s', got '%s'", expectedJSON, trimmedContent)
		}
	})

}

func TestSymbolCache_Set_Get_VerifyAndGet(t *testing.T) {
	cacheDir, cleanupCacheDir := tempDir(t)
	defer cleanupCacheDir()
	projectRoot := tempRootDir(t, cacheDir)

	sc, _ := NewSymbolCache(projectRoot, filepath.Join(cacheDir, "s_g_vg_cache.json"))

	symbolFullName := "my.pkg.SymbolName"
	symbolShortName := "SymbolName"
	absFilePath := filepath.Join(projectRoot, "path", "to", "symbol.go")
	relativeFilePath, _ := sc.makeRelative(absFilePath) // Store this for checks

	// Prepare the file system
	err := os.MkdirAll(filepath.Dir(absFilePath), 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	err = os.WriteFile(absFilePath, []byte("package main"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Set symbol and its file metadata
	err = sc.SetSymbol(symbolFullName, absFilePath)
	if err != nil {
		t.Fatalf("SetSymbol() error: %v", err)
	}

	// SetFileMetadata would typically be called by a higher-level component after scanning the file.
	// For this test, we set it manually.
	err = sc.SetFileMetadata(absFilePath, FileMetadata{Symbols: []string{symbolShortName}})
	if err != nil {
		t.Fatalf("SetFileMetadata() error: %v", err)
	}

	t.Run("SetSymbol_and_Get_existing_file", func(t *testing.T) {
		internalPath := sc.content.Symbols[symbolFullName]
		if internalPath != relativeFilePath { // Check against pre-calculated relative path
			t.Errorf("Expected internal path to be %s, got %s", relativeFilePath, internalPath)
		}

		retPath, found := sc.Get(symbolFullName)
		if !found {
			t.Fatalf("Get() should find key %s", symbolFullName)
		}
		if retPath != absFilePath {
			t.Errorf("Get() expected path %s, got %s", absFilePath, retPath)
		}
	})

	t.Run("VerifyAndGet_existing_file", func(t *testing.T) {
		retPath, found := sc.VerifyAndGet(context.Background(), symbolFullName)
		if !found {
			t.Fatalf("VerifyAndGet() should find key %s for existing file", symbolFullName)
		}
		if retPath != absFilePath {
			t.Errorf("VerifyAndGet() expected path %s, got %s", absFilePath, retPath)
		}
		if _, internalFound := sc.content.Symbols[symbolFullName]; !internalFound {
			t.Errorf("VerifyAndGet() should not remove entry from Symbols map for existing file")
		}
		if fileMeta, ok := sc.content.Files[relativeFilePath]; !ok || len(fileMeta.Symbols) == 0 || fileMeta.Symbols[0] != symbolShortName {
			t.Errorf("VerifyAndGet() should not alter FileMetadata.Symbols for existing file. Got: %v", fileMeta)
		}
	})

	t.Run("VerifyAndGet_non_existent_file", func(t *testing.T) {
		os.Remove(absFilePath) // File is now deleted

		retPath, found := sc.VerifyAndGet(context.Background(), symbolFullName)
		if found {
			t.Errorf("VerifyAndGet() should not find key %s for non-existent file, but got path %s", symbolFullName, retPath)
		}
		// Check if symbol was removed from sc.content.Symbols
		if _, internalFound := sc.content.Symbols[symbolFullName]; internalFound {
			t.Errorf("VerifyAndGet() should remove entry from Symbols map for non-existent file")
		}
		// Check if symbol was removed from sc.content.Files[relativeFilePath].Symbols
		if fileMeta, ok := sc.content.Files[relativeFilePath]; ok {
			foundInFileMeta := false
			for _, sym := range fileMeta.Symbols {
				if sym == symbolShortName {
					foundInFileMeta = true
					break
				}
			}
			if foundInFileMeta {
				t.Errorf("VerifyAndGet() should remove symbol '%s' from FileMetadata.Symbols for non-existent file. Still found in %v", symbolShortName, fileMeta.Symbols)
			}
			// Optionally, if FileMetadata.Symbols becomes empty, the file entry itself could be removed.
			// Current VerifyAndGet does not remove the file entry, only the specific symbol from its list.
			// if len(fileMeta.Symbols) == 0 { // if it was the only symbol
			//    if _, fileEntryExists := sc.content.Files[relativeFilePath]; fileEntryExists {
			//        t.Errorf("VerifyAndGet() could optionally remove FileMetadata entry if Symbols list is empty")
			//    }
			// }
		} else {
			// If the file entry itself was removed, that's also acceptable if VerifyAndGet is designed that way.
			// For now, we assume it only modifies the symbol list within the existing FileMetadata.
		}
	})

	t.Run("Get_non_existent_key", func(t *testing.T) {
		_, found := sc.Get("nonexistent.key")
		if found {
			t.Error("Get() should not find non-existent key")
		}
	})

	t.Run("SetSymbol_path_not_in_project_root", func(t *testing.T) {
		absExternalPath := "/abs/external/path/file.go"
		if runtime.GOOS == "windows" {
			absExternalPath = "X:\\external_path\\file.go"
		}

		err := sc.SetSymbol("external.key", absExternalPath)
		if err == nil {
			t.Errorf("SetSymbol() with external path '%s' (root: '%s') should have returned an error.", absExternalPath, sc.RootDir())
		}
		if err != nil { // If errored as expected
			if _, found := sc.content.Symbols["external.key"]; found {
				t.Error("SetSymbol() errored for external path but still set the key in Symbols map.")
			}
		}
	})
}

func TestSymbolCache_Disabled_When_Path_Is_Empty(t *testing.T) {
	cacheDir, cleanupCacheDir := tempDir(t)
	defer cleanupCacheDir()
	projectRoot := tempRootDir(t, cacheDir)
	cacheFilePathForDummy := filepath.Join(cacheDir, "disabled_cache_test.json")

	sc, _ := NewSymbolCache(projectRoot, "") // Empty path to disable cache

	absFilePath := filepath.Join(projectRoot, "file.go")
	err := os.MkdirAll(filepath.Dir(absFilePath), 0755)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = os.WriteFile(absFilePath, []byte("content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Test SetSymbol on disabled cache
	if err := sc.SetSymbol("key1", absFilePath); err != nil {
		t.Errorf("SetSymbol() on disabled cache should not error, got %v", err)
	}
	if len(sc.content.Symbols) != 0 {
		t.Error("SetSymbol() on disabled cache should not populate Symbols map")
	}

	// Test SetFileMetadata on disabled cache
	meta := FileMetadata{Symbols: []string{"SomeSymbol"}}
	if err := sc.SetFileMetadata(absFilePath, meta); err != nil {
		t.Errorf("SetFileMetadata() on disabled cache should not error, got %v", err)
	}
	if len(sc.content.Files) != 0 {
		t.Error("SetFileMetadata() on disabled cache should not populate Files map")
	}

	if _, found := sc.Get("key1"); found {
		t.Error("Get() on disabled cache should not find data")
	}
	if _, found := sc.VerifyAndGet(context.Background(), "key1"); found {
		t.Error("VerifyAndGet() on disabled cache should not find data")
	}
	if err := sc.Load(context.Background()); err != nil {
		t.Errorf("Load() on disabled cache should not error, got %v", err)
	}

	// Check Save does not modify an unrelated file
	os.WriteFile(cacheFilePathForDummy, []byte(`{"dummykey":"dummyval"}`), 0644)
	if err := sc.Save(); err != nil {
		t.Errorf("Save() on disabled cache should not error, got %v", err)
	}
	content, _ := os.ReadFile(cacheFilePathForDummy)
	if string(content) != `{"dummykey":"dummyval"}` {
		t.Error("Save() on disabled cache should not have modified the dummy file")
	}
	os.Remove(cacheFilePathForDummy)
}

func TestSymbolCache_PathNormalization(t *testing.T) {
	cacheDir, cleanupCacheDir := tempDir(t)
	defer cleanupCacheDir()

	// Define projectRoot using platform-agnostic joins for consistency
	projectRoot := filepath.Join(cacheDir, "my", "project", "root")
	err := os.MkdirAll(projectRoot, 0755)
	if err != nil {
		t.Fatalf("MkdirAll for projectRoot failed: %v", err)
	}

	sc, _ := NewSymbolCache(projectRoot, filepath.Join(cacheDir, "normalization_cache.json"))

	// Path with mixed separators for a file within the project root
	absFilePathMixed := filepath.Join(projectRoot, "src\\app/models", "user.go")
	err = os.MkdirAll(filepath.Dir(absFilePathMixed), 0755)
	if err != nil {
		t.Fatalf("MkdirAll for absFilePathMixed failed: %v", err)
	}
	err = os.WriteFile(absFilePathMixed, []byte("package models"), 0644)
	if err != nil {
		t.Fatalf("WriteFile for absFilePathMixed failed: %v", err)
	}

	err = sc.SetSymbol("user.Model", absFilePathMixed)
	if err != nil {
		t.Fatalf("SetSymbol() error: %v", err)
	}

	// Stored paths should always use forward slashes, as makeRelative uses filepath.ToSlash.
	expectedRelativeStoredPath := "src/app/models/user.go" // Universal forward slashes for storage.
	internalPath := sc.content.Symbols["user.Model"]
	if internalPath != expectedRelativeStoredPath {
		t.Errorf("Expected internally stored path to be '%s' (using forward slashes), got '%s'", expectedRelativeStoredPath, internalPath)
	}

	retPath, found := sc.Get("user.Model")
	if !found {
		t.Fatal("Get() failed to find the key 'user.Model'")
	}

	// For comparison, construct the expected absolute path using the OS-specific separator.
	// filepath.Join will use the correct separator for the current OS.
	// projectRoot is already OS-specific. expectedRelativeStoredPath uses forward slashes.
	// To correctly join, we can split the relative path and join its components.
	// However, `filepath.Join(projectRoot, filepath.FromSlash(expectedRelativeStoredPath))` is simpler.
	expectedAbsPath := filepath.Join(projectRoot, filepath.FromSlash(expectedRelativeStoredPath))

	// Normalize both paths for a robust comparison, cleaning up any redundant separators or dots.
	cleanedRetPath, _ := filepath.Abs(filepath.Clean(retPath))
	cleanedExpectedAbsPath, _ := filepath.Abs(filepath.Clean(expectedAbsPath))

	if cleanedRetPath != cleanedExpectedAbsPath {
		t.Errorf("Get() returned path '%s' (cleaned: '%s'), expected '%s' (cleaned: '%s')",
			retPath, cleanedRetPath, expectedAbsPath, cleanedExpectedAbsPath)
	}

	// Also test SetFileMetadata with mixed path
	meta := FileMetadata{Symbols: []string{"User"}}
	err = sc.SetFileMetadata(absFilePathMixed, meta)
	if err != nil {
		t.Fatalf("SetFileMetadata() with mixed path error: %v", err)
	}
	if _, ok := sc.content.Files[expectedRelativeStoredPath]; !ok {
		t.Errorf("FileMetadata not stored under normalized path '%s' after SetFileMetadata with mixed path. Found: %v", expectedRelativeStoredPath, sc.content.Files)
	}
}

func TestSymbolCache_RootDir(t *testing.T) {
	expectedRootDir := "/tmp/myproject"
	if runtime.GOOS == "windows" {
		// Use a path that's more likely to be valid on Windows for testing purposes,
		// though this specific test doesn't create files/dirs with this root.
		expectedRootDir = "C:\\temp\\myproject"
	}
	// This test is for RootDir(), cache path itself doesn't matter for what RootDir returns.
	// Provide a dummy cache path to enable the cache for the purpose of construction.
	sc, err := NewSymbolCache(expectedRootDir, filepath.Join(os.TempDir(), "dummy_cache_for_rootdir_test.json"))
	if err != nil {
		t.Fatalf("NewSymbolCache failed: %v", err)
	}
	if sc.RootDir() != expectedRootDir {
		t.Errorf("sc.RootDir() was %s, expected %s", sc.RootDir(), expectedRootDir)
	}
}

func TestSymbolCache_FilePath(t *testing.T) {
	expectedFilePath := "/tmp/cachefile.json"
	if runtime.GOOS == "windows" {
		expectedFilePath = "C:\\temp\\cachefile.json"
	}
	// Provide a plausible rootDir for Windows as well
	projDir := "/tmp/proj"
	if runtime.GOOS == "windows" {
		projDir = "C:\\temp\\proj"
	}
	sc, err := NewSymbolCache(projDir, expectedFilePath)
	if err != nil {
		t.Fatalf("NewSymbolCache failed: %v", err)
	}
	if sc.FilePath() != expectedFilePath {
		t.Errorf("sc.FilePath() was %s, expected %s", sc.FilePath(), expectedFilePath)
	}
}

func TestSymbolCache_Set_EmptyRootDir(t *testing.T) {
	tempFile, err := os.CreateTemp("", "empty_root_cache_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file for cache path: %v", err)
	}
	cachePath := tempFile.Name()
	tempFile.Close() // Close it as SymbolCache will open/write it
	defer os.Remove(cachePath)

	sc, err := NewSymbolCache("", cachePath)
	if err != nil {
		t.Fatalf("NewSymbolCache with empty rootDir and explicit cachePath failed: %v", err)
	}

	absPath := "/some/absolute/path.go"
	if runtime.GOOS == "windows" {
		absPath = "C:\\windows\\system32\\somefile.go"
	}

	setError := sc.SetSymbol("key.empty.root", absPath) // Changed from Set to SetSymbol
	if setError == nil {
		t.Errorf("SetSymbol() with empty rootDir should have returned an error for absolute path %s.", absPath)
		storedPath, found := sc.content.Symbols["key.empty.root"] // Corrected to sc.content.Symbols
		if !found || storedPath != filepath.ToSlash(absPath) {
			t.Errorf("Expected SetSymbol with empty rootDir to store absolute path '%s', got '%s'", filepath.ToSlash(absPath), storedPath)
		}
	} else {
		if _, found := sc.content.Symbols["key.empty.root"]; found { // Corrected to sc.content.Symbols
			t.Errorf("SetSymbol() with empty rootDir errored but still stored data: %v", sc.content.Symbols["key.empty.root"])
		}
	}

	_, foundGet := sc.Get("key.empty.root")
	if foundGet {
		t.Error("Get() found key that should not have been set due to empty rootDir error during SetSymbol.")
	}
}

func TestSymbolCache_GetFilesToScan(t *testing.T) {
	baseDir, cleanupBaseDir := tempDir(t)
	defer cleanupBaseDir()

	projectRoot := tempRootDir(t, baseDir) // Creates baseDir/project_root
	cacheFilePath := filepath.Join(baseDir, "getfilestoscan_cache.json")
	sc, _ := NewSymbolCache(projectRoot, cacheFilePath)

	pkg1Path := filepath.Join(projectRoot, "pkg1")
	pkg2Path := filepath.Join(projectRoot, "pkg2")
	os.MkdirAll(pkg1Path, 0755)
	os.MkdirAll(pkg2Path, 0755)

	// Helper to create a file and update its metadata in cache
	createFileAndCache := func(pkgPath, fileName string, symbols []string) string {
		t.Helper()
		absFilePath := filepath.Join(pkgPath, fileName)
		err := os.WriteFile(absFilePath, []byte("package main"), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", absFilePath, err)
		}
		// Manually add to cache for setup
		meta := FileMetadata{Symbols: symbols}
		relPath, _ := sc.makeRelative(absFilePath)
		sc.content.Files[relPath] = meta
		for _, symName := range symbols {
			sc.content.Symbols[filepath.Base(pkgPath)+"."+symName] = relPath
		}
		return absFilePath
	}

	// Helper to check slice equality ignoring order
	slicesEqualIgnoringOrder := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		m := make(map[string]int)
		for _, x := range a {
			m[x]++
		}
		for _, x := range b {
			m[x]--
		}
		for _, count := range m {
			if count != 0 {
				return false
			}
		}
		return true
	}

	// Scenario 1: New files only for pkg1
	file1a := filepath.Join(pkg1Path, "file1a.go")
	os.WriteFile(file1a, []byte("package pkg1"), 0644)
	file1b := filepath.Join(pkg1Path, "file1b.go")
	os.WriteFile(file1b, []byte("package pkg1"), 0644)

	newFiles, existingFiles, err := sc.GetFilesToScan(context.Background(), pkg1Path)
	if err != nil {
		t.Fatalf("GetFilesToScan (new only) failed: %v", err)
	}
	if !slicesEqualIgnoringOrder(newFiles, []string{file1a, file1b}) {
		t.Errorf("Expected new files %v, got %v", []string{file1a, file1b}, newFiles)
	}
	if len(existingFiles) != 0 {
		t.Errorf("Expected no existing files, got %v", existingFiles)
	}

	// Manually populate cache for next scenarios based on these files
	createFileAndCache(pkg1Path, "file1a.go", []string{"SymbolA"}) // Re-create to control cache state
	createFileAndCache(pkg1Path, "file1b.go", []string{"SymbolB"})

	// Scenario 2: Cached files only for pkg1
	newFiles, existingFiles, err = sc.GetFilesToScan(context.Background(), pkg1Path)
	if err != nil {
		t.Fatalf("GetFilesToScan (cached only) failed: %v", err)
	}
	if len(newFiles) != 0 {
		t.Errorf("Expected no new files, got %v", newFiles)
	}
	if !slicesEqualIgnoringOrder(existingFiles, []string{file1a, file1b}) {
		t.Errorf("Expected existing files %v, got %v", []string{file1a, file1b}, existingFiles)
	}

	// Scenario 3: Mixed new and cached for pkg1
	file1c_abs := filepath.Join(pkg1Path, "file1c.go") // New file
	os.WriteFile(file1c_abs, []byte("package pkg1"), 0644)

	newFiles, existingFiles, err = sc.GetFilesToScan(context.Background(), pkg1Path)
	if err != nil {
		t.Fatalf("GetFilesToScan (mixed) failed: %v", err)
	}
	if !slicesEqualIgnoringOrder(newFiles, []string{file1c_abs}) {
		t.Errorf("Expected new files %v, got %v", []string{file1c_abs}, newFiles)
	}
	if !slicesEqualIgnoringOrder(existingFiles, []string{file1a, file1b}) { // file1a, file1b are from createFileAndCache
		t.Errorf("Expected existing files %v, got %v", []string{file1a, file1b}, existingFiles)
	}
	createFileAndCache(pkg1Path, "file1c.go", []string{"SymbolC"}) // Add file1c to cache for next test

	// Scenario 4: File deleted from pkg1
	os.Remove(file1b) // Delete file1b
	relPathFile1b, _ := sc.makeRelative(file1b)

	newFiles, existingFiles, err = sc.GetFilesToScan(context.Background(), pkg1Path)
	if err != nil {
		t.Fatalf("GetFilesToScan (deleted) failed: %v", err)
	}
	if len(newFiles) != 0 { // No new files added, only one deleted
		t.Errorf("Expected no new files after deletion, got %v", newFiles)
	}
	// Expected existing are file1a and file1c
	expectedExistingAfterDelete := []string{file1a, file1c_abs}
	if !slicesEqualIgnoringOrder(existingFiles, expectedExistingAfterDelete) {
		t.Errorf("Expected existing files %v after deletion, got %v", expectedExistingAfterDelete, existingFiles)
	}
	// Check if file1b is removed from cache
	if _, ok := sc.content.Files[relPathFile1b]; ok {
		t.Errorf("FileMetadata for deleted file %s not removed from cache", relPathFile1b)
	}
	if _, ok := sc.content.Symbols["pkg1.SymbolB"]; ok { // Assuming symbol name was PkgName.SymbolName
		t.Errorf("Symbol 'SymbolB' for deleted file %s not removed from symbol cache", relPathFile1b)
	}

	// Scenario 5: Ensure pkg2 is not affected by pkg1 scan
	// Setup pkg2 with one cached file
	file2a_abs := createFileAndCache(pkg2Path, "file2a.go", []string{"SymbolPkg2A"})
	relPathFile2a, _ := sc.makeRelative(file2a_abs)

	// Call GetFilesToScan for pkg1 again (state of pkg1 dir is file1a, file1c)
	sc.GetFilesToScan(context.Background(), pkg1Path)
	// Check if pkg2's cache entry is still intact
	if _, ok := sc.content.Files[relPathFile2a]; !ok {
		t.Errorf("FileMetadata for pkg2 file %s was removed after scanning pkg1", relPathFile2a)
	}
	if _, ok := sc.content.Symbols["pkg2.SymbolPkg2A"]; !ok {
		t.Errorf("Symbol 'SymbolPkg2A' for pkg2 file %s was removed after scanning pkg1", relPathFile2a)
	}
}
