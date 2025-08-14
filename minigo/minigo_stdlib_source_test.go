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
		name   string
		script string
	}{
		{
			"slices.Clone",
			`
package main

import "slices"

func main() {
	s1 := []int{1, 2, 3}
	s2 := slices.Clone(s1)
	println(s2)

	s1[0] = 99
	println(s1)
	println(s2)
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &strings.Reader{}
			var outbuf, errbuf strings.Builder
			interpreter, err := minigo.NewInterpreter(
				minigo.WithStdin(r),
				minigo.WithStdout(&outbuf),
				minigo.WithStderr(&errbuf),
			)
			if err != nil {
				t.Fatal(err)
			}

			err = interpreter.LoadGoSourceAsPackage("slices", string(src))
			if err != nil {
				t.Fatal(err)
			}

			_, err = interpreter.EvalString(tc.script)
			if err != nil {
				t.Fatalf("eval failed: %v\nstderr:\n%s", err, errbuf.String())
			}

			// The script prints s2, then s1, then s2 again.
			// We want to check the final state of s2.
			// output should be:
			// [1 2 3]
			// [99 2 3]
			// [1 2 3]
			expectedSuffix := "[1 2 3]\n"
			if !strings.HasSuffix(outbuf.String(), expectedSuffix) {
				t.Fatalf("unexpected output. expected suffix=%q, but got=%q", expectedSuffix, outbuf.String())
			}
		})
	}
}
