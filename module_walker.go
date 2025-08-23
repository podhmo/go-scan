package goscan

import (
	"context"
	"fmt"
	"go/token"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"bytes"

	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// ModuleWalker is a specialized scanner for dependency graph traversal and analysis.
// It focuses on lightweight, imports-only scanning to quickly understand the relationships
// between packages without the overhead of full code parsing.
type ModuleWalker struct {
	workDir             string // The working directory for the walker.
	locator             *locator.Locator
	scanner             *scanner.Scanner
	packageImportsCache map[string]*scanner.PackageImports
	reverseDepCache     map[string][]string // Cache for the reverse dependency map
	mu                  sync.RWMutex
	fset                *token.FileSet
	useGoModuleResolver bool // To be set by WithGoModuleResolver
	IncludeTests        bool // To be set by WithTests
	Logger              *slog.Logger
	overlay             scanner.Overlay
}

// ModuleWalkerOption is a function that configures a ModuleWalker.
type ModuleWalkerOption func(*ModuleWalker) error

// WithModuleWalkerWorkDir sets the working directory for the walker.
func WithModuleWalkerWorkDir(path string) ModuleWalkerOption {
	return func(w *ModuleWalker) error {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("getting absolute path for workdir %q: %w", path, err)
		}
		w.workDir = absPath
		return nil
	}
}

// WithModuleWalkerLogger sets the logger for the walker.
func WithModuleWalkerLogger(logger *slog.Logger) ModuleWalkerOption {
	return func(w *ModuleWalker) error {
		w.Logger = logger
		return nil
	}
}

// WithModuleWalkerIncludeTests includes test files in the scan.
func WithModuleWalkerIncludeTests(include bool) ModuleWalkerOption {
	return func(w *ModuleWalker) error {
		w.IncludeTests = include
		return nil
	}
}

// WithModuleWalkerGoModuleResolver enables the walker to find packages in the Go module cache and GOROOT.
func WithModuleWalkerGoModuleResolver() ModuleWalkerOption {
	return func(w *ModuleWalker) error {
		w.useGoModuleResolver = true
		return nil
	}
}

// WithModuleWalkerOverlay provides in-memory file content to the walker.
func WithModuleWalkerOverlay(overlay scanner.Overlay) ModuleWalkerOption {
	return func(w *ModuleWalker) error {
		if w.overlay == nil {
			w.overlay = make(scanner.Overlay)
		}
		for k, v := range overlay {
			w.overlay[k] = v
		}
		return nil
	}
}

// NewModuleWalker creates a new ModuleWalker. It finds the module root starting from the given path.
func NewModuleWalker(options ...ModuleWalkerOption) (*ModuleWalker, error) {
	w := &ModuleWalker{
		packageImportsCache: make(map[string]*scanner.PackageImports),
		fset:                token.NewFileSet(),
		overlay:             make(scanner.Overlay),
	}

	for _, option := range options {
		if err := option(w); err != nil {
			return nil, err
		}
	}

	if w.workDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
		w.workDir = cwd
	}

	if w.Logger == nil {
		w.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}

	locatorOpts := []locator.Option{locator.WithOverlay(w.overlay)}
	if w.useGoModuleResolver {
		locatorOpts = append(locatorOpts, locator.WithGoModuleResolver())
	}

	loc, err := locator.New(w.workDir, locatorOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize locator: %w", err)
	}
	w.locator = loc

	// The ModuleWalker does not need a resolver because it does not perform deep type analysis.
	// We pass `nil` for the resolver argument to the internal scanner.
	internalScanner, err := scanner.New(w.fset, nil, w.overlay, loc.ModulePath(), loc.RootDir(), nil, false, w.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal scanner: %w", err)
	}
	w.scanner = internalScanner

	return w, nil
}

// ModulePath returns the module path from the walker's locator.
func (w *ModuleWalker) ModulePath() string {
	if w.locator == nil {
		return ""
	}
	return w.locator.ModulePath()
}

// RootDir returns the module root directory from the walker's locator.
func (w *ModuleWalker) RootDir() string {
	if w.locator == nil {
		return ""
	}
	return w.locator.RootDir()
}

