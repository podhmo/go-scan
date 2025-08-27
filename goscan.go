package goscan

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Package is an alias for scanner.PackageInfo, representing all the extracted information from a single package.
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

// Re-export scanner kinds for convenience.
const (
	StructKind    = scanner.StructKind
	AliasKind     = scanner.AliasKind
	FuncKind      = scanner.FuncKind
	InterfaceKind = scanner.InterfaceKind // Ensure InterfaceKind is available
)

// Scanner is the main entry point for the type scanning library.
// It combines a locator for finding packages, a scanner for parsing them,
// and caches for improving performance over multiple calls.
// Scanner instances are stateful regarding which files have been visited (parsed).
type Scanner struct {
	*Config
	packageCache          map[string]*Package // Cache for PackageInfo from ScanPackage/ScanPackageByImport, key is import path
	visitedFiles          map[string]struct{} // Set of visited (parsed) file absolute paths for this Scanner instance.
	mu                    sync.RWMutex
	CachePath             string
	symbolCache           *symbolCache // Symbol cache (persisted across Scanner instances if path is reused)
	ExternalTypeOverrides scanner.ExternalTypeOverride

	// Walker is responsible for lightweight, dependency-focused scanning operations.
	Walker *ModuleWalker

	// For multi-module workspace support
	isWorkspace bool
	locators    []*locator.Locator
	moduleDirs  []string // temporary holder for module directories
}

// Fset returns the FileSet associated with the scanner.
func (s *Scanner) Fset() *token.FileSet {
	return s.fset
}

// Locator returns the primary locator instance. In workspace mode, this is the
// locator for the first module, which might not be appropriate for all operations.
// Use `locatorForImportPath` for path-specific lookups in workspace mode.
func (s *Scanner) Locator() *locator.Locator {
	if s.isWorkspace && len(s.locators) > 0 {
		return s.locators[0]
	}
	return s.locator
}

// IsWorkspace returns true if the scanner is configured for multi-module mode.
func (s *Scanner) IsWorkspace() bool {
	return s.isWorkspace
}

// ModuleRoots returns the root directories of all modules in the workspace.
func (s *Scanner) ModuleRoots() []string {
	if !s.isWorkspace {
		if s.locator != nil {
			return []string{s.locator.RootDir()}
		}
		return nil
	}
	roots := make([]string, len(s.locators))
	for i, loc := range s.locators {
		roots[i] = loc.RootDir()
	}
	return roots
}

// locatorForImportPath finds the correct locator for a given import path in workspace mode.
func (s *Scanner) locatorForImportPath(importPath string) (*locator.Locator, error) {
	if !s.isWorkspace {
		return s.locator, nil
	}

	var bestMatch *locator.Locator
	var bestMatchLen int

	for _, loc := range s.locators {
		modulePath := loc.ModulePath()
		// Check for exact match or if the import path is a sub-package of the module.
		if importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/") {
			if len(modulePath) > bestMatchLen {
				bestMatch = loc
				bestMatchLen = len(modulePath)
			}
		}
	}

	if bestMatch != nil {
		return bestMatch, nil
	}

	// Fallback for standard library packages: any locator can find them.
	if !strings.Contains(importPath, ".") {
		if len(s.locators) > 0 {
			return s.locators[0], nil
		}
	}

	return nil, fmt.Errorf("could not find a module responsible for import path %q in workspace", importPath)
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
// TODO: Refactor evaluator to use the top-level goscan.Scanner instead.
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

