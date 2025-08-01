package locator

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/podhmo/go-scan/scanner"
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
	goRoot     string
	replaces   []ReplaceDirective
	overlay    scanner.Overlay
}

// New creates a new Locator by searching for a go.mod file.
// It starts searching from startPath and moves up the directory tree.
// It accepts an overlay to provide in-memory content for go.mod.
func New(startPath string, overlay scanner.Overlay) (*Locator, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", startPath, err)
	}

	rootDir, err := findModuleRoot(absPath)
	if err != nil {
		return nil, err
	}

	var goModContent []byte
	if overlay != nil {
		if content, ok := overlay["go.mod"]; ok {
			goModContent = content
		}
	}

	if goModContent == nil {
		goModFilePath := filepath.Join(rootDir, "go.mod")
		goModContent, err = os.ReadFile(goModFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read go.mod at %s: %w", goModFilePath, err)
		}
	}

	modPath, err := getModulePathFromBytes(goModContent)
	if err != nil {
		return nil, fmt.Errorf("failed to get module path from go.mod content: %w", err)
	}

	replaces, err := getReplaceDirectivesFromBytes(goModContent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse replace directives in go.mod: %v\n", err)
	}

	var goRoot string
	cmd := exec.Command("go", "env", "GOROOT")
	output, err := cmd.Output()
	if err == nil {
		goRoot = strings.TrimSpace(string(output))
	} else {
		goRoot = os.Getenv("GOROOT")
	}

	return &Locator{
		modulePath: modPath,
		rootDir:    rootDir,
		goRoot:     goRoot,
		replaces:   replaces,
		overlay:    overlay,
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
	// 0. Check for standard library packages first.
	if !strings.Contains(strings.Split(importPath, "/")[0], ".") {
		if l.goRoot != "" {
			candidatePath := filepath.Join(l.goRoot, "src", importPath)
			if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
				return candidatePath, nil
			}
		}
	}

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
				if stat, statErr := os.Stat(absLocalCandidatePath); statErr == nil && stat.IsDir() {
					return absLocalCandidatePath, nil
				}
			} else {
				newImportPath := r.NewPath
				if remainingPath != "" {
					newImportPath = r.NewPath + "/" + remainingPath
				}
				if strings.HasPrefix(newImportPath, l.modulePath) {
					relPath := strings.TrimPrefix(newImportPath, l.modulePath)
					candidatePath := filepath.Join(l.rootDir, relPath)
					if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
						return candidatePath, nil
					}
				}
			}
		}
	}

	// 2. Try with the current module context first (original logic)
	if strings.HasPrefix(importPath, l.modulePath) {
		relPath := strings.TrimPrefix(importPath, l.modulePath)
		candidatePath := filepath.Join(l.rootDir, relPath)
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
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}

		if strings.HasPrefix(line, "replace") {
			if strings.Contains(line, "(") {
				inReplaceBlock = true
				line = strings.TrimSpace(strings.TrimPrefix(line, "replace ("))
				if line != "" {
					directive, err := parseReplaceLine(line)
					if err == nil {
						directives = append(directives, directive)
					}
				}
			} else {
				directiveLine := strings.Join(strings.Fields(line)[1:], " ")
				directive, err := parseReplaceLine(directiveLine)
				if err == nil {
					directives = append(directives, directive)
				}
			}
		} else if inReplaceBlock {
			if line == ")" {
				inReplaceBlock = false
				continue
			}
			directive, err := parseReplaceLine(line)
			if err == nil {
				directives = append(directives, directive)
			}
		}
	}
	return directives, scanner.Err()
}

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
		return ReplaceDirective{}, fmt.Errorf("malformed replace directive line: %q", line)
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
			return ReplaceDirective{}, fmt.Errorf("local replacement path %q should not have a version", dir.NewPath)
		}
	} else {
		dir.IsLocal = false
		dir.NewPath = newPathOrModule
		if len(newParts) > 1 {
			dir.NewVersion = newParts[1]
		} else {
			return ReplaceDirective{}, fmt.Errorf("non-local replacement path %q requires a version", dir.NewPath)
		}
	}

	return dir, nil
}
