package main

import (
	"context"
	"flag"
	"go/format"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scantest"
)

var update = flag.Bool("update", false, "update golden files")

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

func TestIntegration_WithTags(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"tags.go": `
package tags
import "context"

// @derivingconvert("DstWithTags")
type SrcWithTags struct {
	ID        string
	Name      string ` + "`convert:\"-\"`" + `
	Age       int    ` + "`convert:\"UserAge\"`" + `
	Profile   string ` + "`convert:\",using=convertProfile\"`" + `
	ManagerID *int   ` + "`convert:\",required\"`" + `
}

type DstWithTags struct {
	ID        string
	UserAge   int
	Profile   string
	ManagerID *int
}

func convertProfile(ctx context.Context, s string) string {
	return "profile:" + s
}
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "tags"
	goldenFile := "testdata/tags.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
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

	formattedGenerated, err := format.Source(generatedCode)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := format.Source(golden)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithGlobalRule(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"rules.go": `
package rules
import (
	"context"
	"time"
)

// // convert:rule "time.Time" -> "string", using=convertTimeToString

// @derivingconvert("Dst")
type Src struct {
	CreatedAt time.Time
	UpdatedAt time.Time ` + "`convert:\",using=overrideTime\"`" + `
}

type Dst struct {
	CreatedAt string
	UpdatedAt string
}

func convertTimeToString(ctx context.Context, t time.Time) string {
	return t.Format("2006-01-02")
}

func overrideTime(ctx context.Context, t time.Time) string {
	return "overridden"
}
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "rules"
	goldenFile := "testdata/rules.go.golden"

	// Create a dummy golden file if it doesn't exist
	if _, err := os.Stat(goldenFile); os.IsNotExist(err) {
		os.WriteFile(goldenFile, []byte(""), 0644)
	}

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
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

	formattedGenerated, err := format.Source(generatedCode)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := format.Source(golden)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithMaps(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"maps.go": `
package maps
import "context"

// @derivingconvert("Dst")
type Src struct {
	Items map[string]SrcItem
	ItemPtrs map[string]*SrcItem
}

type Dst struct {
	Items map[string]DstItem
	ItemPtrs map[string]*DstItem
}

// @derivingconvert("DstItem")
type SrcItem struct {
	Value string
}

type DstItem struct {
	Value string
}
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "maps"
	goldenFile := "testdata/maps.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
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

	formattedGenerated, err := format.Source(generatedCode)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := format.Source(golden)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithPointerSlices(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"pointers.go": `
package pointers
import "context"

// @derivingconvert("Dst")
type Src struct {
	Items      []*SrcItem
	ItemsPtr   *[]SrcItem
	ItemsPtrPtr *[]*SrcItem
}

type Dst struct {
	Items      []*DstItem
	ItemsPtr   *[]DstItem
	ItemsPtrPtr *[]*DstItem
}

// @derivingconvert("DstItem")
type SrcItem struct {
	Value string
}

type DstItem struct {
	Value string
}
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "pointers"
	goldenFile := "testdata/pointers.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
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

	formattedGenerated, err := format.Source(generatedCode)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := format.Source(golden)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithSlices(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"slices.go": `
package slices
import "context"

// @derivingconvert("Dst")
type Src struct {
	Items []SrcItem
}

type Dst struct {
	Items []DstItem
}

// @derivingconvert("DstItem")
type SrcItem struct {
	Value string
}

type DstItem struct {
	Value string
}
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "slices"
	goldenFile := "testdata/slices.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
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

	formattedGenerated, err := format.Source(generatedCode)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := format.Source(golden)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}
