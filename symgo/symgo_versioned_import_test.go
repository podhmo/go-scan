package symgo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSymgoVersionedImport(t *testing.T) {
	t.Run("heuristic should guess package name from versioned import path", func(t *testing.T) {
		// Define the source files for the test case.
		// A go.mod is required by symgotest.
		source := map[string]string{
			"go.mod": "module example.com/main",
			"main.go": `
package main

import (
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	_ = chi.NewRouter()
	_ = middleware.Logger
	_ = kingpin.New("my-app", "A command-line application.")
}
`,
		}

		// The scan policy ensures that we do NOT scan the imported packages from source.
		// This forces the evaluator to use the import path heuristic to guess the package name.
		scanPolicy := func(importPath string) bool {
			return !strings.HasPrefix(importPath, "github.com/")
		}

		// Define the test case using the correct struct.
		tc := symgotest.TestCase{
			Source:     source,
			EntryPoint: "example.com/main.main",
			Options: []symgotest.Option{
				symgotest.WithScanPolicy(scanPolicy),
			},
		}

		// Run the test. The action function is where assertions go.
		// For this test, a successful run without errors is the only assertion needed.
		symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
			// No explicit assertions needed. The test will fail if `symgotest.Run`
			// returns an error (e.g., "identifier not found: chi").
		})
	})
}