// Scan scans Go packages based on the provided patterns.
// Each pattern can be a directory path or a file path relative to the scanner's workDir.
// It returns a list of scanned packages.
func (s *Scanner) Scan(ctx context.Context, patterns ...string) ([]*Package, error) {
	pkgsMap := make(map[string]*Package) // Use map to handle duplicates

	for _, pattern := range patterns {
		if strings.Contains(pattern, "...") {
			// Handle wildcard pattern
			basePath := strings.TrimSuffix(pattern, "/...")
			var absBasePath string

			if filepath.IsAbs(basePath) {
				absBasePath = basePath
			} else if strings.HasPrefix(basePath, ".") {
				absBasePath = filepath.Join(s.workDir, basePath)
			} else {
				var err error
				absBasePath, err = s.locator.FindPackageDir(basePath)
				if err != nil {
					return nil, fmt.Errorf("could not find directory for import path pattern %q: %w", pattern, err)
				}
			}

			walkErr := filepath.WalkDir(absBasePath, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					return nil
				}
				// Check if the directory contains any .go files.
				entries, err := os.ReadDir(path)
				if err != nil {
					// Log and continue. Don't let permission errors stop the whole walk.
					slog.DebugContext(ctx, "cannot read directory during walk, skipping", "path", path, "error", err)
					return nil
				}
				hasGoFiles := false
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
						hasGoFiles = true
						break
					}
				}

				if hasGoFiles {
					pkg, err := s.ScanPackage(ctx, path)
					if err != nil {
						// Log error but continue walking
						slog.WarnContext(ctx, "failed to scan package during wildcard walk", "path", path, "error", err)
						return nil
					}
					if pkg != nil && pkg.ImportPath != "" {
						pkgsMap[pkg.ImportPath] = pkg
					}
				}
				return nil
			})

			if walkErr != nil {
				return nil, fmt.Errorf("error walking directory for pattern %q: %w", pattern, walkErr)
			}
		} else {
			// Handle single file or directory pattern (legacy behavior)
			absPath := pattern
			if !filepath.IsAbs(pattern) {
				absPath = filepath.Join(s.workDir, pattern)
			}

			info, err := os.Stat(absPath)
			if err != nil {
				return nil, fmt.Errorf("could not stat pattern %q (resolved to %q): %w", pattern, absPath, err)
			}

			var pkg *Package
			if info.IsDir() {
				pkg, err = s.ScanPackage(ctx, absPath)
			} else {
				pkg, err = s.ScanFiles(ctx, []string{absPath})
			}

			if err != nil {
				return nil, fmt.Errorf("failed to scan path %q: %w", absPath, err)
			}
			if pkg != nil && pkg.ImportPath != "" {
				pkgsMap[pkg.ImportPath] = pkg
			}
		}
	}

	// Convert map to slice
	pkgs := make([]*Package, 0, len(pkgsMap))
	for _, pkg := range pkgsMap {
		pkgs = append(pkgs, pkg)
	}
	// Sort for deterministic output
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].ImportPath < pkgs[j].ImportPath
	})
	return pkgs, nil
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

// WithModuleDirs configures the scanner to operate in workspace mode over a set of modules.
// It stores the directories, and the actual locator initialization happens in `New`.
func WithModuleDirs(moduleDirs []string) ScannerOption {
	return func(s *Scanner) error {
		if len(moduleDirs) > 0 {
			s.isWorkspace = true
			s.moduleDirs = moduleDirs
		}
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

// WithPackageLoadHook sets a hook that is called before a package is scanned.
// If the hook returns false, the package is not scanned and a placeholder is returned.
func WithPackageLoadHook(hook PackageLoadHook) ScannerOption {
	return func(s *Scanner) error {
		s.Config.PackageLoadHook = hook
		return nil
	}
}

// New creates a new Scanner. It finds the module root starting from the given path.
// It also initializes an empty set of visited files for this scanner instance.
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
		Walker: &ModuleWalker{
			Config:              cfg,
			packageImportsCache: make(map[string]*scanner.PackageImports),
		},
	}

	for _, option := range options {
		if err := option(s); err != nil {
			return nil, err
		}
	}

	locatorOpts := []locator.Option{locator.WithOverlay(s.overlay)}
	if s.useGoModuleResolver {
		locatorOpts = append(locatorOpts, locator.WithGoModuleResolver())
	}

	if s.isWorkspace {
		if len(s.moduleDirs) == 0 {
			return nil, fmt.Errorf("scanner is in workspace mode but no module directories were provided")
		}
		s.locators = make([]*locator.Locator, len(s.moduleDirs))
		for i, dir := range s.moduleDirs {
			loc, err := locator.New(dir, locatorOpts...)
			if err != nil {
				return nil, fmt.Errorf("workspace mode: failed to create locator for module %q: %w", dir, err)
			}
			s.locators[i] = loc
		}
		// Set the primary locator to the first one.
		s.locator = s.locators[0]
		s.workDir = s.locator.RootDir() // Set workDir to the first module's root.
	} else {
		if s.workDir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getwd: %w", err)
			}
			s.workDir = cwd
		}
		loc, err := locator.New(s.workDir, locatorOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize locator: %w", err)
		}
		s.locator = loc
	}

	// The internal scanner needs a module path and root dir to initialize.
	// In workspace mode, we use the primary (first) locator's info.
	// This is a slight simplification, but the internal scanner's primary role
	// is parsing, and type resolution logic will use the full workspace-aware Scanner.
	initialScanner, err := scanner.New(s.fset, s.ExternalTypeOverrides, s.overlay, s.locator.ModulePath(), s.locator.RootDir(), s, s.Inspect, s.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal scanner: %w", err)
	}
	s.scanner = initialScanner

	return s, nil
}

// ResolvePath converts a file path to a full Go package path.
// If the path exists on the filesystem, it is treated as a file path and resolved against
// the module's go.mod file. If it does not exist, it is assumed to be a package path
// and is returned as-is, unless it has a relative path prefix (like `./`), in which
// case an error is returned.
//
// This function is a facade for `locator.ResolvePkgPath`.
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
// It's the public entry point for resolving types. It prepares the context
// with necessary loggers and flags for the entire resolution chain.
func (s *Scanner) ResolveType(ctx context.Context, fieldType *scanner.FieldType) (*scanner.TypeInfo, error) {
	if s.scanner == nil {
		return nil, fmt.Errorf("internal scanner is not initialized")
	}

	// Prepare the context for the entire resolution chain starting from this call.
	if s.Logger != nil {
		ctx = context.WithValue(ctx, scanner.LoggerKey, s.Logger)
	}
	ctx = context.WithValue(ctx, scanner.InspectKey, s.Inspect)

	// This delegates to the internal scanner's ResolveType, which now handles
	// the creation of the initial resolution path.
	return s.scanner.ResolveType(ctx, fieldType)
}

