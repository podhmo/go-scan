package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings" // Added
	"sync"
)

const (
	defaultCacheDirName  = ".go-typescanner"
	defaultCacheFileName = "cache.json"
)

// SymbolCache manages the symbol definition cache.
// It is responsible for loading, saving, and providing access to cached symbol locations.
type SymbolCache struct {
	mu        sync.RWMutex
	cacheData map[string]string // Key: "<pkg_path>.<symbol_name>", Value: "filepath"
	filePath  string
	useCache  bool
	rootDir   string // Project root directory, used to make filepaths relative
}

// NewSymbolCache creates a new SymbolCache.
//
// rootDir is the project's root directory. Filepaths in the cache will be relative to this directory.
// configCachePath is the user-configured path for the cache file. If empty, caching is disabled.
func NewSymbolCache(rootDir string, configCachePath string) (*SymbolCache, error) { // Removed configUseCache
	sc := &SymbolCache{
		cacheData: make(map[string]string),
		rootDir:   rootDir,
		// filePath and useCache will be set based on configCachePath
	}

	if configCachePath == "" {
		sc.useCache = false
		sc.filePath = "" // Ensure filePath is empty if no path is provided
		return sc, nil   // Caching is disabled, no further setup needed for path
	}

	// If configCachePath is provided, caching is considered enabled
	sc.useCache = true
	sc.filePath = configCachePath

	return sc, nil
}

// Load loads the cache data from the file if UseCache is true.
// If the file does not exist, it's not an error; an empty cache will be used.
func (sc *SymbolCache) Load() error {
	if !sc.useCache || sc.filePath == "" {
		return nil // Do nothing if cache is disabled or path is not set
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	data, err := os.ReadFile(sc.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			sc.cacheData = make(map[string]string) // Ensure fresh map if file not found
			return nil                             // File not existing is not an error, just means no cache yet
		}
		return fmt.Errorf("failed to read cache file %s: %w", sc.filePath, err)
	}

	if len(data) == 0 { // Handle empty file case
		sc.cacheData = make(map[string]string)
		return nil
	}

	err = json.Unmarshal(data, &sc.cacheData)
	if err != nil {
		// If unmarshalling fails, treat it as a corrupted cache and start fresh
		// Log this event ideally, but for now, just reset.
		fmt.Fprintf(os.Stderr, "warning: failed to unmarshal cache file %s, starting with an empty cache: %v\n", sc.filePath, err)
		sc.cacheData = make(map[string]string)
		return nil // Return nil to allow the program to continue with an empty cache
	}
	return nil
}

// Save saves the current cache data to the file if UseCache is true.
func (sc *SymbolCache) Save() error {
	if !sc.useCache || sc.filePath == "" {
		return nil // Do nothing if cache is disabled or path is not set
	}

	sc.mu.RLock()
	// If cacheData is empty, no need to save, but we might want to ensure the file is also empty or non-existent.
	// For now, only write if there's something to write, to avoid unnecessary I/O or empty "{}" files.
	if len(sc.cacheData) == 0 {
		// Let's consider if we should remove the file if the cache is empty.
		// For now, if the file exists and cache becomes empty, it will remain with old data unless explicitly cleared or overwritten.
		// This behavior might be fine, or we might want to write an empty JSON object {} or delete the file.
		// To ensure the file reflects the empty state, we should proceed to write.
		// Let's remove this early exit.
		// defer sc.mu.RUnlock() // Original position
		// return nil
	}
	// data, err := json.MarshalIndent(sc.cacheData, "", "  ") // Original position

	// To handle saving an empty cache as "{}", we need to marshal even if len is 0.
	currentCacheData := make(map[string]string)
	for k, v := range sc.cacheData {
		currentCacheData[k] = v
	}
	sc.mu.RUnlock() // Unlock RLock before potential write operations (MkdirAll, WriteFile)

	data, err := json.MarshalIndent(currentCacheData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	// Ensure the directory exists
	dir := filepath.Dir(sc.filePath)
	// Check if directory exists before trying to create it
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		// MkdirAll also handles the case where the path already exists as a directory
		err = os.MkdirAll(dir, 0750) // Read/write/execute for user, read/execute for group
		if err != nil {
			return fmt.Errorf("failed to create cache directory %s: %w", dir, err)
		}
	} else if statErr != nil {
		// Some other error occurred when stating the directory
		return fmt.Errorf("failed to stat cache directory %s: %w", dir, statErr)
	}

	err = os.WriteFile(sc.filePath, data, 0640) // Read/write for user, read for group
	if err != nil {
		return fmt.Errorf("failed to write cache file %s: %w", sc.filePath, err)
	}
	return nil
}

