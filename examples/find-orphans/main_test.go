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

	startPatterns := []string{"example.com/find-orphans-test"}
	// Set verbose to false, and asJSON to false
	log.SetOutput(w)
	err := run(context.Background(), true, false, dir, false, false, startPatterns)
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
	err := run(context.Background(), true, false, dir, false, false, startPatterns)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

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

	startPatterns := []string{"example.com/find-orphans-test"}
	// Run with asJSON=true
	err := run(context.Background(), true, false, dir, false, true, startPatterns)
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

	startPatterns := []string{"example.com/find-orphans-test"}
	err := run(context.Background(), true, false, dir, false, false, startPatterns)
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
