package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestFindOrphans_ShallowScan_UnresolvedInterfaceMethodCall(t *testing.T) {
	// This test verifies that a method is NOT considered an orphan if its only
	// usage is a call on an interface from an "unresolved" package (a package
	// disallowed by the scan policy).
	files := map[string]string{
		"go.mod": "module example.com/test\ngo 1.21\n",
		"foreign/iface.go": `
package foreign
// This interface is in a package that will NOT be scanned.
type Caller interface {
    CallMe()
}
`,
		"local/impl.go": `
package local

// This is the concrete type that implements the interface.
type LocalImpl struct{}

// This method is the orphan candidate. We want to prove it's NOT an orphan.
func (l LocalImpl) CallMe() {}

// This function IS an orphan and should be reported.
func UnusedFunction() {}
`,
		"main.go": `
package main
import (
    "example.com/test/foreign"
    "example.com/test/local"
)

func main() {
    var impl local.LocalImpl
    // Assign the concrete type to the interface.
    // symgo needs to see this assignment to track the type.
    var c foreign.Caller = impl
    // Call the method on the interface.
    c.CallMe()
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	startPatterns := []string{"./..."}

	// This is the core of the test: define a policy that prevents the scanner
	// from analyzing the 'foreign' package, making its types "unresolved".
	var scanPolicy symgo.ScanPolicyFunc = func(path string) bool {
		return path != "example.com/test/foreign"
	}

	err := run(context.Background(), true, false, dir, false, false, "auto", startPatterns, []string{"vendor"}, scanPolicy, nil)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	log.SetOutput(os.Stderr)

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// ASSERT: The genuinely unused function IS reported as an orphan.
	if !strings.Contains(output, "UnusedFunction") {
		t.Errorf("expected 'UnusedFunction' to be reported as an orphan, but it was not.\nOutput:\n%s", output)
	}

	// ASSERT: The method called via the unresolved interface is NOT reported as an orphan.
	if strings.Contains(output, "CallMe") {
		t.Errorf("'CallMe' was reported as an orphan, but it should be considered used.\nOutput:\n%s", output)
	}

	// Verify the exact orphan list
	expectedOrphans := []string{
		"example.com/test/local.UnusedFunction",
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
		t.Errorf("find-orphans mismatch (-want +got):\n%s", diff)
	}
}

func TestFindOrphans_MultiFilePackageSymbolResolution(t *testing.T) {
	// This test verifies that symbols defined across multiple files within the
	// same package are resolved correctly. Before the fix, the scanner would
	// process files one-by-one, leading to "identifier not found" errors when
	// a file used a symbol defined in another file of the same package that
	// hadn't been scanned yet.
	files := map[string]string{
		"go.mod": "module example.com/multifile\ngo 1.21\n",
		"multifile/def.go": `
package multifile
type Definition struct {
    Name string
}
`,
		"multifile/user.go": `
package multifile
func UseDefinition() *Definition {
    return &Definition{Name: "used"}
}
`,
		"main.go": `
package main
import "example.com/multifile/multifile"
func main() {
    multifile.UseDefinition()
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(io.Discard)

	startPatterns := []string{"./..."}

	// Run the analysis. We expect this to complete without errors.
	// Before the fix, symgo would log "identifier not found" errors to stderr
	// and this `run` call would fail because the symbol resolution failed inside the analyzer.
	err := run(context.Background(), true, false, dir, false, false, "auto", startPatterns, []string{"vendor"}, nil, nil)
	if err != nil {
		t.Fatalf("run() failed unexpectedly: %v. This might indicate the symbol resolution issue still exists.", err)
	}

	w.Close()
	os.Stdout = oldStdout
	log.SetOutput(os.Stderr)

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// ASSERT: No orphans should be found, as everything is used.
	// The key check is that the run completes without error and doesn't report anything as an orphan.
	if strings.Contains(output, "-- Orphans --") {
		t.Errorf("expected no orphans, but found some.\nOutput:\n%s", output)
	}
	if strings.Contains(output, "UseDefinition") {
		t.Errorf("'UseDefinition' was reported as an orphan, but it is used by main.")
	}
}
