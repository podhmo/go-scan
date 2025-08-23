package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan"
)

func TestFindOrphans(t *testing.T) {
	ctx := context.Background()

	// 1. Create a temporary directory and write test files to it
	tmpdir := t.TempDir()
	files := map[string]string{
		"go.mod":               "module example.com/find-orphans/testdata\n\ngo 1.21.0",
		"main.go":              string(readFile(t, "testdata/main.go")),
		"greeter/greeter.go":   string(readFile(t, "testdata/greeter/greeter.go")),
		"english/english.go":   string(readFile(t, "testdata/english/english.go")),
		"japanese/japanese.go": string(readFile(t, "testdata/japanese/japanese.go")),
		"utils/utils.go":       string(readFile(t, "testdata/utils/utils.go")),
	}
	for path, content := range files {
		fullPath := filepath.Join(tmpdir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("MkdirAll failed for %q: %v", fullPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile failed for %q: %v", fullPath, err)
		}
	}

	// 2. Setup the scanner to use the temporary directory
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewScanner() failed: %v", err)
	}

	// 3. Create the analyzer
	analyzer, err := NewAnalyzer(s, logger)
	if err != nil {
		t.Fatalf("NewAnalyzer() failed: %v", err)
	}

	// 4. Discover packages
	pkgPaths := []string{
		"example.com/find-orphans/testdata",
		"example.com/find-orphans/testdata/greeter",
		"example.com/find-orphans/testdata/english",
		"example.com/find-orphans/testdata/japanese",
		"example.com/find-orphans/testdata/utils",
	}
	pkgs, err := analyzer.DiscoverPackages(ctx, pkgPaths)
	if err != nil {
		t.Fatalf("DiscoverPackages() failed: %v", err)
	}

	// 5. Run the analysis
	result, err := analyzer.Analyze(ctx, pkgs)
	if err != nil {
		t.Fatalf("Analyze() failed: %v", err)
	}

	// 6. Capture the report output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	result.Report(false) // Test non-verbose report first

	w.Close()
	os.Stdout = oldStdout // Restore
	buf.ReadFrom(r)

	// 7. Assert the non-verbose output
	got := buf.String()
	want := `
-- Orphans --
(example.com/find-orphans/testdata/english.Greeter).GreetFormal
  english/english.go:9:1
example.com/find-orphans/testdata/greeter.UnusedMethod
  greeter/greeter.go:19:1
(example.com/find-orphans/testdata/japanese.Greeter).Greet
  japanese/japanese.go:5:1
example.com/find-orphans/testdata/utils.OrphanUtil
  utils/utils.go:5:1
`
	normalize := func(s string) string {
		// remove tmpdir path
		s = strings.ReplaceAll(s, tmpdir, "")
		// remove leading slashes from paths
		s = strings.ReplaceAll(s, "\n  /", "\n  ")
		// normalize pointer receiver representation by removing the *
		s = strings.ReplaceAll(s, ".*", ".")
		// normalize windows line endings
		s = strings.ReplaceAll(s, "\r\n", "\n")
		return strings.TrimSpace(s)
	}

	if diff := cmp.Diff(normalize(want), normalize(got)); diff != "" {
		t.Errorf("Report mismatch (-want +got):\n%s", diff)
		t.Logf("Got output:\n%s", got)
	}

	// 8. Test verbose output
	buf.Reset()
	r, w, _ = os.Pipe()
	os.Stdout = w

	result.Report(true) // Test verbose report

	w.Close()
	os.Stdout = oldStdout // Restore
	gotVerbose := normalize(buf.String())

	expectedSubstrings := []string{
		"-- Used Functions --",
		"(example.com/find-orphans/testdata/english.Greeter).Greet",
		"- used by: example.com/find-orphans/testdata/greeter.SayHello",
		"example.com/find-orphans/testdata/greeter.New",
		"- used by: example.com/find-orphans/testdata.main",
		"example.com/find-orphans/testdata/utils.UsedUtil",
		"-- Orphans --",
		"(example.com/find-orphans/testdata/english.Greeter).GreetFormal",
		"example.com/find-orphans/testdata/utils.OrphanUtil",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(gotVerbose, sub) {
			t.Errorf("Verbose output missing expected substring:\n%q", sub)
		}
	}
	if strings.Contains(gotVerbose, "IgnoredUtil") {
		t.Errorf("Verbose output should not contain ignored function 'IgnoredUtil'")
	}
	if !strings.Contains(gotVerbose, "(example.com/find-orphans/testdata/japanese.Greeter).Greet") {
		t.Errorf("Orphan list in verbose output should contain japanese.Greeter.Greet, but it was missing")
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file %q: %v", path, err)
	}
	return data
}
