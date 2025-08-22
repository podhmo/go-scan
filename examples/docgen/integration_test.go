package main

import (
	"context"
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
		// to find the main `go-scan` project code. `scantest` will
		// automatically convert the relative path to an absolute one.
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

	// Action to be performed by scantest.Run
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// The test is not guaranteed to run with the temporary directory as its CWD.
		// We must construct an absolute path to the patterns file.
		patternsFilePath := filepath.Join(dir, "patterns.go")
		logger := slog.Default()
		_, err := LoadPatternsFromConfig(patternsFilePath, logger, s)
		return err
	}

	// We don't need to specify patterns to scan initially, as the action
	// itself is what we are testing. `scantest` will correctly configure the
	// scanner `s` with the temporary directory `dir` as its root.
	if _, err := scantest.Run(t, dir, nil, action); err != nil {
		t.Fatalf("scantest.Run failed: %+v", err)
	}
}
