package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
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
	err := run(context.Background(), true, false, dir, false, startPatterns)
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
		if strings.HasPrefix(line, "example.com") || strings.HasPrefix(line, "(example.com") {
			foundOrphans = append(foundOrphans, line)
		}
	}

	if diff := cmp.Diff(expectedOrphans, foundOrphans); diff != "" {
		t.Errorf("find-orphans mismatch (-want +got):\n%s\nFull output:\n%s", diff, output)
	}
}
