package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"testing"

	"path/filepath"

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

	startPatterns := []string{"example.com/find-orphans-test/..."}
	// Set verbose to false, and asJSON to false
	log.SetOutput(w)
	err := run(context.Background(), true, false, dir, false, false, "auto", startPatterns, []string{"testdata", "vendor"})
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

func TestFindOrphans_Filtering(t *testing.T) {
	// This test ensures that if we only target one package, we don't see orphans
	// from its dependencies.
	files := map[string]string{
		"go.mod": "module example.com/filter-test\ngo 1.21\n",
		"main.go": `
package main
import "example.com/filter-test/dep"
func main() {
    dep.Used()
}
`,
		"dep/dep.go": `
package dep
func Used() {}
func Unused() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	// We only target the main package, NOT the dependency.
	startPatterns := []string{"example.com/filter-test"}
	err := run(context.Background(), true, false, dir, false, false, "auto", startPatterns, []string{"vendor"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// The orphan from "dep" should NOT be listed.
	if strings.Contains(output, "Unused") {
		t.Errorf("expected no orphans from non-targeted package 'dep', but found some.\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "No orphans found") {
		t.Errorf("expected 'No orphans found' message, but got:\n%s", output)
	}
}

func TestFindOrphans_ExcludeDirs(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/exclude-test\ngo 1.21\n",
		"main.go": `
package main
func main() {}
`,
		"testdata/data.go": `
package testdata
func UnusedInTestdata() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	// Change working directory for relative path testing
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(oldWd)

	startPatterns := []string{"./..."}
	// We explicitly EXCLUDE "testdata"
	err = run(context.Background(), true, false, ".", false, false, "auto", startPatterns, []string{"testdata"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if strings.Contains(output, "UnusedInTestdata") {
		t.Errorf("found orphan in excluded directory 'testdata'.\nOutput:\n%s", output)
	}
}

func TestFindOrphans_multiModuleWorkspace_withGoWork(t *testing.T) {
	files := map[string]string{
		"workspace/go.work": `
go 1.21
use (
	./modulea
	./moduleb
)
`,
		"workspace/modulea/go.mod": "module example.com/modulea\ngo 1.21\nreplace example.com/moduleb => ../moduleb\nrequire golang.org/x/mod v0.27.0\n",
		"workspace/modulea/main.go": `
package main
import "example.com/moduleb/lib"
func main() {
    lib.UsedFunc()
}
`,
		"workspace/moduleb/go.mod": "module example.com/moduleb\ngo 1.21\n",
		"workspace/moduleb/lib/lib.go": `
package lib
import "fmt"
func UsedFunc() {
    fmt.Println("used")
}
func UnusedFunc() {
    fmt.Println("unused")
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	workspaceRoot := filepath.Join(dir, "workspace")
	startPatterns := []string{"./..."}

	// Change working directory to the workspace root for the test
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(workspaceRoot); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(oldWd)

	// Set verbose to false, and asJSON to false
	err = run(context.Background(), true, false, workspaceRoot, false, false, "auto", startPatterns, []string{"testdata", "vendor"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expectedOrphans := []string{
		"example.com/moduleb/lib.UnusedFunc",
	}
	sort.Strings(expectedOrphans)

	var foundOrphans []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "example.com") {
			foundOrphans = append(foundOrphans, line)
		}
	}
	sort.Strings(foundOrphans)

	if diff := cmp.Diff(expectedOrphans, foundOrphans); diff != "" {
		t.Errorf("find-orphans mismatch (-want +got):\n%s\nFull output:\n%s", diff, output)
	}
}

func TestFindOrphans_multiModuleWorkspace(t *testing.T) {
	files := map[string]string{
		"workspace/modulea/go.mod": "module example.com/modulea\ngo 1.21\nreplace example.com/moduleb => ../moduleb\nrequire golang.org/x/mod v0.27.0\n",
		"workspace/modulea/main.go": `
package main
import "example.com/moduleb/lib"
func main() {
    lib.UsedFunc()
}
`,
		"workspace/moduleb/go.mod": "module example.com/moduleb\ngo 1.21\n",
		"workspace/moduleb/lib/lib.go": `
package lib
import "fmt"
func UsedFunc() {
    fmt.Println("used")
}
func UnusedFunc() {
    fmt.Println("unused")
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	workspaceRoot := filepath.Join(dir, "workspace")
	startPatterns := []string{"./..."}

	// Change working directory to the workspace root for the test
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(workspaceRoot); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(oldWd)

	err = run(context.Background(), true, false, workspaceRoot, false, false, "auto", startPatterns, []string{"testdata", "vendor"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expectedOrphans := []string{
		"example.com/moduleb/lib.UnusedFunc",
	}
	sort.Strings(expectedOrphans)

	var foundOrphans []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "example.com") {
			foundOrphans = append(foundOrphans, line)
		}
	}
	sort.Strings(foundOrphans)

	if diff := cmp.Diff(expectedOrphans, foundOrphans); diff != "" {
		t.Errorf("find-orphans mismatch (-want +got):\n%s\nFull output:\n%s", diff, output)
	}
}

func TestFindOrphans_libraryMode(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/find-orphans-test\ngo 1.21\n",
		"lib/lib.go": `
package lib
func ExportedFunc() {
    internalFunc()
}
func internalFunc() {}
func UnusedExportedFunc() {}
func unusedInternalFunc() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	startPatterns := []string{"example.com/find-orphans-test/lib"}
	err := run(context.Background(), true, false, dir, false, false, "auto", startPatterns, []string{"testdata", "vendor"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// In library mode, exported functions are entry points and thus not orphans.
	// Only unexported, unused functions should be reported.
	expectedOrphans := []string{
		"example.com/find-orphans-test/lib.unusedInternalFunc",
	}
	sort.Strings(expectedOrphans)

	var foundOrphans []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "example.com") {
			foundOrphans = append(foundOrphans, line)
		}
	}
	sort.Strings(foundOrphans)

	if diff := cmp.Diff(expectedOrphans, foundOrphans); diff != "" {
		t.Errorf("find-orphans mismatch (-want +got):\n%s\nFull output:\n%s", diff, output)
	}
}

func TestFindOrphans_json(t *testing.T) {
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
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Redirect stdout to capture the output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	startPatterns := []string{"example.com/find-orphans-test/..."}
	// Run with asJSON=true
	err := run(context.Background(), true, false, dir, false, true, "auto", startPatterns, []string{"testdata", "vendor"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// The output should be a JSON array. We unmarshal it and check the contents.
	type Orphan struct {
		Name     string `json:"name"`
		Position string `json:"position"`
		Package  string `json:"package"`
	}
	var foundOrphans []Orphan
	if err := json.Unmarshal(buf.Bytes(), &foundOrphans); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v\nOutput was:\n%s", err, output)
	}

	expectedOrphanNames := []string{
		"example.com/find-orphans-test.unused_main_func",
		"(example.com/find-orphans-test/greeter.*Greeter).UnusedMethod",
		"example.com/find-orphans-test/greeter.UnusedFunc",
	}

	var foundOrphanNames []string
	for _, o := range foundOrphans {
		foundOrphanNames = append(foundOrphanNames, o.Name)
	}

	sort.Strings(expectedOrphanNames)
	sort.Strings(foundOrphanNames)

	if diff := cmp.Diff(expectedOrphanNames, foundOrphanNames); diff != "" {
		t.Errorf("find-orphans JSON mismatch (-want +got):\n%s", diff)
	}
}

func TestFindOrphans_interface(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/find-orphans-test\ngo 1.21\n",
		"main.go": `
package main
import "example.com/find-orphans-test/speaker"
func main() {
    var s speaker.Speaker
    s = &speaker.Dog{}
    s.Speak()
}
`,
		"speaker/speaker.go": `
package speaker
import "fmt"
type Speaker interface {
    Speak()
}
type Dog struct {}
func (d *Dog) Speak() { fmt.Println("woof") }
func (d *Dog) UnusedMethod() {}
type Cat struct {}
func (c *Cat) Speak() { fmt.Println("meow") }
func (c *Cat) UnusedMethod() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	startPatterns := []string{"example.com/find-orphans-test/..."}
	err := run(context.Background(), true, false, dir, false, false, "auto", startPatterns, []string{"testdata", "vendor"})
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expectedOrphans := []string{
		"(example.com/find-orphans-test/speaker.*Dog).UnusedMethod",
		"(example.com/find-orphans-test/speaker.*Cat).UnusedMethod",
	}
	sort.Strings(expectedOrphans)

	var foundOrphans []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "(") {
			foundOrphans = append(foundOrphans, line)
		}
	}
	sort.Strings(foundOrphans)

	if diff := cmp.Diff(expectedOrphans, foundOrphans); diff != "" {
		t.Errorf("find-orphans mismatch (-want +got):\n%s\nFull output:\n%s", diff, output)
	}
}

func TestFindOrphans_modeLib(t *testing.T) {
	// This test ensures that even if a main.main entry point exists,
	// using --mode=lib forces library mode, treating all exported functions
	// as entry points and thus not reporting them as orphans.
	files := map[string]string{
		"go.mod": "module example.com/mode-lib-test\ngo 1.21\n",
		"main.go": `
package main
func main() {}
// This function would be an orphan in app mode, but should NOT be in lib mode.
func ExportedAndUnused() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	startPatterns := []string{"./..."}

	// Change CWD to test running from the module root
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(oldWd)

	// Force library mode
	err = run(context.Background(), true, false, "", false, false, "lib", startPatterns, nil)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// ExportedAndUnused should NOT be in the output because in library mode,
	// all exported funcs are considered entry points.
	if strings.Contains(output, "ExportedAndUnused") {
		t.Errorf("found ExportedAndUnused as an orphan, but it should be treated as a valid entry point in library mode")
	}
}

func TestFindOrphans_modeApp_noMain(t *testing.T) {
	// This test ensures that using --mode=app fails if no main.main is found.
	files := map[string]string{
		"go.mod": "module example.com/mode-app-fail\ngo 1.21\n",
		"lib.go": `package lib
func SomeFunc() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()
	log.SetOutput(io.Discard)

	startPatterns := []string{"./..."}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(oldWd)

	// Force application mode, expecting an error
	err = run(context.Background(), true, false, "", false, false, "app", startPatterns, nil)
	if err == nil {
		t.Fatalf("run() should have failed in app mode with no main function, but it did not")
	}
	if !strings.Contains(err.Error(), "no main entry point was found") {
		t.Errorf("expected error about no main entry point, but got: %v", err)
	}
}
