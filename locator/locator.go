package locator

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
		// It's okay if parsing replace directives fails, just log it or handle as a warning
		// For now, we'll proceed without them.
		// TODO: Add proper logging or error handling strategy.
		fmt.Fprintf(os.Stderr, "warning: could not parse replace directives in go.mod: %v\n", err)
	}

	return &Locator{
		modulePath: modPath,
		rootDir:    rootDir,
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
// It now considers replace directives.
func (l *Locator) FindPackageDir(importPath string) (string, error) {
	// 1. Check replace directives
	for _, r := range l.replaces {
		// Check if the importPath matches the OldPath of a replace directive.
		// This handles cases like:
		// replace old.module/path => ./local/path
		// replace old.module/path => new.module/path v1.2.3
		// replace old.module/path/subpkg => ./local/path/subpkg (implicit if old.module/path is replaced)
		//
		// We need to check if importPath starts with r.OldPath, and if so,
		// construct the new potential import path or local path.
		if strings.HasPrefix(importPath, r.OldPath) {
			remainingPath := strings.TrimPrefix(importPath, r.OldPath)
			if remainingPath != "" && !strings.HasPrefix(remainingPath, "/") {
				// This ensures we are matching a full path component, e.g. "old.mod/pkg" vs "old.modpkg"
				continue
			}
			remainingPath = strings.TrimPrefix(remainingPath, "/")

			if r.IsLocal {
				// Local replacement: construct the full local path
				// The r.NewPath is relative to l.rootDir
				var localCandidatePath string
				if filepath.IsAbs(r.NewPath) {
					localCandidatePath = filepath.Join(r.NewPath, remainingPath)
				} else {
					localCandidatePath = filepath.Join(l.rootDir, r.NewPath, remainingPath)
				}

				absLocalCandidatePath, err := filepath.Abs(localCandidatePath)
				if err != nil {
					// Problematic path, skip
					// fmt.Fprintf(os.Stderr, "warning: could not get absolute path for replaced local path %s: %v\n", localCandidatePath, err)
					continue
				}
				// Temporary debug was here

				stat, statErr := os.Stat(absLocalCandidatePath)
				// Temporary debug was here

				if statErr == nil && stat.IsDir() {
					return absLocalCandidatePath, nil
				}
				// If local replacement doesn't exist, we might fall through or error.
				// For now, if a replace rule matched, we prioritize its outcome.
				// If it doesn't lead to a valid dir, this specific rule fails.
				// We could opt to error here if the replace rule was specific and not found.
				// Or, allow falling through to other rules or default resolution.
				// Current Go behavior: if a replace directive applies, it's used. If the target is invalid, it's an error.
				// So, if we found a matching replace, and it leads to a non-existent local path, we should probably error.
				// However, to allow for multiple replace rules, we'll continue and error at the end if nothing is found.
				// Let's refine this: if a specific replace rule (OldPath matches importPath exactly) fails, it's an error.
				// If it's a prefix match (importPath is a sub-package), and the sub-package doesn't exist, that's also an error for this rule.
				// The original plan was to only do one level. Let's stick to that for now.
				// If a matching replace directive is found and the local path is invalid, we error out for this path.
				// This means we don't fall back to other resolution methods if a specific replace was attempted.
				// return "", fmt.Errorf("replaced import path %q (to local %s) could not be resolved: directory does not exist or is not a directory", importPath, absLocalCandidatePath)
				// Let's allow falling through for now to see if other rules or the default mechanism works.
				// This might need adjustment based on expected go tool behavior.
				// For now, if a local replacement points to a non-existent dir, we just continue to the next rule or default logic.
			} else {
				// Module-to-module replacement: construct the new import path
				newImportPath := r.NewPath
				if remainingPath != "" {
					newImportPath = r.NewPath + "/" + remainingPath
				}

				// Here, we need to resolve this newImportPath.
				// This could be complex if it's outside the current module context,
				// or if it itself is subject to another replace.
				// For simplicity, let's assume this newImportPath should be resolvable
				// within the standard GOPATH/GOROOT or relative to the current module if it matches modulePath.
				// This part is tricky because `FindPackageDir` is designed for the current module context.
				// A replaced module path might point to something outside the current module's structure.
				// The `go list -m -json <pkg>` command would be more robust for this.
				// However, we are trying to implement this within go-scan's locator.

				// If the new import path matches the current module path, resolve it locally.
				if strings.HasPrefix(newImportPath, l.modulePath) {
					relPath := strings.TrimPrefix(newImportPath, l.modulePath)
					candidatePath := filepath.Join(l.rootDir, relPath)
					if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
						return candidatePath, nil
					}
				}
				// TODO: How to handle replaced paths that are *not* part of the current module?
				// This would typically involve looking into GOPATH/pkg/mod or GOROOT.
				// For now, if a module replace points outside, and it's not the main module,
				// this basic locator might not find it.
				// We can log a warning or acknowledge this limitation.
				// fmt.Fprintf(os.Stderr, "warning: replaced import path %s (from %s) is outside current module scope and may not be found by this locator\n", newImportPath, importPath)

				// For now, we will attempt to resolve it as if it were a top-level import,
				// which means it must match the current modulePath if it's to be found by *this* Locator instance.
				// This is a simplification. A full resolver would check GOPATH/GOROOT.
				// If we simply call l.FindPackageDir(newImportPath) recursively, we risk infinite loops if not careful.
				// Let's try a direct resolution attempt based on the newImportPath relative to modulePath.
				// This is what the original logic below does.
				// Fall through to the original logic, but with newImportPath.
				// This is still not quite right for external modules.
				// The current structure of Locator is tied to a single module.
				//
				// Let's simplify: if it's a module-to-module replace, and the new module path
				// is the *same* as our current module path, then we resolve it.
				// Otherwise, we state it's outside our current capability.
				if strings.HasPrefix(newImportPath, l.modulePath) {
					relPath := strings.TrimPrefix(newImportPath, l.modulePath)
					candidatePath := filepath.Join(l.rootDir, relPath)
					if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
						return candidatePath, nil
					}
				} else {
					// If the replaced path points to a different module, this simple locator cannot find it
					// unless that different module's path is passed to a *new* Locator instance for that module.
					// For the current request, we can't resolve it if it's truly external.
					// We will let it fall through, and it will likely fail unless another rule matches,
					// or the original importPath itself matches the current module (which it wouldn't if a replace rule was hit).
					// This implies that module-to-module replaces that point to *other* modules are not fully supported by this iteration.
				}
			}
		}
	}

	// 2. Try with the current module context first (original logic)
	if strings.HasPrefix(importPath, l.modulePath) {
		relPath := strings.TrimPrefix(importPath, l.modulePath)
		candidatePath := filepath.Join(l.rootDir, relPath)
		// Check if directory exists before returning
		if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
			return candidatePath, nil
		}
	}

	// 3. Try standard library packages in GOROOT
	// This is a heuristic: standard library packages usually don't have dots.
	if !strings.Contains(importPath, ".") {
		goRoot := runtime.GOROOT()
		if goRoot != "" {
			candidatePath := filepath.Join(goRoot, "src", importPath)
			if stat, err := os.Stat(candidatePath); err == nil && stat.IsDir() {
				return candidatePath, nil
			}
		}
	}

	// If no replace directive matched and led to a path, and the original logic failed, then error.
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
		return nil, nil // No directives in empty file
	}
	var directives []ReplaceDirective
	scanner := bufio.NewScanner(bytes.NewReader(content))
	inReplaceBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "//") { // Skip comments
			continue
		}

		if line == "" { // Skip empty lines
			continue
		}

		if strings.HasPrefix(line, "replace") {
			if strings.Contains(line, "(") {
				inReplaceBlock = true
				line = strings.TrimSpace(strings.TrimPrefix(line, "replace"))
				line = strings.TrimSpace(strings.TrimPrefix(line, "("))
				// Process first line if it's not just "replace ("
				if line != "" {
					directive, err := parseReplaceLine(line)
					if err != nil {
						// TODO: Log or handle individual line parsing errors more gracefully
						// For now, skip malformed lines.
						fmt.Fprintf(os.Stderr, "warning: skipping malformed replace directive line: %q in go.mod: %v\n", line, err)
						continue
					}
					directives = append(directives, directive)
				}
				continue
			} else {
				// Single line replace
				contentParts := strings.Fields(line) // line is "replace old [v] => new [v]"
				if len(contentParts) < 1 {           // Should not happen if HasPrefix("replace") is true and line is trimmed
					continue
				}
				directiveLine := strings.Join(contentParts[1:], " ") // "old [v] => new [v]"

				// parseReplaceLine will check for "=>"
				directive, err := parseReplaceLine(directiveLine)
				if err != nil {
					// Log the original line for better context if parsing fails
					fmt.Fprintf(os.Stderr, "warning: skipping malformed single-line replace directive content: %q (from line: %q) in go.mod: %v\n", directiveLine, line, err)
					continue
				}
				directives = append(directives, directive)
				// No need for 'continue' here as it's the end of the 'if strings.HasPrefix(line, "replace")' block's else path
			}
		} else if inReplaceBlock { // Ensure this is 'else if' or structure appropriately
			if line == ")" {
				inReplaceBlock = false
				continue
			}
			directive, err := parseReplaceLine(line)
			if err != nil {
				// fmt.Fprintf(os.Stderr, "warning: skipping malformed replace directive line: %q in go.mod: %v\n", line, err)
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

// parseReplaceLine parses a single line of a replace directive.
// Example inputs:
// "old.module/path => new.module/path v1.2.3"
// "old.module/path v0.0.0 => new.module/path v1.2.3"
// "old.module/path => ./local/path"
// "old.module/path v1.0.0 => ./local/path"
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
			// This case should ideally not happen for local paths as per go.mod spec,
			// but we'll capture it if present.
			// The go command itself might error on such go.mod.
			return ReplaceDirective{}, fmt.Errorf("local replacement path %q should not have a version: %q", dir.NewPath, line)
		}
	} else {
		dir.IsLocal = false
		dir.NewPath = newPathOrModule
		if len(newParts) > 1 {
			dir.NewVersion = newParts[1]
		} else {
			// If it's not local and no version is specified, it's an error according to go.mod replace spec
			// unless it's a wildcard replacement (oldpath => newpath vX.Y.Z),
			// but our parsing targets specific versions or local paths.
			// For "oldmodule => newmodule", a version is required for newmodule.
			return ReplaceDirective{}, fmt.Errorf("non-local replacement path %q requires a version: %q", dir.NewPath, line)
		}
	}

	return dir, nil
}