// Get retrieves a filepath for a given symbol key.
// The key is typically "<package_import_path>.<SymbolName>".
// Returns the filepath and true if found, otherwise an empty string and false.
// The returned filepath is an absolute path.
func (sc *SymbolCache) Get(key string) (string, bool) {
	if !sc.useCache {
		return "", false
	}

	sc.mu.RLock()
	defer sc.mu.RUnlock()

	relativePath, found := sc.cacheData[key]
	if !found {
		return "", false
	}
	// Cache stores relative paths, convert to absolute before returning
	if filepath.IsAbs(relativePath) { // Should not happen if saved correctly
		// This might indicate an old cache format or an error in Set.
		// For robustness, return it as is, but ideally paths are relative.
		return relativePath, true
	}
	return filepath.Join(sc.rootDir, relativePath), true
}

// Set stores a filepath for a given symbol key.
// The key is typically "<package_import_path>.<SymbolName>".
// The filepath should be an absolute path; it will be converted to relative before storing.
func (sc *SymbolCache) Set(key string, absoluteFilepath string) error {
	if !sc.useCache {
		return nil
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.rootDir == "" {
		return fmt.Errorf("rootDir is empty in SymbolCache, cannot set key %s. SymbolCache must be initialized with a valid rootDir", key)
	}

	// Ensure absoluteFilepath is truly absolute before prefix check or Rel
	if !filepath.IsAbs(absoluteFilepath) {
		return fmt.Errorf("filepath to cache must be absolute, got %s for key %s", absoluteFilepath, key)
	}

	// Clean both paths to handle trailing slashes etc. for reliable prefix check
	cleanedRootDir := filepath.Clean(sc.rootDir)
	cleanedAbsFilepath := filepath.Clean(absoluteFilepath)

	// Check if the absoluteFilepath is under rootDir.
	// strings.HasPrefix is a simple way. Add path separator to avoid matching /root/a to /root/abc.
	// Ensure rootDir itself ends with a separator for prefix check, unless it's the root "/"
	rootDirPrefix := cleanedRootDir
	// Avoid adding double separator if rootDir is "/"
	if cleanedRootDir != string(filepath.Separator) && !strings.HasSuffix(cleanedRootDir, string(filepath.Separator)) {
		rootDirPrefix += string(filepath.Separator)
	}

	if !strings.HasPrefix(cleanedAbsFilepath, rootDirPrefix) && cleanedAbsFilepath != cleanedRootDir {
		// If cleanedAbsFilepath is exactly cleanedRootDir (e.g. caching the root dir itself, unlikely for a file)
		// it would also be valid.
		return fmt.Errorf("filepath %s is not within the configured rootDir %s for key %s", absoluteFilepath, sc.rootDir, key)
	}

	relativeFilepath, err := filepath.Rel(cleanedRootDir, cleanedAbsFilepath) // Use cleaned paths for Rel
	if err != nil {
		// This error should ideally not happen if the prefix check above is correct and robust,
		// but Rel can have edge cases.
		return fmt.Errorf("internal error: filepath.Rel failed for %s relative to %s for key %s: %w", cleanedAbsFilepath, cleanedRootDir, key, err)
	}

	sc.cacheData[key] = filepath.ToSlash(relativeFilepath)
	return nil
}

// VerifyAndGet checks if the symbol likely still exists at the cached path.
// It returns the absolute path if the file exists, otherwise it removes the
// stale entry from the cache and returns false.
// This is a basic check; it doesn't re-parse the file to confirm the symbol.
func (sc *SymbolCache) VerifyAndGet(key string) (string, bool) {
	if !sc.useCache {
		return "", false
	}

	// Get already handles sc.rootDir and returns an absolute path
	absolutePath, found := sc.Get(key)
	if !found {
		return "", false
	}

	// Check if the file still exists
	// Note: os.Stat returns an error if the path does not exist.
	if _, err := os.Stat(absolutePath); err != nil {
		// If os.Stat fails, the entry might be stale or the file inaccessible.
		// We should remove it from the cache regardless of the exact error,
		// as we can't rely on this cached path.
		sc.mu.Lock()
		delete(sc.cacheData, key)
		sc.mu.Unlock()

		// Log the reason for removal if it's not a simple NotExist error
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: removing cache entry for key %s due to error accessing %s: %v\n", key, absolutePath, err)
		}
		return "", false
	}

	return absolutePath, true
}

// RootDir returns the project root directory used by the cache.
func (sc *SymbolCache) RootDir() string {
	return sc.rootDir
}

// FilePath returns the path to the cache file.
func (sc *SymbolCache) FilePath() string {
	return sc.filePath
}

// IsEnabled returns true if the cache is configured to be used.
func (sc *SymbolCache) IsEnabled() bool {
	return sc.useCache
}
