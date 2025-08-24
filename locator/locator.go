package locator

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/podhmo/go-scan/fs"
	"github.com/podhmo/go-scan/scanner"
	"golang.org/x/mod/module"
)

// ReplaceDirective represents a single replace directive in a go.mod file.
type ReplaceDirective struct {
	OldPath    string
	OldVersion string // Empty if not specified
	NewPath    string
	NewVersion string // Empty if it's a local path or not specified
	IsLocal    bool
}

// Locator helps find the module root and resolve package import paths.
type Locator struct {
	modulePath string
	rootDir    string
	replaces   []ReplaceDirective
	overlay    scanner.Overlay
	FS         fs.FS

	// Options for advanced resolution
	UseGoModuleResolver bool
	goRoot              string
	goModCache          string
	requires            map[string]string // module path -> version
}

// Option is a functional option for configuring the Locator.
type Option func(*Locator)

// WithOverlay provides in-memory file content for go.mod.
func WithOverlay(overlay scanner.Overlay) Option {
	return func(l *Locator) {
		if l.overlay == nil {
			l.overlay = make(scanner.Overlay)
		}
		for k, v := range overlay {
			l.overlay[k] = v
		}
	}
}

// WithFS sets the filesystem implementation for the locator.
// If not provided, it defaults to the OS filesystem.
func WithFS(fs fs.FS) Option {
	return func(l *Locator) {
		l.FS = fs
	}
}

// WithGoModuleResolver enables resolving packages from GOROOT and the module cache.
func WithGoModuleResolver() Option {
	return func(l *Locator) {
		l.UseGoModuleResolver = true
	}
}

// New creates a new Locator by searching for a go.mod file.
// It starts searching from startPath and moves up the directory tree.
func New(startPath string, options ...Option) (*Locator, error) {
	l := &Locator{
		requires: make(map[string]string),
	}
	for _, opt := range options {
		opt(l)
	}
	if l.FS == nil {
		l.FS = fs.NewOSFS()
	}

	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", startPath, err)
	}

	rootDir, err := l.findModuleRoot(absPath)
	if err != nil {
		// If resolver is enabled, not finding a go.mod is not a fatal error
		// as we might be resolving stdlib packages.
		if !l.UseGoModuleResolver {
			return nil, err
		}
		// We can proceed without a module root, but some features will be limited.
		// Let's assign rootDir to startPath to have a reference point.
		rootDir = absPath
	}
	l.rootDir = rootDir

	var goModContent []byte
	if l.overlay != nil {
		goModFilePathAbs := filepath.Join(l.rootDir, "go.mod")
		if content, ok := l.overlay[goModFilePathAbs]; ok {
			goModContent = content
		} else if content, ok := l.overlay["go.mod"]; ok { // Fallback for older tests using relative key
			goModContent = content
		}
	}

	if goModContent == nil && l.rootDir != "" {
		goModFilePath := filepath.Join(l.rootDir, "go.mod")
		// It's okay if go.mod doesn't exist, especially if UseGoModuleResolver is true
		if content, readErr := l.FS.ReadFile(goModFilePath); readErr == nil {
			goModContent = content
		}
	}

	if len(goModContent) > 0 {
		modPath, err := getModulePathFromBytes(goModContent)
		if err != nil {
			return nil, fmt.Errorf("failed to get module path from go.mod content: %w", err)
		}
		l.modulePath = modPath

		replaces, err := getReplaceDirectivesFromBytes(goModContent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not parse replace directives in go.mod: %v\n", err)
		}
		l.replaces = replaces

		if l.UseGoModuleResolver {
			requires, err := getRequireDirectivesFromBytes(goModContent)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not parse require directives in go.mod: %v\n", err)
			}
			l.requires = requires
		}
	}

	if l.UseGoModuleResolver {
		l.goRoot = runtime.GOROOT()
		cache, err := getGoModCache()
		if err != nil {
			return nil, fmt.Errorf("could not determine go mod cache location: %w", err)
		}
		l.goModCache = cache
	}

	return l, nil
}

