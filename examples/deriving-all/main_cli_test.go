package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scantest"
)

// TestMain is a test wrapper for the main function.
// It allows us to run the main function as a subprocess to test the CLI behavior.
func TestMain(m *testing.M) {
	if os.Getenv("GO_TEST_SUBPROCESS") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runCLI(t *testing.T, args ...string) (string, string, error) {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_TEST_SUBPROCESS=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestDryRun(t *testing.T) {
	input := map[string]string{
		"go.mod": "module mytest",
		"models.go": `
package models
// @deriving:unmarshal
type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, input)
	defer cleanup()

	args := []string{"--dry-run", dir}
	stdout, stderr, err := runCLI(t, args...)

	if err != nil {
		t.Fatalf("runCLI failed: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "[dry-run] Skipping file write") {
		t.Errorf("expected stdout to contain '[dry-run] Skipping file write', but it didn't.\nGot:\n%s", stdout)
	}

	// Check that the file was NOT created
	generatedFilePath := filepath.Join(dir, "models_deriving.go")
	if _, err := os.Stat(generatedFilePath); !os.IsNotExist(err) {
		t.Errorf("expected file %q not to be created in dry-run, but it was", generatedFilePath)
	}
}

func TestInspect(t *testing.T) {
	input := map[string]string{
		"go.mod": "module mytest",
		"models.go": `
package models
// @deriving:unmarshal
type User struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

// @deriving:binding
type Input struct {
	Value string
}

type Ignored struct {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, input)
	defer cleanup()

	args := []string{"--inspect", "--log-level=debug", dir}
	_, stderr, err := runCLI(t, args...)

	if err != nil {
		t.Fatalf("runCLI failed: %v\nstderr: %s", err, stderr)
	}

	// Check for 'hit' on User for 'unmarshal'
	if !strings.Contains(stderr, `"found annotation" component=go-scan type_name=User annotation_name="@deriving:unmarshal" result=hit`) {
		t.Errorf("expected inspect log for 'User' unmarshal hit, but not found in stderr.\nGot:\n%s", stderr)
	}

	// Check for 'hit' on Input for 'binding'
	if !strings.Contains(stderr, `"found annotation" component=go-scan type_name=Input annotation_name="@deriving:binding" result=hit`) {
		t.Errorf("expected inspect log for 'Input' binding hit, but not found in stderr.\nGot:\n%s", stderr)
	}

	// Check for 'miss' on Ignored for 'unmarshal'
	if !strings.Contains(stderr, `"checking for annotation" component=go-scan type_name=Ignored annotation_name="@deriving:unmarshal" result=miss`) {
		t.Errorf("expected inspect log for 'Ignored' unmarshal miss, but not found in stderr.\nGot:\n%s", stderr)
	}
}
