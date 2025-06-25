package goscan

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/podhmo/go-scan/cache"
	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Re-export scanner kinds for convenience.
const (
	StructKind = scanner.StructKind
	AliasKind  = scanner.AliasKind
	FuncKind   = scanner.FuncKind
)

// Scanner is the main entry point for the type scanning library.
// It combines a locator for finding packages and a scanner for parsing them.
type Scanner struct {
	locator      *locator.Locator
	scanner      *scanner.Scanner
	packageCache map[string]*scanner.PackageInfo // This is for package AST, not symbol definitions
	mu           sync.RWMutex

	// CachePath is the path to the symbol cache file.
	// If empty, caching will be disabled. Otherwise, this path will be used.
	CachePath string
	// symbolCache *cache.SymbolCache // Added - Note: UseCache field was removed
	symbolCache *cache.SymbolCache
}

// New creates a new Scanner. It finds the module root starting from the given path.
func New(startPath string) (*Scanner, error) {
	loc, err := locator.New(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize locator: %w", err)
	}

	return &Scanner{
		locator:      loc,
		scanner:      scanner.New(),
		packageCache: make(map[string]*scanner.PackageInfo),
		// CachePath is initialized to its zero value "" (empty string), meaning cache disabled by default.
		// symbolCache will be initialized by getOrCreateSymbolCache when/if needed.
	}, nil
}

// ScanPackage scans a single package at a given directory path.
// The path should be relative to the project root or an absolute path.
func (s *Scanner) ScanPackage(pkgPath string) (*scanner.PackageInfo, error) {
	info, err := os.Stat(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not stat path %s: %w", pkgPath, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path %s is not a directory", pkgPath)
	}

	return s.scanner.ScanPackage(pkgPath, s)
}

// ScanPackageByImport scans a single package using its Go import path.
// It uses a cache to avoid re-scanning the same package multiple times.
func (s *Scanner) ScanPackageByImport(importPath string) (*scanner.PackageInfo, error) {
	// This cache (s.packageCache) is for *scanner.PackageInfo objects (parsed ASTs),
	// not the symbol definition location cache (s.symbolCache).
	s.mu.RLock()
	cachedPkg, found := s.packageCache[importPath]
	s.mu.RUnlock()
	if found {
		return cachedPkg, nil
	}

	// If not in packageCache, find directory and scan
	dirPath, err := s.locator.FindPackageDir(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not find directory for import path %s: %w", importPath, err)
	}

	pkgInfo, err := s.ScanPackage(dirPath) // dirPath is absolute physical path
	if err != nil {
		return nil, err
	}

	// After successfully scanning the package, update the symbol cache.
	// The importPath used here is the canonical import path for the package.
	s.updateSymbolCacheWithPackageInfo(importPath, pkgInfo)

	// Store the parsed *scanner.PackageInfo in the packageCache.
	s.mu.Lock()
	s.packageCache[importPath] = pkgInfo
	s.mu.Unlock()

	return pkgInfo, nil
}

// getOrCreateSymbolCache ensures the symbolCache is initialized.
// If s.CachePath is empty, a disabled cache is initialized.
// If s.CachePath is set, an enabled cache is initialized and loaded.
// This method should be called internally before accessing s.symbolCache.
func (s *Scanner) getOrCreateSymbolCache() (*cache.SymbolCache, error) {
	if s.CachePath == "" { // Caching is disabled if CachePath is empty
		if s.symbolCache == nil || s.symbolCache.IsEnabled() { // If not already a disabled cache
			rootDir := ""
			if s.locator != nil { // Ensure locator exists
				rootDir = s.locator.RootDir()
			}
			// Pass empty path to NewSymbolCache to signify a disabled cache
			disabledCache, err := cache.NewSymbolCache(rootDir, "")
			if err != nil {
				// This error should ideally not happen when creating a disabled cache
				return nil, fmt.Errorf("failed to initialize a disabled symbol cache: %w", err)
			}
			s.symbolCache = disabledCache
		}
		return s.symbolCache, nil // Return the disabled cache instance
	}

	// CachePath is not empty, so caching is enabled.
	// If symbolCache is already initialized, enabled, and uses the current CachePath, return it.
	if s.symbolCache != nil && s.symbolCache.IsEnabled() && s.symbolCache.FilePath() == s.CachePath {
		return s.symbolCache, nil
	}

	// Need to initialize or re-initialize an enabled cache.
	rootDir := ""
	if s.locator != nil {
		rootDir = s.locator.RootDir()
	} else {
		// This should not happen if New() was called and succeeded.
		return nil, fmt.Errorf("scanner locator is not initialized, cannot determine root directory for cache")
	}

	// s.CachePath is guaranteed non-empty here.
	sc, err := cache.NewSymbolCache(rootDir, s.CachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize symbol cache with path %s: %w", s.CachePath, err)
	}
	s.symbolCache = sc

	// Load the cache data only if the cache is enabled (which it is if we are here)
	if err := s.symbolCache.Load(); err != nil {
		// SymbolCache.Load() handles non-existent or corrupted files gracefully (prints to stderr, starts empty).
		// An error returned here would be more critical (e.g., I/O error on an existing valid file path).
		fmt.Fprintf(os.Stderr, "warning: could not load symbol cache from %s: %v\n", s.symbolCache.FilePath(), err)
		// Continue with an empty/unloaded cache.
	}
	return s.symbolCache, nil
}

