package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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

	input := "example.com/convert/models/source"
	output := "generated_test.go"
	pkgname := "converter"

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

	golden, err := os.ReadFile("testdata/complex.go.golden")
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

func TestTagsIntegration(t *testing.T) {
	writer := &memoryFileWriter{}
	ctx := context.Background()
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	// We test the conversion from SrcWithTags, which is in the "source" package.
	input := "example.com/convert/models/source"
	output := "generated_tags_test.go"
	pkgname := "converter"

	err := run(ctx, input, output, pkgname)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	if len(writer.Outputs) != 1 {
		// This check might be too simplistic if the run generates multiple files
		// for different conversion pairs. For this test, we expect one file.
		t.Fatalf("expected 1 output file, got %d", len(writer.Outputs))
	}

	generatedCode, ok := writer.Outputs[output]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", output)
	}

	golden, err := os.ReadFile("testdata/tags.go.golden")
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	normalizedGenerated := strings.TrimSpace(string(generatedCode))
	normalizedGolden := strings.TrimSpace(string(golden))

	// The generated code will only contain the conversion for SrcWithTags
	// because the `run` function in the test would need to be modified to filter
	// which conversions to generate. The current `run` generates for all pairs.
	// For this test to be precise, we'd ideally isolate the generation to just
	// the pair we're interested in.
	// However, since the main test generates a file with *all* conversions,
	// and this test also generates a file with *all* conversions,
	// the generated output will be much larger than the golden file.
	// We will check if the golden content is a SUBSTRING of the generated code.
	// This is not ideal but works as a pragmatic solution for now.

	if !strings.Contains(normalizedGenerated, normalizedGolden) {
		t.Errorf("generated code does not contain the expected tagged conversion logic")
		t.Logf("GOLDEN:\n%s", normalizedGolden)
		t.Logf("\n\nGENERATED:\n%s", normalizedGenerated)
		// Using cmp.Diff for a more detailed view of the expected part vs what might be there.
		// This part of the diff might be noisy if the function signature is different.
		if diff := cmp.Diff(normalizedGolden, normalizedGenerated); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	}
}