// RootDir returns the project's root directory (where go.mod is located).
func (l *Locator) RootDir() string {
	return l.rootDir
}

// ModulePath returns the module path from go.mod.
func (l *Locator) ModulePath() string {
	return l.modulePath
}

// FindPackageDir converts an import path to a physical directory path.
func (l *Locator) FindPackageDir(importPath string) (string, error) {
	// 1. Check replace directives
	for _, r := range l.replaces {
		if strings.HasPrefix(importPath, r.OldPath) {
			remainingPath := strings.TrimPrefix(importPath, r.OldPath)
			if remainingPath != "" && !strings.HasPrefix(remainingPath, "/") {
				continue
			}
			remainingPath = strings.TrimPrefix(remainingPath, "/")

			if r.IsLocal {
				var localCandidatePath string
				if filepath.IsAbs(r.NewPath) {
					localCandidatePath = filepath.Join(r.NewPath, remainingPath)
				} else {
					localCandidatePath = filepath.Join(l.rootDir, r.NewPath, remainingPath)
				}
				absLocalCandidatePath, err := filepath.Abs(localCandidatePath)
				if err != nil {
					continue
				}
				if stat, statErr := l.FS.Stat(absLocalCandidatePath); statErr == nil && stat.IsDir() {
					return absLocalCandidatePath, nil
				}
			} else {
				newImportPath := r.NewPath
				if remainingPath != "" {
					newImportPath = r.NewPath + "/" + remainingPath
				}
				if l.modulePath != "" && strings.HasPrefix(newImportPath, l.modulePath) {
					relPath := strings.TrimPrefix(newImportPath, l.modulePath)
					candidatePath := filepath.Join(l.rootDir, relPath)
					if stat, err := l.FS.Stat(candidatePath); err == nil && stat.IsDir() {
						return candidatePath, nil
					}
				}
			}
		}
	}

	// 2. Try with the current module context
	if l.modulePath != "" && strings.HasPrefix(importPath, l.modulePath) {
		relPath := strings.TrimPrefix(importPath, l.modulePath)
		candidatePath := filepath.Join(l.rootDir, relPath)
		if stat, err := l.FS.Stat(candidatePath); err == nil && stat.IsDir() {
			return candidatePath, nil
		}
	}

	// 3. If resolver is enabled, try GOROOT and GOMODCACHE
	if l.UseGoModuleResolver {
		if l.goRoot != "" {
			candidatePath := filepath.Join(l.goRoot, "src", importPath)
			if stat, err := l.FS.Stat(candidatePath); err == nil && stat.IsDir() {
				return candidatePath, nil
			}
		}

		if l.goModCache != "" {
			for mod, ver := range l.requires {
				if strings.HasPrefix(importPath, mod) {
					escapedMod, err := module.EscapePath(mod)
					if err != nil {
						continue
					}
					baseDir := filepath.Join(l.goModCache, escapedMod+"@"+ver)
					remainingPath := strings.TrimPrefix(importPath, mod)
					candidatePath := filepath.Join(baseDir, remainingPath)

					if stat, err := l.FS.Stat(candidatePath); err == nil && stat.IsDir() {
						return candidatePath, nil
					}
				}
			}
		}
	}

	if l.modulePath != "" {
		return "", fmt.Errorf("import path %q could not be resolved. Current module is %q (root: %s)", importPath, l.modulePath, l.rootDir)
	}
	return "", fmt.Errorf("import path %q could not be resolved", importPath)
}

// findModuleRoot searches for any go.mod starting from a given directory and moving upwards.
func (l *Locator) findModuleRoot(dir string) (string, error) {
	currentDir := dir
	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := l.FS.Stat(goModPath); err == nil {
			return currentDir, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", fmt.Errorf("go.mod not found in or above %s", dir)
		}
		currentDir = parentDir
	}
}

