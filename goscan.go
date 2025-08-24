package goscan

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/podhmo/go-scan/fs"
	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Package is an alias for scanner.PackageInfo.
type Package = scanner.PackageInfo

// TypeInfo is an alias for scanner.TypeInfo.
type TypeInfo = scanner.TypeInfo

// ConstantInfo is an alias for scanner.ConstantInfo.
type ConstantInfo = scanner.ConstantInfo

// FunctionInfo is an alias for scanner.FunctionInfo.
type FunctionInfo = scanner.FunctionInfo

// PackageImports is an alias for scanner.PackageImports.
type PackageImports = scanner.PackageImports

// Visitor is an alias for scanner.Visitor.
type Visitor = scanner.Visitor

// VisitorFunc is a helper type that implements the Visitor interface with a function.
type VisitorFunc func(*PackageImports) ([]string, error)

// Visit calls the underlying function.
func (f VisitorFunc) Visit(p *PackageImports) ([]string, error) {
	return f(p)
}

// Re-export scanner kinds for convenience.
const (
	StructKind    = scanner.StructKind
	AliasKind     = scanner.AliasKind
	FuncKind      = scanner.FuncKind
	InterfaceKind = scanner.InterfaceKind
)

// Scanner is the main entry point for the type scanning library.
type Scanner struct {
	*Config
	packageCache          map[string]*Package
	visitedFiles          map[string]struct{}
	mu                    sync.RWMutex
	CachePath             string
	symbolCache           *symbolCache
	ExternalTypeOverrides scanner.ExternalTypeOverride
	Walker                *ModuleWalker
}

// Fset returns the FileSet associated with the scanner.
func (s *Scanner) Fset() *token.FileSet {
	return s.fset
}

// Locator returns the underlying locator instance.
func (s *Scanner) Locator() *locator.Locator {
	return s.locator
}

// BuildImportLookup creates a map of local import names to their full package paths for a given file.
func (s *Scanner) BuildImportLookup(file *ast.File) map[string]string {
	return s.scanner.BuildImportLookup(file)
}

// TypeInfoFromExpr resolves an AST expression that represents a type into a FieldType.
func (s *Scanner) TypeInfoFromExpr(ctx context.Context, expr ast.Expr, currentTypeParams []*scanner.TypeParamInfo, info *scanner.PackageInfo, importLookup map[string]string) *scanner.FieldType {
	return s.scanner.TypeInfoFromExpr(ctx, expr, currentTypeParams, info, importLookup)
}

// ScanPackageByPos finds and scans the package containing the given token.Pos.
func (s *Scanner) ScanPackageByPos(ctx context.Context, pos token.Pos) (*scanner.PackageInfo, error) {
	if !pos.IsValid() {
		return nil, fmt.Errorf("invalid position")
	}
	file := s.fset.File(pos)
	if file == nil {
		return nil, fmt.Errorf("no file found for position")
	}
	pkgDir := filepath.Dir(file.Name())
	return s.ScanPackage(ctx, pkgDir)
}

// ScannerForSymgo is a temporary helper for tests to access the internal scanner.
func (s *Scanner) ScannerForSymgo() (*scanner.Scanner, error) {
	return s.scanner, nil
}

// ModulePath returns the module path from the scanner's locator.
func (s *Scanner) ModulePath() string {
	if s.locator == nil {
		return ""
	}
	return s.locator.ModulePath()
}

// RootDir returns the module root directory from the scanner's locator.
func (s *Scanner) RootDir() string {
	if s.locator == nil {
		return ""
	}
	return s.locator.RootDir()
}

// LookupOverride checks if a fully qualified type name exists in the external type override map.
func (s *Scanner) LookupOverride(qualifiedName string) (*scanner.TypeInfo, bool) {
	if s.ExternalTypeOverrides == nil {
		return nil, false
	}
	ti, ok := s.ExternalTypeOverrides[qualifiedName]
	return ti, ok
}

// Scan traverses the dependency graph starting from the packages matched by the
// given patterns, and returns a list of all discovered packages.
func (s *Scanner) Scan(patterns ...string) ([]*Package, error) {
	ctx := context.Background()
	if s.Logger != nil {
		// slog.Logger is passed through context, no need to create a new one here.
	}

	var packages []*Package
	seen := make(map[string]struct{})

	visitor := VisitorFunc(func(pkgImports *PackageImports) ([]string, error) {
		if _, ok := seen[pkgImports.ImportPath]; ok {
			return nil, nil
		}
		seen[pkgImports.ImportPath] = struct{}{}

		pkg, err := s.ScanPackageByImport(ctx, pkgImports.ImportPath)
		if err != nil {
			slog.WarnContext(ctx, "failed to scan package during walk", "importPath", pkgImports.ImportPath, "error", err)
			return pkgImports.Imports, nil
		}
		if pkg != nil {
			packages = append(packages, pkg)
		}
		return pkgImports.Imports, nil
	})

	if err := s.Walker.Walk(ctx, visitor, patterns...); err != nil {
		return nil, fmt.Errorf("scan failed during package walk: %w", err)
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ImportPath < packages[j].ImportPath
	})
	return packages, nil
}

