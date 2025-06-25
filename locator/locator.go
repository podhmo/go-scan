package locator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Locator helps find the module root and resolve package import paths.
type Locator struct {
	modulePath string
	rootDir    string
}

// New creates a new Locator by searching for a go.mod file.
// It starts searching from startPath and moves up the directory tree.
func New(startPath string) (*Locator, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", startPath, err)
	}

	rootDir, err := findModuleRoot(absPath)
	if err != nil {
		return nil, err
	}

	modPath, err := getModulePath(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		return nil, err
	}

	return &Locator{
		modulePath: modPath,
		rootDir:    rootDir,
	}, nil
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
	// Try with the current module context first
	if strings.HasPrefix(importPath, l.modulePath) {
		relPath := strings.TrimPrefix(importPath, l.modulePath)
		candidatePath := filepath.Join(l.rootDir, relPath)
		// Check if directory exists before returning
		if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
			return candidatePath, nil
		}
	}

	// Fallback for go-scan's own examples/testdata when they are not the primary module
	const goScanModulePath = "github.com/podhmo/go-scan"
	if strings.HasPrefix(importPath, goScanModulePath) {
		// Check if the current locator's context (l.rootDir, l.modulePath) is already for goScanModulePath
		if l.modulePath == goScanModulePath {
			// If current context is already go-scan, try to resolve relative to its root (l.rootDir)
			relPath := strings.TrimPrefix(importPath, goScanModulePath)
			candidatePath := filepath.Join(l.rootDir, relPath)
			if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
				return candidatePath, nil
			}
		} else {
			// Current context is not go-scan (e.g., it's an example's module like derivingjson).
			// Try to find the actual go-scan project root by searching upwards from l.rootDir
			// for a go.mod that defines "module github.com/podhmo/go-scan".
			goScanActualRootDir, err := findModuleRootForPath(l.rootDir, goScanModulePath)
			if err == nil {
				// Now construct path relative to this actual root
				relPath := strings.TrimPrefix(importPath, goScanModulePath)
				candidatePath := filepath.Join(goScanActualRootDir, relPath)
				if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
					return candidatePath, nil
				}
			}
		}
	}

	// Original hardcoded testdata path for "example.com/multipkg-test"
	// This is highly specific and should ideally be refactored or covered by a more general mechanism.
	const multiPkgTestModulePath = "example.com/multipkg-test"
	if strings.HasPrefix(importPath, multiPkgTestModulePath) {
		// This assumes that "example.com/multipkg-test" corresponds to "testdata/multipkg"
		// relative to the true "github.com/podhmo/go-scan" root.
		goScanRepoRoot, err := findModuleRootForPath(l.rootDir, goScanModulePath) // Find actual go-scan root
		if err == nil {
			// Construct path: goScanRepoRoot + "testdata/multipkg" + (importPath - multiPkgTestModulePath)
			relPathFromTestModule := strings.TrimPrefix(importPath, multiPkgTestModulePath)
			// Ensure relPathFromTestModule is treated as relative segments, not an absolute path if empty.
			// For example, if importPath IS multiPkgTestModulePath, relPathFromTestModule will be empty.
			// We want .../testdata/multipkg in that case.
			// If importPath is "example.com/multipkg-test/api", relPathFromTestModule is "/api".
			// filepath.Join handles this correctly by cleaning up.
			candidatePath := filepath.Join(goScanRepoRoot, "testdata/multipkg", relPathFromTestModule)
			if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
				return candidatePath, nil
			}
		}
	}

	return "", fmt.Errorf("import path %q could not be resolved. Current module is %q (root: %s)", importPath, l.modulePath, l.rootDir)
}

// findModuleRootForPath searches upwards from startDir for a go.mod file
// that declares the given targetModulePath.
func findModuleRootForPath(startDir string, targetModulePath string) (string, error) {
	currentDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("findModuleRootForPath: failed to get absolute path for %s: %w", startDir, err)
	}

	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, statErr := os.Stat(goModPath); statErr == nil {
			// go.mod exists, check its module path
			modPath, readErr := getModulePath(goModPath)
			if readErr == nil && modPath == targetModulePath {
				return currentDir, nil // Found the target module root
			}
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir { // Reached root of filesystem or an error occurred
			break
		}
		currentDir = parentDir
	}
	return "", fmt.Errorf("module %q not found in or above %s", targetModulePath, startDir)
}

// findModuleRoot searches for any go.mod starting from a given directory and moving upwards.
func findModuleRoot(dir string) (string, error) {
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

// getModulePath reads the module path from a go.mod file.
func getModulePath(goModPath string) (string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return "", fmt.Errorf("could not open %s: %w", goModPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
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
		return "", fmt.Errorf("error reading %s: %w", goModPath, err)
	}

	return "", fmt.Errorf("module path not found in %s", goModPath)
}