// ScanPackageImports scans a single Go package identified by its import path,
// parsing only the package clause and import declarations for efficiency.
// It returns a lightweight PackageImports struct containing the package name
// and a list of its direct dependencies.
// Results are cached in memory for the lifetime of the ModuleWalker instance.
func (w *ModuleWalker) ScanPackageImports(ctx context.Context, importPath string) (*scanner.PackageImports, error) {
	w.mu.RLock()
	cachedPkg, found := w.packageImportsCache[importPath]
	w.mu.RUnlock()
	if found {
		w.Logger.DebugContext(ctx, "ScanPackageImports CACHE HIT", slog.String("importPath", importPath))
		return cachedPkg, nil
	}
	w.Logger.DebugContext(ctx, "ScanPackageImports CACHE MISS", slog.String("importPath", importPath))

	pkgDirAbs, err := w.locator.FindPackageDir(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not find directory for import path %s: %w", importPath, err)
	}

	allGoFilesInPkg, err := listGoFiles(pkgDirAbs, w.IncludeTests)
	if err != nil {
		return nil, fmt.Errorf("ScanPackageImports: failed to list go files in %s: %w", pkgDirAbs, err)
	}

	if len(allGoFilesInPkg) == 0 {
		// If a directory for an import path exists but has no .go files, cache an empty PackageImports.
		pkgInfo := &scanner.PackageImports{
			ImportPath: importPath,
			Name:       filepath.Base(pkgDirAbs), // Best guess for name
			Imports:    []string{},
		}
		w.mu.Lock()
		w.packageImportsCache[importPath] = pkgInfo
		w.mu.Unlock()
		return pkgInfo, nil
	}

	pkgImports, err := w.scanner.ScanPackageImports(ctx, allGoFilesInPkg, pkgDirAbs, importPath)
	if err != nil {
		return nil, fmt.Errorf("ScanPackageImports: scanning imports for %s failed: %w", importPath, err)
	}

	w.mu.Lock()
	w.packageImportsCache[importPath] = pkgImports
	w.mu.Unlock()

	return pkgImports, nil
}

// FindImporters scans the entire module to find packages that import the targetImportPath.
// It performs an efficient, imports-only scan of all potential package directories in the module.
// The result is a list of packages that have a direct dependency on the target.
func (w *ModuleWalker) FindImporters(ctx context.Context, targetImportPath string) ([]*PackageImports, error) {
	w.mu.RLock()
	rootDir := w.locator.RootDir()
	modulePath := w.locator.ModulePath()
	w.mu.RUnlock()

	if rootDir == "" {
		return nil, fmt.Errorf("module root directory not found, cannot perform reverse dependency search")
	}

	var importers []*PackageImports

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		// Skip hidden directories and vendor
		if d.Name() == "vendor" || (len(d.Name()) > 1 && d.Name()[0] == '.') {
			return filepath.SkipDir
		}

		goFiles, err := listGoFiles(path, w.IncludeTests)
		if err != nil {
			w.Logger.WarnContext(ctx, "could not list go files in directory, skipping", slog.String("path", path), slog.Any("error", err))
			return nil // continue walking
		}

		if len(goFiles) == 0 {
			return nil // Not a package, continue
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			w.Logger.WarnContext(ctx, "could not determine relative path for package, skipping", slog.String("path", path), slog.Any("error", err))
			return nil // Continue walking
		}

		currentPkgImportPath := filepath.ToSlash(filepath.Join(modulePath, relPath))
		if relPath == "." {
			currentPkgImportPath = modulePath
		}

		pkgImports, err := w.ScanPackageImports(ctx, currentPkgImportPath)
		if err != nil {
			w.Logger.WarnContext(ctx, "failed to scan package imports, skipping", "importPath", currentPkgImportPath, "error", err)
			return nil // continue
		}

		for _, imp := range pkgImports.Imports {
			if imp == targetImportPath {
				importers = append(importers, pkgImports)
				break
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking module directory for importers: %w", err)
	}

	sort.Slice(importers, func(i, j int) bool {
		return importers[i].ImportPath < importers[j].ImportPath
	})

	return importers, nil
}

// FindImportersAggressively scans the module using `git grep` to quickly find files
// that likely import the targetImportPath, then confirms them. This can be much
// faster than walking the entire directory structure in large repositories.
// A git repository is required.
func (w *ModuleWalker) FindImportersAggressively(ctx context.Context, targetImportPath string) ([]*PackageImports, error) {
	w.mu.RLock()
	rootDir := w.locator.RootDir()
	modulePath := w.locator.ModulePath()
	w.mu.RUnlock()

	if rootDir == "" {
		return nil, fmt.Errorf("module root directory not found, cannot perform aggressive reverse dependency search")
	}

	pattern := fmt.Sprintf(`"%s"`, targetImportPath)

	cmd := exec.CommandContext(ctx, "git", "grep", "-l", "-F", pattern, "--", "*.go")
	cmd.Dir = rootDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	w.Logger.DebugContext(ctx, "executing git grep", slog.String("dir", cmd.Dir), slog.Any("args", cmd.Args))

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git grep failed: %w\n%s", err, stderr.String())
	}

	potentialFiles := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(potentialFiles) == 0 || (len(potentialFiles) == 1 && potentialFiles[0] == "") {
		return nil, nil // No matches
	}

	packagesToScan := make(map[string]struct{})
	for _, fileRelPath := range potentialFiles {
		if fileRelPath == "" {
			continue
		}
		dir := filepath.Dir(fileRelPath)
		packagesToScan[dir] = struct{}{}
	}

	var importers []*PackageImports
	for relDir := range packagesToScan {
		var currentPkgImportPath string
		if relDir == "." {
			currentPkgImportPath = modulePath
		} else {
			currentPkgImportPath = filepath.ToSlash(filepath.Join(modulePath, relDir))
		}

		pkgImports, err := w.ScanPackageImports(ctx, currentPkgImportPath)
		if err != nil {
			w.Logger.WarnContext(ctx, "failed to scan potential importer package, skipping", "importPath", currentPkgImportPath, "error", err)
			continue
		}

		for _, imp := range pkgImports.Imports {
			if imp == targetImportPath {
				importers = append(importers, pkgImports)
				break
			}
		}
	}

	sort.Slice(importers, func(i, j int) bool {
		return importers[i].ImportPath < importers[j].ImportPath
	})

	return importers, nil
}