// getModulePathFromBytes reads the module path from go.mod content.
func getModulePathFromBytes(content []byte) (string, error) {
	if len(content) == 0 {
		return "", fmt.Errorf("go.mod content is empty")
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "module") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				return parts[1], nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading go.mod content: %w", err)
	}

	return "", fmt.Errorf("module path not found in go.mod content")
}

// getReplaceDirectivesFromBytes reads replace directives from go.mod content.
func getReplaceDirectivesFromBytes(content []byte) ([]ReplaceDirective, error) {
	if len(content) == 0 {
		return nil, nil
	}
	var directives []ReplaceDirective
	scanner := bufio.NewScanner(bytes.NewReader(content))
	inReplaceBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "//") {
			continue
		}

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "replace") {
			if strings.Contains(line, "(") {
				inReplaceBlock = true
				line = strings.TrimSpace(strings.TrimPrefix(line, "replace"))
				line = strings.TrimSpace(strings.TrimPrefix(line, "("))
				if line != "" {
					directive, err := parseReplaceLine(line)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: skipping malformed replace directive line: %q in go.mod: %v\n", line, err)
						continue
					}
					directives = append(directives, directive)
				}
				continue
			} else {
				contentParts := strings.Fields(line)
				if len(contentParts) < 1 {
					continue
				}
				directiveLine := strings.Join(contentParts[1:], " ")

				directive, err := parseReplaceLine(directiveLine)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping malformed single-line replace directive content: %q (from line: %q) in go.mod: %v\n", directiveLine, line, err)
					continue
				}
				directives = append(directives, directive)
			}
		} else if inReplaceBlock {
			if line == ")" {
				inReplaceBlock = false
				continue
			}
			directive, err := parseReplaceLine(line)
			if err != nil {
				continue
			}
			directives = append(directives, directive)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading go.mod content: %w", err)
	}

	return directives, nil
}

// getGoModCache finds the path to the module cache directory by calling `go env GOMODCACHE`.
func getGoModCache() (string, error) {
	cmd := exec.Command("go", "env", "GOMODCACHE")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'go env GOMODCACHE': %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// getRequireDirectivesFromBytes reads require directives from go.mod content.
func getRequireDirectivesFromBytes(content []byte) (map[string]string, error) {
	if len(content) == 0 {
		return nil, nil
	}
	requires := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "//") {
			continue
		}
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "require") {
			if strings.Contains(line, "(") {
				inRequireBlock = true
				line = strings.TrimSpace(strings.TrimPrefix(line, "require"))
				line = strings.TrimSpace(strings.TrimPrefix(line, "("))
			} else {
				parts := strings.Fields(line)
				if len(parts) == 3 {
					requires[parts[1]] = parts[2]
				}
				continue
			}
		}

		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				version := parts[1]
				if len(parts) > 2 && parts[2] == "//" {
				}
				requires[parts[0]] = version
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading go.mod content for require directives: %w", err)
	}

	return requires, nil
}

// parseReplaceLine parses a single line of a replace directive.
func parseReplaceLine(line string) (ReplaceDirective, error) {
	parts := strings.Fields(line)
	arrowIndex := -1
	for i, p := range parts {
		if p == "=>" {
			arrowIndex = i
			break
		}
	}

	if arrowIndex == -1 || arrowIndex == 0 || arrowIndex == len(parts)-1 {
		return ReplaceDirective{}, fmt.Errorf("malformed replace directive line: %q (missing or misplaced '=>')", line)
	}

	var dir ReplaceDirective
	oldParts := parts[:arrowIndex]
	newParts := parts[arrowIndex+1:]

	dir.OldPath = oldParts[0]
	if len(oldParts) > 1 {
		dir.OldVersion = oldParts[1]
	}

	newPathOrModule := newParts[0]
	if strings.HasPrefix(newPathOrModule, "./") || strings.HasPrefix(newPathOrModule, "../") || filepath.IsAbs(newPathOrModule) {
		dir.IsLocal = true
		dir.NewPath = newPathOrModule
		if len(newParts) > 1 {
			return ReplaceDirective{}, fmt.Errorf("local replacement path %q should not have a version: %q", dir.NewPath, line)
		}
	} else {
		dir.IsLocal = false
		dir.NewPath = newPathOrModule
		if len(newParts) > 1 {
			dir.NewVersion = newParts[1]
		} else {
			return ReplaceDirective{}, fmt.Errorf("non-local replacement path %q requires a version: %q", dir.NewPath, line)
		}
	}

	return dir, nil
}