func (s *Scanner) updateSymbolCacheWithPackageInfo(importPath string, pkgInfo *scanner.PackageInfo) {
	if s.CachePath == "" || pkgInfo == nil { // Check CachePath instead of UseCache
		return
	}
	symCache, err := s.getOrCreateSymbolCache() // This will ensure cache is usable if CachePath is set
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting symbol cache for update: %v\n", err)
		return
	}
	if !symCache.IsEnabled() {
		return
	}

	// Types
	for _, typeInfo := range pkgInfo.Types {
		if typeInfo.Name != "" && typeInfo.FilePath != "" {
			key := importPath + "." + typeInfo.Name
			if err := symCache.Set(key, typeInfo.FilePath); err != nil {
				fmt.Fprintf(os.Stderr, "error setting cache for type %s: %v\n", key, err)
			}
		}
	}

	// Functions
	for _, funcInfo := range pkgInfo.Functions {
		if funcInfo.Name != "" && funcInfo.FilePath != "" {
			// TODO: Handle methods. Key format might need to distinguish between funcs and methods.
			// e.g., importPath + "." + receiverType + "." + funcName for methods.
			// For now, assumes top-level functions.
			key := importPath + "." + funcInfo.Name
			if err := symCache.Set(key, funcInfo.FilePath); err != nil {
				fmt.Fprintf(os.Stderr, "error setting cache for func %s: %v\n", key, err)
			}
		}
	}

	// Constants
	for _, constInfo := range pkgInfo.Constants {
		if constInfo.Name != "" && constInfo.FilePath != "" {
			key := importPath + "." + constInfo.Name
			if err := symCache.Set(key, constInfo.FilePath); err != nil {
				fmt.Fprintf(os.Stderr, "error setting cache for const %s: %v\n", key, err)
			}
		}
	}
}

// SaveSymbolCache saves the symbol cache to disk if CachePath is set.
// This should typically be called before application exit, e.g., via defer.
func (s *Scanner) SaveSymbolCache() error {
	if s.CachePath == "" { // If no cache path is set, caching is disabled.
		return nil
	}

	// Ensure cache is initialized before trying to save.
	// getOrCreateSymbolCache will handle initialization if CachePath is set.
	if _, err := s.getOrCreateSymbolCache(); err != nil { // This also ensures s.symbolCache is populated if CachePath is valid
		return fmt.Errorf("cannot save symbol cache, failed to ensure cache initialization for path %s: %w", s.CachePath, err)
	}

	// After getOrCreateSymbolCache, if CachePath was valid, s.symbolCache should be non-nil and enabled.
	if s.symbolCache != nil && s.symbolCache.IsEnabled() {
		if err := s.symbolCache.Save(); err != nil {
			return fmt.Errorf("failed to save symbol cache to %s: %w", s.symbolCache.FilePath(), err)
		}
		return nil
	}
	// This state (CachePath set, but symbolCache nil or disabled after getOrCreate) implies an issue during getOrCreateSymbolCache
	// that might not have returned an error but resulted in a disabled cache (e.g. if rootDir was missing for a new cache).
	// However, getOrCreateSymbolCache should return error in such init failures.
	// So, if we reach here, it means cache is effectively not usable.
	return nil
}

