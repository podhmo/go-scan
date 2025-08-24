package goscan

import (
	"bytes"
	"context"
	"fmt"
	"go/token"
	i_fs "io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/podhmo/go-scan/fs"
	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Config holds shared configuration and state for both Scanner and ModuleWalker.
type Config struct {
	workDir             string
	locator             *locator.Locator
	scanner             *scanner.Scanner // low-level scanner
	fset                *token.FileSet
	FS                  fs.FS
	useGoModuleResolver bool
	IncludeTests        bool
	DryRun              bool
	Inspect             bool
	Logger              *slog.Logger
	overlay             scanner.Overlay
}

// ModuleWalker is responsible for lightweight, dependency-focused scanning operations.
type ModuleWalker struct {
	*Config
	packageImportsCache map[string]*scanner.PackageImports
	reverseDepCache     map[string][]string
	mu                  sync.RWMutex
}

// ScanPackageImports scans a single Go package identified by its import path.
func (w *ModuleWalker) ScanPackageImports(ctx context.Context, importPath string) (*scanner.PackageImports, error) {
	w.mu.RLock()
	cachedPkg, found := w.packageImportsCache[importPath]
	w.mu.RUnlock()
	if found {
		slog.DebugContext(ctx, "ScanPackageImports CACHE HIT", slog.String("importPath", importPath))
		return cachedPkg, nil
	}
	slog.DebugContext(ctx, "ScanPackageImports CACHE MISS", slog.String("importPath", importPath))

	pkgDirAbs, err := w.locator.FindPackageDir(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not find directory for import path %s: %w", importPath, err)
	}

	allGoFilesInPkg, err := w.listGoFiles(pkgDirAbs)
	if err != nil {
		return nil, fmt.Errorf("ScanPackageImports: failed to list go files in %s: %w", pkgDirAbs, err)
	}

	if len(allGoFilesInPkg) == 0 {
		pkgInfo := &scanner.PackageImports{
			ImportPath: importPath,
			Name:       filepath.Base(pkgDirAbs),
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
func (w *ModuleWalker) FindImporters(ctx context.Context, targetImportPath string) ([]*PackageImports, error) {
	rootDir := w.locator.RootDir()
	modulePath := w.locator.ModulePath()

	if rootDir == "" {
		return nil, fmt.Errorf("module root directory not found, cannot perform reverse dependency search")
	}

	var importers []*PackageImports

	err := w.FS.WalkDir(rootDir, func(path string, d i_fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if name := d.Name(); name != "." && path != rootDir {
			if name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
		}

		goFiles, err := w.listGoFiles(path)
		if err != nil {
			slog.WarnContext(ctx, "could not list go files in directory, skipping", slog.String("path", path), slog.Any("error", err))
			return nil
		}

		if len(goFiles) == 0 {
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			slog.WarnContext(ctx, "could not determine relative path for package, skipping", slog.String("path", path), slog.Any("error", err))
			return nil
		}

		currentPkgImportPath := filepath.ToSlash(filepath.Join(modulePath, relPath))
		if relPath == "." {
			currentPkgImportPath = modulePath
		}

		pkgImports, err := w.ScanPackageImports(ctx, currentPkgImportPath)
		if err != nil {
			slog.WarnContext(ctx, "failed to scan package imports, skipping", "importPath", currentPkgImportPath, "error", err)
			return nil
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

// FindImportersAggressively scans the module using `git grep` to quickly find files.
func (w *ModuleWalker) FindImportersAggressively(ctx context.Context, targetImportPath string) ([]*PackageImports, error) {
	rootDir := w.locator.RootDir()
	modulePath := w.locator.ModulePath()

	if rootDir == "" {
		return nil, fmt.Errorf("module root directory not found, cannot perform aggressive reverse dependency search")
	}

	pattern := fmt.Sprintf(`"%s"`, targetImportPath)

	cmd := exec.CommandContext(ctx, "git", "grep", "-l", "-F", pattern, "--", "*.go")
	cmd.Dir = rootDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.DebugContext(ctx, "executing git grep", slog.String("dir", cmd.Dir), slog.Any("args", cmd.Args))

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git grep failed: %w\n%s", err, stderr.String())
	}

	potentialFiles := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(potentialFiles) == 0 || (len(potentialFiles) == 1 && potentialFiles[0] == "") {
		return nil, nil
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
			slog.WarnContext(ctx, "failed to scan potential importer package, skipping", "importPath", currentPkgImportPath, "error", err)
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

	err := w.FS.WalkDir(rootDir, func(path string, d i_fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if name := d.Name(); name != "." && path != rootDir {
			if name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
		}
		goFiles, err := w.listGoFiles(path)
		if err != nil {
			slog.WarnContext(ctx, "could not list go files in directory, skipping", "path", path, "error", err)
			return nil
		}
		if len(goFiles) == 0 {
			return nil
		}
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			slog.WarnContext(ctx, "could not determine relative path for package, skipping", "path", path, "error", err)
			return nil
		}
		currentPkgImportPath := filepath.ToSlash(filepath.Join(modulePath, relPath))
		if relPath == "." {
			currentPkgImportPath = modulePath
		}
		pkgImports, err := w.ScanPackageImports(ctx, currentPkgImportPath)
		if err != nil {
			slog.WarnContext(ctx, "failed to scan package imports, skipping", "importPath", currentPkgImportPath, "error", err)
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

// Walk performs a dependency graph traversal starting from a set of root packages.
func (w *ModuleWalker) Walk(ctx context.Context, visitor Visitor, patterns ...string) error {
	initialQueue, err := w.resolvePatternsToImportPaths(ctx, patterns)
	if err != nil {
		return fmt.Errorf("could not resolve initial patterns for walk: %w", err)
	}

	queue := initialQueue
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

// resolvePatternsToImportPaths resolves package patterns into a list of Go import paths.
func (w *ModuleWalker) resolvePatternsToImportPaths(ctx context.Context, patterns []string) ([]string, error) {
	rootPaths := make(map[string]struct{})

	for _, pattern := range patterns {
		if !strings.Contains(pattern, "...") {
			rootPaths[pattern] = struct{}{}
			continue
		}

		basePattern := strings.TrimSuffix(pattern, "...")
		basePattern = strings.TrimSuffix(basePattern, "/")

		var walkRoot string
		var err error

		isFilePathPattern := strings.HasPrefix(pattern, ".") || filepath.IsAbs(pattern)

		if pattern == "..." {
			if w.locator.ModulePath() == "" {
				return nil, fmt.Errorf("cannot resolve \"...\" pattern: not in a module")
			}
			walkRoot = w.locator.RootDir()
		} else if isFilePathPattern {
			walkRoot = basePattern
			if !filepath.IsAbs(walkRoot) {
				walkRoot = filepath.Join(w.workDir, walkRoot)
			}
		} else {
			walkRoot, err = w.locator.FindPackageDir(basePattern)
			if err != nil {
				return nil, fmt.Errorf("cannot find package %q for pattern %q: %w", basePattern, pattern, err)
			}
		}

		// Also check the walkRoot itself, as WalkDir doesn't process the root with the function.
		if ok, err := w.hasGoFiles(walkRoot); err == nil && ok {
			if importPath, err := w.locator.PathToImport(walkRoot); err == nil {
				rootPaths[importPath] = struct{}{}
			}
		}

		walkErr := w.FS.WalkDir(walkRoot, func(path string, d i_fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) && path == walkRoot {
					return filepath.SkipDir // a non-fatal error for WalkDir
				}
				return err
			}
			if !d.IsDir() {
				return nil
			}

			if name := d.Name(); name != "." && path != walkRoot {
				if name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					return filepath.SkipDir
				}
			}

			ok, err := w.hasGoFiles(path)
			if err != nil {
				slog.DebugContext(ctx, "hasGoFiles failed, skipping", "path", path, "error", err)
				return nil
			}

			if ok {
				importPath, err := w.locator.PathToImport(path)
				if err != nil {
					slog.DebugContext(ctx, "PathToImport failed, skipping", "path", path, "error", err)
					return nil
				}
				rootPaths[importPath] = struct{}{}
			}
			return nil
		})

		if walkErr != nil {
			if os.IsNotExist(walkErr) && isFilePathPattern {
				continue
			}
			return nil, fmt.Errorf("error walking for pattern %q from root %q: %w", pattern, walkRoot, walkErr)
		}
	}

	pathList := make([]string, 0, len(rootPaths))
	for path := range rootPaths {
		pathList = append(pathList, path)
	}
	sort.Strings(pathList)
	return pathList, nil
}

func (w *ModuleWalker) hasGoFiles(dirPath string) (bool, error) {
	entries, err := w.FS.ReadDir(dirPath)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true, nil
		}
	}
	return false, nil
}

func (w *ModuleWalker) listGoFiles(dirPath string) ([]string, error) {
	var files []string
	entries, err := w.FS.ReadDir(dirPath)
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
		if !w.IncludeTests && strings.HasSuffix(name, "_test.go") {
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
