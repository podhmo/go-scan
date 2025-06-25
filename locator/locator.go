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

	return "", fmt.Errorf("import path %q could not be resolved. Current module is %q (root: %s)", importPath, l.modulePath, l.rootDir)
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
