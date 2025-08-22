package minigo_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
)

// Test via loading original go source
func TestStdlibSource(t *testing.T) {
	// t.Skip("Skipping tests for source-loaded stdlib packages due to unresolved issues with environment handling for generics (see docs/trouble-type-list-interface.md)")
	goroot := runtime.GOROOT()
	if goroot == "" {
		t.Skip("GOROOT not found, skipping test")
	}
	srcPath := filepath.Join(goroot, "src", "slices", "slices.go")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	// clone is a function that uses variadic arguments.
	// It is a good test case for the feature.
	// And it is simple enough to be a good first target.
	testCases := []struct {
		name           string
		script         string
		expectedOutput string
	}{
		{
			name: "slices.Clone",
			script: `
package main
import "slices"
func main() {
	s1 := []int{1, 2, 3}
	s2 := slices.Clone(s1)
	s1[0] = 99
	println(s2) // should still be [1 2 3]
}`,
			expectedOutput: "[1 2 3]\n",
		},
		{
			name: "slices.Equal",
			script: `
package main
import "slices"
func main() {
	s1 := []int{1, 2, 3}
	s2 := []int{1, 2, 3}
	s3 := []int{1, 2, 4}
	println(slices.Equal(s1, s2))
	println(slices.Equal(s1, s3))
}`,
			expectedOutput: "true\nfalse\n",
		},
		{
			name: "slices.Compare",
			script: `
package main
import "slices"
func main() {
	s1 := []int{1, 2, 3}
	s2 := []int{1, 2, 3}
	s3 := []int{1, 2, 4}
	s4 := []int{1, 2}
	println(slices.Compare(s1, s2)) // 0
	println(slices.Compare(s1, s3)) // -1
	println(slices.Compare(s3, s1)) // 1
	println(slices.Compare(s1, s4)) // 1
}`,
			expectedOutput: "0\n-1\n1\n1\n",
		},
		{
			name: "slices.Index",
			script: `
package main
import "slices"
func main() {
	s := []int{10, 20, 30}
	println(slices.Index(s, 20))
	println(slices.Index(s, 40))
}`,
			expectedOutput: "1\n-1\n",
		},
		{
			name: "slices.Sort",
			script: `
package main
import "slices"
func main() {
	s := []int{3, 1, 2}
	slices.Sort(s)
	println(s)
}`,
			expectedOutput: "[1 2 3]\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "slices.Equal" || tc.name == "slices.Compare" {
				t.Skip("minigo cannot handle generic functions with multiple arguments of the same generic type")
			}
			if tc.name == "slices.Sort" {
				t.Skip("Skipping because test harness doesn't support multi-file packages yet (Sort is in a different file).")
			}
			r := &strings.Reader{}
			var outbuf, errbuf strings.Builder
			interpreter := newTestInterpreter(t,
				minigo.WithStdin(r),
				minigo.WithStdout(&outbuf),
				minigo.WithStderr(&errbuf),
			)

			err := interpreter.LoadGoSourceAsPackage("slices", string(src))
			if err != nil {
				t.Fatal(err)
			}

			_, err = interpreter.EvalString(tc.script)
			if err != nil {
				t.Fatalf("eval failed: %v\nstderr:\n%s", err, errbuf.String())
			}

			if outbuf.String() != tc.expectedOutput {
				t.Fatalf("unexpected output. expected=%q, but got=%q", tc.expectedOutput, outbuf.String())
			}
		})
	}
}
