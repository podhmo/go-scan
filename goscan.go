package goscan

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"go/token" // Added for fset

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
	scanner      *scanner.Scanner // The actual parser, configured with fset and overrides
	packageCache map[string]*scanner.PackageInfo
	mu           sync.RWMutex
	fset         *token.FileSet // Shared FileSet for all parsing operations

	CachePath             string
	symbolCache           *cache.SymbolCache
	ExternalTypeOverrides scanner.ExternalTypeOverride
}

// New creates a new Scanner. It finds the module root starting from the given path.
func New(startPath string) (*Scanner, error) {
	loc, err := locator.New(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize locator: %w", err)
	}

	fset := token.NewFileSet()
	// Initialize scanner.Scanner with the shared fset and empty overrides initially.
	// Overrides can be set later via SetExternalTypeOverrides.
	initialScanner, err := scanner.New(fset, nil)
	if err != nil {
		// This error would only happen if fset is nil, which it isn't here.
		// But good practice to check.
		return nil, fmt.Errorf("failed to create internal scanner: %w", err)
	}

	return &Scanner{
		locator:               loc,
		scanner:               initialScanner, // Use the initialized scanner
		packageCache:          make(map[string]*scanner.PackageInfo),
		fset:                  fset, // Store the shared fset
		ExternalTypeOverrides: make(scanner.ExternalTypeOverride),
	}, nil
}

