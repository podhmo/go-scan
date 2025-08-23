package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFindOrphans(t *testing.T) {
	tmpDir := t.TempDir()

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

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	ctx := context.Background()
	err := run(ctx, true, false, tmpDir, false)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	log.SetOutput(os.Stderr)

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expectedOrphans := []string{
		"example.com/find-orphans-test.unused_main_func",
		"(example.com/find-orphans-test/greeter.Greeter).UnusedMethod",
		"example.com/find-orphans-test/greeter.UnusedFunc",
	}

	var foundOrphans []string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "example.com") {
			foundOrphans = append(foundOrphans, line)
		}
	}

	if diff := cmp.Diff(expectedOrphans, foundOrphans); diff != "" {
		t.Errorf("find-orphans mismatch (-want +got):\n%s", diff)
		t.Logf("Full output:\n%s", output)
	}
}
