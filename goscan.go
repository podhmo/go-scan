package goscan

import (
	"context"
	"fmt"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/podhmo/go-scan/cache"
	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Package is an alias for scanner.PackageInfo.
type Package = scanner.PackageInfo

// Constants for scanner kinds.
const (
	StructKind    = scanner.StructKind
	AliasKind     = scanner.AliasKind
	FuncKind      = scanner.FuncKind
	InterfaceKind = scanner.InterfaceKind
)

// Scanner is the main entry point for the scanning library.
type Scanner struct {
	workDir               string
	locator               *locator.Locator
	scanner               *scanner.Scanner
	packageCache          map[string]*Package
	visitedFiles          map[string]struct{}
	mu                    sync.RWMutex
	fset                  *token.FileSet
	CachePath             string
	symbolCache           *cache.SymbolCache
	ExternalTypeOverrides scanner.ExternalTypeOverride
	overlay               scanner.Overlay
}

// Fset returns the FileSet associated with the scanner.
func (s *Scanner) Fset() *token.FileSet {
	return s.fset
}

// Scan scans Go packages based on patterns.
func (s *Scanner) Scan(patterns ...string) ([]*Package, error) {
	var pkgs []*Package
	ctx := context.Background()
	for _, pattern := range patterns {
		absPath := pattern
		if !filepath.IsAbs(pattern) {
			absPath = filepath.Join(s.workDir, pattern)
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("could not stat pattern %q: %w", pattern, err)
		}
		var pkg *Package
		if info.IsDir() {
			pkg, err = s.ScanPackage(ctx, absPath)
		} else {
			pkg, err = s.ScanFiles(ctx, []string{absPath})
		}
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

// ScannerOption configures a Scanner.
type ScannerOption func(*Scanner) error

// WithWorkDir sets the working directory.
func WithWorkDir(path string) ScannerOption {
	return func(s *Scanner) error {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("getting absolute path for workdir %q: %w", path, err)
		}
		s.workDir = absPath
		return nil
	}
}

// WithOverlay provides in-memory file content.
func WithOverlay(overlay scanner.Overlay) ScannerOption {
	return func(s *Scanner) error {
		s.overlay = overlay
		return nil
	}
}

// New creates a new Scanner.
func New(options ...ScannerOption) (*Scanner, error) {
	s := &Scanner{
		packageCache:          make(map[string]*Package),
		visitedFiles:          make(map[string]struct{}),
		fset:                  token.NewFileSet(),
		ExternalTypeOverrides: make(scanner.ExternalTypeOverride),
		overlay:               make(scanner.Overlay),
	}
	for _, option := range options {
		if err := option(s); err != nil {
			return nil, err
		}
	}
	if s.workDir == "" {
		s.workDir, _ = os.Getwd()
	}
	loc, err := locator.New(s.workDir, s.overlay)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize locator: %w", err)
	}
	s.locator = loc
	initialScanner, err := scanner.New(s.fset, s.ExternalTypeOverrides, s.overlay, loc.ModulePath(), loc.RootDir(), s)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal scanner: %w", err)
	}
	s.scanner = initialScanner
	return s, nil
}

// SetExternalTypeOverrides sets the external type override map.
func (s *Scanner) SetExternalTypeOverrides(ctx context.Context, overrides scanner.ExternalTypeOverride) {
	s.ExternalTypeOverrides = overrides
	newInternalScanner, err := scanner.New(s.fset, s.ExternalTypeOverrides, s.overlay, s.locator.ModulePath(), s.locator.RootDir(), s)
	if err != nil {
		slog.WarnContext(ctx, "Failed to re-initialize internal scanner with new overrides.", "error", err)
		return
	}
	s.scanner = newInternalScanner
}

// ResolveType starts the type resolution process.
func (s *Scanner) ResolveType(ctx context.Context, fieldType *scanner.FieldType) (*scanner.TypeInfo, error) {
	return s.scanner.ResolveType(ctx, fieldType)
}

func listGoFiles(dirPath string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir %s: %w", dirPath, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, filepath.Join(dirPath, entry.Name()))
		}
	}
	return files, nil
}

// ScanPackage scans a single package.
func (s *Scanner) ScanPackage(ctx context.Context, pkgPath string) (*scanner.PackageInfo, error) {
	absPkgPath, err := filepath.Abs(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for %s: %w", pkgPath, err)
	}
	allFilesInDir, err := listGoFiles(absPkgPath)
	if err != nil {
		return nil, err
	}
	return s.ScanFiles(ctx, allFilesInDir)
}

