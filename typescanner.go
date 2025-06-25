package typescanner

import (
	"fmt"
	"os"
	"sync"

	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/cache"
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
	// If empty, a default path will be used (e.g., $HOME/.go_symbol_analyzer_cache.json).
	CachePath string
	// UseCache enables or disables the symbol cache.
	UseCache bool
	symbolCache *cache.SymbolCache // Added
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
		// Default values for cache settings
		CachePath: "", // Or a sensible default like "$HOME/.go_symbol_analyzer_cache.json"
		UseCache:  false,
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

// getOrCreateSymbolCache ensures the symbolCache is initialized and loaded if UseCache is true.
// This method should be called internally before accessing s.symbolCache.
func (s *Scanner) getOrCreateSymbolCache() (*cache.SymbolCache, error) {
	// If UseCache is false, and symbolCache exists but is enabled, we need to disable it.
	if !s.UseCache {
		if s.symbolCache != nil && s.symbolCache.IsEnabled() {
			// Transitioning from enabled to disabled: re-initialize to a disabled cache.
			// locator should be available if New was successful.
			rootDir := ""
			if s.locator != nil {
				rootDir = s.locator.RootDir()
			}
			// No error expected for creating a disabled cache if rootDir is valid or empty.
			disabledCache, _ := cache.NewSymbolCache(rootDir, s.CachePath, false)
			s.symbolCache = disabledCache
		} else if s.symbolCache == nil {
			// If it's nil and UseCache is false, initialize a disabled one.
			rootDir := ""
			if s.locator != nil {
				rootDir = s.locator.RootDir()
			}
			disabledCache, _ := cache.NewSymbolCache(rootDir, s.CachePath, false)
			s.symbolCache = disabledCache
		}
		// At this point, s.symbolCache is either nil (if locator was nil, unlikely)
		// or a disabled cache instance.
		return s.symbolCache, nil
	}

	// UseCache is true.
	// If symbolCache is already initialized and enabled, return it.
	if s.symbolCache != nil && s.symbolCache.IsEnabled() {
		// Also ensure CachePath matches, in case it was changed after initialization.
		// This check might be overly complex; typically CachePath is set once.
		// For now, we assume if it's enabled, it's correctly configured.
		// If s.CachePath changed, the existing s.symbolCache.FilePath() might be outdated.
		// A robust solution would involve comparing s.CachePath with s.symbolCache.FilePath()
		// and re-initializing if different. Let's simplify: assume if enabled, it's current.
		return s.symbolCache, nil
	}

	// Need to initialize or re-initialize an enabled cache.
	rootDir := ""
	if s.locator != nil {
		rootDir = s.locator.RootDir()
	} else {
		return nil, fmt.Errorf("scanner locator is not initialized, cannot determine root directory for cache")
	}

	sc, err := cache.NewSymbolCache(rootDir, s.CachePath, true)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize symbol cache: %w", err)
	}
	s.symbolCache = sc

	if err := s.symbolCache.Load(); err != nil {
		// As per SymbolCache.Load() behavior, it prints to stderr on unmarshal errors
		// and continues with an empty cache. Other errors (e.g., directory permissions for default path)
		// might be returned here.
		fmt.Fprintf(os.Stderr, "warning: could not load symbol cache from %s: %v\n", s.symbolCache.FilePath(), err)
		// Continue with an empty cache; Load() itself should ensure cacheData is empty on error.
	}
	return s.symbolCache, nil
}

