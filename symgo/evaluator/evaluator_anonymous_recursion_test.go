package evaluator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestAnonymousInfiniteRecursion(t *testing.T) {
	source := `
package main

func main() {
	var endless func()
	endless = func() {
		endless()
	}
	endless()
}
`
	// 1. Create a temporary directory to act as the workspace root
	dir := t.TempDir()
	mainGoPath := filepath.Join(dir, "main.go")
	goModPath := filepath.Join(dir, "go.mod")

	// 2. Create a dummy go.mod to satisfy the module resolver
	if err := os.WriteFile(goModPath, []byte("module myapp"), 0644); err != nil {
		t.Fatalf("failed to write dummy go.mod: %v", err)
	}
	if err := os.WriteFile(mainGoPath, []byte(source), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	// 3. Setup the scanner, providing the temp dir as the WorkDir
	s, err := goscan.New(
		goscan.WithWorkDir(dir), // Set the working directory for path resolution
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("goscan.New failed: %+v", err)
	}

	// 4. Scan the package from the temp dir using "." as the pattern
	pkgs, err := s.Scan(context.Background(), ".")
	if err != nil {
		t.Fatalf("scan failed: %+v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, but got %d", len(pkgs))
	}
	pkg := pkgs[0]

	// 5. Find the AST file from the package's AstFiles map
	astFile, ok := pkg.AstFiles[mainGoPath]
	if !ok {
		// In some OS, the path might be slightly different (e.g. /private/tmp vs /tmp).
		// We iterate to find the file if direct lookup fails.
		found := false
		for path, file := range pkg.AstFiles {
			if filepath.Base(path) == "main.go" {
				astFile = file
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("ast.File not found for main.go in package")
		}
	}

	// 6. Define the AllowAll policy locally
	allowAllPolicy := func(importPath string) bool {
		return true
	}

	// 7. Create the evaluator and run it
	eval := New(s, nil, nil, allowAllPolicy)
	eval.Eval(context.Background(), astFile, object.NewEnvironment(), pkg)

	// The test is expected to time out here because of an infinite recursion.
}