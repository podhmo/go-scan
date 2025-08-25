package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scantest"
)

func TestFindOrphans(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/find-orphans-test\ngo 1.21\n",
		"main.go": `
package main
import "example.com/find-orphans-test/greeter"
func main() {
    g := greeter.New("hello")
    g.SayHello()
}
func unused_main_func() {}
`,
		"greeter/greeter.go": `
package greeter
import "fmt"
type Greeter struct { name string }
func New(name string) *Greeter { return &Greeter{name: name} }
func (g *Greeter) SayHello() { fmt.Println(g.name) }
func (g *Greeter) UnusedMethod() {}
func UnusedFunc() {}
//go:scan:ignore
func IgnoredFunc() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	startPatterns := []string{"./..."}
	err := run(context.Background(), true, false, dir, false, false, startPatterns, []string{"vendor"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}
	log.SetOutput(io.Discard) // Discard logs after run

	w.Close()
	os.Stdout = oldStdout
	log.SetOutput(os.Stderr)

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expectedOrphans := []string{
		"example.com/find-orphans-test.unused_main_func",
		"(example.com/find-orphans-test/greeter.*Greeter).UnusedMethod",
		"example.com/find-orphans-test/greeter.UnusedFunc",
	}
	sort.Strings(expectedOrphans)

	var foundOrphans []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "example.com") || strings.HasPrefix(line, "(example.com") {
			foundOrphans = append(foundOrphans, line)
		}
	}
	sort.Strings(expectedOrphans)
	sort.Strings(foundOrphans)

	if diff := cmp.Diff(expectedOrphans, foundOrphans); diff != "" {
		t.Errorf("find-orphans mismatch (-want +got):\n%s\nFull output:\n%s", diff, output)
	}
}

func TestFindOrphans_scoping(t *testing.T) {
	files := map[string]string{
		"workspace/modulea/go.mod":        "module example.com/modulea\ngo 1.21\nreplace example.com/moduleb => ../moduleb\n",
		"workspace/modulea/main.go":       "package main\n\nimport \"example.com/moduleb/lib\"\n\nfunc main() {\n\tlib.UsedInB()\n}\n",
		"workspace/modulea/a.go":          "package main\n\nfunc UnusedInA() {}\n",
		"workspace/modulea/testdata/t.go": "package testdata\n\nfunc UnusedInTestdata() {}\n",
		"workspace/moduleb/go.mod":        "module example.com/moduleb\ngo 1.21\n",
		"workspace/moduleb/lib/lib.go":    "package lib\n\nfunc UsedInB() {}\nfunc UnusedInB() {}\n",
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	runTest := func(t *testing.T, name string, workDir string, workspaceRoot string, patterns []string, exclude []string, wantOrphans []string) {
		t.Run(name, func(t *testing.T) {
			t.Helper()

			originalCwd, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get cwd: %v", err)
			}
			if err := os.Chdir(workDir); err != nil {
				t.Fatalf("failed to change directory to %s: %v", workDir, err)
			}
			defer os.Chdir(originalCwd)

			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			log.SetOutput(io.Discard)
			defer func() {
				os.Stdout = oldStdout
				log.SetOutput(os.Stderr)
			}()

			absWorkspaceRoot := workspaceRoot
			if absWorkspaceRoot != "" {
				absWorkspaceRoot, err = filepath.Abs(workspaceRoot)
				if err != nil {
					t.Fatalf("failed to get absolute path for workspace root %q: %v", workspaceRoot, err)
				}
			}

			err = run(context.Background(), true, false, absWorkspaceRoot, false, false, patterns, exclude)
			if err != nil {
				t.Fatalf("run() failed: %v", err)
			}
			w.Close()

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			var foundOrphans []string
			lines := strings.Split(strings.TrimSpace(output), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "example.com") {
					foundOrphans = append(foundOrphans, line)
				}
			}
			sort.Strings(wantOrphans)
			sort.Strings(foundOrphans)

			if diff := cmp.Diff(wantOrphans, foundOrphans); diff != "" {
				t.Errorf("find-orphans mismatch (-want +got):\n%s\nFull output:\n%s", diff, output)
			}
		})
	}

	workspaceDir := filepath.Join(dir, "workspace")
	moduleaDir := filepath.Join(workspaceDir, "modulea")

	// Test 1: Target only modulea, should not report orphans from moduleb or testdata
	runTest(t,
		"target modulea",
		workspaceDir,
		workspaceDir,
		[]string{"./modulea/..."},
		[]string{"vendor", "testdata"},
		[]string{"example.com/modulea.UnusedInA"},
	)

	// Test 2: Target the whole workspace, should report from a and b, but not testdata
	runTest(t,
		"target workspace",
		workspaceDir,
		workspaceDir,
		[]string{"./..."},
		[]string{"vendor", "testdata"},
		[]string{
			"example.com/modulea.UnusedInA",
			"example.com/moduleb/lib.UnusedInB",
		},
	)

	// Test 3: Run from a subdirectory with relative paths, but don't exclude testdata this time
	runTest(t,
		"relative path from subdir including testdata",
		moduleaDir,
		"..",
		[]string{"./..."},
		[]string{"vendor"}, // only exclude vendor
		[]string{
			"example.com/modulea.UnusedInA",
			"example.com/modulea/testdata.UnusedInTestdata",
		},
	)
}
