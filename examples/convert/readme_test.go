package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadme(t *testing.T) {
	want, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	got, err := os.ReadFile(filepath.Join("testdata", "README.md.golden"))
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("README.md mismatch (-want +got):\n%s", diff)
	}
}
