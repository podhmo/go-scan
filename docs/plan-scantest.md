# Plan for scantest library

## 1. Overview

The `scantest` library will provide utilities for testing `go-scan` analyzers. It will be heavily inspired by the `golang.org/x/tools/go/analysis/analysistest` package.

The core idea is to allow test authors to write test cases as Go source files in a `testdata` directory. These source files will contain special comments (`// want ...`) that specify the expected diagnostics from the `go-scan` analyzer. The `scantest` library will run `go-scan` on these test files, parse the output, and compare it against the expectations in the `// want` comments.

## 2. Core API

The primary entry point will be a `Run` function with the following signature:

```go
package scantest

import "testing"

// Run runs a set of tests for a go-scan analyzer.
// t is the testing object.
// dir is the directory containing the test files.
// patterns are the patterns to pass to go-scan.
func Run(t *testing.T, dir string, patterns ...string)
```

This is a simplified version of `analysistest.Run`. For the initial version, we will not need to pass the analyzer itself, as `go-scan` discovers analyzers automatically.

## 3. Test Data Layout

The test data will be laid out in a directory structure similar to a standard Go project.

```
testdata/
└── src/
    └── a/
        ├── a.go
        ├── a.go.golden
        ├── b.go
        └── go.mod
```

*   `testdata/src/a/a.go`: A source file for a test case. It will contain `// want` comments.
*   `testdata/src/a/go.mod`: A `go.mod` file to define the module for the test case.
*   `testdata/src/a/a.go.golden`: An optional golden file for testing suggested fixes, similar to `analysistest`.

## 4. `// want` comments

The `// want` comments will be used to specify expected diagnostics. The format will be:

```go
// want "regexp matching the diagnostic message"
```

For example:

```go
package a

import "fmt"

func main() {
    fmt.Println("hello") // want "fmt.Println is used"
}
```

The `scantest` library will parse these comments and use them to verify the output of `go-scan`.

## 5. Golden Files for Suggested Fixes

For analyzers that provide suggested fixes, `scantest` will support golden files. If a file `foo.go` has a corresponding `foo.go.golden` file, `scantest` will:

1.  Run `go-scan` with the suggested fixes enabled.
2.  Apply the suggested fixes to `foo.go`.
3.  Compare the result with the contents of `foo.go.golden`.

This is the same mechanism used by `analysistest`.

## 6. Implementation Details

The `Run` function will perform the following steps:

1.  Create a temporary directory.
2.  Copy the contents of the `testdata` directory to the temporary directory.
3.  Run `go-scan` on the temporary directory.
4.  Parse the output of `go-scan` to extract the diagnostics.
5.  Parse the `// want` comments in the source files to get the expected diagnostics.
6.  Compare the actual diagnostics with the expected diagnostics and report any discrepancies using `t.Errorf`.
7.  If golden files are present, run `go-scan` with suggested fixes, apply them, and compare with the golden files.

## 7. Future Work

*   Support for `txtar` archives for more complex test cases with multiple files.
*   More flexible ways to specify expectations, such as checking for specific fact types.
*   A `RunWithSuggestedFixes` function to make the intent clearer when testing fixes.