func (s *Scanner) updateSymbolCacheWithPackageInfo(importPath string, pkgInfo *scanner.PackageInfo) {
	if !s.UseCache || pkgInfo == nil {
		return
	}
	symCache, err := s.getOrCreateSymbolCache()
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

// SaveSymbolCache saves the symbol cache to disk if it's enabled and has been initialized.
// This should typically be called before application exit, e.g., via defer.
func (s *Scanner) SaveSymbolCache() error {
	// Ensure cache is initialized before trying to save, especially if no operations
	// triggered its lazy creation yet but UseCache is true.
	if s.UseCache {
		// Attempt to get/create the cache. If s.UseCache is true, this will initialize and load it.
		// We need this to ensure s.symbolCache is not nil if we intend to save.
		if _, err := s.getOrCreateSymbolCache(); err != nil {
			// If initialization itself fails (e.g. bad RootDir, perms for default cache path), we can't save.
			return fmt.Errorf("cannot save symbol cache, failed to ensure cache initialization: %w", err)
		}
	}

	if s.symbolCache != nil && s.symbolCache.IsEnabled() {
		// Proceed to save only if symbolCache is not nil and is enabled.
		if err := s.symbolCache.Save(); err != nil {
			return fmt.Errorf("failed to save symbol cache to %s: %w", s.symbolCache.FilePath(), err)
		}
		return nil
	}
	return nil // Nothing to save if cache is not active or not initialized (e.g. UseCache is false)
}

// FindSymbolDefinitionLocation attempts to find the file path for a given symbol.
// symbolFullName should be in the format "package/import/path.SymbolName".
func (s *Scanner) FindSymbolDefinitionLocation(symbolFullName string) (string, error) {
	lastDot := strings.LastIndex(symbolFullName, ".")
	if lastDot == -1 || lastDot == 0 || lastDot == len(symbolFullName)-1 {
		// Try to handle cases where symbolFullName might be just "SymbolName" if current package context is known.
		// However, this function implies full qualification.
		// Also, package paths can contain dots. e.g. "k8s.io/api.Pod"
		// A more robust way to parse this would be needed if package paths with dots are common AND conflict.
		// For "example.com/foo.bar/pkg.MyType", lastDot finds the one before "MyType".
		// If symbol is "toplevel.Constant", it would be parsed as pkg="toplevel", sym="Constant".
		// This needs careful consideration of what symbolFullName represents.
		// Assuming "canonical_import_path.Symbol" for now.
		return "", fmt.Errorf("invalid symbol full name format: %q. Expected 'package/import/path.SymbolName'", symbolFullName)
	}
	importPath := symbolFullName[:lastDot]
	symbolName := symbolFullName[lastDot+1:]
	cacheKey := importPath + "." + symbolName // This is the standard key format.

	if s.UseCache {
		symCache, err := s.getOrCreateSymbolCache()
		if err != nil {
			return "", fmt.Errorf("could not get symbol cache for %q: %w", symbolFullName, err)
		}
		// symCache itself might be nil if getOrCreateSymbolCache returns error for a disabled cache,
		// but getOrCreateSymbolCache should return a disabled cache instance or error.
		if symCache != nil && symCache.IsEnabled() {
			filePath, found := symCache.VerifyAndGet(cacheKey) // VerifyAndGet checks file existence and removes if stale
			if found {
				// Optional: Deeper verification - parse filePath and check if symbolName exists.
				// For now, file existence is the primary check from VerifyAndGet.
				// If symbol was moved to another file in the same package, this cache entry is stale.
				// The fallback scan below would find it and update the cache.
				// So, deeper verification here might be redundant if fallback is efficient.
				// Let's assume for now that if VerifyAndGet returns true, it's a valid candidate.
				// The user's requirement: "キャッシュに指定されているファイルに存在しなかった場合にはディレクトリ中を走査"
				// VerifyAndGet handles "ファイルが存在しなかった場合".
				// If "ファイルは存在するがシンボルがない"場合もフォールバックすべき。
				// This implies we need a way to check symbol in file.
				// For now, let's make a placeholder for this deeper check.
				// If verifySymbolInFile is too complex, we can rely on the fallback scan
				// to correct the cache if the symbol moved within the package.

				// Simplification: If VerifyAndGet says file exists, trust it for now.
				// The fallback will occur if VerifyAndGet returns false (file not found).
				return filePath, nil
			}
		}
	}

	// Fallback: Cache miss, cache disabled, or entry was stale (file not found by VerifyAndGet).
	// Scan the package. This will also update the symbol cache if successful.
	pkgInfo, err := s.ScanPackageByImport(importPath)
	if err != nil {
		return "", fmt.Errorf("fallback scan for package %s (for symbol %s) failed: %w", importPath, symbolName, err)
	}

	// After scan, the cache (if enabled) has been updated by ScanPackageByImport's call to updateSymbolCacheWithPackageInfo.
	// We can now try to get the information from the fresh pkgInfo or re-query the cache.
	// Re-querying cache is cleaner if updateSymbolCacheWithPackageInfo is comprehensive.
	if s.UseCache && s.symbolCache != nil && s.symbolCache.IsEnabled() {
		filePath, found := s.symbolCache.Get(cacheKey) // Use Get, not VerifyAndGet, as it's fresh.
		if found {
			// We should ensure the file actually exists, as Get doesn't check.
			if _, statErr := os.Stat(filePath); statErr == nil {
				return filePath, nil
			}
			// If file doesn't exist here, something is wrong with cache update or file system state.
		}
	} else if pkgInfo != nil { // If cache disabled or issue, check the just-scanned pkgInfo directly.
		// This provides a direct path if cache isn't used or failed.
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
