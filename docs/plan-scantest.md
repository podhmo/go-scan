> [!NOTE]
> This feature has been implemented.
>
> **Implementation Note:** The final implementation differs slightly from this initial plan. Instead of relying on specific helper functions, I/O interception was implemented at a lower level using `context.Context`. This allows any code that uses the context-aware `goscan.WriteFile` to be testable without modification, which is a more powerful and flexible approach than originally envisioned.

# Plan for scantest library

## 1. Overview

The `scantest` library provides helpers to test tasks that use `go-scan`. These tasks can range from pure static checks on scanned code to actions with side effects like code generation.

The core idea is to provide a consistent way to set up a test environment, execute an action on scanned packages, and verify the outcome, whether that outcome is an error state or a generated file.

## 2. Core API

```go
package scantest

import (
	"context"
	"testing"
	"github.com/podhmo/go-scan"
)

// ActionFunc is a function that performs a check or an action based on scan results.
// For actions with side effects, it should use go-scan's top-level functions
// (e.g., goscan.WriteFile) to allow the test harness to capture the results.
type ActionFunc func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error

// Result holds the outcome of a Run that has side effects.
type Result struct {
	// Outputs contains the content of files written by go-scan's helper functions.
	// The key is the file path, and the value is the content.
	Outputs map[string][]byte
}

// Run sets up and executes a test scenario.
// It returns a Result object if the action had side effects captured by the harness.
func Run(t *testing.T, dir string, patterns []string, action ActionFunc) (*Result, error)

// WriteFiles creates a temporary directory and populates it with initial files.
func WriteFiles(t *testing.T, files map[string]string) (string, func())
```

## 3. `Run` Function and `go-scan` Integration

The `Run` function is the central piece of the test harness.

1.  **Setup**: It creates a `goscan.Scanner` and prepares a `context.Context` for the test run.
2.  **Scan**: It scans the packages matching the given `patterns`.
3.  **Action Execution**: It calls the user-provided `action` function.
4.  **Result Capturing**: This is the key integration point.
    *   To capture file I/O, the `action` function **must** use helper functions from the `go-scan` package that are context-aware, specifically `goscan.WriteFile` (or helpers that use it, like `goscan.PackageDirectory.SaveGoFile`).
    *   `scantest.Run` injects a custom `goscan.FileWriter` implementation into the `context` using the exported key `goscan.FileWriterKey`.
    *   The `goscan.WriteFile` function checks the context for this key. If the `FileWriter` is present, `WriteFile` calls its `WriteFile` method. Otherwise, it defaults to `os.WriteFile`.
    *   The implementation in `scantest` uses an in-memory writer that stores generated file content in a map, which is then returned in the `Result` struct. This allows tests to verify generated code without touching the filesystem.
5.  **Return Value**:
    *   If the interceptor captured any file writes, they are returned in the `Result` struct.
    -   If the action performed only checks and returned `nil`, `Run` returns `(nil, nil)`.
    *   If any step fails, an `error` is returned.

## 4. `WriteFiles` Function

The `WriteFiles` function is a helper for creating self-contained test cases. It will:

*   Create a new temporary directory using `t.TempDir()`.
*   Create a `src` subdirectory to mimic a GOPATH structure.
*   Write the files specified in the `files` map to the `src` directory.
*   Return the path to the `src` directory and a no-op cleanup function (as `t.TempDir` handles cleanup).

This allows test authors to easily create isolated test environments without needing a `testdata` directory.

## 5. Example Usage

### Example 1: Pure Check

```go
package main_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestPureCheck(t *testing.T) {
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/me",
		"person.go": `package main; type Person struct { Name string }`,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		personType := pkgs[0].Lookup.Type("Person")
		if personType == nil {
			return fmt.Errorf("type Person not found")
		}
		return nil
	}

	result, err := scantest.Run(t, dir, []string{"example.com/me"}, action)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Errorf("expected a nil result for a pure check, but got %+v", result)
	}
}
```

## 6. Future Work

### File Change Detection

A potential enhancement is to detect not only generated files but also modifications to existing files. This would be useful for testing tools that have side effects like running `go fmt` on the source files.

*   **Mechanism**: Before running the `action`, `scantest.Run` could calculate and store checksums of all `.go` files in the test directory.
*   **Verification**: After the `action` completes, it would re-calculate checksums and compare them against the stored values.
*   **Result**: The `scantest.Result` struct could be extended to include `ModifiedFiles []string` and `UnchangedFiles []string` fields, providing a complete picture of the action's side effects.

### Example 2: Code Generation

This example shows the target usage with the integrated context-based interception mechanism.

```go
package main_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

// This action uses the context-aware goscan.WriteFile.
func generateAction(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
	content := []byte("package main\n\n// Code generated by test\n")
	// Note: s.RootDir is not a field on goscan.Scanner. A real implementation
	// would get the root directory from the locator if needed.
	// For this example, we assume `dir` from WriteFiles is available.
	outputPath := "main_gen.go"

	// This function checks the context for a test-specific writer provided by scantest.Run.
	return goscan.WriteFile(ctx, outputPath, content, 0644)
}

func TestGenerateCode(t *testing.T) {
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": "package main",
	})
	defer cleanup()

	result, err := scantest.Run(t, dir, []string{"example.com/me"}, generateAction)
	if err != nil {
		t.Fatal(err)
	}

	if result == nil {
		t.Fatal("expected a non-nil result for a file generation action")
	}
	if len(result.Outputs) != 1 {
		t.Fatalf("expected 1 generated file, but got %d", len(result.Outputs))
	}

	content, ok := result.Outputs["main_gen.go"]
	if !ok {
		t.Fatal("expected file 'main_gen.go' was not in the result")
	}
	if !strings.Contains(string(content), "Code generated by test") {
		t.Errorf("generated file content is not what was expected")
	}
}
```