// listGoFiles lists all .go files in a directory.
// If includeTests is false, it excludes _test.go files.
// It returns a list of absolute file paths.
func listGoFiles(dirPath string, includeTests bool) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("listGoFiles: failed to read dir %s: %w", dirPath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if !includeTests && strings.HasSuffix(name, "_test.go") {
			continue
		}

		absPath, err := filepath.Abs(filepath.Join(dirPath, name))
		if err != nil {
			return nil, fmt.Errorf("listGoFiles: could not get absolute path for %s: %w", name, err)
		}
		files = append(files, absPath)
	}
	return files, nil
}

// ScanPackage scans a single package at a given directory path (absolute or relative to CWD).
// It parses all .go files (excluding _test.go) in that directory that have not yet been
// visited (parsed) by this Scanner instance.
// The returned PackageInfo contains information derived ONLY from the files parsed in THIS specific call.
// If no unvisited files are found in the package, the returned PackageInfo will be minimal
// (e.g., Path and ImportPath set, but no types/functions unless a previous cached version for the entire package is returned).
// The result of this call (representing the newly parsed files, or a prior cached full result if no new files were parsed and cache existed)
// is stored in an in-memory package cache (s.packageCache) for subsequent calls to ScanPackage or ScanPackageByImport
// for the same import path.
// The global symbol cache (s.symbolCache), if enabled, is updated with symbols from the newly parsed files.
func (s *Scanner) ScanPackage(ctx context.Context, pkgPath string) (*scanner.PackageInfo, error) {
	absPkgPath, err := filepath.Abs(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for package path %s: %w", pkgPath, err)
	}
	info, err := os.Stat(absPkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not stat path %s: %w", absPkgPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path %s is not a directory", absPkgPath)
	}

	moduleRoot := s.locator.RootDir()
	modulePath := s.locator.ModulePath()
	var importPath string

	if modulePath != "" && moduleRoot != "" && strings.HasPrefix(absPkgPath, moduleRoot) {
		relPath, rErr := filepath.Rel(moduleRoot, absPkgPath)
		if rErr != nil {
			return nil, fmt.Errorf("could not determine relative path for %s from module root %s: %w", absPkgPath, moduleRoot, rErr)
		}
		if relPath == "." || relPath == "" {
			importPath = modulePath
		} else {
			importPath = filepath.ToSlash(filepath.Join(modulePath, relPath))
		}
	} else {
		// Try to determine import path for standard library packages or other non-module paths
		// This part might be complex and require go list or similar logic for full accuracy.
		// For now, if not in module, we might not be able to form a canonical import path.
		// However, ScanPackage is often called with a direct path, so importPath might be less critical
		// than for ScanPackageByImport. Let's use the directory name as a fallback package name.
		// If a robust import path is needed for out-of-module packages, this needs enhancement.
		if modulePath == "" && moduleRoot == "" { // Likely not in a module context
			slog.WarnContext(ctx, "ScanPackage called for path likely outside a Go module, import path may be inaccurate.", slog.String("path", absPkgPath))
			importPath = filepath.Base(absPkgPath) // Fallback
		} else if modulePath == "" { // Locator initialized but no go.mod?
			return nil, fmt.Errorf("module path is empty, but ScanPackage called for %s. Locator issue or not in module?", absPkgPath)
		}
		// The original `else` block here was too restrictive. It prevented scanning
		// packages resolved via a `replace` directive that points outside the main
		// module's root directory. The locator is responsible for finding the correct
		// directory for an import path, and `go-scan` should trust that result.
		// A new mechanism is needed to get the import path for a directory that
		// might be outside the module root, but we should not fail here.
		// For now, we'll try to find the import path via the locator again.
		if importPath == "" {
			var err error
			importPath, err = s.locator.PathToImport(absPkgPath)
			if err != nil {
				return nil, fmt.Errorf("could not determine import path for directory %s: %w", absPkgPath, err)
			}
		}
	}

	allFilesInDir, err := listGoFiles(absPkgPath, s.IncludeTests)
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
			for _, fp := range currentCallPkgInfo.Files { // Files actually parsed in this call
				s.visitedFiles[fp] = struct{}{}
			}
			s.mu.Unlock()
			currentCallPkgInfo.ImportPath = importPath // Set import path for this call's result
			currentCallPkgInfo.Path = absPkgPath       // Ensure path is set
			s.updateSymbolCacheWithPackageInfo(ctx, importPath, currentCallPkgInfo)
		}
	}

	// Update the main package cache with the cumulative information for this importPath.
	// This requires merging if a previous entry existed. For now, replace.
	// A more robust strategy might involve storing all PackageInfo from each scan call and merging on demand.
	// For now, the cache will store the result of the latest ScanPackage or ScanPackageByImport call.
	// If no new files were parsed, currentCallPkgInfo will be nil.
	// We should ensure a PackageInfo object is always cached if the package itself is valid (even if empty of new symbols).
	if currentCallPkgInfo == nil { // No new files parsed
		s.mu.RLock()
		existingCachedInfo, found := s.packageCache[importPath]
		s.mu.RUnlock()
		if found {
			return existingCachedInfo, nil // Return existing full cache if nothing new parsed
		}
		// If no cache and no new files, create a minimal PackageInfo
		currentCallPkgInfo = &scanner.PackageInfo{
			Path:       absPkgPath,
			ImportPath: importPath,
			Name:       filepath.Base(absPkgPath), // Best guess for name
			Fset:       s.fset,
			Files:      []string{}, // No files parsed in *this call*
		}
	}

	// Ensure the PackageInfo reflects all known files in the directory for its Files list if it's a full ScanPackage result
	// This is tricky without merging. The current `currentCallPkgInfo.Files` only has *newly* parsed files.
	// For ScanPackage, the expectation is often a view of the whole package.
	// Let's adjust: if currentCallPkgInfo was non-nil (new files parsed), its .Files is correct for *this scan*.
	// If we are to cache a "full" view, we'd need to merge or reconstruct.
	// Given "no merge" for ScanFiles, let's keep ScanPackage simple: its return and cache reflect *this call's parsed files*.
	// This means s.packageCache might hold partial info if ScanPackage is called after ScanFiles visited some.
	// This seems to align with the "no merge" philosophy more consistently.
	// The `Files` field of PackageInfo will list files parsed in *this specific call*.

	s.mu.Lock()
	s.packageCache[importPath] = currentCallPkgInfo // Cache the result of this specific call
	s.mu.Unlock()

	return currentCallPkgInfo, nil
}

