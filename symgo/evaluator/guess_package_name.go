package evaluator

import (
	"regexp"
	"strings"
)

// versionSuffixRegex matches a trailing /vN path segment.
var versionSuffixRegex = regexp.MustCompile(`^v[0-9]+$`)

// guessPackageNameFromImportPath provides a heuristic to determine a package's
// potential names from its import path. It returns a slice of candidates.
func guessPackageNameFromImportPath(path string) []string {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")

	// Start with the last path segment.
	baseName := parts[len(parts)-1]

	// Handle gopkg.in/some-pkg.vN by splitting on the dot.
	if strings.HasPrefix(path, "gopkg.in/") {
		if dotIndex := strings.LastIndex(baseName, "."); dotIndex > 0 {
			baseName = baseName[:dotIndex]
		}
	}

	// If the last segment is a version suffix (e.g., "v5"), use the segment before it.
	if versionSuffixRegex.MatchString(baseName) {
		if len(parts) > 1 {
			baseName = parts[len(parts)-2]
		}
	}

	// Remove ".git" suffix if present
	baseName = strings.TrimSuffix(baseName, ".git")

	// Now generate candidates based on the cleaned baseName.
	candidates := make(map[string]struct{})

	// Candidate 1: a direct sanitization (e.g., "go-isatty" -> "goisatty")
	sanitized := strings.ReplaceAll(baseName, "-", "")
	candidates[sanitized] = struct{}{}

	// Candidate 2: strip "go-" prefix and then sanitize
	if strings.HasPrefix(baseName, "go-") {
		stripped := strings.TrimPrefix(baseName, "go-")
		strippedAndSanitized := strings.ReplaceAll(stripped, "-", "")
		candidates[strippedAndSanitized] = struct{}{}
	}

	// Candidate 3: strip "-go" suffix and then sanitize
	if strings.HasSuffix(baseName, "-go") {
		stripped := strings.TrimSuffix(baseName, "-go")
		strippedAndSanitized := strings.ReplaceAll(stripped, "-", "")
		candidates[strippedAndSanitized] = struct{}{}
	}

	// Convert map to slice to return a stable (though unordered) list of unique names.
	result := make([]string, 0, len(candidates))
	for name := range candidates {
		result = append(result, name)
	}
	return result
}