// ScannerOption is a function that configures a Scanner.
type ScannerOption func(*Scanner) error

// WithWorkDir sets the working directory for the scanner.
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

// WithFS sets the filesystem implementation for the scanner and its components.
func WithFS(fs fs.FS) ScannerOption {
	return func(s *Scanner) error {
		s.FS = fs
		return nil
	}
}

// WithDryRun enables or disables dry-run mode.
func WithDryRun(dryRun bool) ScannerOption {
	return func(s *Scanner) error {
		s.DryRun = dryRun
		return nil
	}
}

// WithInspect enables or disables inspect mode.
func WithInspect(inspect bool) ScannerOption {
	return func(s *Scanner) error {
		s.Inspect = inspect
		return nil
	}
}

// WithLogger sets the logger for the scanner.
func WithLogger(logger *slog.Logger) ScannerOption {
	return func(s *Scanner) error {
		s.Logger = logger
		return nil
	}
}

// WithIncludeTests includes test files in the scan.
func WithIncludeTests(include bool) ScannerOption {
	return func(s *Scanner) error {
		s.IncludeTests = include
		return nil
	}
}

// WithGoModuleResolver enables the scanner to find packages in the Go module cache and GOROOT.
func WithGoModuleResolver() ScannerOption {
	return func(s *Scanner) error {
		s.useGoModuleResolver = true
		return nil
	}
}

// WithOverlay provides in-memory file content to the scanner.
func WithOverlay(overlay scanner.Overlay) ScannerOption {
	return func(s *Scanner) error {
		if s.overlay == nil {
			s.overlay = make(scanner.Overlay)
		}
		for k, v := range overlay {
			s.overlay[k] = v
		}
		return nil
	}
}

// WithExternalTypeOverrides sets the external type override map for the scanner.
func WithExternalTypeOverrides(overrides scanner.ExternalTypeOverride) ScannerOption {
	return func(s *Scanner) error {
		if s.ExternalTypeOverrides == nil {
			s.ExternalTypeOverrides = make(scanner.ExternalTypeOverride)
		}
		for k, v := range overrides {
			s.ExternalTypeOverrides[k] = v
		}
		return nil
	}
}

// New creates a new Scanner.
func New(options ...ScannerOption) (*Scanner, error) {
	cfg := &Config{
		fset:    token.NewFileSet(),
		overlay: make(scanner.Overlay),
	}

	s := &Scanner{
		Config:                cfg,
		packageCache:          make(map[string]*Package),
		visitedFiles:          make(map[string]struct{}),
		ExternalTypeOverrides: make(scanner.ExternalTypeOverride),
	}
	s.Walker = &ModuleWalker{
		Config:              cfg,
		packageImportsCache: make(map[string]*scanner.PackageImports),
	}

	for _, option := range options {
		if err := option(s); err != nil {
			return nil, err
		}
	}
	if s.FS == nil {
		s.FS = fs.NewOSFS()
	}

	if s.workDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
		s.workDir = cwd
	}

	locatorOpts := []locator.Option{
		locator.WithOverlay(s.overlay),
		locator.WithFS(s.FS),
	}
	if s.useGoModuleResolver {
		locatorOpts = append(locatorOpts, locator.WithGoModuleResolver())
	}

	loc, err := locator.New(s.workDir, locatorOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize locator: %w", err)
	}
	s.locator = loc

	initialScanner, err := scanner.New(s.fset, s.ExternalTypeOverrides, s.overlay, loc.ModulePath(), loc.RootDir(), s, s.Inspect, s.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal scanner: %w", err)
	}
	s.scanner = initialScanner

	return s, nil
}

// ResolvePath converts a file path to a full Go package path.
func ResolvePath(ctx context.Context, path string) (string, error) {
	return locator.ResolvePkgPath(ctx, path)
}

