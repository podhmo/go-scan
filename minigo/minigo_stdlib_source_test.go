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
		want   string
	}{
		{
			name: "slices.Clone",
			script: `
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
			want: `[1 2 3]
[99 2 3]
[1 2 3]
`,
		},
		{
			name: "sort.Ints",
			script: `
package main

import "sort"

func main() {
	s := []int{3, 1, 2}
	sort.Ints(s)
	println(s)
}
`,
			want: `[1 2 3]
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

			// For the slices.Clone test, we preload the source.
			// For the sort.Ints test, we rely on the on-demand loader.
			if tc.name == "slices.Clone" {
				err = interpreter.LoadGoSourceAsPackage("slices", string(src))
				if err != nil {
					t.Fatal(err)
				}
			}

			_, err = interpreter.EvalString(tc.script)
			if err != nil {
				t.Fatalf("eval failed: %v\nstderr:\n%s", err, errbuf.String())
			}

			if outbuf.String() != tc.want {
				t.Fatalf("unexpected output.\nwant:\n%s\ngot:\n%s", tc.want, outbuf.String())
			}
		})
	}
}
