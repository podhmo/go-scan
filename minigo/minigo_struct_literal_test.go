package minigo_test

import (
	"bytes"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	_, err = m.EvalString(source)
	require.NoError(t, err, "compilation should succeed")

	assert.Equal(t, "something went wrong\n", out.String(), "output mismatch")
}