// resolveFilePath attempts to resolve a given path string (rawPath) into an absolute file path.
func (s *Scanner) resolveFilePath(rawPath string) (string, error) {
	checkFile := func(p string) (string, bool) {
		absP, err := filepath.Abs(p)
		if err != nil {
			return "", false
		}
		info, err := os.Stat(absP)
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(absP), ".go") { // Check .go case-insensitively for robustness
			return absP, true
		}
		return "", false
	}

	// Try as absolute or CWD-relative path first
	if absPath, ok := checkFile(rawPath); ok {
		return absPath, nil
	}

	// Try as module-qualified path
	if s.locator != nil {
		modulePath := s.locator.ModulePath()
		moduleRoot := s.locator.RootDir()
		if modulePath != "" && moduleRoot != "" && strings.HasPrefix(rawPath, modulePath) {
			prefixToTrim := modulePath
			// Ensure we are trimming "modulePath/" not just "modulePath" if there's more path
			if !strings.HasSuffix(modulePath, "/") && len(rawPath) > len(modulePath) && rawPath[len(modulePath)] == '/' {
				prefixToTrim += "/"
			} else if rawPath == modulePath { // rawPath is just the module path, not a file in it
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
//
// File paths in the `filePaths` argument can be provided in three forms:
//  1. Absolute path (e.g., "/path/to/your/project/pkg/file.go").
//  2. Path relative to the current working directory (CWD) (e.g., "pkg/file.go").
//  3. Module-qualified path (e.g., "github.com/your/module/pkg/file.go"), which is resolved
//     using the Scanner's associated module information (from go.mod).
//
// All provided file paths, after resolution, must belong to the same directory,
// effectively meaning they must be part of the same Go package.
//
// This function only parses files that have not been previously visited (parsed)
// by this specific Scanner instance (tracked in `s.visitedFiles`).
//
// The returned `scanner.PackageInfo` contains information derived *only* from the
// files that were newly parsed in *this specific call*. If all specified files
// were already visited, the `PackageInfo.Files` list (and consequently Types, Functions, etc.)
// will be empty, though `Path` and `ImportPath` will be set according to the files' package.
//
// Results from `ScanFiles` are *not* stored in the main package cache (`s.packageCache`)
// because they represent partial package information. However, the global symbol
// cache (`s.symbolCache`), if enabled, *is* updated with symbols from the newly parsed files.
// Files parsed by this function are marked as visited in `s.visitedFiles`.
func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string) (*scanner.PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no file paths provided to ScanFiles")
	}
	if s.locator == nil {
		return nil, fmt.Errorf("scanner locator is not initialized")
	}
	moduleRoot := s.locator.RootDir()
	modulePath := s.locator.ModulePath()
	if modulePath == "" && moduleRoot == "" { // Heuristic: not in a module context at all
		// Allow scanning if files are absolute paths and locator isn't strictly needed for path resolution itself
		// but import path calculation will be severely limited.
		slog.WarnContext(ctx, "ScanFiles called likely outside a Go module context. Import path resolution will be affected.")
	} else if modulePath == "" || moduleRoot == "" { // Inconsistent module info
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

	if modulePath != "" && moduleRoot != "" { // Only attempt module-based import path if module context is valid
		// if !strings.HasPrefix(pkgDirAbs, moduleRoot) {
		// 	// This check is too strict for modules that use `replace` directives
		// 	// to point to a parent directory. The locator has already resolved the
		// 	// import path to this directory, so we should trust it.
		// 	return nil, fmt.Errorf("package directory %s is outside the module root %s, cannot determine module-relative import path", pkgDirAbs, moduleRoot)
		// }
		relPath, err := filepath.Rel(moduleRoot, pkgDirAbs)
		if err != nil {
			return nil, fmt.Errorf("could not determine relative path for %s from module root %s: %w", pkgDirAbs, moduleRoot, err)
		}
		if relPath == "." || relPath == "" {
			importPath = modulePath
		} else {
			importPath = filepath.ToSlash(filepath.Join(modulePath, relPath))
		}
	} else { // Fallback if not in a clear module context (e.g. scanning /usr/local/go/src/fmt)
		// This part needs careful consideration for how to represent non-module packages.
		// For now, use the directory path as a pseudo-import path.
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

	if len(filesToParse) == 0 { // All specified files already visited
		// Return an empty PackageInfo but with correct Path/ImportPath
		return &scanner.PackageInfo{
			Path:       pkgDirAbs,
			ImportPath: importPath,
			Name:       "", // Name would require parsing or looking up a cached full PackageInfo
			Fset:       s.fset,
			Files:      []string{}, // No files *newly* parsed
		}, nil
	}

	pkgInfo, err := s.scanner.ScanFiles(ctx, filesToParse, pkgDirAbs) // Scan only unvisited files
	if err != nil {
		return nil, fmt.Errorf("failed to scan files in %s (import path %s): %w", pkgDirAbs, importPath, err)
	}

	if pkgInfo != nil {
		pkgInfo.ImportPath = importPath // Set the calculated import path
		pkgInfo.Path = pkgDirAbs        // Ensure directory path is also set
		s.mu.Lock()
		for _, fp := range pkgInfo.Files { // Mark newly parsed files as visited
			s.visitedFiles[fp] = struct{}{}
		}
		s.mu.Unlock()
		// Results from ScanFiles (which are partial by design based on unvisited files)
		// are NOT cached in s.packageCache. Only symbol cache is updated.
		s.updateSymbolCacheWithPackageInfo(ctx, importPath, pkgInfo)
	}
	return pkgInfo, nil
}

// UnscannedGoFiles returns a list of absolute paths to .go files
// (and optionally _test.go files) within the specified package that have not
// yet been visited (parsed) by this Scanner instance.
//
// The `packagePathOrImportPath` argument can be:
//  1. An absolute directory path to the package.
//  2. A directory path relative to the current working directory (CWD).
//  3. A Go import path (e.g., "github.com/your/module/pkg"), which will be resolved
//     to a directory using the Scanner's locator.
//
// This method lists all relevant .go files in the identified package directory
// and filters out those already present in the Scanner's `visitedFiles` set.
// It is useful for discovering which files in a package still need to be processed
// if performing iterative scanning.
func (s *Scanner) UnscannedGoFiles(packagePathOrImportPath string) ([]string, error) {
	if s.locator == nil && !(filepath.IsAbs(packagePathOrImportPath) && isDir(packagePathOrImportPath)) {
		// If locator is nil, we can only proceed if packagePathOrImportPath is an absolute directory path.
		return nil, fmt.Errorf("scanner locator is not initialized, and path is not an absolute directory to a package")
	}

	var pkgDirAbs string
	var err error

	// Try as a direct file system path first (absolute or CWD-relative directory)
	pathAsDir, err := filepath.Abs(packagePathOrImportPath)
	if err == nil {
		info, statErr := os.Stat(pathAsDir)
		if statErr == nil && info.IsDir() {
			pkgDirAbs = pathAsDir
		}
	}

	// If not resolved as a direct directory path, try as an import path via locator (if locator exists)
	if pkgDirAbs == "" {
		if s.locator == nil { // Guard again, as locator might be nil
			return nil, fmt.Errorf("cannot resolve %q as import path: locator not available", packagePathOrImportPath)
		}
		pkgDirAbs, err = s.locator.FindPackageDir(packagePathOrImportPath)
		if err != nil {
			return nil, fmt.Errorf("could not find package directory for %q (tried as path and import path): %w", packagePathOrImportPath, err)
		}
	}

	allGoFilesInDir, err := listGoFiles(pkgDirAbs, s.IncludeTests) // listGoFiles returns absolute paths
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

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ScanPackageByImport scans a single Go package identified by its import path.
//
// This function resolves the import path to a directory using the Scanner's locator.
// It then attempts to parse all .go files (excluding _test.go files) in that directory
// that have not yet been visited by this Scanner instance (`s.visitedFiles`).
// The selection of files to parse may also be influenced by the state of the
// symbol cache (`s.symbolCache`), if enabled, to avoid re-parsing unchanged files
// for which symbol information is already cached and deemed valid.
//
// The returned `scanner.PackageInfo` contains information derived from the files
// parsed or processed in *this specific call*.
//
// The result of this call is stored in an in-memory package cache (`s.packageCache`)
// and is intended to represent the Scanner's current understanding of the package,
// which might be based on a full parse of unvisited files or a combination of
// cached data and newly parsed information.
// The global symbol cache (`s.symbolCache`), if enabled, is updated with symbols
// from any newly parsed files. Files parsed by this function are marked as visited
// in `s.visitedFiles`.
func (s *Scanner) ScanPackageByImport(ctx context.Context, importPath string) (*scanner.PackageInfo, error) {
	// If a load hook is defined, check if we should proceed.
	if s.Config.PackageLoadHook != nil {
		if !s.Config.PackageLoadHook(importPath) {
			if s.Logger != nil {
				s.Logger.DebugContext(ctx, "skipping scan of package due to load hook", "package", importPath)
			}
			// Return a placeholder package. This prevents deep scanning of stdlib or third-party code.
			return &scanner.PackageInfo{
				ImportPath: importPath,
				Name:       filepath.Base(importPath),
				Fset:       s.fset,
			}, nil
		}
	}

	s.mu.RLock()
	cachedPkg, found := s.packageCache[importPath]
	s.mu.RUnlock()
	if found {
		slog.DebugContext(ctx, "ScanPackageByImport CACHE HIT", slog.String("importPath", importPath), slog.Int("types", len(cachedPkg.Types)))
		return cachedPkg, nil
	}
	slog.DebugContext(ctx, "ScanPackageByImport CACHE MISS", slog.String("importPath", importPath))

	loc, err := s.locatorForImportPath(importPath)
	if err != nil {
		return nil, fmt.Errorf("ScanPackageByImport: %w", err)
	}

	pkgDirAbs, err := loc.FindPackageDir(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not find directory for import path %s: %w", importPath, err)
	}
	slog.DebugContext(ctx, "ScanPackageByImport resolved import path", slog.String("importPath", importPath), slog.String("pkgDirAbs", pkgDirAbs), slog.String("module", loc.ModulePath()))

	allGoFilesInPkg, err := listGoFiles(pkgDirAbs, s.IncludeTests) // Gets absolute paths
	if err != nil {
		return nil, fmt.Errorf("ScanPackageByImport: failed to list go files in %s: %w", pkgDirAbs, err)
	}
	slog.DebugContext(ctx, "ScanPackageByImport found .go files", slog.Int("count", len(allGoFilesInPkg)), slog.String("pkgDirAbs", pkgDirAbs), slog.Any("files", allGoFilesInPkg))

	if len(allGoFilesInPkg) == 0 {
		// If a directory for an import path exists but has no .go files, cache an empty PackageInfo.
		slog.DebugContext(ctx, "ScanPackageByImport found no .go files. Caching empty PackageInfo.", slog.String("pkgDirAbs", pkgDirAbs))
		pkgInfo := &scanner.PackageInfo{Path: pkgDirAbs, ImportPath: importPath, Name: "", Fset: s.fset, Files: []string{}, Types: []*scanner.TypeInfo{}}
		s.mu.Lock()
		s.packageCache[importPath] = pkgInfo
		s.mu.Unlock()
		return pkgInfo, nil
	}

	var filesToParseThisCall []string
	symCache, _ := s.getOrCreateSymbolCache(ctx) // Error getting cache is not fatal here
	slog.DebugContext(ctx, "ScanPackageByImport symbol cache status", slog.String("importPath", importPath), slog.Bool("enabled", symCache != nil && symCache.isEnabled()))

	filesConsideredBySymCache := make(map[string]struct{})

	if symCache != nil && symCache.isEnabled() {
		newDiskFiles, existingDiskFiles, errSym := symCache.getFilesToScan(ctx, pkgDirAbs)
		if errSym != nil {
			slog.WarnContext(ctx, "getFilesToScan failed. Will scan all unvisited files in the package.", slog.String("import_path", importPath), slog.String("package_dir", pkgDirAbs), slog.Any("error", errSym))
			// Fallback: scan all files in the package that this Scanner instance hasn't visited.
			s.mu.RLock()
			for _, f := range allGoFilesInPkg {
				if _, visited := s.visitedFiles[f]; !visited {
					filesToParseThisCall = append(filesToParseThisCall, f)
				}
			}
			s.mu.RUnlock()
		} else {
			// Add files symCache identified as new/changed
			for _, f := range newDiskFiles {
				filesToParseThisCall = append(filesToParseThisCall, f)
				filesConsideredBySymCache[f] = struct{}{}
			}
			// For files symCache says are existing (potentially unchanged),
			// only parse if this Scanner instance hasn't visited them yet.
			s.mu.RLock()
			for _, f := range existingDiskFiles {
				filesConsideredBySymCache[f] = struct{}{} // Mark as considered
				if _, visited := s.visitedFiles[f]; !visited {
					filesToParseThisCall = append(filesToParseThisCall, f)
				}
			}
			s.mu.RUnlock()
		}
	}

	// Add any file in the directory not mentioned by symCache (e.g. untracked) if unvisited by this Scanner instance
	s.mu.RLock()
	for _, f := range allGoFilesInPkg {
		if _, considered := filesConsideredBySymCache[f]; !considered {
			if _, visited := s.visitedFiles[f]; !visited {
				filesToParseThisCall = append(filesToParseThisCall, f)
			}
		}
	}
	s.mu.RUnlock()

	// Deduplicate filesToParseThisCall (abs paths, so simple map is fine)
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
		// Heuristic to check if it's a standard library package.
		// Determine if the package is outside the main module (e.g., in GOROOT or GOMODCACHE).
		// If so, we must use ScanFilesWithKnownImportPath to prevent incorrect import path derivation.
		isExternalModule := !strings.HasPrefix(pkgDirAbs, s.RootDir())

		if isExternalModule {
			currentCallPkgInfo, err = s.scanner.ScanFilesWithKnownImportPath(ctx, filesToParseThisCall, pkgDirAbs, importPath)
		} else {
			currentCallPkgInfo, err = s.scanner.ScanFiles(ctx, filesToParseThisCall, pkgDirAbs)
		}

		if err != nil {
			return nil, fmt.Errorf("ScanPackageByImport: scanning files for %s failed: %w", importPath, err)
		}

		if currentCallPkgInfo != nil {
			// For non-std-lib packages, ScanFiles already calculates the import path.
			// For std-lib, ScanFilesWithKnownImportPath sets it.
			// We can still enforce it here to be safe, or trust the scanner.
			// Let's ensure it's what we expect.
			currentCallPkgInfo.ImportPath = importPath
			currentCallPkgInfo.Path = pkgDirAbs // Ensure path
			s.mu.Lock()
			for _, fp := range currentCallPkgInfo.Files { // Mark newly parsed files as visited by this instance
				s.visitedFiles[fp] = struct{}{}
			}
			s.mu.Unlock()
			s.updateSymbolCacheWithPackageInfo(ctx, importPath, currentCallPkgInfo) // Update global symbol cache
		}
	}

	// If no new files were parsed in this call, but the package is not empty,
	// it means all files were either already visited or symcache deemed them unchanged & visited.
	// We should return a PackageInfo that reflects the package structure.
	if currentCallPkgInfo == nil {
		currentCallPkgInfo = &scanner.PackageInfo{
			Path:       pkgDirAbs,
			ImportPath: importPath,
			Name:       "", // Name might be derivable if any file was ever parsed for this package
			Fset:       s.fset,
			Files:      []string{}, // No files *newly* parsed in this call.
		}
		// Attempt to set a name if possible from a previously (partially) cached PackageInfo
		// This is a bit of a workaround for not merging.
		s.mu.RLock()
		if prevInfo, ok := s.packageCache[importPath]; ok && prevInfo.Name != "" {
			currentCallPkgInfo.Name = prevInfo.Name
		} else if len(allGoFilesInPkg) > 0 { // Try to get from any already visited file if no cache
			// This is complex; for now, leave Name blank if not easily found.
		}
		s.mu.RUnlock()
	}

	// The PackageInfo cached by ScanPackageByImport should represent the state of the package
	// as understood by this call (i.e., including all files parsed up to this point for this package).
	// Since "no merge" is a principle, the cache stores the result of *this specific call*.
	// If this call parsed new files, currentCallPkgInfo has them. If not, it's minimal.
	// This means the packageCache might not always have the "fullest" possible PackageInfo
	// if ScanFiles was used to visit parts of the package before this.
	// This is a known trade-off of the "no merge" + "instance-visited" design.

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
// The pkgInfo provided should typically represent the symbols parsed from a specific set of files
// in the context of the given importPath.
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
			// Ensure absFilePath is truly absolute for consistency
			absFilePath, _ = filepath.Abs(absFilePath) // error unlikely if path came from system
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

	for _, absFilePath := range pkgInfo.Files { // These are files that were actually parsed for pkgInfo
		absFilePath, _ = filepath.Abs(absFilePath) // Ensure absolute
		if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
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

// ListExportedSymbols scans a package by its import path and returns a list of all
// its exported top-level symbol names (functions, types, and constants).
func (s *Scanner) ListExportedSymbols(ctx context.Context, pkgPath string) ([]string, error) {
	pkgInfo, err := s.ScanPackageByImport(ctx, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package %s: %w", pkgPath, err)
	}

	var exportedSymbols []string

	for _, f := range pkgInfo.Functions {
		// We only care about top-level functions, not methods on types.
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
// The `symbolFullName` should be in the format "package/import/path.SymbolName".
//
// It first checks the persistent symbol cache (if enabled and loaded).
// If not found in the cache, it triggers a scan of the relevant package
// (using `ScanPackageByImport`) to populate caches and then re-checks.
// Finally, it inspects the `PackageInfo` obtained from the scan.
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
	// If symbol not found in cache, try to scan the package.
	pkgInfo, err := s.ScanPackageByImport(ctx, importPath) // This will parse unvisited files and update caches
	if err != nil {
		return "", fmt.Errorf("scan for package %s (for symbol %s) failed: %w", importPath, symbolName, err)
	}

	// After scan, check cache again (if enabled)
	if s.CachePath != "" {
		if s.symbolCache != nil && s.symbolCache.isEnabled() {
			filePath, found := s.symbolCache.get(cacheKey) // Get does not need context
			if found {
				if _, statErr := os.Stat(filePath); statErr == nil {
					return filePath, nil
				}
				slog.WarnContext(ctx, "Symbol found in cache after scan, but file does not exist.", slog.String("symbol", symbolFullName), slog.String("path", filePath))
			}
		}
	}

	// If still not found via cache, check the pkgInfo returned by the ScanPackageByImport call.
	// This pkgInfo contains symbols from files *parsed in that specific call*.
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
			return "", fmt.Errorf("symbol %s found in package %s at %s by scan, but file does not exist", symbolName, importPath, targetFilePath)
		}
	}

	return "", fmt.Errorf("symbol %s not found in package %s even after scan and cache check", symbolName, importPath)
}

// FindSymbolInPackage searches for a specific symbol within a package by scanning its files one by one.
// It only scans files that have not yet been visited by this scanner instance.
// If the symbol is found, it returns a cumulative PackageInfo of all files scanned in the package up to that point
// and marks the file as visited. If the symbol is not found after checking all unscanned files, it returns an error.
func (s *Scanner) FindSymbolInPackage(ctx context.Context, importPath string, symbolName string) (*scanner.PackageInfo, error) {
	unscannedFiles, err := s.UnscannedGoFiles(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not get unscanned files for package %s: %w", importPath, err)
	}

	// Heuristic to check if it's a standard library package.
	isStdLib := !strings.Contains(importPath, ".")
	var pkgDirAbs string
	if isStdLib {
		// We need the absolute path for ScanFilesWithKnownImportPath
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
			// For stdlib packages, we need to bypass the public `ScanFiles` because it cannot
			// calculate the import path for paths outside the module root.
			// We call the internal scanner directly and then manually perform the necessary updates
			// that the public `ScanFiles` would have done (updating visited files and symbol cache).
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
			// For in-module packages, the public `ScanFiles` method works correctly.
			pkgInfo, scanErr = s.ScanFiles(ctx, []string{fileToScan})
		}

		if scanErr != nil {
			// Log the error but continue trying other files. A single file might have syntax errors.
			slog.WarnContext(ctx, "failed to scan file while searching for symbol", "file", fileToScan, "symbol", symbolName, "error", scanErr)
			continue
		}

		if pkgInfo == nil {
			continue
		}

		// Merge the just-scanned info into a cumulative PackageInfo for this package.
		if cumulativePkgInfo == nil {
			cumulativePkgInfo = pkgInfo
		} else {
			// This is a simplified merge. A more robust implementation would handle conflicts.
			cumulativePkgInfo.Types = append(cumulativePkgInfo.Types, pkgInfo.Types...)
			cumulativePkgInfo.Functions = append(cumulativePkgInfo.Functions, pkgInfo.Functions...)
			cumulativePkgInfo.Constants = append(cumulativePkgInfo.Constants, pkgInfo.Constants...)

			// Merge AstFiles and Files lists
			if cumulativePkgInfo.AstFiles == nil {
				cumulativePkgInfo.AstFiles = make(map[string]*ast.File)
			}
			for path, ast := range pkgInfo.AstFiles {
				if _, exists := cumulativePkgInfo.AstFiles[path]; !exists {
					cumulativePkgInfo.AstFiles[path] = ast
				}
			}

			// Avoid duplicating file paths
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

	// Now, check for the symbol in the fully cumulative package info.
	for _, t := range cumulativePkgInfo.Types {
		if t.Name == symbolName {
			return cumulativePkgInfo, nil // Found it
		}
	}
	for _, f := range cumulativePkgInfo.Functions {
		if f.Name == symbolName {
			return cumulativePkgInfo, nil // Found it
		}
	}
	for _, c := range cumulativePkgInfo.Constants {
		if c.Name == symbolName {
			return cumulativePkgInfo, nil // Found it
		}
	}

	return nil, fmt.Errorf("symbol %q not found in package %q", symbolName, importPath)
}
