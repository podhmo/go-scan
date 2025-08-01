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
	"github.com/podhmo/go-scan/scantest"
	"golang.org/x/tools/imports"
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

func convertProfile(ctx context.Context, ec *model.ErrorCollector, s string) string {
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

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithPointerAwareGlobalRule(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"pointer_rules.go": `
package pointerrules
import (
	"context"
	"time"
	"example.com/m/model"
)

// convert:rule "time.Time" -> "string", using=convertTimeToString

// @derivingconvert("Dst")
type Src struct {
	CreatedAt *time.Time
}

type Dst struct {
	CreatedAt *string
}

func convertTimeToString(ctx context.Context, ec *model.ErrorCollector, t time.Time) string {
	return t.Format("2006-01-02")
}
`,
		"model/model.go": `
package model
import "fmt"
type ErrorCollector struct {
	errors []error
}
func NewErrorCollector(max int) *ErrorCollector { return &ErrorCollector{} }
func (ec *ErrorCollector) Add(err error) { ec.errors = append(ec.errors, err) }
func (ec *ErrorCollector) Enter(s string) {}
func (ec *ErrorCollector) Leave() {}
func (ec *ErrorCollector) HasErrors() bool { return len(ec.errors) > 0 }
func (ec *ErrorCollector) Errors() []error { return ec.errors }
func (ec *ErrorCollector) MaxErrorsReached() bool { return false }
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "pointerrules"
	goldenFile := "testdata/pointer_rules.go.golden"

	// Create a dummy golden file if it doesn't exist
	if _, err := os.Stat(goldenFile); os.IsNotExist(err) {
		os.WriteFile(goldenFile, []byte(""), 0644)
	}

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithComplexTypes(t *testing.T) {
	sourceDir := "testdata"
	targetFile := "complex.go"

	tmpdir, err := os.MkdirTemp("", "complex-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	srcPath := filepath.Join(sourceDir, targetFile)
	dstPath := filepath.Join(tmpdir, targetFile)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read source file %s: %v", srcPath, err)
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		t.Fatalf("failed to write to temp file %s: %v", dstPath, err)
	}

	goModPath := filepath.Join(tmpdir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/m\ngo 1.24"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "complex"
	goldenFile := "testdata/complex.go.golden"

	// Create a dummy golden file if it doesn't exist
	if _, err := os.Stat(goldenFile); os.IsNotExist(err) {
		if err := os.WriteFile(goldenFile, []byte(""), 0644); err != nil {
			t.Fatalf("could not create dummy golden file: %v", err)
		}
	}

	err = run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithRecursiveGeneration(t *testing.T) {
	sourceDir := "testdata/recursive"
	tmpdir, err := os.MkdirTemp("", "recursive-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create subdirectories for source and destination packages
	sourcePkgDir := filepath.Join(tmpdir, "source")
	destPkgDir := filepath.Join(tmpdir, "destination")
	if err := os.Mkdir(sourcePkgDir, 0755); err != nil {
		t.Fatalf("failed to create source package dir: %v", err)
	}
	if err := os.Mkdir(destPkgDir, 0755); err != nil {
		t.Fatalf("failed to create destination package dir: %v", err)
	}

	// Copy source files into the correct subdirectories
	filesToCopy := map[string]string{
		"source.go":      sourcePkgDir,
		"destination.go": destPkgDir,
	}
	for fname, destDir := range filesToCopy {
		srcPath := filepath.Join(sourceDir, fname)
		dstPath := filepath.Join(destDir, fname)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("failed to read source file %s: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			t.Fatalf("failed to write to temp file %s: %v", dstPath, err)
		}
	}

	goModPath := filepath.Join(tmpdir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/m\ngo 1.24"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m/source" // Start scanning from the source package
	outputFile := "generated.go"
	pkgname := "recursive"
	goldenFile := "testdata/recursive.go.golden"

	outputPkgPath := "example.com/m/recursive"
	err = run(ctx, pkgpath, tmpdir, outputFile, pkgname, outputPkgPath)
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithTimeTime(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"timetime.go": `
package timetime
import (
	"context"
	"time"
	"example.com/m/model"
)

// convert:rule "time.Time" -> "string", using=TimeToString

// @derivingconvert("Dst")
type Src struct {
	Timestamp time.Time
}

type Dst struct {
	Timestamp string
}

func TimeToString(ctx context.Context, ec *model.ErrorCollector, t time.Time) string {
	return t.Format("2006-01-02")
}
`,
		"model/model.go": `
package model
import "fmt"
type ErrorCollector struct {
	errors []error
}
func NewErrorCollector(max int) *ErrorCollector { return &ErrorCollector{} }
func (ec *ErrorCollector) Add(err error) { ec.errors = append(ec.errors, err) }
func (ec *ErrorCollector) Enter(s string) {}
func (ec *ErrorCollector) Leave() {}
func (ec *ErrorCollector) HasErrors() bool { return len(ec.errors) > 0 }
func (ec *ErrorCollector) Errors() []error { return ec.errors }
func (ec *ErrorCollector) MaxErrorsReached() bool { return false }
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "timetime"
	goldenFile := "testdata/timetime.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithRecursiveAnnotations(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"a/a.go": `
package a
import "example.com/m/b"

// @derivingconvert("DstA")
type SrcA struct {
	B b.SrcB
}
type DstA struct {
	B b.DstB
}
`,
		"b/b.go": `
package b
import "example.com/m/c"

// @derivingconvert("DstB")
type SrcB struct {
	C c.SrcC
}
type DstB struct {
	C c.DstC
}
`,
		"c/c.go": `
package c

// @derivingconvert("DstC")
type SrcC struct {
	Name string
}
type DstC struct {
	Name string
}
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	// Start scanning from package "a"
	pkgpath := "example.com/m/a"
	outputFile := "generated.go"
	pkgname := "a"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
	}

	// For this test, we just check that the generated code contains all three conversion functions.
	// This proves that the annotations were found recursively.
	generatedStr := string(generatedCode)
	expectedFunctions := []string{
		"ConvertSrcAToDstA",
		"ConvertSrcBToDstB",
		"ConvertSrcCToDstC",
	}

	for _, fn := range expectedFunctions {
		if !strings.Contains(generatedStr, "func "+fn) {
			t.Errorf("generated code did not contain expected function %q", fn)
		}
	}
}

