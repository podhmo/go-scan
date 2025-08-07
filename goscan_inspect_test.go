package goscan_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
)

func TestInspectAndDryRun(t *testing.T) {
	// 1. Setup: Create a temporary directory with a sample Go file.
	files := map[string]string{
		"go.mod": "module a.b/c",
		"models/user.go": `
package models

// @deriving:json
type User struct {
	ID   int
	Name string
}

type Group struct { // No annotation
	ID int
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 2. Capture logs
	var logBuf bytes.Buffer
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug) // Ensure DEBUG logs are captured
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	logger := slog.New(handler)

	// 3. Configure the scanner with inspect and dry-run options
	scannerOptions := []goscan.ScannerOption{
		goscan.WithInspect(true),
		goscan.WithLogger(logger),
		goscan.WithDryRun(true),
		goscan.WithWorkDir(dir), // Set workdir to the temp dir
	}
	s, err := goscan.New(scannerOptions...)
	require.NoError(t, err)

	// 4. Define the test action
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*scanner.PackageInfo) error {
		for _, pkg := range pkgs {
			for _, ti := range pkg.Types {
				_, _ = ti.Annotation("deriving:json")
			}
		}
		assert.True(t, s.DryRun, "DryRun flag should be true on the scanner instance")
		return nil
	}

	// The patterns to scan, relative to the temp dir.
	patterns := []string{"models"}

	// 5. Run the test using the pre-configured scanner
	result, err := scantest.Run(t, dir, patterns, action, scantest.WithScanner(s))
	require.NoError(t, err)

	// 6. Assertions
	assert.Nil(t, result, "Expected no output files in dry-run mode")

	logOutput := logBuf.String()
	t.Logf("Captured logs:\n%s", logOutput)

	// Check for the successful "hit" on User type
	assert.Contains(t, logOutput, `level=INFO msg="found annotation"`)
	assert.Contains(t, logOutput, `type_name=User`)
	assert.Contains(t, logOutput, `annotation_name=@deriving:json`)
	assert.Contains(t, logOutput, `annotation_value=""`)

	// Check for the "miss" on Group type
	assert.Contains(t, logOutput, `level=DEBUG msg="checking for annotation"`)
	assert.Contains(t, logOutput, `type_name=Group`)
	assert.Contains(t, logOutput, `result=miss`)
}