// SetExternalTypeOverrides sets the external type override map for the scanner.
func (s *Scanner) SetExternalTypeOverrides(ctx context.Context, overrides scanner.ExternalTypeOverride) {
	if overrides == nil {
		overrides = make(scanner.ExternalTypeOverride)
	}
	s.ExternalTypeOverrides = overrides
	newInternalScanner, err := scanner.New(s.fset, s.ExternalTypeOverrides, s.overlay, s.locator.ModulePath(), s.locator.RootDir(), s, s.Inspect, s.Logger)
	if err != nil {
		slog.WarnContext(ctx, "Failed to re-initialize internal scanner with new overrides. Continuing with previous scanner settings.", slog.Any("error", err))
		return
	}
	s.scanner = newInternalScanner
}

// ResolveType starts the type resolution process for a given field type.
func (s *Scanner) ResolveType(ctx context.Context, fieldType *scanner.FieldType) (*scanner.TypeInfo, error) {
	if s.scanner == nil {
		return nil, fmt.Errorf("internal scanner is not initialized")
	}
	if s.Logger != nil {
		ctx = context.WithValue(ctx, scanner.LoggerKey, s.Logger)
	}
	ctx = context.WithValue(ctx, scanner.InspectKey, s.Inspect)
	return s.scanner.ResolveType(ctx, fieldType)
}

// ScanPackage scans a single package at a given directory path.
func (s *Scanner) ScanPackage(ctx context.Context, pkgPath string) (*scanner.PackageInfo, error) {
	absPkgPath, err := filepath.Abs(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for package path %s: %w", pkgPath, err)
	}
	info, err := s.FS.Stat(absPkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not stat path %s: %w", absPkgPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path %s is not a directory", absPkgPath)
	}

	var importPath string
	if s.locator != nil {
		var err error
		importPath, err = s.locator.PathToImport(absPkgPath)
		if err != nil {
			return nil, fmt.Errorf("could not determine import path for directory %s: %w", absPkgPath, err)
		}
	} else {
		importPath = filepath.Base(absPkgPath)
	}

	allFilesInDir, err := s.Walker.listGoFiles(absPkgPath)
	if err != nil {
		return nil, fmt.Errorf("ScanPackage: could not list go files in %s: %w", absPkgPath, err)
	}

	var filesToParseNow []string
	s.mu.RLock()
	for _, fp := range allFilesInDir {
		if _, visited := s.visitedFiles[fp]; !visited {
			filesToParseNow = append(filesToParseNow, fp)
		}
	}
	s.mu.RUnlock()

	var currentCallPkgInfo *scanner.PackageInfo
	if len(filesToParseNow) > 0 {
		currentCallPkgInfo, err = s.scanner.ScanFiles(ctx, filesToParseNow, absPkgPath)
		if err != nil {
			return nil, fmt.Errorf("ScanPackage: internal scan of files for package %s failed: %w", absPkgPath, err)
		}
		if currentCallPkgInfo != nil {
			s.mu.Lock()
			for _, fp := range currentCallPkgInfo.Files {
				s.visitedFiles[fp] = struct{}{}
			}
			s.mu.Unlock()
			currentCallPkgInfo.ImportPath = importPath
			currentCallPkgInfo.Path = absPkgPath
			s.updateSymbolCacheWithPackageInfo(ctx, importPath, currentCallPkgInfo)
		}
	}

	if currentCallPkgInfo == nil {
		s.mu.RLock()
		existingCachedInfo, found := s.packageCache[importPath]
		s.mu.RUnlock()
		if found {
			return existingCachedInfo, nil
		}
		currentCallPkgInfo = &scanner.PackageInfo{
			Path:       absPkgPath,
			ImportPath: importPath,
			Name:       filepath.Base(absPkgPath),
			Fset:       s.fset,
			Files:      []string{},
		}
	}
	s.mu.Lock()
	s.packageCache[importPath] = currentCallPkgInfo
	s.mu.Unlock()

	return currentCallPkgInfo, nil
}

// resolveFilePath attempts to resolve a given path string into an absolute file path.
func (s *Scanner) resolveFilePath(rawPath string) (string, error) {
	checkFile := func(p string) (string, bool) {
		absP, err := filepath.Abs(p)
		if err != nil {
			return "", false
		}
		info, err := s.FS.Stat(absP)
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(absP), ".go") {
			return absP, true
		}
		return "", false
	}

	if absPath, ok := checkFile(rawPath); ok {
		return absPath, nil
	}

	if s.locator != nil {
		modulePath := s.locator.ModulePath()
		moduleRoot := s.locator.RootDir()
		if modulePath != "" && moduleRoot != "" && strings.HasPrefix(rawPath, modulePath) {
			prefixToTrim := modulePath
			if !strings.HasSuffix(modulePath, "/") && len(rawPath) > len(modulePath) && rawPath[len(modulePath)] == '/' {
				prefixToTrim += "/"
			} else if rawPath == modulePath {
				return "", fmt.Errorf("path %q is a module path, not a file path within the module", rawPath)
			}

			if strings.HasPrefix(rawPath, prefixToTrim) {
				suffixPath := strings.TrimPrefix(rawPath, prefixToTrim)
				candidatePath := filepath.Join(moduleRoot, suffixPath)
				if absPath, ok := checkFile(candidatePath); ok {
					return absPath, nil
				}
			}
		}
	}
	return "", fmt.Errorf("could not resolve path %q to an existing .go file", rawPath)
}