// SetExternalTypeOverrides sets the external type override map for the scanner.
// This map allows specifying how types from external (or internal) packages
// should be interpreted. For example, mapping "github.com/google/uuid.UUID" to "string".
func (s *Scanner) SetExternalTypeOverrides(overrides scanner.ExternalTypeOverride) {
	if overrides == nil {
		overrides = make(scanner.ExternalTypeOverride)
	}
	s.ExternalTypeOverrides = overrides
	// Re-initialize the internal scanner.Scanner with the new overrides, using the existing shared fset.
	newInternalScanner, err := scanner.New(s.fset, s.ExternalTypeOverrides)
	if err != nil {
		// This error should ideally not happen if s.fset is always valid.
		// Log this or handle more gracefully if s.fset could become invalid.
		// For now, we might panic or return an error if this is a user-facing configuration method.
		// However, this method doesn't return an error. We'll assume s.fset is good.
		// If scanner.New can return error for other reasons than nil fset, that needs handling.
		// For simplicity, let's assume scanner.New only errors on nil fset, which we guard in goscan.New.
		// If it could error otherwise, this Set method should probably return an error.
		fmt.Fprintf(os.Stderr, "warning: failed to re-initialize internal scanner with new overrides: %v. Continuing with previous scanner settings.\n", err)
		// Do not replace s.scanner if re-initialization fails, to keep it in a working state.
		return
	}
	s.scanner = newInternalScanner
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

	var pkgInfo *scanner.PackageInfo
	symCache, cacheErr := s.getOrCreateSymbolCache()
	if cacheErr != nil {
		// Log the error but proceed as if cache is disabled or missed.
		fmt.Fprintf(os.Stderr, "warning: could not get symbol cache for import path %s: %v. Proceeding with full scan.\n", importPath, cacheErr)
	}

	if symCache != nil && symCache.IsEnabled() {
		newFilesToScan, existingFilesFromCache, err := symCache.GetFilesToScan(dirPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: GetFilesToScan failed for %s: %v. Proceeding with full scan of all files.\n", dirPath, err)
			// Fallback: scan all files in the directory if GetFilesToScan fails
			pkgInfo, err = s.scanner.ScanPackage(dirPath, s) // ScanPackage uses s.scanner which has fset
			if err != nil {
				return nil, fmt.Errorf("fallback full scan for package %s failed: %w", importPath, err)
			}
		} else {
			// Partial scan logic
			filesToActuallyScan := newFilesToScan // Scan new files

			// For existing files, we'd ideally check their modification times if that feature was kept.
			// Since it's removed, for now, we assume existingFilesInCache don't need re-scanning unless GetFilesToScan implies otherwise.
			// However, the current GetFilesToScan returns existing files that *are* in cache, implying they might still be valid.
			// The critical part is that scanner.ScanFiles needs the *list of files to parse now*.
			// If we want to rebuild PackageInfo from cached data + newly scanned data, it's more complex.
			// For now, let's assume we re-scan existing files too, to ensure PackageInfo is complete,
			// or adjust ScanFiles to build PackageInfo from a mix.
			// A simpler approach for now: if there are new files, or if we decide to re-validate existing, scan them.
			// If GetFilesToScan is robust, existingFilesInCache are files that *are* in cache and *do* exist on disk.
			// We might need a strategy: always re-scan existing to be safe, or only scan new.
			// Let's re-scan existing files as well to ensure data integrity, until a more sophisticated check (like content hashing) is in place.
			filesToActuallyScan = append(filesToActuallyScan, existingFilesFromCache...)

			if len(filesToActuallyScan) == 0 {
				// All files are cached and assumed up-to-date. We need to construct PackageInfo from cache.
				// This is a complex step not yet implemented.
				// For now, if all files are "existing", re-scan them to build PackageInfo.
				// This means GetFilesToScan helps identify deleted files, but not avoid re-parsing existing ones yet.
				// A true optimization would involve loading FileMetadata and constructing PackageInfo without re-parsing.
				//
				// If there are no files to scan (e.g., package is empty after _test.go filtering),
				// ScanFiles will handle it.
				// If filesToActuallyScan is empty because all files were in existingFilesInCache and we decide not to rescan them,
				// then we would need to load PackageInfo from cache.
				// For now, let's just scan all files identified (new + existing)
				fmt.Fprintf(os.Stderr, "info: no new files to scan for %s, re-scanning %d existing files.\n", importPath, len(existingFilesFromCache))
				// If filesToActuallyScan is empty (no new, no existing), ScanFiles will return error.
				// This can happen if a directory contains only _test.go files or no .go files.
				// os.ReadDir in ScanPackage (used by s.scanner.ScanPackage) or in symCache.GetFilesToScan should handle this.
			}

			if len(filesToActuallyScan) > 0 {
				// Use s.scanner.ScanFiles directly, which uses the shared fset
				pkgInfo, err = s.scanner.ScanFiles(filesToActuallyScan, dirPath, s)
				if err != nil {
					return nil, fmt.Errorf("partial scan for package %s with files %v failed: %w", importPath, filesToActuallyScan, err)
				}
			} else {
				// No files to scan at all (e.g. empty directory, or only _test.go files)
				// Create a minimal PackageInfo or handle as error.
				// scanner.ScanFiles would return error if filePaths is empty.
				// We can pre-emptively create an empty PackageInfo or let it error.
				// Let's create an empty one, as the package might genuinely be empty of scannable files.
				pkgInfo = &scanner.PackageInfo{
					Path:  dirPath,
					Name:  "", // Will be determined by ScanFiles if it were called, or needs to be found another way.
					Fset:  s.fset,
					Files: []string{},
				}
				// Attempt to determine package name if possible, e.g. from dir name or a known go.mod.
				// For now, leave it potentially blank if no files are scanned.
				// This might need adjustment based on how consumers handle PackageInfo with no files/name.
			}
		}
	} else { // Cache disabled or error getting it, perform a full scan of the package.
		pkgInfo, err = s.scanner.ScanPackage(dirPath, s) // ScanPackage uses s.scanner which has fset
		if err != nil {
			return nil, fmt.Errorf("full scan for package %s (cache disabled/error) failed: %w", importPath, err)
		}
	}

	// After successfully scanning the package (either fully or partially), update the symbol cache.
	// The importPath used here is the canonical import path for the package.
	if pkgInfo != nil { // pkgInfo might be nil if ScanPackage/ScanFiles failed and returned error earlier
		s.updateSymbolCacheWithPackageInfo(importPath, pkgInfo)
	}

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

	// Group symbols by their absolute file path
	symbolsByFile := make(map[string][]string)

	// Helper to add symbol to file map and set in cache
	addSymbol := func(symbolName, absFilePath string) {
		if symbolName != "" && absFilePath != "" {
			key := importPath + "." + symbolName
			if err := symCache.SetSymbol(key, absFilePath); err != nil {
				fmt.Fprintf(os.Stderr, "error setting cache for symbol %s: %v\n", key, err)
			}
			symbolsByFile[absFilePath] = append(symbolsByFile[absFilePath], symbolName)
		}
	}

	// Types
	for _, typeInfo := range pkgInfo.Types {
		addSymbol(typeInfo.Name, typeInfo.FilePath)
	}

	// Functions
	for _, funcInfo := range pkgInfo.Functions {
		// TODO: Handle methods. Key format might need to distinguish between funcs and methods.
		// e.g., importPath + "." + receiverType + "." + funcName for methods.
		// For now, assumes top-level functions.
		addSymbol(funcInfo.Name, funcInfo.FilePath)
	}

	// Constants
	for _, constInfo := range pkgInfo.Constants {
		addSymbol(constInfo.Name, constInfo.FilePath)
	}

	// Now, update FileMetadata for each file processed in this package scan
	// pkgInfo.Files contains the list of absolute file paths that were part of this scan.
	for _, absFilePath := range pkgInfo.Files {
		if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
			// If a file processed by the scanner somehow doesn't exist, skip it.
			// This shouldn't happen if scanner.ScanFiles populates pkgInfo.Files correctly.
			fmt.Fprintf(os.Stderr, "warning: file %s from pkgInfo.Files not found, skipping for FileMetadata update\n", absFilePath)
			continue
		}

		fileSymbols := symbolsByFile[absFilePath]
		if fileSymbols == nil {
			fileSymbols = []string{} // Ensure it's an empty slice, not nil, for JSON marshalling
		}

		// Note: ModTime was removed from FileMetadata. If it were present, it would be set here:
		// fileStat, err := os.Stat(absFilePath)
		// var modTime time.Time
		// if err == nil {
		// 	modTime = fileStat.ModTime()
		// } else {
		// 	fmt.Fprintf(os.Stderr, "warning: could not stat file %s for modtime: %v\n", absFilePath, err)
		// }
		// metadata := cache.FileMetadata{ModTime: modTime, Symbols: fileSymbols}

		metadata := cache.FileMetadata{Symbols: fileSymbols}
		if err := symCache.SetFileMetadata(absFilePath, metadata); err != nil {
			fmt.Fprintf(os.Stderr, "error setting file metadata for %s: %v\n", absFilePath, err)
		}
	}

	// It's also important to consider files that might have been removed from the package.
	// symCache.GetFilesToScan() handles removing entries for deleted files.
	// So, updateSymbolCacheWithPackageInfo focuses on adding/updating currently scanned files.
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