// FindSymbolDefinitionLocation attempts to find the file path for a given symbol.
// symbolFullName should be in the format "package/import/path.SymbolName".
func (s *Scanner) FindSymbolDefinitionLocation(symbolFullName string) (string, error) {
	lastDot := strings.LastIndex(symbolFullName, ".")
	if lastDot == -1 || lastDot == 0 || lastDot == len(symbolFullName)-1 {
		return "", fmt.Errorf("invalid symbol full name format: %q. Expected 'package/import/path.SymbolName'", symbolFullName)
	}
	importPath := symbolFullName[:lastDot]
	symbolName := symbolFullName[lastDot+1:]
	cacheKey := importPath + "." + symbolName

	if s.CachePath != "" { // Only try cache if CachePath is set (i.e., cache enabled)
		symCache, err := s.getOrCreateSymbolCache()
		if err != nil {
			// Log this, but proceed to fallback scan as if cache was a miss or disabled.
			fmt.Fprintf(os.Stderr, "warning: could not get symbol cache for %q: %v. Proceeding with full scan.\n", symbolFullName, err)
		} else if symCache != nil && symCache.IsEnabled() { // Ensure symCache is valid and enabled
			filePath, found := symCache.VerifyAndGet(cacheKey)
			if found {
				return filePath, nil // Cache hit and file exists
			}
		}
		// If err getting cache, or cache disabled, or VerifyAndGet fails, proceed to fallback.
	}

	// Fallback: CachePath is empty, or cache miss, or cache access error, or VerifyAndGet failed.
	pkgInfo, err := s.ScanPackageByImport(importPath) // This will update cache if CachePath is set and scan is successful
	if err != nil {
		return "", fmt.Errorf("fallback scan for package %s (for symbol %s) failed: %w", importPath, symbolName, err)
	}

	// After fallback scan, if cache is enabled, it should have been updated.
	// Try to get from cache again. This is useful if the symbol was found by the scan.
	if s.CachePath != "" {
		// s.symbolCache should have been initialized by getOrCreateSymbolCache if called by ScanPackageByImport's updateSymbolCache.
		// Or, if FindSymbolDefinitionLocation called getOrCreateSymbolCache itself.
		// To be safe, ensure it's valid before using.
		var symCache *cache.SymbolCache
		if s.symbolCache != nil && s.symbolCache.IsEnabled() && s.symbolCache.FilePath() == s.CachePath {
			symCache = s.symbolCache
		} else {
			// Attempt to get/create it again if it wasn't set or was wrong one,
			// though ScanPackageByImport should have done this.
			// This path might be redundant if ScanPackageByImport guarantees cache update.
			symCache, _ = s.getOrCreateSymbolCache() // Ignore error here, if it fails, symCache will be nil/disabled
		}

		if symCache != nil && symCache.IsEnabled() {
			filePath, found := symCache.Get(cacheKey) // Use Get, not VerifyAndGet, as it's fresh from scan.
			if found {
				// Verify file exists, as Get() doesn't.
				if _, statErr := os.Stat(filePath); statErr == nil {
					return filePath, nil
				}
				// If file doesn't exist, something is wrong (e.g. cache Set a non-existent file path).
				// This shouldn't happen if scanner.PopulateSymbolInfo sets FilePath correctly.
				fmt.Fprintf(os.Stderr, "warning: symbol %s found in cache at %s after scan, but file does not exist.\n", symbolFullName, filePath)
			}
		}
	}

	// If cache is disabled, or re-querying cache failed, check the just-scanned pkgInfo directly.
	if pkgInfo != nil {
		targetFilePath := ""
		for _, t := range pkgInfo.Types {
			if t.Name == symbolName {
				targetFilePath = t.FilePath
				break
			}
		}
		if targetFilePath == "" {
			for _, f := range pkgInfo.Functions {
				if f.Name == symbolName {
					targetFilePath = f.FilePath
					break
				}
			}
		}
		if targetFilePath == "" {
			for _, c := range pkgInfo.Constants {
				if c.Name == symbolName {
					targetFilePath = c.FilePath
					break
				}
			}
		}
		if targetFilePath != "" {
			if _, statErr := os.Stat(targetFilePath); statErr == nil {
				return targetFilePath, nil
			}
			return "", fmt.Errorf("symbol %s found in package %s at %s, but file does not exist", symbolName, importPath, targetFilePath)
		}
	}

	return "", fmt.Errorf("symbol %s not found in package %s even after scan", symbolName, importPath)
}