// ScanFiles scans a specified set of Go files.
func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string) (*scanner.PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no file paths provided to ScanFiles")
	}
	if s.locator == nil {
		return nil, fmt.Errorf("scanner locator is not initialized")
	}
	moduleRoot := s.locator.RootDir()
	modulePath := s.locator.ModulePath()
	if modulePath == "" && moduleRoot == "" {
		slog.WarnContext(ctx, "ScanFiles called likely outside a Go module context. Import path resolution will be affected.")
	} else if modulePath == "" || moduleRoot == "" {
		return nil, fmt.Errorf("module path or root is empty, ensure a go.mod file exists and is discoverable by the scanner's locator")
	}

	var resolvedAbsFilePaths []string
	var firstFileDir string

	for i, rawFp := range filePaths {
		absFp, err := s.resolveFilePath(rawFp)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve file path %q: %w", rawFp, err)
		}
		resolvedAbsFilePaths = append(resolvedAbsFilePaths, absFp)
		currentFileDir := filepath.Dir(absFp)
		if i == 0 {
			firstFileDir = currentFileDir
		} else if currentFileDir != firstFileDir {
			return nil, fmt.Errorf("all files must belong to the same directory (package); %s is in %s, but expected %s", absFp, currentFileDir, firstFileDir)
		}
	}

	pkgDirAbs := firstFileDir
	var importPath string

	if modulePath != "" && moduleRoot != "" {
		relPath, err := filepath.Rel(moduleRoot, pkgDirAbs)
		if err != nil {
			return nil, fmt.Errorf("could not determine relative path for %s from module root %s: %w", pkgDirAbs, moduleRoot, err)
		}
		if relPath == "." || relPath == "" {
			importPath = modulePath
		} else {
			importPath = filepath.ToSlash(filepath.Join(modulePath, relPath))
		}
	} else {
		importPath = filepath.ToSlash(pkgDirAbs)
		slog.WarnContext(ctx, "Creating pseudo import path for package", slog.String("import_path", importPath), slog.String("package_dir", pkgDirAbs))
	}

	var filesToParse []string
	s.mu.RLock()
	for _, absFp := range resolvedAbsFilePaths {
		if _, visited := s.visitedFiles[absFp]; !visited {
			filesToParse = append(filesToParse, absFp)
		}
	}
	s.mu.RUnlock()

	if len(filesToParse) == 0 {
		return &scanner.PackageInfo{
			Path:       pkgDirAbs,
			ImportPath: importPath,
			Name:       "",
			Fset:       s.fset,
			Files:      []string{},
		}, nil
	}

	pkgInfo, err := s.scanner.ScanFiles(ctx, filesToParse, pkgDirAbs)
	if err != nil {
		return nil, fmt.Errorf("failed to scan files in %s (import path %s): %w", pkgDirAbs, importPath, err)
	}

	if pkgInfo != nil {
		pkgInfo.ImportPath = importPath
		pkgInfo.Path = pkgDirAbs
		s.mu.Lock()
		for _, fp := range pkgInfo.Files {
			s.visitedFiles[fp] = struct{}{}
		}
		s.mu.Unlock()
		s.updateSymbolCacheWithPackageInfo(ctx, importPath, pkgInfo)
	}
	return pkgInfo, nil
}