func TestIntegration_WithVariable(t *testing.T) {
	sourceDir := "testdata"
	targetFile := "variable.go"

	tmpdir, err := os.MkdirTemp("", "variable-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	srcPath := filepath.Join(sourceDir, targetFile)
	dstPath := filepath.Join(tmpdir, targetFile)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read source file %s: %v", srcPath, err)
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		t.Fatalf("failed to write to temp file %s: %v", dstPath, err)
	}

	goModPath := filepath.Join(tmpdir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/m\ngo 1.24"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "testdata"
	goldenFile := "testdata/variable.go.golden"

	err = run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithImports(t *testing.T) {
	// This test involves multiple packages, so we set up a proper module structure.
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"main.go": `
package main
import (
	"context"
	"time"
	ext "example.com/m/external"
)

// convert:import ext "example.com/m/external"
// convert:rule "time.Time" -> "string", using=ext.TimeToString
// convert:rule "string", validator=ext.ValidateString

// @derivingconvert("Dst")
type Src struct {
	Timestamp time.Time
	Comment   string
}

type Dst struct {
	Timestamp string
	Comment   string
}
`,
		"external/funcs.go": `
package external

import (
	"context"
	"fmt"
	"time"
	"example.com/m/model"
)

func TimeToString(ctx context.Context, ec *model.ErrorCollector, t time.Time) string {
	return t.Format("2006-01-02")
}
func ValidateString(ctx context.Context, ec *model.ErrorCollector, s string) {
	if s == "" {
		ec.Add(fmt.Errorf("string is empty"))
	}
}
`,
		// model is needed by the validator
		"model/model.go": `
package model
import "fmt"
type ErrorCollector struct {
	errors []error
	path []string
}
func NewErrorCollector(max int) *ErrorCollector {
	return &ErrorCollector{}
}
func (ec *ErrorCollector) Add(err error) {
	ec.errors = append(ec.errors, err)
}
func (ec *ErrorCollector) Enter(s string) {
	ec.path = append(ec.path, s)
}
func (ec *ErrorCollector) Leave() {
	if len(ec.path) > 0 {
		ec.path = ec.path[:len(ec.path)-1]
	}
}
func (ec *ErrorCollector) MaxErrorsReached() bool { return false }
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	// The package to scan is the main package
	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "main"
	goldenFile := "testdata/imports.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithValidator(t *testing.T) {
	files := map[string]string{
		"go.mod": `
module example.com/m
go 1.24
`,
		"validator.go": `
package validator

import (
	"fmt"
)

type ErrorCollector struct {
	errors []error
	max    int
}

func NewErrorCollector(max int) *ErrorCollector {
	return &ErrorCollector{max: max}
}
func (ec *ErrorCollector) Add(err error) {
	if ec.max > 0 && len(ec.errors) >= ec.max {
		return
	}
	ec.errors = append(ec.errors, err)
}
func (ec *ErrorCollector) Errors() []error {
	return ec.errors
}
func (ec *ErrorCollector) HasErrors() bool {
	return len(ec.errors) > 0
}
func (ec *ErrorCollector) Enter(name string) {}
func (ec *ErrorCollector) Leave()            {}
func (ec *ErrorCollector) MaxErrorsReached() bool {
	if ec.max <= 0 {
		return false
	}
	return len(ec.errors) >= ec.max
}

// // convert:rule "string", validator=validateString
// // convert:rule "int", validator=validateInt

// @derivingconvert("Dst")
type Src struct {
	Name string
	Age  int
}

type Dst struct {
	Name string
	Age  int
}

func validateString(ec *model.ErrorCollector, s string) {
	if s == "" {
		ec.Add(fmt.Errorf("string is empty"))
	}
}

func validateInt(ec *model.ErrorCollector, i int) {
	if i < 0 {
		ec.Add(fmt.Errorf("int is negative"))
	}
}
`,
		"validator_test.go": `
package validator

import (
	"context"
	"strings"
	"testing"
)

func TestValidation(t *testing.T) {
	cases := []struct {
		name          string
		src           *Src
		expectErr     bool
		expectedErrs []string
	}{
		{
			name: "valid",
			src:  &Src{Name: "test", Age: 20},
			expectErr: false,
		},
		{
			name: "invalid string",
			src:  &Src{Name: "", Age: 20},
			expectErr: true,
			expectedErrs: []string{"Name: string is empty"},
		},
		{
			name: "invalid int",
			src:  &Src{Name: "test", Age: -1},
			expectErr: true,
			expectedErrs: []string{"Age: int is negative"},
		},
		{
			name: "multiple errors",
			src:  &Src{Name: "", Age: -1},
			expectErr: true,
			expectedErrs: []string{"Name: string is empty", "Age: int is negative"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ConvertSrcToDst(context.Background(), tc.src)
			if tc.expectErr {
				if err == nil {
					t.Fatal("expected an error, but got nil")
				}
				errStr := err.Error()
				for _, sub := range tc.expectedErrs {
					if !strings.Contains(errStr, sub) {
						t.Errorf("expected error to contain %q, but it was %q", sub, errStr)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, but got: %v", err)
				}
			}
		})
	}
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
	pkgname := "validator"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
	}

	goldenFile := "testdata/validator.go.golden"
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithEmbeddedFields(t *testing.T) {
	// For this test, we use actual source files instead of in-memory files
	// to test the scanner's ability to handle file paths correctly.
	sourceDir := "testdata/embedded"

	// Create a temporary directory and copy the source files there.
	// This is because the tool might create a go.mod file, and we don't
	// want to pollute the original testdata directory.
	tmpdir, err := os.MkdirTemp("", "embedded-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Copy source files to temp dir
	srcEntries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatalf("failed to read source dir %s: %v", sourceDir, err)
	}
	for _, entry := range srcEntries {
		srcPath := filepath.Join(sourceDir, entry.Name())
		dstPath := filepath.Join(tmpdir, entry.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("failed to read source file %s: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			t.Fatalf("failed to write to temp file %s: %v", dstPath, err)
		}
	}

	// Create a go.mod file in the temp directory
	goModPath := filepath.Join(tmpdir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/m\ngo 1.24"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "embedded"
	goldenFile := "testdata/embedded.go.golden"

	// run() expects a single directory path for scanning.
	err = run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithFieldMatching(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\ngo 1.24",
		"fieldmatching.go": `
package fieldmatching

// @derivingconvert("Dst")
type Src struct {
	UserID   string ` + "`json:\"user_id\"`" + `
	UserName string ` + "`json:\"user_name\"`" + `
	User_Age int    // normalized name match
}

type Dst struct {
	ID   string ` + "`json:\"user_id\"`" + `
	Name string ` + "`json:\"user_name\"`" + `
	UserAge  int
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
	pkgname := "fieldmatching"
	goldenFile := "testdata/fieldmatching.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_WithErrorHandling(t *testing.T) {
	files := map[string]string{
		"go.mod": `
module example.com/m
go 1.24
`,
		"errors.go": `
package errors

import (
	"context"
	"errors"
)

type ErrorCollector struct {
	errors []error
	max    int
}

func NewErrorCollector(max int) *ErrorCollector {
	return &ErrorCollector{max: max}
}
func (ec *ErrorCollector) Add(err error) {
	if ec.max > 0 && len(ec.errors) >= ec.max {
		return
	}
	ec.errors = append(ec.errors, err)
}
func (ec *ErrorCollector) Errors() []error {
	return ec.errors
}
func (ec *ErrorCollector) HasErrors() bool {
	return len(ec.errors) > 0
}
func (ec *ErrorCollector) Enter(name string) {}
func (ec *ErrorCollector) Leave()            {}
func (ec *ErrorCollector) MaxErrorsReached() bool {
	if ec.max <= 0 {
		return false
	}
	return len(ec.errors) >= ec.max
}

// @derivingconvert("Dst")
type Src struct {
	Name      string    ` + "`convert:\",using=convertNameWithError\"`" + `
	ManagerID *int      ` + "`convert:\",required\"`" + `
	SpouseID  *int      ` + "`convert:\",required\"`" + `
}

type Dst struct {
	Name      string
	ManagerID *int
	SpouseID  *int
}

func convertNameWithError(ctx context.Context, ec *model.ErrorCollector, name string) string {
	ec.Add(errors.New("name conversion failed"))
	return "error-name"
}
`,
		"errors_test.go": `
package errors
import (
	"context"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	src := &Src{
		Name: "test",
		ManagerID: nil, // required field is nil
		SpouseID: nil,
	}
	_, err := ConvertSrcToDst(context.Background(), src)
	if err == nil {
		t.Fatal("expected an error, but got nil")
	}

	expectedErrors := []string{
		"Name: name conversion failed",
		"ManagerID: ManagerID is required",
		"SpouseID: SpouseID is required",
	}

	errStr := err.Error()
	for _, sub := range expectedErrors {
		if !strings.Contains(errStr, sub) {
			t.Errorf("expected error to contain %q, but it was %q", sub, errStr)
		}
	}
}
`,
	}

	// We expect the generator to succeed, but the generated code to fail at runtime.
	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	ctx := context.Background()
	writer := &memoryFileWriter{}
	ctx = context.WithValue(ctx, FileWriterKey, writer)

	pkgpath := "example.com/m"
	outputFile := "generated.go"
	pkgname := "errors"

	// 1. Generate the converter code
	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	generatedCode, ok := writer.Outputs[outputFile]
	if !ok {
		t.Fatalf("output file %q not found in captured outputs", outputFile)
	}

	goldenFile := "testdata/errors.go.golden"
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
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

// convert:rule "time.Time" -> "string", using=convertTimeToString

// @derivingconvert("Dst")
type Src struct {
	CreatedAt time.Time
	UpdatedAt time.Time ` + "`convert:\",using=overrideTime\"`" + `
}

type Dst struct {
	CreatedAt string
	UpdatedAt string
}

func convertTimeToString(ctx context.Context, ec *model.ErrorCollector, t time.Time) string {
	return t.Format("2006-01-02")
}

func overrideTime(ctx context.Context, ec *model.ErrorCollector, t time.Time) string {
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

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v\n---\n%s", err, string(generatedCode))
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
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
	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
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

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
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

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}

// func TestIntegration_WithMaxErrors(t *testing.T) {
// 	files := map[string]string{
// 		"go.mod": `
// module example.com/m
// go 1.24
// `,
// 		"errors.go": `
// package errors

// import (
// 	"context"
// 	"errors"
// 	"example.com/convert/model"
// )

// // @derivingconvert("Dst", max_errors=1)
// type Src struct {
// 	Name      string    ` + "`convert:\",using=convertNameWithError\"`" + `
// 	ManagerID *int      ` + "`convert:\",required\"`" + `
// }

// type Dst struct {
// 	Name      string
// 	ManagerID *int
// }

// func convertNameWithError(ec *model.ErrorCollector, name string) string {
// 	ec.Add(errors.New("name conversion failed"))
// 	return "error-name"
// }
// `,
// 		"errors_test.go": `
// package errors
// import (
// 	"context"
// 	"strings"
// 	"testing"
// )

// func TestRun(t *testing.T) {
// 	src := &Src{
// 		Name: "test",
// 		ManagerID: nil, // required field is nil
// 	}
// 	_, err := ConvertSrcToDst(context.Background(), src)
// 	if err == nil {
// 		t.Fatal("expected an error, but got nil")
// 	}

// 	errStr := err.Error()
// 	if strings.Contains(errStr, "ManagerID") {
// 		t.Errorf("expected only one error, but got more: %s", errStr)
// 	}
// 	if !strings.Contains(errStr, "name conversion failed") {
// 		t.Errorf("expected error to contain %q, but it was %q", "name conversion failed", errStr)
// 	}
// }
// `,
// 	}

// 	tmpdir, cleanup := scantest.WriteFiles(t, files)
// 	defer cleanup()

// 	ctx := context.Background()
// 	writer := &memoryFileWriter{}
// 	ctx = context.WithValue(ctx, FileWriterKey, writer)

// 	pkgpath := "example.com/m"
// 	outputFile := "generated.go"
// 	pkgname := "errors"

// 	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname)
// 	if err != nil {
// 		t.Fatalf("run() failed: %v", err)
// 	}

// 	generatedCode, ok := writer.Outputs[outputFile]
// 	if !ok {
// 		t.Fatalf("output file %q not found in captured outputs", outputFile)
// 	}

// 	generatedPath := filepath.Join(tmpdir, "generated.go")
// 	if err := os.WriteFile(generatedPath, generatedCode, 0644); err != nil {
// 		t.Fatalf("failed to write generated code: %v", err)
// 	}

// 	cmd := exec.Command("go", "mod", "tidy")
// 	cmd.Dir = tmpdir
// 	if out, err := cmd.CombinedOutput(); err != nil {
// 		t.Fatalf("go mod tidy failed: %s\n%s", err, out)
// 	}

// 	cmd = exec.Command("go", "test", "-v")
// 	cmd.Dir = tmpdir
// 	output, err := cmd.CombinedOutput()
// 	if err != nil {
// 		t.Errorf("expected go test to succeed, but it failed. Output:\n%s", output)
// 	}
// }

func TestIntegration_WithMapKeyConversion(t *testing.T) {
	files := map[string]string{
		"go.mod": `
module example.com/m
go 1.24
`,
		"mapkeys.go": `
package mapkeys
import (
	"context"
	"strconv"
)
// @derivingconvert("Dst")
type Src struct {
	Data map[int]string
}
type Dst struct {
	Data map[string]string
}
// convert:rule "int" -> "string", using=convertIntToString
func convertIntToString(ctx context.Context, ec *model.ErrorCollector, i int) string {
	return strconv.Itoa(i)
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
	pkgname := "mapkeys"
	goldenFile := "testdata/mapkeys.go.golden"

	err := run(ctx, pkgpath, tmpdir, outputFile, pkgname, "")
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

	formattedGenerated, err := imports.Process(outputFile, generatedCode, nil)
	if err != nil {
		t.Fatalf("failed to format generated code: %v", err)
	}
	formattedGolden, err := imports.Process(goldenFile, golden, nil)
	if err != nil {
		t.Fatalf("failed to format golden file: %v", err)
	}

	if diff := cmp.Diff(string(formattedGolden), string(formattedGenerated)); diff != "" {
		t.Errorf("generated code mismatch (-want +got):\n%s", diff)
	}
}
