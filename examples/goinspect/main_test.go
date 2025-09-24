package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scantest"
)

var update = flag.Bool("update", false, "update golden files")

func TestGolden(t *testing.T) {
	// Golden files are stored relative to the package directory.
	// We need to ensure the testdata directory exists.
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("failed to create testdata dir: %v", err)
	}

	files := map[string]string{
		"go.mod": "module example.com/a\ngo 1.20\n",
		"a.go": `
package a

func F(s S) {
	log()
	F0()
	H()
}

func F0() {
	log()
	F1()
}

func F1() {
	H()
}

func G() {
	// G calls nothing
}

func H() {
	// H calls nothing, but is called by F and F1
}

func log() func() {
	return func() {}
}

type S struct{}

func (s S) M() {
	F()
}

// Recursive function
func Recur(n int) {
	if n > 0 {
		Recur(n - 1)
	}
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Redirect stdout to a buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	// Store the original working directory to construct the golden file path.
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}

	// Change working directory to the temp dir for the scanner to work correctly.
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(originalWd)

	// Set command-line flags for the run
	flag.CommandLine = flag.NewFlagSet("goinspect", flag.ExitOnError)
	flagPkg = flag.String("pkg", "./...", "package pattern (required)")
	flagIncludeUnexported = flag.Bool("include-unexported", true, "include unexported functions")
	flagShort = flag.Bool("short", false, "short output")
	flagExpand = flag.Bool("expand", false, "expand output")
	flagVerbose = flag.Bool("v", false, "verbose output")


	err = run(context.Background())
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout
	log.SetOutput(os.Stderr)

	var buf bytes.Buffer
	io.Copy(&buf, r)

	// Normalize line endings for comparison
	got := strings.ReplaceAll(buf.String(), "\r\n", "\n")

	goldenFile := filepath.Join(originalWd, "testdata", "default.golden")

	if *update {
		if err := os.WriteFile(goldenFile, []byte(got), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenFile)
		return // Do not compare when updating
	}

	expectedBytes, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
	}
	expected := strings.ReplaceAll(string(expectedBytes), "\r\n", "\n")

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("output does not match golden file %s\n%s", goldenFile, diff)
	}
}