// UnscannedGoFiles returns a list of absolute paths to unscanned .go files.
func (s *Scanner) UnscannedGoFiles(packagePathOrImportPath string) ([]string, error) {
	if s.locator == nil && !(filepath.IsAbs(packagePathOrImportPath) && isDir(s, packagePathOrImportPath)) {
		return nil, fmt.Errorf("scanner locator is not initialized, and path is not an absolute directory to a package")
	}

	var pkgDirAbs string
	var err error

	pathAsDir, err := filepath.Abs(packagePathOrImportPath)
	if err == nil {
		info, statErr := s.FS.Stat(pathAsDir)
		if statErr == nil && info.IsDir() {
			pkgDirAbs = pathAsDir
		}
	}

	if pkgDirAbs == "" {
		if s.locator == nil {
			return nil, fmt.Errorf("cannot resolve %q as import path: locator not available", packagePathOrImportPath)
		}
		pkgDirAbs, err = s.locator.FindPackageDir(packagePathOrImportPath)
		if err != nil {
			return nil, fmt.Errorf("could not find package directory for %q (tried as path and import path): %w", packagePathOrImportPath, err)
		}
	}

	allGoFilesInDir, err := s.Walker.listGoFiles(pkgDirAbs)
	if err != nil {
		return nil, fmt.Errorf("UnscannedGoFiles: could not list go files in %s: %w", pkgDirAbs, err)
	}

	var unscannedFiles []string
	s.mu.RLock()
	for _, absFilePath := range allGoFilesInDir {
		if _, visited := s.visitedFiles[absFilePath]; !visited {
			unscannedFiles = append(unscannedFiles, absFilePath)
		}
	}
	s.mu.RUnlock()
	return unscannedFiles, nil
}

