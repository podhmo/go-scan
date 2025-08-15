package minigo_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo"
)

// TestStructLiteralShorthand tests support for struct literals with shorthand
// field initialization (e.g., `MyStruct{Field}` instead of `MyStruct{Field: Field}`).
func TestStructLiteralShorthand(t *testing.T) {
	source := `
package main

type MyError struct {
	msg string
}

func newError(msg string) *MyError {
	return &MyError{msg}
}

func main() {
	err := newError("something went wrong")
	println(err.msg)
}
`
	var out bytes.Buffer
	m, err := minigo.NewInterpreter(
		minigo.WithStdout(&out),
	)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %+v", err)
	}

	_, err = m.EvalString(source)
	if err != nil {
		t.Fatalf("EvalString failed: %+v", err)
	}

	want := "something went wrong\n"
	got := out.String()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}
