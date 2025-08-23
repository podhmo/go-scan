package goscan

import (
	"go/token"
	"log/slog"

	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Config holds shared components and settings that can be used across
// a ModuleWalker and a Scanner to ensure they operate consistently.
// This is useful for advanced tools that perform both dependency analysis
// and deep code inspection.
type Config struct {
	// Fset is the shared file set for parsing. Sharing a FileSet is crucial
	// for correlating position information between different scans.
	Fset *token.FileSet

	// Locator is the shared package locator. Sharing a Locator improves
	// performance by caching module and package lookups.
	Locator *locator.Locator

	// Logger is the shared logger for all components.
	Logger *slog.Logger

	// Overlay provides a shared in-memory file overlay, allowing tools to
	// analyze modified or unsaved content consistently.
	Overlay scanner.Overlay
}