func isDir(s *Scanner, path string) bool {
	info, err := s.FS.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ScanPackageByImport scans a single Go package identified by its import path.
func (s *Scanner) ScanPackageByImport(ctx context.Context, importPath string) (*scanner.PackageInfo, error) {
	s.mu.RLock()
	cachedPkg, found := s.packageCache[importPath]
	s.mu.RUnlock()
	if found {
		slog.DebugContext(ctx, "ScanPackageByImport CACHE HIT", slog.String("importPath", importPath), slog.Int("types", len(cachedPkg.Types)))
		return cachedPkg, nil
	}
	slog.DebugContext(ctx, "ScanPackageByImport CACHE MISS", slog.String("importPath", importPath))

	pkgDirAbs, err := s.locator.FindPackageDir(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not find directory for import path %s: %w", importPath, err)
	}
	slog.DebugContext(ctx, "ScanPackageByImport resolved import path", slog.String("importPath", importPath), slog.String("pkgDirAbs", pkgDirAbs))

	allGoFilesInPkg, err := s.Walker.listGoFiles(pkgDirAbs)
	if err != nil {
		return nil, fmt.Errorf("ScanPackageByImport: failed to list go files in %s: %w", pkgDirAbs, err)
	}
	slog.DebugContext(ctx, "ScanPackageByImport found .go files", slog.Int("count", len(allGoFilesInPkg)), slog.String("pkgDirAbs", pkgDirAbs), slog.Any("files", allGoFilesInPkg))

	if len(allGoFilesInPkg) == 0 {
		slog.DebugContext(ctx, "ScanPackageByImport found no .go files. Caching empty PackageInfo.", slog.String("pkgDirAbs", pkgDirAbs))
		pkgInfo := &scanner.PackageInfo{Path: pkgDirAbs, ImportPath: importPath, Name: "", Fset: s.fset, Files: []string{}, Types: []*scanner.TypeInfo{}}
		s.mu.Lock()
		s.packageCache[importPath] = pkgInfo
		s.mu.Unlock()
		return pkgInfo, nil
	}

	var filesToParseThisCall []string
	symCache, _ := s.getOrCreateSymbolCache(ctx)
	slog.DebugContext(ctx, "ScanPackageByImport symbol cache status", slog.String("importPath", importPath), slog.Bool("enabled", symCache != nil && symCache.isEnabled()))

	filesConsideredBySymCache := make(map[string]struct{})

	if symCache != nil && symCache.isEnabled() {
		newDiskFiles, existingDiskFiles, errSym := symCache.getFilesToScan(ctx, pkgDirAbs)
		if errSym != nil {
			slog.WarnContext(ctx, "getFilesToScan failed. Will scan all unvisited files in the package.", slog.String("import_path", importPath), slog.String("package_dir", pkgDirAbs), slog.Any("error", errSym))
			s.mu.RLock()
			for _, f := range allGoFilesInPkg {
				if _, visited := s.visitedFiles[f]; !visited {
					filesToParseThisCall = append(filesToParseThisCall, f)
				}
			}
			s.mu.RUnlock()
		} else {
			for _, f := range newDiskFiles {
				filesToParseThisCall = append(filesToParseThisCall, f)
				filesConsideredBySymCache[f] = struct{}{}
			}
			s.mu.RLock()
			for _, f := range existingDiskFiles {
				filesConsideredBySymCache[f] = struct{}{}
				if _, visited := s.visitedFiles[f]; !visited {
					filesToParseThisCall = append(filesToParseThisCall, f)
				}
			}
			s.mu.RUnlock()
		}
	}

	s.mu.RLock()
	for _, f := range allGoFilesInPkg {
		if _, considered := filesConsideredBySymCache[f]; !considered {
			if _, visited := s.visitedFiles[f]; !visited {
				filesToParseThisCall = append(filesToParseThisCall, f)
			}
		}
	}
	s.mu.RUnlock()

	uniqueFilesToParse := make(map[string]struct{})
	var dedupedFilesToParse []string
	for _, f := range filesToParseThisCall {
		if _, exists := uniqueFilesToParse[f]; !exists {
			uniqueFilesToParse[f] = struct{}{}
			dedupedFilesToParse = append(dedupedFilesToParse, f)
		}
	}
	filesToParseThisCall = dedupedFilesToParse

	var currentCallPkgInfo *scanner.PackageInfo
	if len(filesToParseThisCall) > 0 {
		isStdLib := !strings.Contains(importPath, ".")

		if isStdLib {
			currentCallPkgInfo, err = s.scanner.ScanFilesWithKnownImportPath(ctx, filesToParseThisCall, pkgDirAbs, importPath)
		} else {
			currentCallPkgInfo, err = s.scanner.ScanFiles(ctx, filesToParseThisCall, pkgDirAbs)
		}

		if err != nil {
			return nil, fmt.Errorf("ScanPackageByImport: scanning files for %s failed: %w", importPath, err)
		}

		if currentCallPkgInfo != nil {
			currentCallPkgInfo.ImportPath = importPath
			currentCallPkgInfo.Path = pkgDirAbs
			s.mu.Lock()
			for _, fp := range currentCallPkgInfo.Files {
				s.visitedFiles[fp] = struct{}{}
			}
			s.mu.Unlock()
			s.updateSymbolCacheWithPackageInfo(ctx, importPath, currentCallPkgInfo)
		}
	}

	if currentCallPkgInfo == nil {
		currentCallPkgInfo = &scanner.PackageInfo{
			Path:       pkgDirAbs,
			ImportPath: importPath,
			Name:       "",
			Fset:       s.fset,
			Files:      []string{},
		}
		s.mu.RLock()
		if prevInfo, ok := s.packageCache[importPath]; ok && prevInfo.Name != "" {
			currentCallPkgInfo.Name = prevInfo.Name
		} else if len(allGoFilesInPkg) > 0 {
		}
		s.mu.RUnlock()
	}
	s.mu.Lock()
	s.packageCache[importPath] = currentCallPkgInfo
	s.mu.Unlock()

	return currentCallPkgInfo, nil
}

// getOrCreateSymbolCache ensures the symbolCache is initialized.
func (s *Scanner) getOrCreateSymbolCache(ctx context.Context) (*symbolCache, error) {
	if s.CachePath == "" {
		if s.symbolCache == nil || s.symbolCache.isEnabled() {
			rootDir := ""
			if s.locator != nil {
				rootDir = s.locator.RootDir()
			}
			disabledCache, err := newSymbolCache(rootDir, "")
			if err != nil {
				return nil, fmt.Errorf("failed to initialize a disabled symbol cache: %w", err)
			}
			s.symbolCache = disabledCache
		}
		return s.symbolCache, nil
	}

	if s.symbolCache != nil && s.symbolCache.isEnabled() && s.symbolCache.getFilePath() == s.CachePath {
		return s.symbolCache, nil
	}

	rootDir := ""
	if s.locator != nil {
		rootDir = s.locator.RootDir()
	} else {
		return nil, fmt.Errorf("scanner locator is not initialized, cannot determine root directory for cache")
	}

	sc, err := newSymbolCache(rootDir, s.CachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize symbol cache with path %s: %w", s.CachePath, err)
	}
	s.symbolCache = sc

	if err := s.symbolCache.load(ctx); err != nil {
		slog.WarnContext(ctx, "Could not load symbol cache", slog.String("path", s.symbolCache.getFilePath()), slog.Any("error", err))
	}
	return s.symbolCache, nil
}

// updateSymbolCacheWithPackageInfo updates the symbol cache with information from a given PackageInfo.
func (s *Scanner) updateSymbolCacheWithPackageInfo(ctx context.Context, importPath string, pkgInfo *scanner.PackageInfo) {
	if s.CachePath == "" || pkgInfo == nil || len(pkgInfo.Files) == 0 {
		return
	}
	symCache, err := s.getOrCreateSymbolCache(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Error getting symbol cache for update", slog.Any("error", err))
		return
	}
	if !symCache.isEnabled() {
		return
	}

	symbolsByFile := make(map[string][]string)
	addSymbol := func(symbolName, absFilePath string) {
		if symbolName != "" && absFilePath != "" {
			absFilePath, _ = filepath.Abs(absFilePath)
			key := importPath + "." + symbolName
			if err := symCache.setSymbol(key, absFilePath); err != nil {
				slog.ErrorContext(ctx, "Error setting cache for symbol", slog.String("symbol_key", key), slog.Any("error", err))
			}
			symbolsByFile[absFilePath] = append(symbolsByFile[absFilePath], symbolName)
		}
	}

	for _, typeInfo := range pkgInfo.Types {
		addSymbol(typeInfo.Name, typeInfo.FilePath)
	}
	for _, funcInfo := range pkgInfo.Functions {
		addSymbol(funcInfo.Name, funcInfo.FilePath)
	}
	for _, constInfo := range pkgInfo.Constants {
		addSymbol(constInfo.Name, constInfo.FilePath)
	}

	for _, absFilePath := range pkgInfo.Files {
		absFilePath, _ = filepath.Abs(absFilePath)
		if _, err := s.FS.Stat(absFilePath); os.IsNotExist(err) {
			slog.WarnContext(ctx, "File from pkgInfo.Files not found, skipping for fileMetadata update", slog.String("file", absFilePath))
			continue
		}
		fileSymbols := symbolsByFile[absFilePath]
		if fileSymbols == nil {
			fileSymbols = []string{}
		}
		metadata := fileMetadata{Symbols: fileSymbols}
		if err := symCache.setFileMetadata(absFilePath, metadata); err != nil {
			slog.ErrorContext(ctx, "Error setting file metadata", slog.String("file", absFilePath), slog.Any("error", err))
		}
	}
}

// SaveSymbolCache saves the symbol cache to disk if CachePath is set.
func (s *Scanner) SaveSymbolCache(ctx context.Context) error {
	if s.CachePath == "" {
		return nil
	}
	if _, err := s.getOrCreateSymbolCache(ctx); err != nil {
		return fmt.Errorf("cannot save symbol cache, failed to ensure cache initialization for path %s: %w", s.CachePath, err)
	}
	if s.symbolCache != nil && s.symbolCache.isEnabled() {
		if err := s.symbolCache.save(); err != nil {
			return fmt.Errorf("failed to save symbol cache to %s: %w", s.symbolCache.getFilePath(), err)
		}
	}
	return nil
}

// ListExportedSymbols scans a package by its import path and returns a list of all its exported symbols.
func (s *Scanner) ListExportedSymbols(ctx context.Context, pkgPath string) ([]string, error) {
	pkgInfo, err := s.ScanPackageByImport(ctx, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package %s: %w", pkgPath, err)
	}

	var exportedSymbols []string

	for _, f := range pkgInfo.Functions {
		if f.Receiver == nil && f.AstDecl != nil && f.AstDecl.Name != nil && f.AstDecl.Name.IsExported() {
			exportedSymbols = append(exportedSymbols, f.Name)
		}
	}

	for _, t := range pkgInfo.Types {
		if typeSpec, ok := t.Node.(*ast.TypeSpec); ok {
			if typeSpec.Name != nil && typeSpec.Name.IsExported() {
				exportedSymbols = append(exportedSymbols, t.Name)
			}
		}
	}

	for _, c := range pkgInfo.Constants {
		if c.IsExported {
			exportedSymbols = append(exportedSymbols, c.Name)
		}
	}

	sort.Strings(exportedSymbols)
	return exportedSymbols, nil
}

// FindSymbolDefinitionLocation attempts to find the absolute file path where a given symbol is defined.
func (s *Scanner) FindSymbolDefinitionLocation(ctx context.Context, symbolFullName string) (string, error) {
	lastDot := strings.LastIndex(symbolFullName, ".")
	if lastDot == -1 || lastDot == 0 || lastDot == len(symbolFullName)-1 {
		return "", fmt.Errorf("invalid symbol full name format: %q. Expected 'package/import/path.SymbolName'", symbolFullName)
	}
	importPath := symbolFullName[:lastDot]
	symbolName := symbolFullName[lastDot+1:]
	cacheKey := importPath + "." + symbolName

	if s.CachePath != "" {
		symCache, err := s.getOrCreateSymbolCache(ctx)
		if err != nil {
			slog.WarnContext(ctx, "Could not get symbol cache. Proceeding with full scan.", slog.String("symbol", symbolFullName), slog.Any("error", err))
		} else if symCache != nil && symCache.isEnabled() {
			filePath, found := symCache.verifyAndGet(ctx, cacheKey)
			if found {
				return filePath, nil
			}
		}
	}
	pkgInfo, err := s.ScanPackageByImport(ctx, importPath)
	if err != nil {
		return "", fmt.Errorf("scan for package %s (for symbol %s) failed: %w", importPath, symbolName, err)
	}

	if s.CachePath != "" {
		if s.symbolCache != nil && s.symbolCache.isEnabled() {
			filePath, found := s.symbolCache.get(cacheKey)
			if found {
				if _, statErr := s.FS.Stat(filePath); statErr == nil {
					return filePath, nil
				}
				slog.WarnContext(ctx, "Symbol found in cache after scan, but file does not exist.", slog.String("symbol", symbolFullName), slog.String("path", filePath))
			}
		}
	}

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
			if _, statErr := s.FS.Stat(targetFilePath); statErr == nil {
				return targetFilePath, nil
			}
			return "", fmt.Errorf("symbol %s found in package %s at %s by scan, but file does not exist", symbolName, importPath, targetFilePath)
		}
	}

	return "", fmt.Errorf("symbol %s not found in package %s even after scan and cache check", symbolName, importPath)
}

