package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update golden files")

// memoryFileWriter is an in-memory implementation of FileWriter for testing.
type memoryFileWriter struct {
	mu      sync.Mutex
	Outputs map[string][]byte
}

func (w *memoryFileWriter) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Outputs == nil {
		w.Outputs = make(map[string][]byte)
	}
	w.Outputs[filepath.Base(path)] = data
	return nil
}

func TestMainIntegration(t *testing.T) {
	writer := &memoryFileWriter{}
	ctx := context.Background()
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	input := "example.com/convert/sampledata/source"
	output := "generated_test.go"
	pkgname := "converter"
	goldenFile := "testdata/complex.go.golden"

	err := run(ctx, input, output, pkgname)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	if len(writer.Outputs) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(writer.Outputs))
	}

	generatedCode, ok := writer.Outputs[output]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", output)
	}

	if *update {
		if err := os.WriteFile(goldenFile, generatedCode, 0644); err != nil {
			t.Fatalf("failed to update golden file: %v", err)
		}
		t.Logf("golden file updated: %s", goldenFile)
		return
	}

	golden, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	normalizedGenerated := strings.TrimSpace(string(generatedCode))
	normalizedGolden := strings.TrimSpace(string(golden))

	if diff := cmp.Diff(normalizedGolden, normalizedGenerated); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
		t.Logf("Dumping generated code:\n\n%s", normalizedGenerated)
	}
}
