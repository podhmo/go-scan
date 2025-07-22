# Plan for scantest library

## 1. Overview

The `scantest` library will provide utilities for testing `go-scan` analyzers. It will be inspired by the `golang.org/x/tools/go/analysis/analysistest` package, but with a simplified feature set as requested.

The core idea is to provide helpers to run `go-scan` on a test codebase, which can be located in a `testdata` directory or created in a temporary directory.

## 2. Core API

The library will provide utilities to test code generation tasks that use `go-scan`.

```go
package scantest

import (
	"testing"
	"github.com/podhmo/go-scan"
)

// ActionFunc is a function that performs an action, such as code generation,
// based on the results of a go-scan.
// It receives the scanner instance and the scanned packages.
type ActionFunc func(s *goscan.Scanner, pkgs []*goscan.Package) error

// Run sets up a test environment, runs go-scan, executes a user-provided action,
// and returns the results for verification.
// t is the testing object.
// dir is the root directory for the scanner (where go.mod is).
// patterns are the import path patterns to scan.
// action is the function to execute after scanning.
// It returns a map of generated/modified file paths to their content.
func Run(t *testing.T, dir string, patterns []string, action ActionFunc) (map[string][]byte, error)

// WriteFiles creates a temporary directory and populates it with the given files.
// It returns the path to the temporary directory and a cleanup function.
func WriteFiles(t *testing.T, files map[string]string) (string, func())
```

## 3. `Run` Function

The `Run` function orchestrates the testing of a code generation task. It is responsible for:

1.  **Setup**: Initializing a `goscan.Scanner` for the given `dir`.
2.  **Scan**: Scanning all packages that match the provided `patterns`.
3.  **Execute Action**: Invoking the user-provided `action` function with the scanner instance and the list of scanned packages. This is where the actual code generation logic is executed.
4.  **Capture Results**: After the action completes, `Run` will detect which files in the directory were created or modified.
5.  **Return**: It returns a map where keys are the paths of the new/modified files and values are their complete content. This allows the test to easily verify the output of the code generation.
6.  **Error Handling**: It will return any error that occurs during scanning or the execution of the action.

This workflow enables testing of complex code generation logic (like in `examples/derivingjson`) in a self-contained and verifiable way.

## 4. `WriteFiles` Function

The `WriteFiles` function is a helper for creating self-contained test cases. It will:

*   Create a new temporary directory using `t.TempDir()`.
*   Create a `src` subdirectory to mimic a GOPATH structure.
*   Write the files specified in the `files` map to the `src` directory.
*   Return the path to the `src` directory and a no-op cleanup function (as `t.TempDir` handles cleanup).

This allows test authors to easily create isolated test environments without needing a `testdata` directory.

## 5. Example Usage

Here's how the library might be used in a test:

```go
package main_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

// This is a simplified version of a code generation action,
// similar to what examples/derivingjson might do.
func generateJSONMarshaller(s *goscan.Scanner, pkgs []*goscan.Package) error {
	var target *goscan.Type
	for _, p := range pkgs {
		for _, t := range p.Types {
			if t.Name == "Person" {
				target = t
				break
			}
		}
	}
	if target == nil {
		return fmt.Errorf("type Person not found")
	}

	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("package %s\n\n", target.Package.Name))
	b.WriteString(fmt.Sprintf("func (p *%s) MarshalJSON() ([]byte, error) {\n", target.Name))
	// simplified marshalling logic
	b.WriteString(`	return []byte("{\"name\":\"" + p.Name + "\"}"), nil`)
	b.WriteString("\n}\n")

	// Output to a new file
	outputPath := filepath.Join(filepath.Dir(target.Position.Filename), "person_gen.go")
	return os.WriteFile(outputPath, b.Bytes(), 0644)
}

func TestGenerateCode(t *testing.T) {
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/me",
		"person.go": `
package main

// +gen:json
type Person struct {
	Name string
}`,
	})
	defer cleanup()

	// Action: run the json marshaller code generator
	generatedFiles, err := scantest.Run(t, dir, []string{"example.com/me"}, generateJSONMarshaller)
	if err != nil {
		t.Fatal(err)
	}

	// Verification
	if len(generatedFiles) != 1 {
		t.Fatalf("expected 1 generated file, but got %d", len(generatedFiles))
	}

	genFile, ok := generatedFiles["person_gen.go"]
	if !ok {
		t.Fatalf("expected file 'person_gen.go' was not generated")
	}

	expected := `
package main

func (p *Person) MarshalJSON() ([]byte, error) {
	return []byte("{\"name\":\"" + p.Name + "\"}"), nil
}
`
	if !strings.Contains(string(genFile), strings.TrimSpace(expected)) {
		t.Errorf("generated file content mismatch:\n--- expected ---\n%s\n--- got ---\n%s", expected, string(genFile))
	}
}
```
