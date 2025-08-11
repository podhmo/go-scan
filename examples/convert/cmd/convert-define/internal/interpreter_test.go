package internal

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

func TestRunner(t *testing.T) {
	wd := filepath.Join("..", "testdata", "success")

	// The user pointed out that the scanner must be configured correctly
	// and passed into the runner.
	// 1. WithWorkDir sets the module root.
	// 2. WithGoModuleResolver allows finding stdlib packages like "time".
	overrides := scanner.ExternalTypeOverride{
		"time.Time": &scanner.TypeInfo{
			Name:    "Time",
			PkgPath: "time",
			Kind:    scanner.StructKind,
		},
	}
	runner, err := NewRunner(
		goscan.WithWorkDir(wd),
		goscan.WithGoModuleResolver(),
		goscan.WithExternalTypeOverrides(overrides),
	)
	if err != nil {
		t.Fatalf("NewRunner() failed: %+v", err)
	}

	defineFile := filepath.Join(wd, "define.go")
	if err := runner.Run(context.Background(), defineFile); err != nil {
		t.Fatalf("Run() failed: %+v", err)
	}

	// Assertions
	if got, want := len(runner.Info.GlobalRules), 1; got != want {
		t.Fatalf("expected %d global rule, but got %d", want, got)
	}

	rule := runner.Info.GlobalRules[0]
	want := "time.Time"
	if got := rule.SrcTypeName; got != want {
		t.Errorf("SrcTypeName: want %q, got %q", want, got)
	}
	if rule.SrcTypeInfo == nil {
		t.Fatal("SrcTypeInfo should not be nil")
	}
	if got, want := rule.SrcTypeInfo.Name, "Time"; got != want {
		t.Errorf("SrcTypeInfo.Name: want %q, got %q", want, got)
	}

	want = "string"
	if got := rule.DstTypeName; got != want {
		t.Errorf("DstTypeName: want %q, got %q", want, got)
	}
	if rule.DstTypeInfo != nil {
		// string is a builtin, so its TypeInfo is expected to be nil
		t.Errorf("DstTypeInfo should be nil for builtin string, but was not: %v", rule.DstTypeInfo)
	}

	want = "convutil.TimeToString"
	if got := rule.UsingFunc; got != want {
		t.Errorf("UsingFunc: want %q, got %q", want, got)
	}

	wantImports := map[string]string{
		"convutil": "example.com/test/convutil",
	}
	if diff := cmp.Diff(wantImports, runner.Info.Imports); diff != "" {
		t.Errorf("Imports mismatch (-want +got):\n%s", diff)
	}
}
