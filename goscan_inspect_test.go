package goscan_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

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
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	// 4. Define the test action
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*scanner.PackageInfo) error {
		for _, pkg := range pkgs {
			for _, ti := range pkg.Types {
				_, _ = ti.Annotation("deriving:json")
			}
		}
		if !s.DryRun {
			t.Error("DryRun flag should be true on the scanner instance")
		}
		return nil
	}

	// The patterns to scan, relative to the temp dir.
	patterns := []string{"models"}

	// 5. Run the test using the pre-configured scanner
	result, err := scantest.Run(t, dir, patterns, action, scantest.WithScanner(s))
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	// 6. Assertions
	if result != nil {
		t.Errorf("Expected no output files in dry-run mode, but got %d", len(result.Outputs))
	}

	logOutput := logBuf.String()
	t.Logf("Captured logs:\n%s", logOutput)

	// Check for the successful "hit" on User type
	if !strings.Contains(logOutput, `level=INFO msg="found annotation"`) {
		t.Errorf("log output did not contain expected INFO message for found annotation")
	}
	if !strings.Contains(logOutput, `type_name=User`) {
		t.Errorf("log output did not contain expected type_name=User")
	}
	if !strings.Contains(logOutput, `annotation_name=@deriving:json`) {
		t.Errorf("log output did not contain expected annotation_name=@deriving:json")
	}

	// Check for the "miss" on Group type
	if !strings.Contains(logOutput, `level=DEBUG msg="checking for annotation"`) {
		t.Errorf("log output did not contain expected DEBUG message for checking annotation")
	}
	if !strings.Contains(logOutput, `type_name=Group`) {
		t.Errorf("log output did not contain expected type_name=Group")
	}
	if !strings.Contains(logOutput, `result=miss`) {
		t.Errorf("log output did not contain expected result=miss")
	}
}