// BuildReverseDependencyMap scans the entire module to build a map of reverse dependencies.
// The key of the map is an import path, and the value is a list of packages that import it.
// The result is cached within the walker instance.
func (w *ModuleWalker) BuildReverseDependencyMap(ctx context.Context) (map[string][]string, error) {
	w.mu.RLock()
	if w.reverseDepCache != nil {
		w.mu.RUnlock()
		return w.reverseDepCache, nil
	}
	w.mu.RUnlock()

	rootDir := w.locator.RootDir()
	modulePath := w.locator.ModulePath()

	if rootDir == "" {
		return nil, fmt.Errorf("module root directory not found, cannot build reverse dependency map")
	}

	reverseDeps := make(map[string][]string)

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == "vendor" || (len(d.Name()) > 1 && d.Name()[0] == '.') {
			return filepath.SkipDir
		}
		goFiles, err := listGoFiles(path, w.IncludeTests)
		if err != nil {
			w.Logger.WarnContext(ctx, "could not list go files in directory, skipping", "path", path, "error", err)
			return nil
		}
		if len(goFiles) == 0 {
			return nil
		}
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			w.Logger.WarnContext(ctx, "could not determine relative path for package, skipping", "path", path, "error", err)
			return nil
		}
		currentPkgImportPath := filepath.ToSlash(filepath.Join(modulePath, relPath))
		if relPath == "." {
			currentPkgImportPath = modulePath
		}
		pkgImports, err := w.ScanPackageImports(ctx, currentPkgImportPath)
		if err != nil {
			w.Logger.WarnContext(ctx, "failed to scan package imports, skipping", "importPath", currentPkgImportPath, "error", err)
			return nil
		}
		for _, imp := range pkgImports.Imports {
			reverseDeps[imp] = append(reverseDeps[imp], currentPkgImportPath)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking module directory for reverse dependency map: %w", err)
	}

	for _, importers := range reverseDeps {
		sort.Strings(importers)
	}

	w.mu.Lock()
	w.reverseDepCache = reverseDeps
	w.mu.Unlock()

	return reverseDeps, nil
}

// Walk performs a dependency graph traversal starting from a root import path.
// It uses the efficient ScanPackageImports method to fetch dependencies at each step.
// The provided Visitor's Visit method is called for each discovered package,
// allowing the caller to inspect the package and control which of its dependencies
// are followed next.
func (w *ModuleWalker) Walk(ctx context.Context, rootImportPath string, visitor Visitor) error {
	queue := []string{rootImportPath}
	visited := make(map[string]struct{})

	for len(queue) > 0 {
		currentImportPath := queue[0]
		queue = queue[1:]

		if _, ok := visited[currentImportPath]; ok {
			continue
		}
		visited[currentImportPath] = struct{}{}

		pkgImports, err := w.ScanPackageImports(ctx, currentImportPath)
		if err != nil {
			return fmt.Errorf("error scanning imports for %s: %w", currentImportPath, err)
		}

		importsToFollow, err := visitor.Visit(pkgImports)
		if err != nil {
			return fmt.Errorf("visitor failed for package %s: %w", currentImportPath, err)
		}

		for _, imp := range importsToFollow {
			if _, ok := visited[imp]; !ok {
				queue = append(queue, imp)
			}
		}
	}
	return nil
}
