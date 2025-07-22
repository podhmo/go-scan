# Plan for scantest library

## 1. Overview

The `scantest` library will provide utilities for testing `go-scan` analyzers. It will be inspired by the `golang.org/x/tools/go/analysis/analysistest` package, but with a simplified feature set as requested.

The core idea is to provide helpers to run `go-scan` on a test codebase, which can be located in a `testdata` directory or created in a temporary directory.

## 2. Core API

The library will expose two main functions:

```go
package scantest

import "testing"

// Run runs go-scan on a given directory.
// t is the testing object.
// dir is the directory to run go-scan in.
// patterns are the patterns to pass to go-scan.
// It returns the combined output of stdout and stderr from the go-scan command.
func Run(t *testing.T, dir string, patterns ...string) (string, error)

// WriteFiles creates a temporary directory and populates it with the given files.
// It returns the path to the temporary directory and a cleanup function.
func WriteFiles(t *testing.T, files map[string]string) (string, func())
```

## 3. `Run` Function

The `Run` function will execute the `go-scan` command in the specified directory. It will be responsible for:

*   Changing the working directory to `dir`.
*   Executing the `go-scan` command with the provided patterns.
*   **Module-aware execution:** Before running `go-scan`, it will check for a `go.mod` file in the test directory (`dir`).
    *   If `go.mod` exists in `dir`, it will be used. This allows for self-contained test modules.
    *   If `go.mod` does not exist, the `go-scan` command will be run in a way that it discovers the main project's `go.mod`. This will be achieved by setting the working directory appropriately.
*   Capturing and returning the combined output (stdout and stderr).
*   Returning an error if the command fails to execute.

Test authors can then inspect the output of `go-scan` to verify its behavior.

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
package myanalyzer_test

import (
    "strings"
    "testing"

    "github.com/your-org/go-scan/scantest"
)

func TestMyAnalyzer(t *testing.T) {
    dir, cleanup := scantest.WriteFiles(t, map[string]string{
        "a/a.go": `
package a

func main() {
    // some code that should be flagged by my-analyzer
}
`,
        "a/go.mod": "module a",
    })
    defer cleanup()

    output, err := scantest.Run(t, dir, "./...")
    if err != nil {
        t.Fatal(err)
    }

    if !strings.Contains(output, "my-analyzer found an issue") {
        t.Errorf("expected diagnostic not found in output: %s", output)
    }
}
```
