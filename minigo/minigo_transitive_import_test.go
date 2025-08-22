package minigo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
)

func TestTransitiveImport(t *testing.T) {
	script := `
package main

import "github.com/podhmo/go-scan/minigo/testdata/pkga"

func main() {
	println(pkga.FuncA())
}
`
	r := &strings.Reader{}
	var outbuf, errbuf strings.Builder
	interpreter := newTestInterpreter(t,
		minigo.WithStdin(r),
		minigo.WithStdout(&outbuf),
		minigo.WithStderr(&errbuf),
	)

	_, err := interpreter.EvalString(script)
	if err != nil {
		t.Fatalf("eval failed: %v\nstderr:\n%s", err, errbuf.String())
	}

	want := "A says: B\n"
	if outbuf.String() != want {
		t.Errorf("unexpected output:\nwant: %q\ngot:  %q", want, outbuf.String())
	}
}

func TestTransitiveImportMultiFilePackage(t *testing.T) {
	// This test case is designed to fail if the transitive dependency logic is incorrect.
	// It simulates the `sort` -> `slices` problem.
	// `pkge` has two files. The test calls FuncE2 (in pkge2.go).
	// FuncE2 calls FuncE1 (in pkge1.go).
	// FuncE1 imports `pkgf` and calls a function from it.
	// The scanner must find `FuncE2`, and in the process, load the AST for `pkge1.go`
	// so that the interpreter can find `FuncE1`.
	// The interpreter must then correctly create a FileScope for the *entire* `pkge` package,
	// including the import of `pkgf` from `pkge1.go`, so that when `FuncE1` is executed,
	// it can resolve `pkgf`.
	script := `
package main

import "github.com/podhmo/go-scan/minigo/testdata/pkge"

func main() {
	println(pkge.FuncE2())
}
`
	r := &strings.Reader{}
	var outbuf, errbuf strings.Builder
	interpreter := newTestInterpreter(t,
		minigo.WithStdin(r),
		minigo.WithStdout(&outbuf),
		minigo.WithStderr(&errbuf),
	)

	_, err := interpreter.EvalString(script)
	if err != nil {
		t.Fatalf("eval failed: %v\nstderr:\n%s", err, errbuf.String())
	}

	want := "E2 says: E1 says: F\n"
	if outbuf.String() != want {
		t.Errorf("unexpected output:\nwant: %q\ngot:  %q", want, outbuf.String())
	}
}
