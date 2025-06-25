package cache

import (
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

	t.Run("UseCache_false", func(t *testing.T) {
		sc, err := NewSymbolCache(projectRoot, "", false)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if sc.IsEnabled() {
			t.Errorf("Expected cache to be disabled")
		}
		if sc.FilePath() != "" {
			t.Errorf("Expected empty file path for disabled cache, got %s", sc.FilePath())
		}
	})

	t.Run("UseCache_true_default_path", func(t *testing.T) {
		// Test default path construction using the actual os.UserHomeDir()
		// This test is now less about mocking and more about correct composition.
		actualHomeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get actual user home directory: %v", err)
		}

		sc, err := NewSymbolCache(projectRoot, "", true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !sc.IsEnabled() {
			t.Errorf("Expected cache to be enabled")
		}
		expectedPath := filepath.Join(actualHomeDir, defaultCacheDirName, defaultCacheFileName)
		if sc.FilePath() != expectedPath {
			t.Errorf("Expected default path %s, got %s", expectedPath, sc.FilePath())
		}
	})

	t.Run("UseCache_true_custom_path", func(t *testing.T) {
		customPath := filepath.Join(projectRoot, "custom_cache.json")
		sc, err := NewSymbolCache(projectRoot, customPath, true)
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
		sc, _ := NewSymbolCache(projectRoot, cacheFilePath, true)
		err := sc.Load()
		if err != nil {
			t.Fatalf("Load() from non-existent file should not error, got %v", err)
		}
		if len(sc.cacheData) != 0 {
			t.Errorf("Expected empty cacheData, got %d items", len(sc.cacheData))
		}
	})

	t.Run("Save_and_Load_data", func(t *testing.T) {
		scWrite, _ := NewSymbolCache(projectRoot, cacheFilePath, true)
		absPath1 := filepath.Join(projectRoot, "src/file1.go")
		absPath2 := filepath.Join(projectRoot, "pkg/file2.go")

		err := scWrite.Set("key1", absPath1)
		if err != nil {
			t.Fatalf("Set error: %v", err)
		}
		err = scWrite.Set("key2", absPath2)
		if err != nil {
			t.Fatalf("Set error: %v", err)
		}

		err = scWrite.Save()
		if err != nil {
			t.Fatalf("Save() error: %v", err)
		}

		// Verify file content (optional, but good for sanity)
		data, _ := os.ReadFile(cacheFilePath)
		var raw map[string]string
		json.Unmarshal(data, &raw)
		if raw["key1"] != "src/file1.go" && raw["key1"] != "src\\file1.go" { // Handle path sep
			t.Errorf("Expected key1 path 'src/file1.go', got '%s'", raw["key1"])
		}

		scRead, _ := NewSymbolCache(projectRoot, cacheFilePath, true)
		err = scRead.Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if len(scRead.cacheData) != 2 {
			t.Fatalf("Expected 2 items in cache, got %d", len(scRead.cacheData))
		}
		val1, ok1 := scRead.Get("key1")
		if !ok1 || val1 != absPath1 {
			t.Errorf("Expected key1 to be %s, got %s (found: %v)", absPath1, val1, ok1)
		}
		val2, ok2 := scRead.Get("key2")
		// Normalize for comparison if needed, though Join should be consistent
		expectedPath2 := filepath.Join(projectRoot, filepath.FromSlash("pkg/file2.go"))
		if !ok2 || val2 != expectedPath2 {
			t.Errorf("Expected key2 to be %s, got %s (found: %v)", expectedPath2, val2, ok2)
		}
	})

	t.Run("Load_corrupted_json", func(t *testing.T) {
		err := os.WriteFile(cacheFilePath, []byte("this is not json"), 0644)
		if err != nil {
			t.Fatalf("Failed to write corrupted file: %v", err)
		}

		sc, _ := NewSymbolCache(projectRoot, cacheFilePath, true)
		loadErr := sc.Load()
		if loadErr != nil {
			// Load itself now returns nil on unmarshal error, and prints to stderr
			t.Fatalf("Load() from corrupted file returned error %v, expected nil (and reset cache)", loadErr)
		}
		if len(sc.cacheData) != 0 {
			t.Errorf("Expected empty cacheData after loading corrupted file, got %d items", len(sc.cacheData))
		}

		sc.Set("key_after_corrupt", filepath.Join(projectRoot, "file_after.go"))
		saveErr := sc.Save()
		if saveErr != nil {
			t.Fatalf("Save after corrupted load failed: %v", saveErr)
		}

		data, _ := os.ReadFile(cacheFilePath)
		var raw map[string]string
		json.Unmarshal(data, &raw)
		if _, ok := raw["key_after_corrupt"]; !ok {
			t.Errorf("Cache not properly saved after loading corrupted file and setting new data.")
		}
	})

	t.Run("Save_empty_cache", func(t *testing.T) {
		if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
			os.WriteFile(cacheFilePath, []byte(`{"oldkey":"oldvalue"}`), 0644)
		}

		sc, _ := NewSymbolCache(projectRoot, cacheFilePath, true)
		err := sc.Save()
		if err != nil {
			t.Fatalf("Save() empty cache error: %v", err)
		}
		content, err := os.ReadFile(cacheFilePath)
		if err != nil {
			t.Fatalf("Failed to read cache file after saving empty cache: %v", err)
		}
		if strings.TrimSpace(string(content)) != "{}" {
			t.Errorf("Expected empty cache file to be '{}', got '%s'", string(content))
		}
	})

}