// ScanFiles scans a set of Go files, resolving paths relative to the scanner's workDir.
func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string) (*scanner.PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no file paths provided")
	}

	var filesToParse []string
	var firstFileDir string

	for i, fp := range filePaths {
		var absFp string
		if filepath.IsAbs(fp) {
			absFp = fp
		} else if strings.HasPrefix(fp, s.locator.ModulePath()) {
			// Handle module-qualified paths like "example.com/mymodule/myfile.go"
			var err error
			absFp, err = s.locator.ResolvePath(fp)
			if err != nil {
				return nil, fmt.Errorf("could not resolve module path %q: %w", fp, err)
			}
		} else {
			// Handle file-system relative paths from the scanner's workDir
			absFp = filepath.Join(s.workDir, fp)
		}

		// Clean the path to have a consistent format
		absFp = filepath.Clean(absFp)

		if i == 0 {
			firstFileDir = filepath.Dir(absFp)
		}

		s.mu.RLock()
		_, visited := s.visitedFiles[absFp]
		s.mu.RUnlock()

		if !visited {
			filesToParse = append(filesToParse, absFp)
		}
	}

	if len(filesToParse) == 0 {
		// All files were already visited. Return an empty PackageInfo.
		// The package directory is taken from the first file provided.
		return &scanner.PackageInfo{Path: firstFileDir, Fset: s.fset}, nil
	}

	// All files in a single ScanFiles call are assumed to belong to the same package,
	// so we can use the directory of the first file to determine the package context.
	pkgDirAbs := filepath.Dir(filesToParse[0])

	pkgInfo, err := s.scanner.ScanFiles(ctx, filesToParse, pkgDirAbs)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	for _, fp := range pkgInfo.Files {
		s.visitedFiles[fp] = struct{}{}
	}
	// Only cache if we got a valid import path.
	if pkgInfo.ImportPath != "" {
		s.packageCache[pkgInfo.ImportPath] = pkgInfo
		s.updateSymbolCacheWithPackageInfo(ctx, pkgInfo.ImportPath, pkgInfo)
	}
	s.mu.Unlock()

	return pkgInfo, nil
}

// UnscannedGoFiles returns a list of unscanned .go files in a package.
func (s *Scanner) UnscannedGoFiles(packagePathOrImportPath string) ([]string, error) {
	pkgDirAbs, err := s.locator.FindPackageDir(packagePathOrImportPath)
	if err != nil {
		return nil, err
	}
	allGoFilesInDir, err := listGoFiles(pkgDirAbs)
	if err != nil {
		return nil, err
	}
	var unscannedFiles []string
	for _, absFilePath := range allGoFilesInDir {
		if _, visited := s.visitedFiles[absFilePath]; !visited {
			unscannedFiles = append(unscannedFiles, absFilePath)
		}
	}
	return unscannedFiles, nil
}

// ScanPackageByImport scans a single Go package by its import path.
func (s *Scanner) ScanPackageByImport(ctx context.Context, importPath string) (*scanner.PackageInfo, error) {
	s.mu.RLock()
	cachedPkg, found := s.packageCache[importPath]
	s.mu.RUnlock()
	if found {
		return cachedPkg, nil
	}

	pkgDirAbs, err := s.locator.FindPackageDir(importPath)
	if err != nil {
		return nil, err
	}

	allGoFilesInPkg, err := listGoFiles(pkgDirAbs)
	if err != nil {
		return nil, err
	}
	if len(allGoFilesInPkg) == 0 {
		pkgInfo := &scanner.PackageInfo{Path: pkgDirAbs, ImportPath: importPath, Name: "", Fset: s.fset}
		s.mu.Lock()
		s.packageCache[importPath] = pkgInfo
		s.mu.Unlock()
		return pkgInfo, nil
	}

	var filesToParseThisCall []string
	for _, f := range allGoFilesInPkg {
		if _, visited := s.visitedFiles[f]; !visited {
			filesToParseThisCall = append(filesToParseThisCall, f)
		}
	}
	if len(filesToParseThisCall) == 0 {
		return &scanner.PackageInfo{Path: pkgDirAbs, ImportPath: importPath, Fset: s.fset}, nil
	}

	var currentCallPkgInfo *scanner.PackageInfo
	isStdLib := !strings.Contains(strings.Split(importPath, "/")[0], ".")
	if isStdLib {
		currentCallPkgInfo, err = s.scanner.ScanFilesWithKnownImportPath(ctx, filesToParseThisCall, pkgDirAbs, importPath)
	} else {
		currentCallPkgInfo, err = s.scanner.ScanFiles(ctx, filesToParseThisCall, pkgDirAbs)
	}
	if err != nil {
		return nil, err
	}

	for _, fp := range currentCallPkgInfo.Files {
		s.visitedFiles[fp] = struct{}{}
	}
	s.updateSymbolCacheWithPackageInfo(ctx, importPath, currentCallPkgInfo)
	s.mu.Lock()
	s.packageCache[importPath] = currentCallPkgInfo
	s.mu.Unlock()
	return currentCallPkgInfo, nil
}

