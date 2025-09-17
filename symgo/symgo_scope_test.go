package symgo_test

import (
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

// NOTE: This test cannot be easily refactored with symgotest, because it inspects
// the result of the scanning phase (checking for a nil function body) rather than
// the result of a full execution. symgotest is optimized for running code and
// checking the final result. Leaving this test as-is is the clearest path.
func TestWithSymbolicDependencyScope(t *testing.T) {
	ctx := t.Context()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/myapp\ngo 1.21",
		"main.go": `
package main
import "example.com/myapp/lib"
func main() { lib.DoSomething() }
`,
		"lib/lib.go": `
package lib
func DoSomething() {}
`,
	})
	defer cleanup()

	scanner, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}

	interp, err := symgo.NewInterpreter(scanner,
		symgo.WithSymbolicDependencyScope("example.com/myapp/lib"),
	)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	libPkg, err := interp.Scanner().ScanPackageByImport(ctx, "example.com/myapp/lib")
	if err != nil {
		t.Fatalf("ScanPackageByImport for lib failed: %v", err)
	}

	doSomethingFunc := findFunc(t, libPkg, "DoSomething")
	if doSomethingFunc.AstDecl.Body != nil {
		t.Errorf("expected function body to be nil for symbolic dependency, but it was not")
	}
}

func TestScopesAndUnexportedResolution(t *testing.T) {
	baseFiles := map[string]string{
		"myapp/go.mod": "module example.com/myapp\ngo 1.21\nreplace example.com/lib => ../lib",
		"myapp/main.go": `
package main
import "example.com/lib"
func main() string { return lib.GetGreeting() }
`,
		"lib/go.mod": "module example.com/lib\ngo 1.21",
	}

	tests := []struct {
		name        string
		libGoSource string
		scanPolicy  symgo.ScanPolicyFunc
		checkResult func(t *testing.T, r *symgotest.Result)
	}{
		{
			name: "primary scope: in-scope",
			libGoSource: `
package lib
func GetGreeting() string { return "from lib" }`,
			scanPolicy: func(path string) bool {
				return strings.HasPrefix(path, "example.com/myapp") || strings.HasPrefix(path, "example.com/lib")
			},
			checkResult: func(t *testing.T, r *symgotest.Result) {
				str := symgotest.AssertAs[*object.String](r, t, 0)
				if str.Value != "from lib" {
					t.Errorf("want %q, got %q", "from lib", str.Value)
				}
			},
		},
		{
			name: "primary scope: out-of-scope",
			libGoSource: `
package lib
func GetGreeting() string { return "from lib" }`,
			scanPolicy: func(path string) bool {
				// This policy deliberately excludes "example.com/lib"
				return strings.HasPrefix(path, "example.com/myapp")
			},
			checkResult: func(t *testing.T, r *symgotest.Result) {
				if _, ok := r.ReturnValue.(*object.SymbolicPlaceholder); !ok {
					t.Errorf("expected SymbolicPlaceholder for out-of-scope call, got %T", r.ReturnValue)
				}
			},
		},
		{
			name: "unexported resolution: full",
			libGoSource: `
package lib
var secretPrefix = "hello from"
const secretSuffix = " unexported func"
func getSecretMessage() string {
	return secretPrefix + secretSuffix
}
func GetGreeting() string {
	return getSecretMessage()
}`,
			scanPolicy: func(path string) bool { return true }, // scan everything
			checkResult: func(t *testing.T, r *symgotest.Result) {
				str := symgotest.AssertAs[*object.String](r, t, 0)
				if str.Value != "hello from unexported func" {
					t.Errorf("want %q, got %q", "hello from unexported func", str.Value)
				}
			},
		},
		{
			name: "unexported resolution: with var",
			libGoSource: `
package lib
var secret = "hello from unexported var"
func GetGreeting() string {
	return secret
}`,
			scanPolicy: func(path string) bool { return true }, // scan everything
			checkResult: func(t *testing.T, r *symgotest.Result) {
				str := symgotest.AssertAs[*object.String](r, t, 0)
				if str.Value != "hello from unexported var" {
					t.Errorf("want %q, got %q", "hello from unexported var", str.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := map[string]string{}
			for k, v := range baseFiles {
				source[k] = v
			}
			source["lib/lib.go"] = tt.libGoSource

			tc := symgotest.TestCase{
				Source:     source,
				WorkDir:    "myapp",
				EntryPoint: "example.com/myapp.main",
				Options: []symgotest.Option{
					symgotest.WithScanPolicy(tt.scanPolicy),
				},
			}

			action := func(t *testing.T, r *symgotest.Result) {
				if r.Error != nil {
					t.Fatalf("Execution failed unexpectedly: %v", r.Error)
				}
				tt.checkResult(t, r)
			}

			symgotest.Run(t, tc, action)
		})
	}
}