func TestSymbolCache_Set_Get_VerifyAndGet(t *testing.T) {
	cacheDir, cleanupCacheDir := tempDir(t)
	defer cleanupCacheDir()
	projectRoot := tempRootDir(t, cacheDir)

	sc, _ := NewSymbolCache(projectRoot, filepath.Join(cacheDir, "s_g_vg_cache.json"), true)

	key := "my.symbol.Key"
	absFilePath := filepath.Join(projectRoot, "path", "to", "symbol.go")
	relativeFilePath := filepath.Join("path", "to", "symbol.go")

	err := os.MkdirAll(filepath.Dir(absFilePath), 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	err = os.WriteFile(absFilePath, []byte("package main"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	t.Run("Set_and_Get_existing_file", func(t *testing.T) {
		err := sc.Set(key, absFilePath)
		if err != nil {
			t.Fatalf("Set() error: %v", err)
		}
		internalPath := sc.cacheData[key]
		if internalPath != filepath.ToSlash(relativeFilePath) {
			t.Errorf("Expected internal path to be %s, got %s", filepath.ToSlash(relativeFilePath), internalPath)
		}

		retPath, found := sc.Get(key)
		if !found {
			t.Fatalf("Get() should find key %s", key)
		}
		if retPath != absFilePath {
			t.Errorf("Get() expected path %s, got %s", absFilePath, retPath)
		}
	})

	t.Run("VerifyAndGet_existing_file", func(t *testing.T) {
		retPath, found := sc.VerifyAndGet(key)
		if !found {
			t.Fatalf("VerifyAndGet() should find key %s for existing file", key)
		}
		if retPath != absFilePath {
			t.Errorf("VerifyAndGet() expected path %s, got %s", absFilePath, retPath)
		}
		if _, internalFound := sc.cacheData[key]; !internalFound {
			t.Errorf("VerifyAndGet() should not remove entry for existing file")
		}
	})

	t.Run("VerifyAndGet_non_existent_file", func(t *testing.T) {
		os.Remove(absFilePath)

		retPath, found := sc.VerifyAndGet(key)
		if found {
			t.Errorf("VerifyAndGet() should not find key %s for non-existent file, but got path %s", key, retPath)
		}
		if _, internalFound := sc.cacheData[key]; internalFound {
			t.Errorf("VerifyAndGet() should remove entry for non-existent file")
		}
	})

	t.Run("Get_non_existent_key", func(t *testing.T) {
		_, found := sc.Get("nonexistent.key")
		if found {
			t.Error("Get() should not find non-existent key")
		}
	})

	t.Run("Set_path_not_in_project_root", func(t *testing.T) {
		// Use a path that is guaranteed to be outside the projectRoot for filepath.Rel to fail.
		// projectRoot is something like /tmp/cache_test_XXXX/project_root
		// absExternalPath needs to be something like /some_other_root/file.go
		// On Windows, this would be like C:\temp_root vs D:\other_file
		absExternalPath := "/abs/external/path/file.go"
		if runtime.GOOS == "windows" {
			// Assuming tests don't run on a system with only one drive or specific drive needs.
			// This might need adjustment if C: is where projectRoot lives.
			// A more robust way would be to find a different drive letter if possible.
			// Forcing a path that's unlikely to be relative to a temp dir on C:
			absExternalPath = "X:\\external_path\\file.go"
		}

		// Current Set returns error if path cannot be made relative to rootDir
		err := sc.Set("external.key", absExternalPath)
		if err == nil {
			t.Errorf("Set() with external path '%s' (root: '%s') should have returned an error.", absExternalPath, sc.RootDir())
		}
		if err != nil {
			if _, found := sc.cacheData["external.key"]; found {
				t.Error("Set() errored for external path but still set the key.")
			}
		}
	})
}

func TestSymbolCache_UseCache_False(t *testing.T) {
	cacheDir, cleanupCacheDir := tempDir(t)
	defer cleanupCacheDir()
	projectRoot := tempRootDir(t, cacheDir)
	cacheFilePath := filepath.Join(cacheDir, "disabled_cache_test.json")

	sc, _ := NewSymbolCache(projectRoot, cacheFilePath, false)

	absFilePath := filepath.Join(projectRoot, "file.go")
	err := os.MkdirAll(filepath.Dir(absFilePath), 0755)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = os.WriteFile(absFilePath, []byte("content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := sc.Set("key1", absFilePath); err != nil {
		t.Errorf("Set() on disabled cache should not error, got %v", err)
	}
	if len(sc.cacheData) != 0 {
		t.Error("Set() on disabled cache should not populate data")
	}

	if _, found := sc.Get("key1"); found {
		t.Error("Get() on disabled cache should not find data")
	}

	if _, found := sc.VerifyAndGet("key1"); found {
		t.Error("VerifyAndGet() on disabled cache should not find data")
	}

	if err := sc.Load(); err != nil {
		t.Errorf("Load() on disabled cache should not error, got %v", err)
	}

	os.WriteFile(cacheFilePath, []byte(`{"dummykey":"dummyval"}`), 0644)

	if err := sc.Save(); err != nil {
		t.Errorf("Save() on disabled cache should not error, got %v", err)
	}

	content, _ := os.ReadFile(cacheFilePath)
	if string(content) != `{"dummykey":"dummyval"}` {
		t.Error("Save() on disabled cache should not have modified the file")
	}
	os.Remove(cacheFilePath)
}

func TestSymbolCache_PathNormalization(t *testing.T) {
	cacheDir, cleanupCacheDir := tempDir(t)
	defer cleanupCacheDir()

	projectRoot := filepath.Join(cacheDir, "my/project\\root")
	err := os.MkdirAll(projectRoot, 0755)
	if err != nil {
		t.Fatalf("MkdirAll for projectRoot failed: %v", err)
	}

	sc, _ := NewSymbolCache(projectRoot, "", true)

	absFilePathMixed := filepath.Join(projectRoot, "src", "app", "models", "user.go") // Corrected
	err = os.MkdirAll(filepath.Dir(absFilePathMixed), 0755)
	if err != nil {
		t.Fatalf("MkdirAll for absFilePathMixed failed: %v", err)
	}
	err = os.WriteFile(absFilePathMixed, []byte("package models"), 0644)
	if err != nil {
		t.Fatalf("WriteFile for absFilePathMixed failed: %v", err)
	}

	err = sc.Set("user.Model", absFilePathMixed)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	expectedRelativeStoredPath := "src/app/models/user.go"
	internalPath := sc.cacheData["user.Model"]
	if internalPath != expectedRelativeStoredPath {
		t.Errorf("Expected internally stored path to be '%s', got '%s'", expectedRelativeStoredPath, internalPath)
	}

	retPath, found := sc.Get("user.Model")
	if !found {
		t.Fatal("Get() failed to find the key 'user.Model'")
	}
	normalizedAbsFilePathMixed, _ := filepath.Abs(absFilePathMixed)

	if retPath != normalizedAbsFilePathMixed {
		t.Errorf("Get() returned path '%s', expected '%s'", retPath, normalizedAbsFilePathMixed)
	}
}

func TestSymbolCache_RootDir(t *testing.T) {
	expectedRootDir := "/tmp/myproject"
	if runtime.GOOS == "windows" {
		// Use a path that's more likely to be valid on Windows for testing purposes,
		// though this specific test doesn't create files/dirs with this root.
		expectedRootDir = "C:\\temp\\myproject"
	}
	sc, err := NewSymbolCache(expectedRootDir, "", true)
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
	sc, err := NewSymbolCache(projDir, expectedFilePath, true)
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

	sc, err := NewSymbolCache("", cachePath, true)
	if err != nil {
		// This might error if default path construction fails when root is empty,
		// but here we provide an explicit cachePath.
		// NewSymbolCache itself doesn't use rootDir unless for default path construction.
		t.Fatalf("NewSymbolCache with empty rootDir and explicit cachePath failed: %v", err)
	}

	absPath := "/some/absolute/path.go"
	if runtime.GOOS == "windows" {
		absPath = "C:\\windows\\system32\\somefile.go" // Use a path that would exist for Stat checks if any were done by Set
		// For Set, it's more about path manipulation.
	}

	// Current implementation of Set returns an error if rootDir is empty,
	// because filepath.Rel("", absPath) returns an error.
	setError := sc.Set("key.empty.root", absPath)
	if setError == nil {
		t.Errorf("Set() with empty rootDir should have returned an error for absolute path %s.", absPath)
		// Check what was stored if it didn't error
		storedPath, found := sc.cacheData["key.empty.root"]
		if !found || storedPath != filepath.ToSlash(absPath) { // Expect absolute path stored as fallback
			t.Errorf("Expected Set with empty rootDir to store absolute path '%s', got '%s'", filepath.ToSlash(absPath), storedPath)
		}
	} else {
		// If it errored as expected, ensure nothing was stored.
		if _, found := sc.cacheData["key.empty.root"]; found {
			t.Errorf("Set() with empty rootDir errored but still stored data: %v", sc.cacheData["key.empty.root"])
		}
	}

	_, foundGet := sc.Get("key.empty.root")
	if foundGet {
		t.Error("Get() found key that should not have been set due to empty rootDir error during Set.")
	}
}
