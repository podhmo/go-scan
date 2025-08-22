package main

import (
	"log/slog"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestDocgen_integrationWithSharedScanner(t *testing.T) {
	// This test ensures that a host tool (like docgen) can pass its own
	// pre-configured scanner to the minigo interpreter, and that the
	// interpreter can use it to resolve imports within a complex,
	// nested module structure created by scantest.

	files := map[string]string{
		// Main module for the test.
		// The `replace` directive is crucial. It allows the temporary module
		// to find the main `go-scan` project code.
		"go.mod": "module my-test\n\ngo 1.24\n\nreplace github.com/podhmo/go-scan => ../../\n",

		// The minigo script that will be loaded by docgen's loader.
		// It imports a package from a nested module.
		"patterns.go": `
package patterns
import (
	"my-test/api"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
)
var Patterns = []patterns.PatternConfig{
	{Key: "my-test/api.Hello", Type: patterns.RequestBody},
}
var _ = api.Hello
`,
		// The nested module and the package to be imported.
		"api/go.mod": "module my-test/api\n\ngo 1.24\n",
		"api/api.go": `
package api
func Hello() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Manually create the scanner, pointing its workdir to the temp dir.
	// This is the most direct way to ensure the scanner has the correct module context.
	s, err := goscan.New(goscan.WithWorkDir(dir))
	if err != nil {
		t.Fatalf("failed to create scanner: %+v", err)
	}

	// Construct the full path to the patterns file and load it.
	patternsFilePath := filepath.Join(dir, "patterns.go")
	logger := slog.Default()
	if _, err := LoadPatternsFromConfig(patternsFilePath, logger, s); err != nil {
		t.Fatalf("LoadPatternsFromConfig failed: %+v", err)
	}
}