// PathToImport converts an absolute directory path to its corresponding Go import path.
func (l *Locator) PathToImport(absPath string) (string, error) {
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %w", absPath, err)
	}

	if strings.HasPrefix(absPath, l.rootDir) {
		relPath, err := filepath.Rel(l.rootDir, absPath)
		if err != nil {
			return "", fmt.Errorf("failed to get relative path for %s from root %s: %w", absPath, l.rootDir, err)
		}
		if relPath == "." {
			return l.modulePath, nil
		}
		return filepath.ToSlash(filepath.Join(l.modulePath, relPath)), nil
	}

	for _, r := range l.replaces {
		if !r.IsLocal {
			continue
		}
		var replacedDirAbs string
		if filepath.IsAbs(r.NewPath) {
			replacedDirAbs = r.NewPath
		} else {
			replacedDirAbs = filepath.Join(l.rootDir, r.NewPath)
		}

		if strings.HasPrefix(absPath, replacedDirAbs) {
			relPath, err := filepath.Rel(replacedDirAbs, absPath)
			if err != nil {
				return "", fmt.Errorf("failed to get relative path for %s from replaced dir %s: %w", absPath, replacedDirAbs, err)
			}
			if relPath == "." {
				return r.OldPath, nil
			}
			return filepath.ToSlash(filepath.Join(r.OldPath, relPath)), nil
		}
	}

	return "", fmt.Errorf("could not determine import path for directory %s", absPath)
}

// ResolvePkgPath converts a file path to a full Go package path.
func ResolvePkgPath(ctx context.Context, path string) (string, error) {
	isFilePathLike := strings.HasPrefix(path, ".") || filepath.IsAbs(path)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if isFilePathLike {
				return "", fmt.Errorf("path %q does not exist: %w", path, err)
			}
			return path, nil
		}
		return "", fmt.Errorf("error checking path %q: %w", path, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for %q: %w", path, err)
	}

	searchDir := absPath
	if !info.IsDir() {
		searchDir = filepath.Dir(absPath)
	}

	modRoot, err := findModuleRootStandalone(searchDir)
	if err != nil {
		return "", fmt.Errorf("could not find go.mod for path %q: %w", path, err)
	}
	goModPath := filepath.Join(modRoot, "go.mod")

	modBytes, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("could not read go.mod at %s: %w", goModPath, err)
	}

	modulePath, err := getModulePathFromBytes(modBytes)
	if err != nil {
		return "", fmt.Errorf("could not parse module path from %s: %w", goModPath, err)
	}

	relPath, err := filepath.Rel(modRoot, searchDir)
	if err != nil {
		return "", fmt.Errorf("could not determine relative path of %s from %s: %w", searchDir, modRoot, err)
	}

	pkgPath := filepath.ToSlash(relPath)
	if pkgPath == "." {
		return modulePath, nil
	}
	return modulePath + "/" + pkgPath, nil
}

// findModuleRootStandalone is a version of findModuleRoot that uses the os package directly.
func findModuleRootStandalone(dir string) (string, error) {
	currentDir := dir
	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return currentDir, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", fmt.Errorf("go.mod not found in or above %s", dir)
		}
		currentDir = parentDir
	}
}
