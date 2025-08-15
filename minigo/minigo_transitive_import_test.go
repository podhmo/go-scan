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
	interpreter, err := minigo.NewInterpreter(
		minigo.WithStdin(r),
		minigo.WithStdout(&outbuf),
		minigo.WithStderr(&errbuf),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = interpreter.EvalString(script)
	if err != nil {
		t.Fatalf("eval failed: %v\nstderr:\n%s", err, errbuf.String())
	}

	want := "A says: B\n"
	if outbuf.String() != want {
		t.Errorf("unexpected output:\nwant: %q\ngot:  %q", want, outbuf.String())
	}
}
