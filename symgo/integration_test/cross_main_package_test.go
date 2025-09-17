package integration_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgotest"
)

// TestCrossMainPackageSymbolCollision reproduces a bug where the symgo evaluator
// would confuse functions with the same name from different `main` packages when
// analyzing a whole workspace.
func TestCrossMainPackageSymbolCollision(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":        "module example.com/workspace\n\ngo 1.21",
			"go.work":       "go 1.21\nuse (\n./pkg_a\n./pkg_b\n)\n",
			"pkg_a/go.mod":  "module example.com/pkg_a\n\ngo 1.21",
			"pkg_a/main.go": `
package main
import "fmt"
func main() { run() }
func run() { helper_a() }
func helper_a() { fmt.Println("helper_a called") }
`,
			"pkg_b/go.mod":  "module example.com/pkg_b\n\ngo 1.21",
			"pkg_b/main.go": `
package main
import "fmt"
func main() { run() }
func run() { fmt.Println("pkg_b.run called") }
`,
		},
		EntryPoint: "example.com/workspace/pkg_b.main",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(path string) bool {
				return strings.HasPrefix(path, "example.com/workspace/pkg_a") ||
					strings.HasPrefix(path, "example.com/workspace/pkg_b")
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symbolic execution failed unexpectedly: %v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}
