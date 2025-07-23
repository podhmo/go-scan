# Testing with `scantest`

The `scantest` package provides a lightweight testing harness for tools built on top of `go-scan`. It simplifies the process of writing tests that involve parsing Go source files by managing temporary file creation and scanner invocation.

## Core Concepts

The primary goal of `scantest` is to allow you to test your tool's logic against a set of virtual Go source files without needing to manage testdata directories manually. It provides two key helper functions:

-   `scantest.WriteFiles(t, files)`: Creates a temporary directory and writes a map of file paths to their content. It's perfect for setting up your test cases, including `go.mod` files and one or more Go source files.
-   `scantest.Run(t, dir, patterns, action)`: This is the main test runner. It initializes a `go-scan` scanner, scans the specified patterns within the given directory, and invokes your custom `ActionFunc` with the results.

## How to Use `scantest`

The typical workflow for testing a tool that uses `go-scan` involves these steps:

1.  **Define Test Cases**: Create a table-driven test with each case defining a set of Go source files.
2.  **Set Up Files**: In your test loop, use `scantest.WriteFiles` to create a temporary directory populated with your test source files.
3.  **Implement the Action**: Create an `ActionFunc` that receives the `*scan.Scanner` and `[]*scan.Package` from `go-scan`. This is where you place the core logic of your test.
4.  **Run the Test**: Call `scantest.Run` with the temporary directory and your action function.
5.  **Assert Results**: Inside the `ActionFunc`, perform assertions on the data you extract from the `scan.Package` results.

### Example: Testing an Annotation Parser

Let's say you are building a tool that finds structs with a `@mytool:generate` annotation and records the struct name.

First, you would define your tool's parsing logic. This function is what you want to test. It operates on the results of a `go-scan` scan, not on a specific file path.

```go
// mytool/parser.go

package mytool

import (
    "strings"
    scan "github.com/podhmo/go-scan"
)

// This is the function we want to test.
// It takes the scan results and extracts the names of structs with the annotation.
func ExtractAnnotatedStructs(pkgs []*scan.Package) []string {
    var names []string
    for _, pkg := range pkgs {
        for _, t := range pkg.Types {
            if strings.Contains(t.Doc, "@mytool:generate") {
                names = append(names, t.Name)
            }
        }
    }
    return names
}
```

Next, you would write a test for `ExtractAnnotatedStructs` using `scantest`.

```go
// mytool/parser_test.go

package mytool

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestExtractAnnotatedStructs(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/hello",
		"hello.go": `
package hello

// @mytool:generate
type Foo struct {}

type Bar struct {} // No annotation

// @mytool:generate
type Baz struct {}
`,
	}

	// 1. Set up the temporary directory and files.
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 2. Define the action to be performed on the scan results.
	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		// 3. Call the function you are testing.
		got := ExtractAnnotatedStructs(pkgs)
		want := []string{"Foo", "Baz"}

		// 4. Assert the results.
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("ExtractAnnotatedStructs() mismatch (-want +got):\n%s", diff)
		}
		return nil
	}

	// 5. Run the test. scantest handles the scanning and calls our action.
	if _, err := scantest.Run(t, dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
```

This approach effectively decouples your tool's core logic from the file system and the `go-scan` boilerplate, leading to cleaner and more maintainable tests. Your parser logic is tested against the direct output of `go-scan`, which is exactly what it will receive in production.