// FindSymbolDefinitionLocation attempts to find the file where a symbol is defined.
func (s *Scanner) FindSymbolDefinitionLocation(ctx context.Context, symbolFullName string) (string, error) {
	lastDot := strings.LastIndex(symbolFullName, ".")
	if lastDot == -1 {
		return "", fmt.Errorf("invalid symbol format: %q", symbolFullName)
	}
	importPath := symbolFullName[:lastDot]
	symbolName := symbolFullName[lastDot+1:]

	pkgInfo, err := s.ScanPackageByImport(ctx, importPath)
	if err != nil {
		return "", fmt.Errorf("could not scan package %s: %w", importPath, err)
	}

	for _, t := range pkgInfo.Types {
		if t.Name == symbolName {
			return t.FilePath, nil
		}
	}
	for _, f := range pkgInfo.Functions {
		if f.Name == symbolName {
			return f.FilePath, nil
		}
	}
	for _, c := range pkgInfo.Constants {
		if c.Name == symbolName {
			return c.FilePath, nil
		}
	}
	return "", fmt.Errorf("symbol %q not found in package %q", symbolName, importPath)
}

func (s *Scanner) debugDump(ctx context.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fmt.Println("--- visited files ---")
	for k := range s.visitedFiles {
		fmt.Println(k)
	}

	fmt.Println("--- package cache ---")
	for k, v := range s.packageCache {
		fmt.Printf("%s: %v\n", k, v.Files)
	}

	fmt.Println("--- symbol cache ---")
	if s.symbolCache != nil {
		s.symbolCache.DebugDump()
	}
}

// ... (other helper methods like getOrCreateSymbolCache, updateSymbolCacheWithPackageInfo, SaveSymbolCache)
func (s *Scanner) getOrCreateSymbolCache(ctx context.Context) (*cache.SymbolCache, error) {
	if s.CachePath == "" {
		if s.symbolCache == nil {
			disabledCache, err := cache.NewSymbolCache(s.locator.RootDir(), "")
			if err != nil {
				return nil, fmt.Errorf("failed to initialize a disabled symbol cache: %w", err)
			}
			s.symbolCache = disabledCache
		}
		return s.symbolCache, nil
	}
	if s.symbolCache != nil && s.symbolCache.FilePath() == s.CachePath {
		return s.symbolCache, nil
	}
	sc, err := cache.NewSymbolCache(s.locator.RootDir(), s.CachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize symbol cache with path %s: %w", s.CachePath, err)
	}
	s.symbolCache = sc
	if err := s.symbolCache.Load(ctx); err != nil {
		slog.WarnContext(ctx, "Could not load symbol cache", "path", s.CachePath, "error", err)
	}
	return s.symbolCache, nil
}
func (s *Scanner) updateSymbolCacheWithPackageInfo(ctx context.Context, importPath string, pkgInfo *scanner.PackageInfo) {
	if s.CachePath == "" || pkgInfo == nil || len(pkgInfo.Files) == 0 {
		return
	}
	symCache, err := s.getOrCreateSymbolCache(ctx)
	if err != nil || !symCache.IsEnabled() {
		return
	}

	filesToSymbols := make(map[string][]string)

	for _, t := range pkgInfo.Types {
		filesToSymbols[t.FilePath] = append(filesToSymbols[t.FilePath], t.Name)
		key := importPath + "." + t.Name
		if err := symCache.SetSymbol(key, t.FilePath); err != nil {
			slog.WarnContext(ctx, "Failed to set symbol in cache", "key", key, "error", err)
		}
	}
	for _, f := range pkgInfo.Functions {
		filesToSymbols[f.FilePath] = append(filesToSymbols[f.FilePath], f.Name)
		key := importPath + "." + f.Name
		if err := symCache.SetSymbol(key, f.FilePath); err != nil {
			slog.WarnContext(ctx, "Failed to set symbol in cache", "key", key, "error", err)
		}
	}
	for _, c := range pkgInfo.Constants {
		filesToSymbols[c.FilePath] = append(filesToSymbols[c.FilePath], c.Name)
		key := importPath + "." + c.Name
		if err := symCache.SetSymbol(key, c.FilePath); err != nil {
			slog.WarnContext(ctx, "Failed to set symbol in cache", "key", key, "error", err)
		}
	}

	for filePath, symbols := range filesToSymbols {
		metadata := cache.FileMetadata{
			Symbols: symbols,
		}
		if err := symCache.SetFileMetadata(filePath, metadata); err != nil {
			slog.WarnContext(ctx, "Failed to set file metadata in cache", "file", filePath, "error", err)
		}
	}
}
func (s *Scanner) SaveSymbolCache(ctx context.Context) error {
	if s.CachePath == "" {
		return nil
	}
	if _, err := s.getOrCreateSymbolCache(ctx); err != nil {
		return err
	}
	if s.symbolCache != nil && s.symbolCache.IsEnabled() {
		return s.symbolCache.Save()
	}
	return nil
}