// FindSymbolInPackage searches for a specific symbol within a package by scanning its files one by one.
func (s *Scanner) FindSymbolInPackage(ctx context.Context, importPath string, symbolName string) (*scanner.PackageInfo, error) {
	unscannedFiles, err := s.UnscannedGoFiles(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not get unscanned files for package %s: %w", importPath, err)
	}

	isStdLib := !strings.Contains(importPath, ".")
	var pkgDirAbs string
	if isStdLib {
		pkgDirAbs, err = s.locator.FindPackageDir(importPath)
		if err != nil {
			return nil, fmt.Errorf("could not find directory for stdlib import path %s: %w", importPath, err)
		}
	}

	var cumulativePkgInfo *scanner.PackageInfo

	for _, fileToScan := range unscannedFiles {
		var pkgInfo *scanner.PackageInfo
		var scanErr error

		if isStdLib {
			pkgInfo, scanErr = s.scanner.ScanFilesWithKnownImportPath(ctx, []string{fileToScan}, pkgDirAbs, importPath)
			if scanErr == nil && pkgInfo != nil {
				s.mu.Lock()
				for _, fp := range pkgInfo.Files {
					s.visitedFiles[fp] = struct{}{}
				}
				s.mu.Unlock()
				s.updateSymbolCacheWithPackageInfo(ctx, importPath, pkgInfo)
			}
		} else {
			pkgInfo, scanErr = s.ScanFiles(ctx, []string{fileToScan})
		}

		if scanErr != nil {
			slog.WarnContext(ctx, "failed to scan file while searching for symbol", "file", fileToScan, "symbol", symbolName, "error", scanErr)
			continue
		}

		if pkgInfo == nil {
			continue
		}
		if cumulativePkgInfo == nil {
			cumulativePkgInfo = pkgInfo
		} else {
			cumulativePkgInfo.Types = append(cumulativePkgInfo.Types, pkgInfo.Types...)
			cumulativePkgInfo.Functions = append(cumulativePkgInfo.Functions, pkgInfo.Functions...)
			cumulativePkgInfo.Constants = append(cumulativePkgInfo.Constants, pkgInfo.Constants...)
			if cumulativePkgInfo.AstFiles == nil {
				cumulativePkgInfo.AstFiles = make(map[string]*ast.File)
			}
			for path, ast := range pkgInfo.AstFiles {
				if _, exists := cumulativePkgInfo.AstFiles[path]; !exists {
					cumulativePkgInfo.AstFiles[path] = ast
				}
			}
			existingFiles := make(map[string]struct{}, len(cumulativePkgInfo.Files))
			for _, f := range cumulativePkgInfo.Files {
				existingFiles[f] = struct{}{}
			}
			for _, f := range pkgInfo.Files {
				if _, exists := existingFiles[f]; !exists {
					cumulativePkgInfo.Files = append(cumulativePkgInfo.Files, f)
					existingFiles[f] = struct{}{}
				}
			}
		}

	}

	if cumulativePkgInfo == nil {
		return nil, fmt.Errorf("no unscanned files found and symbol %q not in cache for package %q", symbolName, importPath)
	}
	for _, t := range cumulativePkgInfo.Types {
		if t.Name == symbolName {
			return cumulativePkgInfo, nil
		}
	}
	for _, f := range cumulativePkgInfo.Functions {
		if f.Name == symbolName {
			return cumulativePkgInfo, nil
		}
	}
	for _, c := range cumulativePkgInfo.Constants {
		if c.Name == symbolName {
			return cumulativePkgInfo, nil
		}
	}

	return nil, fmt.Errorf("symbol %q not found in package %q", symbolName, importPath)
}
